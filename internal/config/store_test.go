package config

import (
	"path/filepath"
	"testing"
)

func TestSaveAndLoadPersistsWindowWidth(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")

	want := AppConfig{
		WorkspacePath:    "/tmp/workspace",
		SilenceTimeoutMS: 1800,
		WindowWidth:      1280,
	}
	if err := Save(path, want); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	got := Load(path)
	if got.WindowWidth != want.WindowWidth {
		t.Fatalf("WindowWidth = %d, want %d", got.WindowWidth, want.WindowWidth)
	}
}
