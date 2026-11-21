package antigravity

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jefflunt/xnk-iterate/internal/model"
)

// SessionConfig configures an owned Google Antigravity (`agy`) child session.
type SessionConfig struct {
	WorkspacePath   string
	ConversationID  string
	ContinueSession bool
	Model           string
	Mode            string // "accept-edits", "auto", "plan"
	AgyBinary       string // Optional override for testing
}

// Callbacks receive real-time events streamed from the owned `agy` process.
type Callbacks struct {
	OnStateChange      func(state model.AgentState, statusText string)
	OnConversationID   func(convID string)
	OnTextDelta        func(delta string)
	OnAssistantMessage func(fullText string)
	OnToolCall         func(toolName string, detail string)
	OnPermissionReq    func(req model.PermissionRequest)
	OnError            func(err error)
}

// Controller spawns, owns, and communicates with a live Google Antigravity (`agy`) session
// using newline-delimited JSON (`--input-format stream-json --output-format stream-json`).
type Controller struct {
	mu          sync.Mutex
	cmd         *exec.Cmd
	stdin       io.WriteCloser
	cancel      context.CancelFunc
	running     bool
	cfg         SessionConfig
	callbacks   Callbacks
	pendingPerm *model.PermissionRequest
}

// NewController creates an Antigravity Session Controller.
func NewController(cb Callbacks) *Controller {
	return &Controller{
		callbacks: cb,
	}
}

// FindAgyBinary locates the `agy` CLI executable on PATH or in standard install locations.
func FindAgyBinary() (string, bool) {
	if p, err := exec.LookPath("agy"); err == nil {
		return p, true
	}
	home, _ := os.UserHomeDir()
	candidates := []string{
		filepath.Join(home, ".local", "bin", "agy"),
		"/usr/local/bin/agy",
		"/opt/homebrew/bin/agy",
	}
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c, true
		}
	}
	return "", false
}

// Start spawns the owned `agy` process in stream-json mode for the given workspace.
func (c *Controller) Start(ctx context.Context, cfg SessionConfig) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.running {
		return nil
	}

	bin := cfg.AgyBinary
	if bin == "" {
		found, ok := FindAgyBinary()
		if !ok {
			c.emitState(model.AgentError, "agy CLI not found on PATH")
			return fmt.Errorf("agy CLI binary not found")
		}
		bin = found
	}

	args := []string{
		"--input-format", "stream-json",
		"--output-format", "stream-json",
	}
	if cfg.ConversationID != "" {
		args = append(args, "--conversation", cfg.ConversationID)
	} else if cfg.ContinueSession {
		args = append(args, "--continue")
	}
	if cfg.Model != "" {
		args = append(args, "--model", cfg.Model)
	}
	if cfg.Mode != "" {
		args = append(args, "--mode", cfg.Mode)
	}
	// Attach empty print prompt so agy enters streaming NDJSON loop on stdin.
	args = append(args, "-p=")

	runCtx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(runCtx, bin, args...)
	if cfg.WorkspacePath != "" {
		cmd.Dir = cfg.WorkspacePath
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return err
	}

	c.emitState(model.AgentStarting, "Launching Google Antigravity session...")
	if err := cmd.Start(); err != nil {
		cancel()
		c.emitState(model.AgentError, "Failed to launch agy: "+err.Error())
		return err
	}

	c.cmd = cmd
	c.stdin = stdin
	c.cancel = cancel
	c.cfg = cfg
	c.running = true

	c.emitState(model.AgentListening, "Antigravity live session active")

	go c.readStdout(stdout)
	go c.readStderr(stderr)
	go func() {
		err := cmd.Wait()
		c.mu.Lock()
		wasRunning := c.running
		c.running = false
		c.stdin = nil
		c.cmd = nil
		c.mu.Unlock()
		if wasRunning {
			if err != nil && runCtx.Err() == nil {
				c.emitState(model.AgentError, "agy exited: "+err.Error())
			} else {
				c.emitState(model.AgentIdle, "Antigravity session ended")
			}
		}
	}()

	return nil
}

// IsRunning reports whether an owned `agy` process is active.
func (c *Controller) IsRunning() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.running
}

