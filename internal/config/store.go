package config

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/jefflunt/xnk-iterate/internal/model"
)

// AppConfig persists user preferences across launches in ~/Library/Application Support/XNK Iterate/config.json.
type AppConfig struct {
	WorkspacePath    string               `json:"workspace_path"`
	ConversationID   string               `json:"conversation_id,omitempty"`
	ContinueSession  bool                 `json:"continue_session"`
	SilenceTimeoutMS int                  `json:"silence_timeout_ms"`
	VoiceOutputMuted bool                 `json:"voice_output_muted"`
	Region           *model.CaptureRegion `json:"region,omitempty"`
}

// DefaultPath returns the standard macOS Application Support config file path.
func DefaultPath() string {
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, "Library", "Application Support", "XNK Iterate", "config.json")
	}
	return filepath.Join(os.TempDir(), "xnk-iterate-config.json")
}

// Load reads persisted configuration, returning sensible defaults if the file doesn't exist.
func Load(path string) AppConfig {
	if path == "" {
		path = DefaultPath()
	}
	wd, _ := os.Getwd()
	if wd == "/" || wd == "" {
		if home, err := os.UserHomeDir(); err == nil {
			wd = home
		}
	}
	cfg := AppConfig{
		WorkspacePath:    wd,
		ContinueSession:  true,
		SilenceTimeoutMS: 1800, // 1.8s natural pause before auto-sending
		VoiceOutputMuted: true, // AI TTS OFF by default
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return cfg
	}
	_ = json.Unmarshal(raw, &cfg)
	if cfg.SilenceTimeoutMS < 500 {
		cfg.SilenceTimeoutMS = 1800
	}
	if cfg.WorkspacePath == "" || cfg.WorkspacePath == "/" {
		cfg.WorkspacePath = wd
	}
	return cfg
}

// Save writes AppConfig atomically to disk.
func Save(path string, cfg AppConfig) error {
	if path == "" {
		path = DefaultPath()
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}
