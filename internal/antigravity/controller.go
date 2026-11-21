package antigravity

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jefflunt/xnk-iterate/internal/applog"
	"github.com/jefflunt/xnk-iterate/internal/model"
)

// SessionConfig configures an owned Google Antigravity (`agy acp`) child session.
type SessionConfig struct {
	WorkspacePath   string
	ConversationID  string
	ContinueSession bool
	Model           string
	Mode            string
	AgyBinary       string // Optional override for testing
}

// Callbacks receive real-time events streamed from the owned `agy acp` process.
type Callbacks struct {
	OnStateChange      func(state model.AgentState, statusText string)
	OnConversationID   func(convID string)
	OnTextDelta        func(delta string)
	OnAssistantMessage func(fullText string)
	OnToolCall         func(toolID string, title string, status string)
	OnPermissionReq    func(req model.PermissionRequest)
	OnError            func(err error)
}

type pendingACPPerm struct {
	rpcID   any
	options []model.PermissionOption
}

// Controller spawns, owns, and communicates with a live Google Antigravity session
// using the Agent Client Protocol (`agy acp --stdio`, JSON-RPC 2.0), enabling true
// interactive tool-permission prompts, native image attachments, and streaming replies.
type Controller struct {
	mu           sync.Mutex
	cmd          *exec.Cmd
	stdin        io.WriteCloser
	cancel       context.CancelFunc
	running      bool
	sessionID    string
	sessionReady chan struct{}
	cfg          SessionConfig
	callbacks    Callbacks
	nextRPCID    atomic.Int64
	activePrompt int64
	pendingPerms map[string]pendingACPPerm
	assembled    strings.Builder
	queuedTurns  []queuedPrompt
}

type queuedPrompt struct {
	text       string
	framePaths []string
}

// NewController creates an Antigravity ACP Session Controller.
func NewController(cb Callbacks) *Controller {
	return &Controller{
		callbacks:    cb,
		pendingPerms: make(map[string]pendingACPPerm),
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

// Start spawns `agy acp --stdio` and performs the ACP initialize + session/new handshake.
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
			applog.Error("agy-acp", "agy CLI binary not found")
			c.emitState(model.AgentError, "agy CLI not found on PATH")
			return fmt.Errorf("agy CLI binary not found")
		}
		bin = found
	}

	cwd := cfg.WorkspacePath
	if cwd == "" || cwd == "/" {
		if home, err := os.UserHomeDir(); err == nil {
			cwd = home
		}
	}

	args := []string{"acp", "--stdio"}
	runCtx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(runCtx, bin, args...)
	cmd.Dir = cwd

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

	applog.Info("agy-acp", "spawning %s %s (cwd=%s)", bin, strings.Join(args, " "), cwd)
	c.emitState(model.AgentStarting, "Connecting to Google Antigravity (ACP)...")
	if err := cmd.Start(); err != nil {
		cancel()
		c.mu.Unlock()
		applog.Error("agy-acp", "failed to start agy acp: %v", err)
		c.emitState(model.AgentError, "Failed to launch agy acp: "+err.Error())
		return err
	}

	c.cmd = cmd
	c.stdin = stdin
	c.cancel = cancel
	c.cfg = cfg
	c.cfg.WorkspacePath = cwd
	c.running = true
	c.sessionID = ""
	c.sessionReady = make(chan struct{})
	c.activePrompt = 0
	c.assembled.Reset()
	c.pendingPerms = make(map[string]pendingACPPerm)
	c.queuedTurns = nil
	c.mu.Unlock()

	applog.Info("agy-acp", "agy acp started (pid=%d)", cmd.Process.Pid)

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
		applog.Info("agy-acp", "agy acp exited (wasRunning=%v err=%v)", wasRunning, err)
		if wasRunning {
			if err != nil && runCtx.Err() == nil {
				c.emitState(model.AgentError, "agy acp exited: "+err.Error())
			} else {
				c.emitState(model.AgentIdle, "Antigravity session ended")
			}
		}
	}()

	// Kick off ACP handshake: initialize (id=1), then session/new (id=2) on response
	_ = c.writeRPC(map[string]any{
		"jsonrpc": "2.0",
		"id":      c.nextRPCID.Add(1), // id 1 -> initialize
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion":    1,
			"clientCapabilities": map[string]any{},
		},
	})

	return nil
}

