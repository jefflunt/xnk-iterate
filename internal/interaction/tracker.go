package interaction

import (
	"sync"

	"github.com/jefflunt/xnk-iterate/internal/model"
)

// Timeline stores a thread-safe chronological log of user interaction events.
type Timeline struct {
	mu      sync.RWMutex
	events  []model.InteractionEvent
	maxSize int
	onEvent func(model.InteractionEvent)
}

// NewTimeline creates an interaction timeline retaining up to maxSize events.
func NewTimeline(maxSize int) *Timeline {
	if maxSize <= 0 {
		maxSize = 5000
	}
	return &Timeline{
		events:  make([]model.InteractionEvent, 0, 128),
		maxSize: maxSize,
	}
}

// SetCallback registers an optional callback invoked whenever a new interaction is recorded.
func (t *Timeline) SetCallback(fn func(model.InteractionEvent)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.onEvent = fn
}

// Record appends an interaction event to the timeline.
func (t *Timeline) Record(ev model.InteractionEvent) {
	t.mu.Lock()
	t.events = append(t.events, ev)
	if len(t.events) > t.maxSize {
		t.events = t.events[len(t.events)-t.maxSize:]
	}
	cb := t.onEvent
	t.mu.Unlock()

	if cb != nil {
		cb(ev)
	}
}

// InRange returns all interaction events with timestamp in [startSec, endSec].
func (t *Timeline) InRange(startSec, endSec float64) []model.InteractionEvent {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var out []model.InteractionEvent
	for _, ev := range t.events {
		if ev.Timestamp >= startSec && ev.Timestamp <= endSec {
			out = append(out, ev)
		}
	}
	return out
}

// All returns a copy of all recorded interactions.
func (t *Timeline) All() []model.InteractionEvent {
	t.mu.RLock()
	defer t.mu.RUnlock()

	out := make([]model.InteractionEvent, len(t.events))
	copy(out, t.events)
	return out
}

// Clear resets the timeline.
func (t *Timeline) Clear() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.events = t.events[:0]
}

// Tracker is the cross-platform OS input/focus monitoring interface.
type Tracker interface {
	Start(elapsedFn func() float64, timeline *Timeline) error
	Stop()
	AccessibilityStatus() string
}
