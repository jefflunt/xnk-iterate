package batcher

import (
	"testing"

	"github.com/jefflunt/xnk-iterate/internal/model"
)

func TestAutoStreamAndBatchHold(t *testing.T) {
	var batches [][]model.Utterance
	d := New(func(us []model.Utterance) {
		batches = append(batches, us)
	})

	// Default is auto-stream
	if !d.Submit(model.Utterance{ID: 1, Transcript: "Fix the button"}) {
		t.Fatalf("expected immediate dispatch in auto-stream mode")
	}
	if len(batches) != 1 || len(batches[0]) != 1 {
		t.Fatalf("expected 1 batch of 1 utterance")
	}

	// Enable Batch-Hold
	d.SetBatchMode(true)
	if d.Submit(model.Utterance{ID: 2, Transcript: "First part"}) {
		t.Fatalf("expected buffered in batch mode")
	}
	if d.Submit(model.Utterance{ID: 3, Transcript: "Misspoken"}) {
		t.Fatalf("expected buffered in batch mode")
	}
	if _, ok := d.DiscardLastPending(); !ok {
		t.Fatalf("expected discard to succeed")
	}
	d.Submit(model.Utterance{ID: 4, Transcript: "Second part"})

	if d.PendingCount() != 2 {
		t.Fatalf("expected 2 pending, got %d", d.PendingCount())
	}

	// Disabling batch mode flushes combined batch
	d.SetBatchMode(false)
	if len(batches) != 2 || len(batches[1]) != 2 {
		t.Fatalf("expected combined batch of 2 utterances, got %+v", batches)
	}
	if batches[1][0].ID != 2 || batches[1][1].ID != 4 {
		t.Fatalf("unexpected batched IDs: %+v", batches[1])
	}
}
