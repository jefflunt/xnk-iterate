package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Provider defines the runtime hooks invoked by the Vision MCP server tools.
type Provider interface {
	GrabFrame(timestamp *float64) (framePath string, actualTS float64, err error)
	GetClip(startSec, endSec float64) (clipPath string, err error)
	SessionStatus() map[string]any
}

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      any       `json:"id,omitempty"`
	Result  any       `json:"result,omitempty"`
	Error   *rpcError `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Server implements a lightweight JSON-RPC 2.0 MCP server exposing xnk_grab_frame,
// xnk_get_clip, and xnk_session_status over either stdio or HTTP POST.
type Server struct {
	provider Provider
}

// NewServer creates a Vision MCP server backed by provider.
func NewServer(provider Provider) *Server {
	return &Server{provider: provider}
}

// HandleRequest processes a single JSON-RPC 2.0 MCP message and returns the response payload.
func (s *Server) HandleRequest(raw []byte) ([]byte, bool) {
	var req rpcRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return mustMarshal(rpcResponse{
			JSONRPC: "2.0",
			Error:   &rpcError{Code: -32700, Message: "parse error"},
		}), true
	}

	// Notifications without ID require no response.
	if req.ID == nil && req.Method == "notifications/initialized" {
		return nil, false
	}

	switch req.Method {
	case "initialize":
		return mustMarshal(rpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"protocolVersion": "2024-11-05",
				"capabilities": map[string]any{
					"tools": map[string]any{},
				},
				"serverInfo": map[string]any{
					"name":    "xnk-iterate",
					"version": "1.2.0",
				},
			},
		}), true

	case "tools/list":
		return mustMarshal(rpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"tools": []map[string]any{
					{
						"name":        "xnk_grab_frame",
						"description": "Extract a frame snapshot from the active xnk-iterate rolling ring buffer (defaults to latest frame) and return its local file path.",
						"inputSchema": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"timestamp": map[string]any{
									"type":        "number",
									"description": "Optional recording-relative timestamp in seconds.",
								},
							},
						},
					},
					{
						"name":        "xnk_get_clip",
						"description": "Slice a lightweight sub-clip (.mp4) from the active fMP4 recording between start and end seconds.",
						"inputSchema": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"start": map[string]any{"type": "number"},
								"end":   map[string]any{"type": "number"},
							},
							"required": []string{"start", "end"},
						},
					},
					{
						"name":        "xnk_session_status",
						"description": "Return current xnk-iterate recording state, target window metadata, elapsed time, and recent utterance IDs.",
						"inputSchema": map[string]any{
							"type":       "object",
							"properties": map[string]any{},
						},
					},
				},
			},
		}), true

	case "tools/call":
		var params struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return mustMarshal(rpcResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   &rpcError{Code: -32602, Message: "invalid params"},
			}), true
		}
		return s.handleToolCall(req.ID, params.Name, params.Arguments), true

	default:
		return mustMarshal(rpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &rpcError{Code: -32601, Message: "method not found: " + req.Method},
		}), true
	}
}

func (s *Server) handleToolCall(id any, name string, args map[string]any) []byte {
	if s.provider == nil {
		return toolTextResult(id, `{"error":"provider not configured"}`, true)
	}

	switch name {
	case "xnk_grab_frame":
		var ts *float64
		if v, ok := args["timestamp"].(float64); ok {
			ts = &v
		}
		path, actualTS, err := s.provider.GrabFrame(ts)
		if err != nil {
			return toolTextResult(id, fmt.Sprintf(`{"error":%q}`, err.Error()), true)
		}
		payload, _ := json.Marshal(map[string]any{
			"frame_path": path,
			"timestamp":  actualTS,
		})
		return toolTextResult(id, string(payload), false)

	case "xnk_get_clip":
		startSec, _ := args["start"].(float64)
		endSec, _ := args["end"].(float64)
		clipPath, err := s.provider.GetClip(startSec, endSec)
		if err != nil {
			return toolTextResult(id, fmt.Sprintf(`{"error":%q}`, err.Error()), true)
		}
		payload, _ := json.Marshal(map[string]any{
			"clip_path": clipPath,
			"start":     startSec,
			"end":       endSec,
		})
		return toolTextResult(id, string(payload), false)

	case "xnk_session_status":
		payload, _ := json.Marshal(s.provider.SessionStatus())
		return toolTextResult(id, string(payload), false)

	default:
		return mustMarshal(rpcResponse{
			JSONRPC: "2.0",
			ID:      id,
			Error:   &rpcError{Code: -32601, Message: "unknown tool: " + name},
		})
	}
}

func toolTextResult(id any, text string, isError bool) []byte {
	return mustMarshal(rpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result: map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": text},
			},
			"isError": isError,
		},
	})
}

func mustMarshal(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

// ServeStdio runs the MCP JSON-RPC loop over stdin/stdout.
func (s *Server) ServeStdio(r io.Reader, w io.Writer) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		resp, ok := s.HandleRequest(line)
		if !ok {
			continue
		}
		if _, err := w.Write(append(resp, '\n')); err != nil {
			return err
		}
	}
	return scanner.Err()
}

// ServeHTTP exposes the MCP server over HTTP POST for local/embedded integration.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 4*1024*1024))
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}
	resp, ok := s.HandleRequest(body)
	w.Header().Set("Content-Type", "application/json")
	if !ok {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	_, _ = w.Write(resp)
}
