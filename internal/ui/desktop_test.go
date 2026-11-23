package ui

import (
	"sync"
	"testing"
	"time"
)

func TestNormalizeWindowWidth(t *testing.T) {
	cases := []struct {
		in   int
		want int
	}{
		{0, DefaultWindowWidth},
		{-100, DefaultWindowWidth},
		{500, MinWindowWidth},
		{MinWindowWidth, MinWindowWidth},
		{1180, 1180},
		{MaxWindowWidth + 500, MaxWindowWidth},
	}

	for _, tc := range cases {
		if got := NormalizeWindowWidth(tc.in); got != tc.want {
			t.Errorf("NormalizeWindowWidth(%d) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestDesktopAppResizePersistence(t *testing.T) {
	app := NewDesktopApp("XNK Iterate", "http://127.0.0.1:9292")
	app.SetInitialWidth(1120)

	if got := app.currentWidth(); got != 1120 {
		t.Fatalf("initial currentWidth = %d, want 1120", got)
	}

	var (
		mu    sync.Mutex
		saved []int
		done  = make(chan struct{}, 2)
	)
	app.SetOnWidthChange(func(width int) {
		mu.Lock()
		saved = append(saved, width)
		mu.Unlock()
		done <- struct{}{}
	})

	// Live resize updates in-memory width without persisting immediately.
	app.handleNativeResize(1240, true)
	if got := app.currentWidth(); got != 1240 {
		t.Fatalf("live resize currentWidth = %d, want 1240", got)
	}
	mu.Lock()
	if len(saved) != 0 {
		mu.Unlock()
		t.Fatalf("expected 0 saves during live resize, got %v", saved)
	}
	mu.Unlock()

	// Ending live resize persists the final width once.
	app.handleNativeResize(1240, false)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for width persistence callback")
	}

	// Duplicate non-live resize with same width should not trigger another save.
	app.handleNativeResize(1240, false)
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(saved) != 1 || saved[0] != 1240 {
		t.Fatalf("saved widths = %v, want [1240]", saved)
	}
}
