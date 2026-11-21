package bundle

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jefflunt/xnk-iterate/internal/model"
)

func TestExportBundle(t *testing.T) {
	dir := t.TempDir()
	m := model.CaptureManifest{
		CaptureID:       "cap_test_01",
		CreatedAt:       time.Now(),
		DurationSeconds: 42.5,
		Target: model.CaptureTarget{
			ID:         "display:0",
			Type:       model.TargetDisplay,
			Title:      "Display 1",
			Resolution: model.Size{Width: 1920, Height: 1080},
		},
		VideoFile: "recording.mp4",
		Utterances: []model.Utterance{
			{
				ID:          1,
				Transcript:  "The button alignment is off by 4px.",
				SpeechStart: 10.0,
				SpeechEnd:   13.5,
				LookbackWindow: model.LookbackRange{
					StartSeconds:   6.5,
					EndSeconds:     13.5,
					TriggerSeconds: 7.5,
					Duration:       7.0,
				},
			},
		},
	}

	outDir, err := Export(dir, m)
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(outDir, "manifest.json"))
	if err != nil {
		t.Fatalf("missing manifest.json: %v", err)
	}
	var parsed model.CaptureManifest
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("invalid manifest json: %v", err)
	}
	if parsed.CaptureID != "cap_test_01" || len(parsed.Utterances) != 1 {
		t.Fatalf("unexpected manifest contents: %+v", parsed)
	}

	md, err := os.ReadFile(filepath.Join(outDir, "analysis.md"))
	if err != nil {
		t.Fatalf("missing analysis.md: %v", err)
	}
	if !strings.Contains(string(md), "The button alignment is off by 4px.") {
		t.Fatalf("analysis.md missing transcript")
	}
}
