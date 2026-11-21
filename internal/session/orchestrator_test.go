package session

import (
	"testing"
)

func TestOrchestratorBatchAndUtteranceFlow(t *testing.T) {
	o := NewOrchestrator(t.TempDir())
	o.ringBuf.Push(1.0, "png", []byte("fake-png-1"))
	o.ringBuf.Push(4.0, "png", []byte("fake-png-2"))

	// Toggle batch mode on, submit 2 utterances, discard 1, flush
	if !o.ToggleBatchMode() {
		t.Fatalf("expected batch mode enabled")
	}
	o.SubmitUtterance("Check the header alignment", 0.95)
	o.SubmitUtterance("Oops ignore that", 0.90)
	if !o.DiscardLastUtterance() {
		t.Fatalf("expected discard to succeed")
	}
	flushed := o.FlushBatch()
	if flushed != 1 {
		t.Fatalf("expected 1 flushed utterance, got %d", flushed)
	}

	snap := o.StateSnapshot()
	if len(snap.Utterances) != 2 {
		t.Fatalf("expected 2 utterance records (1 discarded), got %d", len(snap.Utterances))
	}
	if !snap.Utterances[1].Discarded {
		t.Fatalf("expected second utterance marked discarded")
	}
	if len(snap.ChatHistory) != 1 || snap.ChatHistory[0].Role != "user" {
		t.Fatalf("expected 1 user chat entry, got %+v", snap.ChatHistory)
	}
}
