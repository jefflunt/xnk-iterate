package antigravity

import (
	"strings"
	"testing"

	"github.com/jefflunt/xnk-iterate/internal/model"
)

func TestParseVoiceApprovalIntent(t *testing.T) {
	cases := []struct {
		in   string
		want ApprovalDecision
	}{
		{"Approve", DecisionApprove},
		{"yes go ahead", DecisionApprove},
		{"Okay, do it.", DecisionApprove},
		{"deny", DecisionDeny},
		{"No, cancel that!", DecisionDeny},
		{"Wait what file is that?", DecisionNone},
	}
	for _, tc := range cases {
		if got := ParseVoiceApprovalIntent(tc.in); got != tc.want {
			t.Fatalf("ParseVoiceApprovalIntent(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestHandleACPStreamingAndPermissionRequest(t *testing.T) {
	var deltas []string
	var finalMsg string
	var convID string
	var permReq *model.PermissionRequest
	var toolTitle string

	ctrl := NewController(Callbacks{
		OnConversationID:   func(id string) { convID = id },
		OnTextDelta:        func(d string) { deltas = append(deltas, d) },
		OnAssistantMessage: func(full string) { finalMsg = full },
		OnToolCall:         func(id, title, status string) { toolTitle = title },
		OnPermissionReq:    func(req model.PermissionRequest) { permReq = &req },
	})

	ctrl.handleACPLine(`{"jsonrpc":"2.0","id":2,"result":{"sessionId":"3fd5d7a1-9f56-469e-ad42-846bd13acffe"}}`)
	if convID != "3fd5d7a1-9f56-469e-ad42-846bd13acffe" {
		t.Fatalf("expected sessionId, got %q", convID)
	}

	ctrl.mu.Lock()
	ctrl.activePrompt = 3
	ctrl.mu.Unlock()

	ctrl.handleACPLine(`{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"3fd5d7a1","update":{"sessionUpdate":"tool_call","toolCallId":"tc_1","title":"Edit game_renderer.js","status":"in_progress"}}}`)
	if toolTitle != "Edit game_renderer.js" {
		t.Fatalf("expected tool title, got %q", toolTitle)
	}

	ctrl.handleACPLine(`{"jsonrpc":"2.0","id":99,"method":"session/request_permission","params":{"sessionId":"3fd5d7a1","toolCall":{"title":"Edit game_renderer.js","rawInput":{"Path":"/Users/jefflunt/code/kingsclaim/app/javascript/game_renderer.js"}},"options":[{"optionId":"allow_once","name":"Allow once","kind":"allow_once"},{"optionId":"allow_always","name":"Allow always","kind":"allow_always"},{"optionId":"reject_once","name":"Reject","kind":"reject_once"}]}}`)
	if permReq == nil || len(permReq.Options) != 3 || !strings.Contains(permReq.Command, "game_renderer.js") {
		t.Fatalf("unexpected permission request: %+v", permReq)
	}

	ctrl.handleACPLine(`{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"3fd5d7a1","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"Fixed drawScoreOverlay()."}}}}`)
	ctrl.handleACPLine(`{"jsonrpc":"2.0","id":3,"result":{"stopReason":"end_turn"}}`)

	if len(deltas) != 1 || finalMsg != "Fixed drawScoreOverlay()." {
		t.Fatalf("unexpected deltas=%v final=%q", deltas, finalMsg)
	}
}

func TestFormatDirectorTurn(t *testing.T) {
	u := model.Utterance{
		ID:                4,
		Transcript:        "Player 1 color is inverted.",
		SpeechStart:       28.5,
		SpeechEnd:         33.2,
		LookbackWindow:    model.LookbackRange{StartSeconds: 24.8, EndSeconds: 33.2, TriggerSeconds: 25.8, Duration: 8.4},
		TriggerFramePath:  "/tmp/trigger.jpg",
		ReactionFramePath: "/tmp/reaction.jpg",
	}
	prompt := FormatDirectorTurn([]model.Utterance{u}, &model.CaptureTarget{Title: "Safari", Type: model.TargetWindow})
	if !strings.Contains(prompt, "Player 1 color is inverted.") || !strings.Contains(prompt, "/tmp/trigger.jpg") {
		t.Fatalf("formatted prompt missing expected fields: %s", prompt)
	}
}