// SendUtterances formats one or more finalized director utterances (with lookback frames
// and interaction context) and writes a single NDJSON user turn to `agy` stdin.
func (c *Controller) SendUtterances(utterances []model.Utterance, target *model.CaptureTarget) error {
	if len(utterances) == 0 {
		return nil
	}
	prompt := FormatDirectorTurn(utterances, target)
	return c.SendPrompt(prompt)
}

// SendPrompt writes a raw prompt string as an NDJSON message to the owned `agy` process.
func (c *Controller) SendPrompt(prompt string) error {
	c.mu.Lock()
	stdin := c.stdin
	running := c.running
	c.mu.Unlock()

	if !running || stdin == nil {
		return fmt.Errorf("antigravity session is not running")
	}

	c.emitState(model.AgentThinking, "Antigravity analyzing director feedback...")

	payload := map[string]any{
		"type":    "user",
		"message": prompt,
		"content": prompt,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stdin == nil {
		return fmt.Errorf("antigravity stdin closed")
	}
	_, err = c.stdin.Write(append(raw, '\n'))
	return err
}

// ResolvePermission sends a voice or keyboard approve/deny decision back to `agy`.
func (c *Controller) ResolvePermission(requestID string, decision ApprovalDecision) error {
	c.mu.Lock()
	stdin := c.stdin
	running := c.running
	c.pendingPerm = nil
	c.mu.Unlock()

	if !running || stdin == nil {
		return fmt.Errorf("antigravity session is not running")
	}

	approved := decision == DecisionApprove
	payload := map[string]any{
		"type":       "permission_response",
		"request_id": requestID,
		"approved":   approved,
		"decision":   string(decision),
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stdin == nil {
		return fmt.Errorf("antigravity stdin closed")
	}
	if _, err := c.stdin.Write(append(raw, '\n')); err != nil {
		return err
	}
	if approved {
		c.emitState(model.AgentWorking, "Permission approved — continuing...")
	} else {
		c.emitState(model.AgentListening, "Permission denied — listening")
	}
	return nil
}

// Stop terminates the owned `agy` session cleanly.
func (c *Controller) Stop() {
	c.mu.Lock()
	c.running = false
	if c.stdin != nil {
		_ = c.stdin.Close()
		c.stdin = nil
	}
	if c.cancel != nil {
		c.cancel()
		c.cancel = nil
	}
	if c.cmd != nil && c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
		c.cmd = nil
	}
	c.pendingPerm = nil
	c.mu.Unlock()

	c.emitState(model.AgentIdle, "Session stopped")
}

func (c *Controller) readStdout(r io.Reader) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	var assembled strings.Builder

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		c.handleStreamLine(line, &assembled)
	}
}

func (c *Controller) readStderr(r io.Reader) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		// Ignore verbose glog lines; surface fatal errors if needed.
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "Failed to start:") || strings.HasPrefix(line, "Error:") {
			if c.callbacks.OnError != nil {
				c.callbacks.OnError(fmt.Errorf("%s", line))
			}
		}
	}
}

