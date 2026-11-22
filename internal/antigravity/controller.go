package antigravity

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jefflunt/xnk-iterate/internal/applog"
	"github.com/jefflunt/xnk-iterate/internal/model"
)

// ConversationMeta holds metadata returned by `agentapi get-conversation-metadata`.
type ConversationMeta struct {
	ConversationID string `json:"conversation_id"`
	RepoName       string `json:"repo_name,omitempty"`
	WorkspacePath  string `json:"workspace_path,omitempty"`
	BranchName     string `json:"branch_name,omitempty"`
}

// Bridge injects captured voice + vision director feedback directly into an
// existing Google Antigravity Desktop session via the official `agentapi` CLI.
type Bridge struct {
	BinaryOverride string

	mu          sync.RWMutex
	lsAddress   string
	csrfToken   string
	agentAPIExe string
}

// NewBridge creates a new agentapi bridge.
func NewBridge() *Bridge {
	return &Bridge{}
}

// SetConnectionEnv stores the Antigravity Language Server connection environment
// supplied by `/xnk-iterate` during `/api/bind`, so standalone `.app` launches
// (which don't inherit Antigravity's shell env) can invoke `agentapi`.
func (b *Bridge) SetConnectionEnv(lsAddress, csrfToken, agentAPIExe string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if strings.TrimSpace(lsAddress) != "" {
		b.lsAddress = strings.TrimSpace(lsAddress)
	}
	if strings.TrimSpace(csrfToken) != "" {
		b.csrfToken = strings.TrimSpace(csrfToken)
	}
	if strings.TrimSpace(agentAPIExe) != "" {
		b.agentAPIExe = strings.TrimSpace(agentAPIExe)
	}
}

func (b *Bridge) cmdEnv() []string {
	b.mu.RLock()
	lsAddr := b.lsAddress
	csrf := b.csrfToken
	exe := b.agentAPIExe
	b.mu.RUnlock()

	env := os.Environ()
	if lsAddr != "" {
		env = append(env, "ANTIGRAVITY_LS_ADDRESS="+lsAddr)
	}
	if csrf != "" {
		env = append(env, "ANTIGRAVITY_CSRF_TOKEN="+csrf)
	}
	if exe != "" {
		env = append(env, "ANTIGRAVITY_AGENTAPI_EXE="+exe)
	}
	return env
}

// FindAgentAPIBinary locates Google Antigravity's `agentapi` CLI.
func FindAgentAPIBinary() (string, bool) {
	if p, err := exec.LookPath("agentapi"); err == nil {
		return p, true
	}
	home, _ := os.UserHomeDir()
	candidates := []string{
		filepath.Join(home, ".gemini", "antigravity", "bin", "agentapi"),
		filepath.Join(home, ".local", "bin", "agentapi"),
		"/usr/local/bin/agentapi",
	}
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c, true
		}
	}
	return "", false
}

func (b *Bridge) binary() (string, error) {
	if b.BinaryOverride != "" {
		return b.BinaryOverride, nil
	}
	p, ok := FindAgentAPIBinary()
	if !ok {
		return "", fmt.Errorf("agentapi CLI not found (expected at ~/.gemini/antigravity/bin/agentapi)")
	}
	return p, nil
}

