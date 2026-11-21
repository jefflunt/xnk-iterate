package session

import (
	"testing"

	"github.com/jefflunt/xnk-iterate/internal/model"
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

func TestCaptureRegionDetachAndLockBehavior(t *testing.T) {
	o := NewOrchestrator(t.TempDir())

	// Can resize & detach before recording
	o.UpdateRegion(model.CaptureRegion{X: 200, Y: 100, Width: 1440, Height: 900, DisplayIndex: 1}, false)
	if !o.ToggleDetachCaptureBox() {
		t.Fatalf("expected detached=true")
	}
	snap := o.StateSnapshot()
	if snap.Region.Width != 1440 || snap.Region.DisplayIndex != 1 || !snap.Region.Detached {
		t.Fatalf("unexpected region state: %+v", snap.Region)
	}

	// Simulate locked recording state: mutations must be ignored
	o.mu.Lock()
	o.recording = true
	o.region.Locked = true
	o.mu.Unlock()

	o.UpdateRegion(model.CaptureRegion{X: 0, Y: 0, Width: 500, Height: 500}, false)
	if !o.ToggleDetachCaptureBox() {
		t.Fatalf("expected detached state to remain locked during recording")
	}
	snap2 := o.StateSnapshot()
	if snap2.Region.Width != 1440 || !snap2.Region.Locked {
		t.Fatalf("expected region to stay locked at 1440 during recording, got %+v", snap2.Region)
	}
}

func TestStopSessionDoesNotDeadlock(t *testing.T) {
	o := NewOrchestrator(t.TempDir())
	o.mu.Lock()
	o.recording = true
	o.captureID = "cap_deadlock_test"
	o.bundleDir = t.TempDir()
	o.startedAt = o.startedAt.Add(-1)
	o.mu.Unlock()

	done := make(chan struct{})
	go func() {
		_, _ = o.StopSession()
		close(done)
	}()

	<-done
	if o.StateSnapshot().Recording {
		t.Fatalf("expected recording=false after StopSession")
	}
}
