package lookback

import (
	"os"
	"testing"

	"github.com/jefflunt/xnk-iterate/internal/interaction"
	"github.com/jefflunt/xnk-iterate/internal/model"
	"github.com/jefflunt/xnk-iterate/internal/ringbuffer"
)

func TestLookbackWithAnchorAction(t *testing.T) {
	tl := interaction.NewTimeline(100)
	tl.Record(model.InteractionEvent{
		Timestamp:  25.80,
		Type:       model.InteractionMouseDown,
		Location:   &model.Point{X: 1380, Y: 42},
		FocusedApp: "com.apple.Safari",
	})

	eng := New()
	res := eng.Calculate(28.50, 33.20, tl)

	if res.AnchorAction == nil {
		t.Fatalf("expected anchor action")
	}
	if res.Range.TriggerSeconds != 25.80 {
		t.Fatalf("expected trigger at 25.80, got %.2f", res.Range.TriggerSeconds)
	}
	if res.Range.StartSeconds != 24.80 {
		t.Fatalf("expected start at 24.80 (1s before click), got %.2f", res.Range.StartSeconds)
	}
	if res.Range.EndSeconds != 33.20 {
		t.Fatalf("expected end at 33.20, got %.2f", res.Range.EndSeconds)
	}
	if len(res.CarouselTime) != 0 {
		t.Fatalf("did not expect carousel for short comment")
	}
}

func TestLookbackExtendedMonologueFallback(t *testing.T) {
	eng := New()
	// 40-second monologue with no clicks -> buffer = max(3.5, 40*0.25) = 10s
	res := eng.Calculate(20.0, 60.0, nil)
	if res.Range.StartSeconds != 10.0 {
		t.Fatalf("expected start at 10.0, got %.2f", res.Range.StartSeconds)
	}
	if len(res.CarouselTime) != 5 {
		t.Fatalf("expected 5 carousel timestamps for extended comment, got %d", len(res.CarouselTime))
	}
}

func TestExtractFrames(t *testing.T) {
	rb := ringbuffer.New(30, 60)
	rb.Push(25.5, "png", []byte("trigger-bytes"))
	rb.Push(33.1, "png", []byte("reaction-bytes"))

	eng := New()
	res := eng.Calculate(28.5, 33.2, nil)

	dir := t.TempDir()
	trig, react, _, snaps, err := eng.ExtractFrames(1, res, rb, dir)
	if err != nil {
		t.Fatalf("ExtractFrames failed: %v", err)
	}
	if trig == "" || react == "" || len(snaps) != 2 {
		t.Fatalf("expected trigger and reaction frames, got trig=%q react=%q snaps=%d", trig, react, len(snaps))
	}
	if _, err := os.Stat(trig); err != nil {
		t.Fatalf("trigger file missing: %v", err)
	}
}
