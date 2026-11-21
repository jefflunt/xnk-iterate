package ringbuffer

import "testing"

func TestRingBufferNearestAndEviction(t *testing.T) {
	rb := New(10.0, 5)
	if _, ok := rb.Latest(); ok {
		t.Fatalf("expected empty buffer")
	}

	for i := 0; i < 8; i++ {
		ts := float64(i) * 2.0 // 0, 2, 4, 6, 8, 10, 12, 14
		rb.Push(ts, "png", []byte{byte(i)})
	}

	// maxFrames is 5, so frames at 6, 8, 10, 12, 14 should remain
	if got := rb.Len(); got != 5 {
		t.Fatalf("expected 5 frames, got %d", got)
	}

	f, ok := rb.Nearest(9.6)
	if !ok {
		t.Fatalf("expected nearest frame")
	}
	if f.Timestamp != 10.0 {
		t.Fatalf("expected timestamp 10.0, got %.2f", f.Timestamp)
	}

	latest, ok := rb.Latest()
	if !ok || latest.Timestamp != 14.0 {
		t.Fatalf("expected latest 14.0, got %.2f (ok=%v)", latest.Timestamp, ok)
	}

	rb.Clear()
	if rb.Len() != 0 {
		t.Fatalf("expected 0 after clear")
	}
}
