package mcp

import (
	"encoding/json"
	"strings"
	"testing"
)

type fakeProvider struct{}

func (fakeProvider) GrabFrame(ts *float64) (string, float64, error) {
	if ts != nil {
		return "/tmp/frame_10.png", *ts, nil
	}
	return "/tmp/frame_latest.png", 12.5, nil
}
func (fakeProvider) GetClip(start, end float64) (string, error) {
	return "/tmp/clip.mp4", nil
}
func (fakeProvider) SessionStatus() map[string]any {
	return map[string]any{"recording": true, "elapsed_seconds": 12.5}
}

func TestMCPToolsListAndCall(t *testing.T) {
	srv := NewServer(fakeProvider{})

	respBytes, ok := srv.HandleRequest([]byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	if !ok || !strings.Contains(string(respBytes), "xnk_grab_frame") {
		t.Fatalf("expected xnk_grab_frame in tools/list, got %s", string(respBytes))
	}

	callResp, ok := srv.HandleRequest([]byte(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"xnk_grab_frame","arguments":{"timestamp":9.5}}}`))
	if !ok {
		t.Fatalf("expected response")
	}
	var parsed map[string]any
	if err := json.Unmarshal(callResp, &parsed); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if !strings.Contains(string(callResp), "/tmp/frame_10.png") {
		t.Fatalf("unexpected call response: %s", string(callResp))
	}
}