// IsRunning reports whether an owned `agy acp` process is active.
func (c *Controller) IsRunning() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.running
}

// SendUtterances formats director utterances and attaches Trigger/Reaction frames
// directly as native ACP image blocks alongside the text prompt.
func (c *Controller) SendUtterances(utterances []model.Utterance, target *model.CaptureTarget) error {
	if len(utterances) == 0 {
		return nil
	}
	prompt := FormatDirectorTurn(utterances, target)
	var frames []string
	seen := map[string]bool{}
	for _, u := range utterances {
		for _, fp := range []string{u.TriggerFramePath, u.ReactionFramePath} {
			if fp != "" && !seen[fp] {
				seen[fp] = true
				frames = append(frames, fp)
			}
		}
	}
	return c.SendPromptWithFrames(prompt, frames)
}

// SendPrompt writes a text-only prompt turn to the active ACP session.
func (c *Controller) SendPrompt(prompt string) error {
	return c.SendPromptWithFrames(prompt, nil)
}

// SendPromptWithFrames sends a `session/prompt` JSON-RPC request (queuing cleanly if
// ACP session initialization or an earlier turn is still in flight).
func (c *Controller) SendPromptWithFrames(prompt string, framePaths []string) error {
	c.mu.Lock()
	if !c.running || c.stdin == nil {
		c.mu.Unlock()
		return fmt.Errorf("antigravity ACP session is not running")
	}
	if c.sessionID == "" || c.activePrompt != 0 {
		c.queuedTurns = append(c.queuedTurns, queuedPrompt{text: prompt, framePaths: framePaths})
		applog.Info("agy-acp", "queued turn (sessionID=%q activePrompt=%d queueLen=%d)", c.sessionID, c.activePrompt, len(c.queuedTurns))
		c.mu.Unlock()
		return nil
	}
	sessID := c.sessionID
	reqID := c.nextRPCID.Add(1)
	c.activePrompt = reqID
	c.assembled.Reset()
	c.mu.Unlock()

	return c.dispatchACPPrompt(reqID, sessID, prompt, framePaths)
}

func (c *Controller) dispatchACPPrompt(reqID int64, sessID, prompt string, framePaths []string) error {
	blocks := []map[string]any{
		{"type": "text", "text": prompt},
	}
	// Attach up to 2 recent frames natively as base64 image blocks (ACP promptCapabilities.image=true!)
	for i, fp := range framePaths {
		if i >= 2 {
			break
		}
		data, err := os.ReadFile(fp)
		if err != nil || len(data) == 0 || len(data) > 4*1024*1024 {
			continue
		}
		mime := "image/jpeg"
		if strings.HasSuffix(strings.ToLower(fp), ".png") {
			mime = "image/png"
		}
		blocks = append(blocks, map[string]any{
			"type":     "image",
			"data":     base64.StdEncoding.EncodeToString(data),
			"mimeType": mime,
		})
	}

	c.emitState(model.AgentThinking, "Antigravity analyzing director feedback...")
	applog.Info("agy-acp", "sending session/prompt id=%d session=%s blocks=%d", reqID, sessID, len(blocks))

	return c.writeRPC(map[string]any{
		"jsonrpc": "2.0",
		"id":      reqID,
		"method":  "session/prompt",
		"params": map[string]any{
			"sessionId": sessID,
			"prompt":    blocks,
		},
	})
}

