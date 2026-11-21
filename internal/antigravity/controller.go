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

	"github.com/jefflunt/xnk-iterate/internal/applog"
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
	if c.running {
		c.mu.Unlock()
		return nil
	}

	bin := cfg.AgyBinary
	if bin == "" {
		found, ok := FindAgyBinary()
		if !ok {
			c.mu.Unlock()
			applog.Error("agy", "agy CLI binary not found on PATH or standard locations")
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
		c.mu.Unlock()
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		c.mu.Unlock()
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		c.mu.Unlock()
		return err
	}

	applog.Info("agy", "spawning %s %s (cwd=%s)", bin, strings.Join(args, " "), cmd.Dir)
	c.emitState(model.AgentStarting, "Launching Google Antigravity session...")
	if err := cmd.Start(); err != nil {
		cancel()
		c.mu.Unlock()
		applog.Error("agy", "failed to start agy: %v", err)
		c.emitState(model.AgentError, "Failed to launch agy: "+err.Error())
		return err
	}

	c.cmd = cmd
	c.stdin = stdin
	c.cancel = cancel
	c.cfg = cfg
	c.running = true
	c.mu.Unlock()

	applog.Info("agy", "agy started (pid=%d)", cmd.Process.Pid)
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
		applog.Info("agy", "agy process exited (wasRunning=%v err=%v)", wasRunning, err)
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

// SendPrompt writes a raw prompt string using agy's exact stream-json input schema:
// {"event":"user","message":{"content":"..."}}
func (c *Controller) SendPrompt(prompt string) error {
	c.mu.Lock()
	stdin := c.stdin
	running := c.running
	c.mu.Unlock()

	if !running || stdin == nil {
		applog.Warn("agy", "SendPrompt called while session not running")
		return fmt.Errorf("antigravity session is not running")
	}

	c.emitState(model.AgentThinking, "Antigravity analyzing director feedback...")

	payload := map[string]any{
		"event": "user",
		"message": map[string]any{
			"content": prompt,
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	applog.Info("agy", "sending user turn (%d bytes)", len(raw))
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stdin == nil {
		return fmt.Errorf("antigravity stdin closed")
	}
	_, err = c.stdin.Write(append(raw, '\n'))
	if err != nil {
		applog.Error("agy", "stdin write failed: %v", err)
	}
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
		"event":      "permission_response",
		"request_id": requestID,
		"approved":   approved,
		"decision":   string(decision),
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	applog.Info("agy", "sending permission_response id=%s decision=%s", requestID, decision)
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

// Stop terminates the owned `agy` session immediately without blocking.
func (c *Controller) Stop() {
	c.mu.Lock()
	c.running = false
	stdin := c.stdin
	c.stdin = nil
	cancel := c.cancel
	c.cancel = nil
	cmd := c.cmd
	c.cmd = nil
	c.pendingPerm = nil
	c.mu.Unlock()

	applog.Info("agy", "stopping agy session")
	if stdin != nil {
		_ = stdin.Close()
	}
	if cancel != nil {
		cancel()
	}
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}

	c.emitState(model.AgentIdle, "Session stopped")
}

func (c *Controller) readStdout(r io.Reader) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	var assembled strings.Builder

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if len(line) > 300 {
			applog.Info("agy-out", "%s…", line[:300])
		} else {
			applog.Info("agy-out", "%s", line)
		}
		c.handleStreamLine(line, &assembled)
	}
	if err := scanner.Err(); err != nil {
		applog.Warn("agy-out", "stdout scanner ended: %v", err)
	}
}

func (c *Controller) readStderr(r io.Reader) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		applog.Warn("agy-err", "%s", line)
		if strings.HasPrefix(line, "error:") || strings.HasPrefix(line, "Failed to start:") || strings.HasPrefix(line, "Error:") {
			if c.callbacks.OnError != nil {
				c.callbacks.OnError(fmt.Errorf("%s", line))
			}
		}
	}
}

