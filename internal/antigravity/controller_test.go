package antigravity

import (
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
	want := "Player 1 color is inverted.\n\n`#4 · 24.8s–33.2s` · [trigger](/tmp/trigger.jpg) · [reaction](/tmp/reaction.jpg)"
	if prompt != want {
		t.Fatalf("unexpected formatted prompt:\ngot:  %q\nwant: %q", prompt, want)
	}
}
