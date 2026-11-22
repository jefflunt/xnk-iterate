package antigravity

import (
	"strings"
	"testing"

	"github.com/jefflunt/xnk-iterate/internal/model"
)

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
	prompt := FormatDirectorTurn([]model.Utterance{u}, &model.CaptureTarget{Title: "Display 1", Resolution: model.Size{Width: 1920, Height: 1080}})
	if !strings.Contains(prompt, "Player 1 color is inverted.") || !strings.Contains(prompt, "/tmp/trigger.jpg") {
		t.Fatalf("formatted prompt missing expected fields: %s", prompt)
	}
}