// ResolvePermission replies to a pending `session/request_permission` ACP request.
// If optionID is empty, selects the best matching option for decision (allow_once vs reject_once).
func (c *Controller) ResolvePermission(requestID string, optionID string, decision ApprovalDecision) error {
	c.mu.Lock()
	pending, ok := c.pendingPerms[requestID]
	if ok {
		delete(c.pendingPerms, requestID)
	}
	running := c.running
	c.mu.Unlock()

	if !running {
		return fmt.Errorf("antigravity ACP session is not running")
	}
	if !ok {
		applog.Warn("agy-acp", "ResolvePermission: unknown requestID=%s", requestID)
		return nil
	}

	chosen := optionID
	if chosen == "" {
		chosen = pickOptionID(pending.options, decision)
	}

	applog.Info("agy-acp", "responding to session/request_permission id=%v optionId=%s decision=%s", pending.rpcID, chosen, decision)
	var outcome map[string]any
	if chosen != "" {
		outcome = map[string]any{
			"outcome":  "selected",
			"optionId": chosen,
		}
	} else {
		outcome = map[string]any{
			"outcome": "cancelled",
		}
	}

	if err := c.writeRPC(map[string]any{
		"jsonrpc": "2.0",
		"id":      pending.rpcID,
		"result": map[string]any{
			"outcome": outcome,
		},
	}); err != nil {
		return err
	}

	if decision == DecisionApprove || strings.Contains(chosen, "allow") {
		c.emitState(model.AgentWorking, "Permission approved — continuing...")
	} else {
		c.emitState(model.AgentListening, "Permission denied — listening")
	}
	return nil
}

func pickOptionID(opts []model.PermissionOption, decision ApprovalDecision) string {
	if decision == DecisionApprove {
		for _, o := range opts {
			if o.Kind == "allow_once" {
				return o.OptionID
			}
		}
		for _, o := range opts {
			if strings.HasPrefix(o.Kind, "allow") {
				return o.OptionID
			}
		}
	} else {
		for _, o := range opts {
			if o.Kind == "reject_once" {
				return o.OptionID
			}
		}
		for _, o := range opts {
			if strings.HasPrefix(o.Kind, "reject") {
				return o.OptionID
			}
		}
	}
	return ""
}

// Stop terminates the owned `agy acp` session cleanly without blocking.
func (c *Controller) Stop() {
	c.mu.Lock()
	c.running = false
	stdin := c.stdin
	c.stdin = nil
	cancel := c.cancel
	c.cancel = nil
	cmd := c.cmd
	c.cmd = nil
	c.sessionID = ""
	c.activePrompt = 0
	c.queuedTurns = nil
	c.pendingPerms = make(map[string]pendingACPPerm)
	c.mu.Unlock()

	applog.Info("agy-acp", "stopping agy acp session")
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

func (c *Controller) writeRPC(obj map[string]any) error {
	raw, err := json.Marshal(obj)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stdin == nil {
		return fmt.Errorf("agy acp stdin closed")
	}
	_, err = c.stdin.Write(append(raw, '\n'))
	return err
}

func (c *Controller) readStdout(r io.Reader) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if len(line) > 300 {
			applog.Info("agy-acp-out", "%s…", line[:300])
		} else {
			applog.Info("agy-acp-out", "%s", line)
		}
		c.handleACPLine(line)
	}
	if err := scanner.Err(); err != nil {
		applog.Warn("agy-acp-out", "stdout scanner ended: %v", err)
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
		applog.Warn("agy-acp-err", "%s", line)
	}
}

func (c *Controller) handleACPLine(line string) {
	var msg map[string]any
	if err := json.Unmarshal([]byte(line), &msg); err != nil {
		return
	}

	method, _ := msg["method"].(string)
	_, hasID := msg["id"]

	// 1. Incoming JSON-RPC Request from Agent (e.g. session/request_permission!)
	if method != "" && hasID {
		if method == "session/request_permission" {
			c.handlePermissionRequest(msg["id"], msg["params"])
		}
		return
	}

	// 2. Incoming JSON-RPC Notification from Agent (session/update)
	if method == "session/update" {
		c.handleSessionUpdate(msg["params"])
		return
	}

	// 3. Incoming JSON-RPC Response to our Client Requests (initialize, session/new, session/prompt)
	if hasID {
		c.handleRPCResponse(msg)
	}
}

