package applog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoggerWritesAndRetainsTail(t *testing.T) {
	p := filepath.Join(t.TempDir(), "test.log")
	gotPath := Init(p)
	if gotPath != p {
		t.Fatalf("expected %q, got %q", p, gotPath)
	}
	Info("test", "hello %d", 42)
	Error("agy", "sample error")

	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read log failed: %v", err)
	}
	if !strings.Contains(string(raw), "hello 42") || !strings.Contains(string(raw), "[ERROR] [agy] sample error") {
		t.Fatalf("unexpected log contents: %s", string(raw))
	}
	recent := Recent(5)
	if len(recent) < 2 {
		t.Fatalf("expected recent lines, got %v", recent)
	}
}
