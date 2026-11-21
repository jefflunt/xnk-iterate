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
	"github.com/jefflunt/xnk-iterate/internal/antigravity"
	"github.com/jefflunt/xnk-iterate/internal/mcp"
	"github.com/jefflunt/xnk-iterate/internal/session"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 16384,
	CheckOrigin: func(r *http.Request) bool {
		// Localhost desktop WebView / local tooling only
		return true
	},
}

// Server hosts the embedded XNK UI, REST endpoints, WebSocket hub, and Vision MCP endpoint.
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
	s.mux.Handle("/api/mcp", s.mcpServer)
	s.mux.HandleFunc("/ws", s.handleWS)

	if assetsFS != nil {
		s.mux.Handle("/", http.FileServer(http.FS(assetsFS)))
	}

	return s
}

// Start binds to addr (e.g. "127.0.0.1:0" or "127.0.0.1:9292") and returns the actual URL.
func (s *Server) Start(addr string) (string, error) {
	ln, err := net.Listen("tcp", addr)
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
	cleanRoot := filepath.Clean(s.orch.StorageRoot())
	if !strings.HasPrefix(cleanTarget, cleanRoot+string(filepath.Separator)) {
		http.Error(w, "forbidden path", http.StatusForbidden)
		return
	}
	http.ServeFile(w, r, cleanTarget)
}

type wsClientCommand struct {
	Action          string `json:"action"`
	TargetID        string `json:"target_id,omitempty"`
	WorkspacePath   string `json:"workspace_path,omitempty"`
	ConversationID  string `json:"conversation_id,omitempty"`
	ContinueSession bool   `json:"continue_session,omitempty"`
	Model           string `json:"model,omitempty"`
	Mode            string `json:"mode,omitempty"`
	Text            string `json:"text,omitempty"`
	RequestID       string `json:"request_id,omitempty"`
	Decision        string `json:"decision,omitempty"`
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	// Send initial snapshot immediately
	_ = conn.WriteJSON(session.LiveEvent{
		Type:    "state",
		Payload: s.orch.StateSnapshot(),
	})

	sub := s.orch.Subscribe()
	defer s.orch.Unsubscribe(sub)

	done := make(chan struct{})
	// Writer loop
	go func() {
		defer close(done)
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
				// Keep elapsed timer ticking smoothly while recording
				snap := s.orch.StateSnapshot()
				if snap.Recording && !snap.Paused {
					if err := conn.WriteJSON(session.LiveEvent{Type: "state", Payload: snap}); err != nil {
						return
					}
				}
			}
		}
	}()

	// Reader loop
	for {
		var cmd wsClientCommand
		if err := conn.ReadJSON(&cmd); err != nil {
			return
		}
		s.dispatchCommand(cmd)
	}
}

func (s *Server) dispatchCommand(cmd wsClientCommand) {
	switch cmd.Action {
	case "start_session":
		go func() {
			_ = s.orch.StartSession(session.StartOptions{
				TargetID:        cmd.TargetID,
				WorkspacePath:   cmd.WorkspacePath,
				ConversationID:  cmd.ConversationID,
				ContinueSession: cmd.ContinueSession,
				Model:           cmd.Model,
				Mode:            cmd.Mode,
			})
		}()
	case "stop_session":
		go func() {
			_, _ = s.orch.StopSession()
		}()
	case "toggle_pause":
		s.orch.TogglePause()
	case "toggle_batch":
		s.orch.ToggleBatchMode()
	case "flush_batch":
		s.orch.FlushBatch()
	case "discard_last":
		s.orch.DiscardLastUtterance()
	case "toggle_mute":
		s.orch.ToggleVoiceMute()
	case "toggle_hud_mode":
		s.orch.ToggleHUDCompactMode()
	case "inject_utterance":
		s.orch.SubmitUtterance(cmd.Text, 1.0)
	case "resolve_approval":
		dec := antigravity.DecisionDeny
		if cmd.Decision == "approve" {
			dec = antigravity.DecisionApprove
		}
		_ = s.orch.ResolveApproval(cmd.RequestID, dec)
	}
}
