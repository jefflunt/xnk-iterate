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

func TestHandleRealAgyStreamJSONEvents(t *testing.T) {
	var deltas []string
	var finalMsg string
	var convID string
	var gotErr error

	ctrl := NewController(Callbacks{
		OnConversationID:   func(id string) { convID = id },
		OnTextDelta:        func(d string) { deltas = append(deltas, d) },
		OnAssistantMessage: func(full string) { finalMsg = full },
		OnError:            func(err error) { gotErr = err },
	})

	var buf strings.Builder
	ctrl.handleStreamLine(`{"event":"init","conversation_id":"4f49aa39-8df4-4d0b-a4bc-b408b76d222b"}`, &buf)
	ctrl.handleStreamLine(`{"event":"step_update","step_update":{"conversation_id":"4f49aa39-8df4-4d0b-a4bc-b408b76d222b","step_index":1,"state":"ACTIVE","step_type":"agent_response","text_delta":"Fixed the button alignment."}}`, &buf)
	ctrl.handleStreamLine(`{"event":"result","result":{"conversation_id":"4f49aa39-8df4-4d0b-a4bc-b408b76d222b","status":"SUCCESS","response":"Fixed the button alignment.\n"}}`, &buf)

	if convID != "4f49aa39-8df4-4d0b-a4bc-b408b76d222b" {
		t.Fatalf("expected conversation ID, got %q", convID)
	}
	if len(deltas) != 1 || finalMsg != "Fixed the button alignment." {
		t.Fatalf("unexpected deltas=%v final=%q", deltas, finalMsg)
	}
	if gotErr != nil {
		t.Fatalf("did not expect error, got %v", gotErr)
	}

	// Verify ERROR result surfaces cleanly
	ctrl.handleStreamLine(`{"event":"result","result":{"status":"ERROR","error":"stream input error"}}`, &buf)
	if gotErr == nil || !strings.Contains(gotErr.Error(), "stream input error") {
		t.Fatalf("expected error callback, got %v", gotErr)
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
