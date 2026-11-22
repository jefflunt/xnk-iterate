package server

import (
	"encoding/json"
	"io/fs"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/jefflunt/xnk-iterate/internal/applog"
	"github.com/jefflunt/xnk-iterate/internal/mcp"
	"github.com/jefflunt/xnk-iterate/internal/model"
	"github.com/jefflunt/xnk-iterate/internal/session"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 16384,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// Server hosts the embedded XNK Companion HUD, REST endpoints (/api/bind, /api/stop, /api/logs),
// WebSocket hub (/ws), and Vision MCP endpoint (/api/mcp).
type Server struct {
	orch      *session.Orchestrator
	mcpServer *mcp.Server
	mux       *http.ServeMux
	listener  net.Listener
	httpSrv   *http.Server
}

// New creates a local server bound to orchestrator and static frontend FS.
func New(orch *session.Orchestrator, assetsFS fs.FS) *Server {
	s := &Server{
		orch:      orch,
		mcpServer: mcp.NewServer(orch),
		mux:       http.NewServeMux(),
	}

	s.mux.HandleFunc("/api/state", s.handleState)
	s.mux.HandleFunc("/api/targets", s.handleTargets)
	s.mux.HandleFunc("/api/frame", s.handleFrame)
	s.mux.HandleFunc("/api/logs", s.handleLogs)
	s.mux.HandleFunc("/api/stop", s.handleStop)
	s.mux.HandleFunc("/api/bind", s.handleBind)
	s.mux.HandleFunc("/api/unbind", s.handleUnbind)
	s.mux.Handle("/api/mcp", s.mcpServer)
	s.mux.HandleFunc("/ws", s.handleWS)

	if assetsFS != nil {
		s.mux.Handle("/", http.FileServer(http.FS(assetsFS)))
	}

	return s
}

// Start binds to addr (falling back to 127.0.0.1:0 if the preferred port is occupied).
func (s *Server) Start(addr string) (string, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil && addr != "127.0.0.1:0" {
		applog.Warn("server", "preferred addr %s unavailable (%v); falling back to ephemeral port", addr, err)
		ln, err = net.Listen("tcp", "127.0.0.1:0")
	}
	if err != nil {
		return "", err
	}
	s.listener = ln
	s.httpSrv = &http.Server{
		Handler:           s.mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		_ = s.httpSrv.Serve(ln)
	}()
	return "http://" + ln.Addr().String(), nil
}

// Close shuts down the HTTP listener.
func (s *Server) Close() error {
	if s.httpSrv != nil {
		return s.httpSrv.Close()
	}
	return nil
}

// Handler exposes the underlying http.Handler for testing.
func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.orch.StateSnapshot())
}

func (s *Server) handleTargets(w http.ResponseWriter, r *http.Request) {
	targets, err := s.orch.ListTargets()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(targets)
}

func (s *Server) handleFrame(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Query().Get("path")
	if p == "" {
		http.Error(w, "missing path", http.StatusBadRequest)
		return
	}
	cleanTarget := filepath.Clean(p)
	allowedRoots := []string{
		filepath.Clean(s.orch.StorageRoot()),
		filepath.Clean(s.orch.BrainRoot()),
	}
	allowed := false
	for _, root := range allowedRoots {
		if strings.HasPrefix(cleanTarget, root+string(filepath.Separator)) {
			allowed = true
			break
		}
	}
	if !allowed {
		http.Error(w, "forbidden path", http.StatusForbidden)
		return
	}
	http.ServeFile(w, r, cleanTarget)
}

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"path":  applog.Path(),
		"lines": applog.Recent(120),
	})
}

func (s *Server) handleStop(w http.ResponseWriter, r *http.Request) {
	applog.Info("server", "POST /api/stop received")
	path, err := s.orch.StopSession()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"stopped":     true,
		"bundle_path": path,
		"error":       errString(err),
	})
}