func (c *Controller) handleRPCResponse(msg map[string]any) {
	idFloat, _ := msg["id"].(float64)
	id := int64(idFloat)

	if errObj, ok := msg["error"].(map[string]any); ok && errObj != nil {
		errMsg, _ := firstString(errObj, "message")
		if errMsg == "" {
			errMsg = "ACP error"
		}
		applog.Error("agy-acp", "RPC error for id=%d: %s", id, errMsg)
		c.mu.Lock()
		if id == c.activePrompt {
			c.activePrompt = 0
		}
		c.mu.Unlock()
		c.emitState(model.AgentError, errMsg)
		if c.callbacks.OnError != nil {
			c.callbacks.OnError(fmt.Errorf("%s", errMsg))
		}
		c.drainQueue()
		return
	}

	result, _ := msg["result"].(map[string]any)
	if result == nil {
		return
	}

	// Response to `initialize` -> open or load session
	if _, isInit := result["protocolVersion"]; isInit {
		c.mu.Lock()
		cwd := c.cfg.WorkspacePath
		convID := c.cfg.ConversationID
		c.mu.Unlock()

		newID := c.nextRPCID.Add(1)
		if convID != "" {
			applog.Info("agy-acp", "initialize OK -> loading session %s (cwd=%s)", convID, cwd)
			_ = c.writeRPC(map[string]any{
				"jsonrpc": "2.0",
				"id":      newID,
				"method":  "session/load",
				"params": map[string]any{
					"sessionId":  convID,
					"cwd":        cwd,
					"mcpServers": []any{},
				},
			})
		} else {
			applog.Info("agy-acp", "initialize OK -> creating session/new (cwd=%s)", cwd)
			_ = c.writeRPC(map[string]any{
				"jsonrpc": "2.0",
				"id":      newID,
				"method":  "session/new",
				"params": map[string]any{
					"cwd":        cwd,
					"mcpServers": []any{},
				},
			})
		}
		return
	}

	// Response to `session/new` or `session/load`
	if sessID, ok := firstString(result, "sessionId"); ok && sessID != "" {
		c.mu.Lock()
		c.sessionID = sessID
		c.mu.Unlock()
		applog.Info("agy-acp", "ACP session ready: %s", sessID)
		if c.callbacks.OnConversationID != nil {
			c.callbacks.OnConversationID(sessID)
		}
		c.emitState(model.AgentListening, "Antigravity ACP ready — listening")
		c.drainQueue()
		return
	}

	// Response to `session/prompt` (turn complete!)
	c.mu.Lock()
	isPromptResp := id == c.activePrompt
	var finalText string
	if isPromptResp {
		c.activePrompt = 0
		finalText = strings.TrimSpace(c.assembled.String())
		c.assembled.Reset()
	}
	c.mu.Unlock()

	if isPromptResp {
		if finalText != "" && c.callbacks.OnAssistantMessage != nil {
			c.callbacks.OnAssistantMessage(finalText)
		}
		c.emitState(model.AgentListening, "Listening for director commentary")
		c.drainQueue()
	}
}

func (c *Controller) drainQueue() {
	c.mu.Lock()
	if !c.running || c.sessionID == "" || c.activePrompt != 0 || len(c.queuedTurns) == 0 {
		c.mu.Unlock()
		return
	}
	next := c.queuedTurns[0]
	c.queuedTurns = c.queuedTurns[1:]
	sessID := c.sessionID
	reqID := c.nextRPCID.Add(1)
	c.activePrompt = reqID
	c.assembled.Reset()
	c.mu.Unlock()

	_ = c.dispatchACPPrompt(reqID, sessID, next.text, next.framePaths)
}

