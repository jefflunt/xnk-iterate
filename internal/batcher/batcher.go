package batcher

import (
	"sync"

	"github.com/jefflunt/xnk-iterate/internal/model"
)

// Dispatcher manages Auto-Stream vs Batch-Hold dispatch cadence for director utterances.
type Dispatcher struct {
	mu        sync.Mutex
	batchMode bool
	pending   []model.Utterance
	onFlush   func([]model.Utterance)
}

// New creates a Dispatcher. onFlush is called whenever utterances are ready to be sent to Antigravity.
func New(onFlush func([]model.Utterance)) *Dispatcher {
	return &Dispatcher{
		onFlush: onFlush,
	}
}

// IsBatchMode returns whether Batch-Hold mode is currently active.
func (d *Dispatcher) IsBatchMode() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.batchMode
}

// PendingCount returns the number of utterances buffered in Batch-Hold mode.
func (d *Dispatcher) PendingCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.pending)
}

// SetBatchMode enables or disables Batch-Hold mode. Disabling Batch-Hold automatically flushes pending utterances.
func (d *Dispatcher) SetBatchMode(enabled bool) {
	var toFlush []model.Utterance

	d.mu.Lock()
	d.batchMode = enabled
	if !enabled && len(d.pending) > 0 {
		toFlush = make([]model.Utterance, len(d.pending))
		copy(toFlush, d.pending)
		d.pending = d.pending[:0]
	}
	cb := d.onFlush
	d.mu.Unlock()

	if len(toFlush) > 0 && cb != nil {
		cb(toFlush)
	}
}

// ToggleBatchMode flips Batch-Hold mode and returns the new state.
func (d *Dispatcher) ToggleBatchMode() bool {
	d.mu.Lock()
	next := !d.batchMode
	d.mu.Unlock()
	d.SetBatchMode(next)
	return next
}

// Submit accepts a finalized utterance. In Auto-Stream mode it flushes immediately;
// in Batch-Hold mode it buffers until Flush() or disabling Batch-Hold.
func (d *Dispatcher) Submit(u model.Utterance) (dispatchedImmediately bool) {
	d.mu.Lock()
	if d.batchMode {
		d.pending = append(d.pending, u)
		d.mu.Unlock()
		return false
	}
	cb := d.onFlush
	d.mu.Unlock()

	if cb != nil {
		cb([]model.Utterance{u})
	}
	return true
}

// Flush immediately dispatches any buffered utterances.
func (d *Dispatcher) Flush() int {
	d.mu.Lock()
	if len(d.pending) == 0 {
		d.mu.Unlock()
		return 0
	}
	toFlush := make([]model.Utterance, len(d.pending))
	copy(toFlush, d.pending)
	d.pending = d.pending[:0]
	cb := d.onFlush
	d.mu.Unlock()

	if cb != nil {
		cb(toFlush)
	}
	return len(toFlush)
}

// DiscardLastPending removes the most recently buffered utterance in Batch-Hold mode.
func (d *Dispatcher) DiscardLastPending() (model.Utterance, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.pending) == 0 {
		return model.Utterance{}, false
	}
	last := d.pending[len(d.pending)-1]
	d.pending = d.pending[:len(d.pending)-1]
	return last, true
}

// Clear drops all pending buffered utterances without dispatching.
func (d *Dispatcher) Clear() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.pending = d.pending[:0]
}