// VerifyConversation checks that conversationID exists in the running Antigravity app
// and returns its workspace/repository metadata.
func (b *Bridge) VerifyConversation(ctx context.Context, conversationID string) (ConversationMeta, error) {
	conversationID = strings.TrimSpace(conversationID)
	meta := ConversationMeta{ConversationID: conversationID}
	if conversationID == "" {
		return meta, fmt.Errorf("conversation ID is required")
	}

	bin, err := b.binary()
	if err != nil {
		return meta, err
	}

	runCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	cmd := exec.CommandContext(runCtx, bin, "get-conversation-metadata", conversationID)
	cmd.Env = b.cmdEnv()
	out, err := cmd.CombinedOutput()
	if err != nil {
		applog.Warn("agentapi", "get-conversation-metadata(%s) failed: %v (%s)", conversationID, err, strings.TrimSpace(string(out)))
		return meta, fmt.Errorf("failed to verify conversation: %s", strings.TrimSpace(string(out)))
	}

	var parsed struct {
		Response struct {
			ConversationMetadata struct {
				Metadata struct {
					Workspaces []struct {
						WorkspaceFolderAbsoluteURI string `json:"workspaceFolderAbsoluteUri"`
						BranchName                 string `json:"branchName"`
						Repository                 struct {
							ComputedName string `json:"computedName"`
						} `json:"repository"`
					} `json:"workspaces"`
				} `json:"metadata"`
			} `json:"conversationMetadata"`
		} `json:"response"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(out, &parsed); err == nil {
		if parsed.Error != "" {
			return meta, fmt.Errorf("%s", parsed.Error)
		}
		if ws := parsed.Response.ConversationMetadata.Metadata.Workspaces; len(ws) > 0 {
			meta.RepoName = ws[0].Repository.ComputedName
			meta.BranchName = ws[0].BranchName
			meta.WorkspacePath = strings.TrimPrefix(ws[0].WorkspaceFolderAbsoluteURI, "file://")
		}
	}

	applog.Info("agentapi", "linked conversation %s (repo=%s branch=%s)", conversationID, meta.RepoName, meta.BranchName)
	return meta, nil
}

// SendUtterances formats one or more director utterances (with lookback timecodes,
// triggering OS actions, and Trigger/Reaction frame paths) and injects them directly
// into the bound Google Antigravity session via `agentapi send-message`.
func (b *Bridge) SendUtterances(ctx context.Context, conversationID string, utterances []model.Utterance, target *model.CaptureTarget) error {
	if len(utterances) == 0 {
		return nil
	}
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		return fmt.Errorf("no Antigravity conversation linked — run /xnk-iterate in Antigravity to bind")
	}

	bin, err := b.binary()
	if err != nil {
		return err
	}

	content := FormatDirectorTurn(utterances, target)
	runCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	applog.Info("agentapi", "injecting %d utterance(s) into conversation %s (%d bytes)", len(utterances), conversationID, len(content))
	cmd := exec.CommandContext(runCtx, bin, "send-message", "--title=XNK Iterate Director", conversationID, content)
	cmd.Env = b.cmdEnv()
	out, err := cmd.CombinedOutput()
	if err != nil {
		applog.Error("agentapi", "send-message failed: %v (%s)", err, strings.TrimSpace(string(out)))
		return fmt.Errorf("agentapi send-message failed: %s", strings.TrimSpace(string(out)))
	}

	// Check for JSON-wrapped RPC error even on exit 0
	var resp struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(out, &resp) == nil && resp.Error != "" {
		applog.Error("agentapi", "send-message rpc error: %s", resp.Error)
		return fmt.Errorf("%s", resp.Error)
	}

	applog.Info("agentapi", "send-message delivered to %s", conversationID)
	return nil
}

// FormatDirectorTurn builds a structured multimodal message for Google Antigravity
// containing the director's spoken commentary, dynamic lookback timestamps, triggering
// OS interactions, and local Trigger/Reaction frame paths for Antigravity to inspect.
func FormatDirectorTurn(utterances []model.Utterance, target *model.CaptureTarget) string {
	var b strings.Builder
	b.WriteString("[XNK ITERATE — LIVE DIRECTOR FEEDBACK]\n")
	if target != nil {
		b.WriteString(fmt.Sprintf("Target: %s (%dx%d)\n", target.Title, target.Resolution.Width, target.Resolution.Height))
	}
	for _, u := range utterances {
		b.WriteString(fmt.Sprintf("\nUtterance #%d [%.2fs–%.2fs] (Lookback: %.2fs–%.2fs):\n",
			u.ID, u.SpeechStart, u.SpeechEnd, u.LookbackWindow.StartSeconds, u.LookbackWindow.EndSeconds))
		b.WriteString(fmt.Sprintf("> %s\n", u.Transcript))
		if u.TriggerAction != nil {
			b.WriteString(fmt.Sprintf("- Trigger Action: `%s` @ `%.2fs`", u.TriggerAction.Type, u.TriggerAction.Timestamp))
			if u.TriggerAction.Location != nil {
				b.WriteString(fmt.Sprintf(" at `(%.0f, %.0f)`", u.TriggerAction.Location.X, u.TriggerAction.Location.Y))
			}
			b.WriteString("\n")
		}
		if u.TriggerFramePath != "" {
			b.WriteString(fmt.Sprintf("- Trigger Frame (inspect with read_file): %s\n", u.TriggerFramePath))
		}
		if u.ReactionFramePath != "" {
			b.WriteString(fmt.Sprintf("- Reaction Frame (inspect with read_file): %s\n", u.ReactionFramePath))
		}
		for idx, cp := range u.CarouselPaths {
			b.WriteString(fmt.Sprintf("- Carousel Frame %d: %s\n", idx+1, cp))
		}
	}
	return b.String()
}
