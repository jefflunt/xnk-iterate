package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jefflunt/xnk-iterate/internal/model"
	"github.com/jefflunt/xnk-iterate/internal/session"
	"github.com/jefflunt/xnk-iterate/internal/ui"
)

func TestAPIStateAndTargets(t *testing.T) {
	orch := session.NewOrchestrator(t.TempDir())
	srv := New(orch, ui.AssetsFS())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/state", nil)
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var snap model.SessionStateSnapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &snap); err != nil {
		t.Fatalf("invalid state json: %v", err)
	}
	if snap.Recording {
		t.Fatalf("expected not recording initially")
	}

	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/api/targets", nil)
	srv.Handler().ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("expected 200 for targets, got %d", rec2.Code)
	}
}

func TestEmbeddedXNKAssetsAndMCPEndpoint(t *testing.T) {
	orch := session.NewOrchestrator(t.TempDir())
	srv := New(orch, ui.AssetsFS())

	// 1. Verify root HTML
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "XNK Iterate") {
		t.Fatalf("expected embedded index.html, got status=%d", rec.Code)
	}

	// 2. Verify authentic XNK Design System v2.0 tokens CSS
	recCSS := httptest.NewRecorder()
	srv.Handler().ServeHTTP(recCSS, httptest.NewRequest(http.MethodGet, "/xnk/tokens/colors.css", nil))
	if recCSS.Code != http.StatusOK || !strings.Contains(recCSS.Body.String(), "--xnk-accent-cobalt") {
		t.Fatalf("expected XNK colors.css token sheet, got status=%d", recCSS.Code)
	}

	// 3. Verify embedded Vision MCP endpoint
	mcpBody := bytes.NewBufferString(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	recMCP := httptest.NewRecorder()
	srv.Handler().ServeHTTP(recMCP, httptest.NewRequest(http.MethodPost, "/api/mcp", mcpBody))
	if recMCP.Code != http.StatusOK || !strings.Contains(recMCP.Body.String(), "xnk_grab_frame") {
		t.Fatalf("expected xnk_grab_frame from /api/mcp, got status=%d body=%s", recMCP.Code, recMCP.Body.String())
	}
}

func TestLiveCommandDispatchAndSubscription(t *testing.T) {
	orch := session.NewOrchestrator(t.TempDir())
	srv := New(orch, ui.AssetsFS())

	sub := orch.Subscribe()
	defer orch.Unsubscribe(sub)

	srv.dispatchCommand(wsClientCommand{
		Action: "inject_utterance",
		Text:   "Align the telemetry strip to 16px padding.",
	})

	found := false
	timeout := time.After(2 * time.Second)
	for !found {
		select {
		case ev := <-sub:
			raw, _ := json.Marshal(ev.Payload)
			if strings.Contains(string(raw), "Align the telemetry strip") {
				found = true
			}
		case <-timeout:
			t.Fatalf("timed out waiting for subscription event containing injected utterance")
		}
	}
}