func (s *Server) handleBind(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ConversationID string `json:"conversation_id"`
		LSAddress      string `json:"ls_address,omitempty"`
		CSRFToken      string `json:"csrf_token,omitempty"`
		AgentAPIExe    string `json:"agentapi_exe,omitempty"`
		AppDataDir     string `json:"app_data_dir,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	applog.Info("server", "POST /api/bind conversation_id=%s ls_address=%s", body.ConversationID, body.LSAddress)
	meta, err := s.orch.BindConversationWithEnv(body.ConversationID, body.LSAddress, body.CSRFToken, body.AgentAPIExe, body.AppDataDir)
	w.Header().Set("Content-Type", "application/json")
	resp := map[string]any{
		"bound":           err == nil,
		"conversation_id": meta.ConversationID,
		"repo_name":       meta.RepoName,
		"branch_name":     meta.BranchName,
		"workspace_path":  meta.WorkspacePath,
		"warning":         errString(err),
	}
	if err != nil && body.LSAddress == "" {
		resp["hint"] = "Re-read ~/.gemini/config/skills/xnk-iterate/SKILL.md and include ls_address ($ANTIGRAVITY_LS_ADDRESS), csrf_token ($ANTIGRAVITY_CSRF_TOKEN), and agentapi_exe ($ANTIGRAVITY_AGENTAPI_EXE) in the POST /api/bind JSON body."
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleUnbind(w http.ResponseWriter, r *http.Request) {
	s.orch.UnbindConversation()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"bound": false})
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

type wsClientCommand struct {
	Action           string               `json:"action"`
	TargetID         string               `json:"target_id,omitempty"`
	ConversationID   string               `json:"conversation_id,omitempty"`
	SilenceTimeoutMS int                  `json:"silence_timeout_ms,omitempty"`
	Text             string               `json:"text,omitempty"`
	DrawerOpen       bool                 `json:"drawer_open,omitempty"`
	Region           *model.CaptureRegion `json:"region,omitempty"`
	Hotkeys          *model.HotkeyConfig  `json:"hotkeys,omitempty"`
	HotkeyRecording  bool                 `json:"hotkey_recording,omitempty"`
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	_ = conn.WriteJSON(session.LiveEvent{
		Type:    "state",
		Payload: s.orch.StateSnapshot(),
	})

	sub := s.orch.Subscribe()
	defer s.orch.Unsubscribe(sub)

	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case ev, ok := <-sub:
				if !ok {
					return
				}
				if err := conn.WriteJSON(ev); err != nil {
					return
				}
			case <-ticker.C:
				snap := s.orch.StateSnapshot()
				if snap.Recording && !snap.Paused {
					if err := conn.WriteJSON(session.LiveEvent{Type: "state", Payload: snap}); err != nil {
						return
					}
				}
			}
		}
	}()

	for {
		var cmd wsClientCommand
		if err := conn.ReadJSON(&cmd); err != nil {
			return
		}
		s.dispatchCommand(cmd)
	}
}

func (s *Server) dispatchCommand(cmd wsClientCommand) {
	applog.Info("ws", "command received: action=%s", cmd.Action)
	switch cmd.Action {
	case "start_session":
		go func() {
			if err := s.orch.StartSession(session.StartOptions{
				TargetID:       cmd.TargetID,
				ConversationID: cmd.ConversationID,
			}); err != nil {
				applog.Error("ws", "start_session error: %v", err)
			}
		}()
	case "stop_session":
		go func() {
			if _, err := s.orch.StopSession(); err != nil {
				applog.Error("ws", "stop_session error: %v", err)
			}
		}()
	case "toggle_pause":
		s.orch.TogglePause()
	case "send_turn":
		s.orch.SendCurrentTurn()
	case "discard_turn":
		s.orch.DiscardCurrentTurn()
	case "toggle_box_visible":
		s.orch.ToggleCaptureBoxVisible()
	case "set_drawer":
		s.orch.SetDrawerOpen(cmd.DrawerOpen)
	case "set_region":
		if cmd.Region != nil {
			s.orch.UpdateRegion(*cmd.Region, false)
		}
	case "bind_conversation":
		go func() {
			_, _ = s.orch.BindConversation(cmd.ConversationID)
		}()
	case "unbind_conversation":
		s.orch.UnbindConversation()
	case "update_settings":
		s.orch.UpdateSettings(cmd.ConversationID, cmd.SilenceTimeoutMS)
	case "update_hotkeys":
		if cmd.Hotkeys != nil {
			_ = s.orch.UpdateHotkeys(*cmd.Hotkeys)
		}
	case "reset_hotkeys":
		s.orch.ResetHotkeys()
	case "set_hotkey_recording":
		s.orch.SetHotkeyRecording(cmd.HotkeyRecording)
	case "inject_utterance":
		s.orch.SubmitUtterance(cmd.Text, 1.0)
	}
}
