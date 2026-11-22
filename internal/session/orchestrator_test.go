package session

import (
	"path/filepath"
	"testing"

	"github.com/jefflunt/xnk-iterate/internal/model"
)

func TestOrchestratorExplicitSendAndDiscardFlow(t *testing.T) {
	o := NewOrchestrator(t.TempDir())
	o.ringBuf.Push(1.0, "png", []byte("fake-png-1"))
	o.ringBuf.Push(4.0, "png", []byte("fake-png-2"))

	o.SubmitUtterance("Check the header alignment", 0.95)
	snap := o.StateSnapshot()
	if len(snap.Utterances) != 1 || !snap.Utterances[0].Dispatched {
		t.Fatalf("expected 1 dispatched utterance, got %+v", snap.Utterances)
	}

	o.DiscardCurrentTurn()
	if o.StateSnapshot().LiveTranscript != "" {
		t.Fatalf("expected empty live transcript after discard")
	}
}

func TestCaptureRegionLockBehavior(t *testing.T) {
	o := NewOrchestrator(t.TempDir())
	o.UpdateRegion(model.CaptureRegion{X: 200, Y: 100, Width: 1440, Height: 900, DisplayIndex: 1}, false)
	snap := o.StateSnapshot()
	if snap.Region.Width != 1440 || snap.Region.DisplayIndex != 1 || !snap.Region.Detached {
		t.Fatalf("unexpected region state: %+v", snap.Region)
	}

	o.mu.Lock()
	o.recording = true
	o.region.Locked = true
	o.mu.Unlock()

	o.UpdateRegion(model.CaptureRegion{X: 0, Y: 0, Width: 500, Height: 500}, false)
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

func TestFramesOutputDirFollowsLinkedConversation(t *testing.T) {
	o := NewOrchestrator(t.TempDir())
	fakeAppData := t.TempDir()
	fakeBundle := t.TempDir()

	o.mu.Lock()
	o.appDataDir = fakeAppData
	o.bundleDir = fakeBundle
	o.captureID = "cap_123"
	o.conversationID = "conv-aaa"
	o.mu.Unlock()

	wantA := filepath.Join(fakeAppData, "brain", "conv-aaa", "xnk-frames", "cap_123")
	if got := o.FramesOutputDir(); got != wantA {
		t.Fatalf("expected %q, got %q", wantA, got)
	}

	// Switch to a different conversation -> path follows automatically
	o.mu.Lock()
	o.conversationID = "conv-bbb"
	o.mu.Unlock()

	wantB := filepath.Join(fakeAppData, "brain", "conv-bbb", "xnk-frames", "cap_123")
	if got := o.FramesOutputDir(); got != wantB {
		t.Fatalf("expected %q after relink, got %q", wantB, got)
	}

	// Unlink -> falls back to bundle frames dir
	o.UnbindConversation()
	wantUnlinked := filepath.Join(fakeBundle, "frames")
	if got := o.FramesOutputDir(); got != wantUnlinked {
		t.Fatalf("expected %q when unlinked, got %q", wantUnlinked, got)
	}
}