func (c *Controller) handleStreamLine(line string, assembled *strings.Builder) {
	var msg map[string]any
	if err := json.Unmarshal([]byte(line), &msg); err != nil {
		// Non-JSON plain text fallback
		if c.callbacks.OnTextDelta != nil {
			c.callbacks.OnTextDelta(line + "\n")
		}
		assembled.WriteString(line)
		assembled.WriteByte('\n')
		return
	}

	if convID, ok := firstString(msg, "conversation_id", "session_id"); ok && convID != "" {
		if c.callbacks.OnConversationID != nil {
			c.callbacks.OnConversationID(convID)
		}
	}

	evType, _ := firstString(msg, "type", "event")
	switch evType {
	case "init", "session_started":
		c.emitState(model.AgentListening, "Antigravity ready — listening")

	case "delta", "text_delta", "content_block_delta":
		if text, ok := firstString(msg, "delta", "text", "content"); ok && text != "" {
			assembled.WriteString(text)
			c.emitState(model.AgentSpeaking, "Antigravity responding...")
			if c.callbacks.OnTextDelta != nil {
				c.callbacks.OnTextDelta(text)
			}
		}

	case "tool_use", "tool_call", "step_update":
		toolName, _ := firstString(msg, "tool", "name", "tool_name")
		detail, _ := firstString(msg, "summary", "status", "detail")
		if toolName == "" {
			toolName = "agent_step"
		}
		c.emitState(model.AgentWorking, fmt.Sprintf("Working: %s", toolName))
		if c.callbacks.OnToolCall != nil {
			c.callbacks.OnToolCall(toolName, detail)
		}

	case "permission_request", "approval_required":
		reqID, _ := firstString(msg, "request_id", "id")
		if reqID == "" {
			reqID = fmt.Sprintf("perm_%d", time.Now().UnixNano())
		}
		toolName, _ := firstString(msg, "tool_name", "tool", "name")
		desc, _ := firstString(msg, "description", "message")
		cmdStr, _ := firstString(msg, "command")
		req := model.PermissionRequest{
			ID:          reqID,
			ToolName:    toolName,
			Description: desc,
			Command:     cmdStr,
			RequestedAt: time.Now(),
		}
		c.mu.Lock()
		c.pendingPerm = &req
		c.mu.Unlock()
		c.emitState(model.AgentAwaitingApproval, fmt.Sprintf("Approval needed: %s", toolName))
		if c.callbacks.OnPermissionReq != nil {
			c.callbacks.OnPermissionReq(req)
		}

	case "assistant", "result", "turn_complete", "done":
		text, _ := firstString(msg, "result", "text", "content", "message")
		if text == "" {
			text = strings.TrimSpace(assembled.String())
		}
		assembled.Reset()
		if text != "" && c.callbacks.OnAssistantMessage != nil {
			c.callbacks.OnAssistantMessage(text)
		}
		c.emitState(model.AgentListening, "Listening for director commentary")

	case "error":
		errMsg, _ := firstString(msg, "error", "message")
		if errMsg == "" {
			errMsg = "unknown antigravity error"
		}
		c.emitState(model.AgentError, errMsg)
		if c.callbacks.OnError != nil {
			c.callbacks.OnError(fmt.Errorf("%s", errMsg))
		}
	}
}

func (c *Controller) emitState(state model.AgentState, status string) {
	if c.callbacks.OnStateChange != nil {
		c.callbacks.OnStateChange(state, status)
	}
}

func firstString(m map[string]any, keys ...string) (string, bool) {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s, true
			}
		}
	}
	return "", false
}

// FormatDirectorTurn builds a concise, structured multimodal prompt for Google Antigravity
// combining the director's spoken commentary, dynamic lookback timecodes, triggering OS actions,
// and local Trigger/Reaction frame paths.
func FormatDirectorTurn(utterances []model.Utterance, target *model.CaptureTarget) string {
	var b strings.Builder
	b.WriteString("[XNK LIVE DIRECTOR FEEDBACK — VOICE + VISION MODE]\n")
	b.WriteString("Keep your spoken reply concise (1-2 short sentences) while making edits or running checks.\n")
	if target != nil {
		b.WriteString(fmt.Sprintf("Target: %s (%s, %dx%d)\n", target.Title, target.Type, target.Resolution.Width, target.Resolution.Height))
	}
	for _, u := range utterances {
		b.WriteString(fmt.Sprintf("\nUtterance #%d [%.2fs–%.2fs] (Lookback %.2fs–%.2fs):\n",
			u.ID, u.SpeechStart, u.SpeechEnd, u.LookbackWindow.StartSeconds, u.LookbackWindow.EndSeconds))
		b.WriteString(fmt.Sprintf("Director said: %q\n", u.Transcript))
		if u.TriggerAction != nil {
			b.WriteString(fmt.Sprintf("Trigger Action: %s at %.2fs", u.TriggerAction.Type, u.TriggerAction.Timestamp))
			if u.TriggerAction.Location != nil {
				b.WriteString(fmt.Sprintf(" (x=%.0f, y=%.0f)", u.TriggerAction.Location.X, u.TriggerAction.Location.Y))
			}
			b.WriteString("\n")
		}
		if u.TriggerFramePath != "" {
			b.WriteString(fmt.Sprintf("Trigger Frame (inspect via read_file): %s\n", u.TriggerFramePath))
		}
		if u.ReactionFramePath != "" {
			b.WriteString(fmt.Sprintf("Reaction Frame (inspect via read_file): %s\n", u.ReactionFramePath))
		}
		for idx, cp := range u.CarouselPaths {
			b.WriteString(fmt.Sprintf("Carousel Frame %d: %s\n", idx+1, cp))
		}
	}
	return b.String()
}