func (c *Controller) handleSessionUpdate(rawParams any) {
	params, _ := rawParams.(map[string]any)
	if params == nil {
		return
	}
	upd, _ := params["update"].(map[string]any)
	if upd == nil {
		return
	}

	kind, _ := firstString(upd, "sessionUpdate")
	switch kind {
	case "agent_message_chunk":
		content, _ := upd["content"].(map[string]any)
		if content == nil {
			return
		}
		if text, ok := firstString(content, "text"); ok && text != "" {
			c.mu.Lock()
			c.assembled.WriteString(text)
			c.mu.Unlock()
			c.emitState(model.AgentSpeaking, "Antigravity responding...")
			if c.callbacks.OnTextDelta != nil {
				c.callbacks.OnTextDelta(text)
			}
		}

	case "tool_call", "tool_call_update":
		toolID, _ := firstString(upd, "toolCallId", "id")
		title, _ := firstString(upd, "title", "kind")
		status, _ := firstString(upd, "status")
		if title == "" {
			title = "tool_step"
		}
		c.emitState(model.AgentWorking, fmt.Sprintf("Working: %s", title))
		if c.callbacks.OnToolCall != nil {
			c.callbacks.OnToolCall(toolID, title, status)
		}
	}
}

func (c *Controller) handlePermissionRequest(rpcID any, rawParams any) {
	params, _ := rawParams.(map[string]any)
	if params == nil {
		return
	}

	toolName := "Tool Action"
	desc := "Antigravity is requesting permission to proceed."
	cmdStr := ""

	if tc, ok := params["toolCall"].(map[string]any); ok && tc != nil {
		if t, ok := firstString(tc, "title", "kind"); ok {
			toolName = t
		}
		if ri, ok := tc["rawInput"].(map[string]any); ok && ri != nil {
			if cmd, ok := firstString(ri, "CommandLine", "command", "Path", "path"); ok {
				cmdStr = cmd
			}
		}
	}

	var opts []model.PermissionOption
	if rawOpts, ok := params["options"].([]any); ok {
		for _, item := range rawOpts {
			m, _ := item.(map[string]any)
			if m == nil {
				continue
			}
			optID, _ := firstString(m, "optionId")
			name, _ := firstString(m, "name")
			kind, _ := firstString(m, "kind")
			if optID != "" {
				opts = append(opts, model.PermissionOption{
					OptionID: optID,
					Name:     name,
					Kind:     kind,
				})
			}
		}
	}

	reqID := fmt.Sprintf("acp_%v", rpcID)
	req := model.PermissionRequest{
		ID:          reqID,
		ToolName:    toolName,
		Description: desc,
		Command:     cmdStr,
		Options:     opts,
		RequestedAt: time.Now(),
	}

	c.mu.Lock()
	c.pendingPerms[reqID] = pendingACPPerm{rpcID: rpcID, options: opts}
	c.mu.Unlock()

	applog.Info("agy-acp", "received session/request_permission id=%s tool=%s cmd=%q options=%d", reqID, toolName, cmdStr, len(opts))
	c.emitState(model.AgentAwaitingApproval, fmt.Sprintf("Approval needed: %s", toolName))
	if c.callbacks.OnPermissionReq != nil {
		c.callbacks.OnPermissionReq(req)
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
// combining the director's spoken commentary, dynamic lookback timecodes, and triggering OS actions.
func FormatDirectorTurn(utterances []model.Utterance, target *model.CaptureTarget) string {
	var b strings.Builder
	b.WriteString("[XNK ITERATE LIVE DIRECTOR FEEDBACK]\n")
	b.WriteString("Respond concisely. Trigger/Reaction screen frames are attached when available.\n")
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
			b.WriteString(fmt.Sprintf("Trigger Frame Path: %s\n", u.TriggerFramePath))
		}
		if u.ReactionFramePath != "" {
			b.WriteString(fmt.Sprintf("Reaction Frame Path: %s\n", u.ReactionFramePath))
		}
	}
	return b.String()
}
