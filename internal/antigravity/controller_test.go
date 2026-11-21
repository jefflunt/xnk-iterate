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

func TestHandleStreamLines(t *testing.T) {
	var deltas []string
	var finalMsg string
	var permReq *model.PermissionRequest
	var convID string

	ctrl := NewController(Callbacks{
		OnConversationID: func(id string) { convID = id },
		OnTextDelta:      func(d string) { deltas = append(deltas, d) },
		OnAssistantMessage: func(full string) {
			finalMsg = full
		},
		OnPermissionReq: func(req model.PermissionRequest) {
			permReq = &req
		},
	})

	var buf strings.Builder
	ctrl.handleStreamLine(`{"type":"init","conversation_id":"conv_999"}`, &buf)
	ctrl.handleStreamLine(`{"type":"text_delta","delta":"Fixing the scoreboard now."}`, &buf)
	ctrl.handleStreamLine(`{"type":"permission_request","request_id":"p1","tool_name":"run_command","command":"swift test","description":"Run unit tests"}`, &buf)
	ctrl.handleStreamLine(`{"type":"turn_complete"}`, &buf)

	if convID != "conv_999" {
		t.Fatalf("expected conv_999, got %q", convID)
	}
	if len(deltas) != 1 || finalMsg != "Fixing the scoreboard now." {
		t.Fatalf("unexpected deltas=%v final=%q", deltas, finalMsg)
	}
	if permReq == nil || permReq.ID != "p1" || permReq.Command != "swift test" {
		t.Fatalf("unexpected permission request: %+v", permReq)
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
