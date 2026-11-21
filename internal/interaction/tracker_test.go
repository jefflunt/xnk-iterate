package interaction

import (
	"testing"

	"github.com/jefflunt/xnk-iterate/internal/model"
)

func TestTimelineInRangeAndEviction(t *testing.T) {
	tl := NewTimeline(3)
	tl.Record(model.InteractionEvent{Timestamp: 1.0, Type: model.InteractionMouseDown})
	tl.Record(model.InteractionEvent{Timestamp: 5.0, Type: model.InteractionMouseDown})
	tl.Record(model.InteractionEvent{Timestamp: 9.0, Type: model.InteractionKeyPress})
	tl.Record(model.InteractionEvent{Timestamp: 12.0, Type: model.InteractionMouseDown})

	if len(tl.All()) != 3 {
		t.Fatalf("expected 3 retained events, got %d", len(tl.All()))
	}

	r := tl.InRange(4.0, 10.0)
	if len(r) != 2 {
		t.Fatalf("expected 2 events in [4, 10], got %d", len(r))
	}
	if r[0].Timestamp != 5.0 || r[1].Timestamp != 9.0 {
		t.Fatalf("unexpected timestamps: %+v", r)
	}
}