func (c *Controller) handleStreamLine(line string, assembled *strings.Builder) {
	var msg map[string]any
	if err := json.Unmarshal([]byte(line), &msg); err != nil {
		return
	}

	if convID, ok := firstString(msg, "conversation_id", "session_id"); ok && convID != "" {
		if c.callbacks.OnConversationID != nil {
			c.callbacks.OnConversationID(convID)
		}
	}

	evType, _ := firstString(msg, "event", "type")
	switch evType {
	case "init", "session_started":
		c.emitState(model.AgentListening, "Antigravity ready — listening")

	case "step_update":
		su, _ := msg["step_update"].(map[string]any)
		if su == nil {
			return
		}
		if convID, ok := firstString(su, "conversation_id"); ok && convID != "" && c.callbacks.OnConversationID != nil {
			c.callbacks.OnConversationID(convID)
		}
		stepType, _ := firstString(su, "step_type")
		if delta, ok := firstString(su, "text_delta", "delta"); ok && delta != "" {
			assembled.WriteString(delta)
			c.emitState(model.AgentSpeaking, "Antigravity responding...")
			if c.callbacks.OnTextDelta != nil {
				c.callbacks.OnTextDelta(delta)
			}
		} else if stepType != "" && stepType != "user_input" && stepType != "agent_response" {
			detail, _ := firstString(su, "state", "summary", "status")
			c.emitState(model.AgentWorking, fmt.Sprintf("Working: %s", stepType))
			if c.callbacks.OnToolCall != nil {
				c.callbacks.OnToolCall(stepType, detail)
			}
		}

	case "delta", "text_delta", "content_block_delta":
		if text, ok := firstString(msg, "delta", "text", "content"); ok && text != "" {
			assembled.WriteString(text)
			c.emitState(model.AgentSpeaking, "Antigravity responding...")
			if c.callbacks.OnTextDelta != nil {
				c.callbacks.OnTextDelta(text)
			}
		}

	case "tool_use", "tool_call":
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

	case "result":
		res, _ := msg["result"].(map[string]any)
		var finalText, errText, status string
		if res != nil {
			status, _ = firstString(res, "status")
			finalText, _ = firstString(res, "response", "text", "content")
			errText, _ = firstString(res, "error")
			if convID, ok := firstString(res, "conversation_id"); ok && convID != "" && c.callbacks.OnConversationID != nil {
				c.callbacks.OnConversationID(convID)
			}
		} else {
			finalText, _ = firstString(msg, "result", "response", "text", "content")
			errText, _ = firstString(msg, "error")
		}
		if strings.EqualFold(status, "ERROR") || errText != "" {
			assembled.Reset()
			if errText == "" {
				errText = "Antigravity returned an error"
			}
			applog.Error("agy", "turn error: %s", errText)
			c.emitState(model.AgentError, errText)
			if c.callbacks.OnError != nil {
				c.callbacks.OnError(fmt.Errorf("%s", errText))
			}
			return
		}
		if strings.TrimSpace(finalText) == "" {
			finalText = strings.TrimSpace(assembled.String())
		} else {
			finalText = strings.TrimSpace(finalText)
		}
		assembled.Reset()
		if finalText != "" && c.callbacks.OnAssistantMessage != nil {
			c.callbacks.OnAssistantMessage(finalText)
		}
		c.emitState(model.AgentListening, "Listening for director commentary")

	case "assistant", "turn_complete", "done":
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
		applog.Error("agy", "stream error: %s", errMsg)
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
	b.WriteString("[XNK ITERATE LIVE DIRECTOR FEEDBACK — VOICE + VISION MODE]\n")
	b.WriteString("Respond concisely and clearly. Inspect the attached frame paths via view_file/read_file if helpful.\n")
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
			b.WriteString(fmt.Sprintf("Trigger Frame: %s\n", u.TriggerFramePath))
		}
		if u.ReactionFramePath != "" {
			b.WriteString(fmt.Sprintf("Reaction Frame: %s\n", u.ReactionFramePath))
		}
		for idx, cp := range u.CarouselPaths {
			b.WriteString(fmt.Sprintf("Carousel Frame %d: %s\n", idx+1, cp))
		}
	}
	return b.String()
}
