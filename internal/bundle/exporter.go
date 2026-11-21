package bundle

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jefflunt/xnk-iterate/internal/model"
)

// Export seals a self-contained director recording bundle on disk:
//   - manifest.json
//   - analysis.md (pre-compiled human/agent readable summary)
//   - recording.mp4 (already written in bundleDir during capture)
//   - frames/*.png|jpg (already extracted in bundleDir/frames)
func Export(bundleDir string, manifest model.CaptureManifest) (string, error) {
	if err := os.MkdirAll(bundleDir, 0o755); err != nil {
		return "", err
	}

	manifestPath := filepath.Join(bundleDir, "manifest.json")
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(manifestPath, raw, 0o644); err != nil {
		return "", err
	}

	analysisPath := filepath.Join(bundleDir, "analysis.md")
	if err := os.WriteFile(analysisPath, []byte(renderAnalysisMarkdown(manifest)), 0o644); err != nil {
		return "", err
	}

	return bundleDir, nil
}

func renderAnalysisMarkdown(m model.CaptureManifest) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("# XNK Director Session — `%s`\n\n", m.CaptureID))
	b.WriteString(fmt.Sprintf("- **Created:** %s\n", m.CreatedAt.Format(time.RFC3339)))
	b.WriteString(fmt.Sprintf("- **Duration:** %.1fs\n", m.DurationSeconds))
	b.WriteString(fmt.Sprintf("- **Target:** %s (`%s`, %dx%d)\n", m.Target.Title, m.Target.Type, m.Target.Resolution.Width, m.Target.Resolution.Height))
	if m.WorkspacePath != "" {
		b.WriteString(fmt.Sprintf("- **Workspace:** `%s`\n", m.WorkspacePath))
	}
	if m.ConversationID != "" {
		b.WriteString(fmt.Sprintf("- **Antigravity Session:** `%s`\n", m.ConversationID))
	}
	b.WriteString(fmt.Sprintf("- **Utterances:** %d | **Interactions:** %d | **Frames:** %d\n\n", len(m.Utterances), len(m.Interactions), len(m.Frames)))

	b.WriteString("## Director Commentary Timeline\n\n")
	if len(m.Utterances) == 0 {
		b.WriteString("_No utterances recorded._\n")
		return b.String()
	}

	for _, u := range m.Utterances {
		if u.Discarded {
			continue
		}
		b.WriteString(fmt.Sprintf("### #%d `[%.2fs – %.2fs]` (Lookback: `%.2fs – %.2fs`)\n",
			u.ID, u.SpeechStart, u.SpeechEnd, u.LookbackWindow.StartSeconds, u.LookbackWindow.EndSeconds))
		b.WriteString(fmt.Sprintf("> %s\n\n", u.Transcript))
		if u.TriggerAction != nil {
			b.WriteString(fmt.Sprintf("- **Trigger Action:** `%s` @ `%.2fs`", u.TriggerAction.Type, u.TriggerAction.Timestamp))
			if u.TriggerAction.Location != nil {
				b.WriteString(fmt.Sprintf(" at `(%.0f, %.0f)`", u.TriggerAction.Location.X, u.TriggerAction.Location.Y))
			}
			b.WriteString("\n")
		}
		if u.TriggerFramePath != "" {
			b.WriteString(fmt.Sprintf("- **Trigger Frame:** `%s`\n", u.TriggerFramePath))
		}
		if u.ReactionFramePath != "" {
			b.WriteString(fmt.Sprintf("- **Reaction Frame:** `%s`\n", u.ReactionFramePath))
		}
		b.WriteString("\n")
	}

	return b.String()
}
