package session

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/jefflunt/xnk-iterate/internal/antigravity"
	"github.com/jefflunt/xnk-iterate/internal/applog"
	"github.com/jefflunt/xnk-iterate/internal/bundle"
	"github.com/jefflunt/xnk-iterate/internal/capture"
	"github.com/jefflunt/xnk-iterate/internal/config"
	"github.com/jefflunt/xnk-iterate/internal/interaction"
	"github.com/jefflunt/xnk-iterate/internal/lookback"
	"github.com/jefflunt/xnk-iterate/internal/model"
	"github.com/jefflunt/xnk-iterate/internal/ringbuffer"
	"github.com/jefflunt/xnk-iterate/internal/speech"
)

// StartOptions configures a new live director capture session.
type StartOptions struct {
	TargetID       string `json:"target_id,omitempty"`
	ConversationID string `json:"conversation_id,omitempty"`
}

// LiveEvent is broadcast to connected UI clients over WebSocket.
type LiveEvent struct {
	Type    string `json:"type"` // "state", "audio_level"
	Payload any    `json:"payload"`
}

// Orchestrator coordinates screen capture, interaction tracking, live speech-to-text,
// dynamic lookback frame extraction, archive bundling, and direct injection into an
// existing Google Antigravity session via `agentapi`.
type Orchestrator struct {
	mu sync.RWMutex

	storageRoot string
	capturer    capture.Capturer
	tracker     interaction.Tracker
	speechEng   speech.Engine
	lookbackEng *lookback.Engine
	bridge      *antigravity.Bridge
	ringBuf     *ringbuffer.FrameRingBuffer
	timeline    *interaction.Timeline

	// Active session state
	captureID         string
	bundleDir         string
	recording         bool
	paused            bool
	startedAt         time.Time
	pausedAccum       time.Duration
	pausedAt          time.Time
	speechStartedAt   float64
	utteranceSeq      int
	target            *model.CaptureTarget
	region            model.CaptureRegion
	hotkeys           model.HotkeyConfig
	conversationID    string
	lsAddress         string
	csrfToken         string
	agentAPIExe       string
	appDataDir        string
	linkedRepo        string
	linkedBranch      string
	linkedWorkspace   string
	silenceTimeoutMS  int
	agentState        model.AgentState
	agentStatusText   string
	injectedCount     int
	lastError         string
	liveTranscript    string
	audioLevel        float64
	utterances        []model.Utterance
	frames            []model.FrameSnapshot
	lastBundlePath    string
	drawerOpen        bool
	recentInteraction *model.InteractionEvent
	sessionCancel     context.CancelFunc

	subMu            sync.RWMutex
	subscribers      map[chan LiveEvent]struct{}
	onDrawerToggle   func(expanded bool)
	onRegionEvent    func(region model.CaptureRegion)
	onHotkeysChange  func(cfg model.HotkeyConfig)
	onHotkeysSuspend func(suspended bool)
}

// NewOrchestrator wires all capture and Antigravity partner subsystems.
func NewOrchestrator(storageRoot string) *Orchestrator {
	if storageRoot == "" {
		if home, err := os.UserHomeDir(); err == nil {
			storageRoot = filepath.Join(home, "Library", "Application Support", "XNK Iterate", "recordings")
		} else {
			wd, _ := os.Getwd()
			storageRoot = filepath.Join(wd, "storage", "recordings")
		}
	}

	persisted := config.Load("")
	defaultRegion := model.CaptureRegion{
		X:            120,
		Y:            140,
		Width:        1280,
		Height:       720,
		DisplayIndex: 0,
		Detached:     true,
		Locked:       false,
		Visible:      true,
	}
	if persisted.Region != nil && persisted.Region.Width >= 240 && persisted.Region.Height >= 160 {
		defaultRegion = *persisted.Region
		defaultRegion.Detached = true
		defaultRegion.Locked = false
		defaultRegion.Visible = true
	}

	defaultHotkeys := model.DefaultHotkeyConfig()
	if persisted.Hotkeys != nil {
		if sendHK, err := model.NormalizeHotkeyBinding(persisted.Hotkeys.SendTurn); err == nil {
			defaultHotkeys.SendTurn = sendHK
		}
		if discHK, err := model.NormalizeHotkeyBinding(persisted.Hotkeys.DiscardTurn); err == nil {
			defaultHotkeys.DiscardTurn = discHK
		}
	}

	o := &Orchestrator{
		storageRoot:      storageRoot,
		capturer:         capture.NewPlatformCapturer(),
		tracker:          interaction.NewPlatformTracker(),
		speechEng:        speech.NewPlatformEngine(),
		lookbackEng:      lookback.New(),
		bridge:           antigravity.NewBridge(),
		ringBuf:          ringbuffer.New(30.0, 90),
		timeline:         interaction.NewTimeline(5000),
		region:           defaultRegion,
		hotkeys:          defaultHotkeys,
		conversationID:   persisted.ConversationID,
		lsAddress:        persisted.LSAddress,
		csrfToken:        persisted.CSRFToken,
		agentAPIExe:      persisted.AgentAPIExe,
		appDataDir:       persisted.AppDataDir,
		silenceTimeoutMS: persisted.SilenceTimeoutMS,
		agentState:       model.AgentIdle,
		agentStatusText:  "Run /xnk-iterate in Antigravity to link",
		subscribers:      make(map[chan LiveEvent]struct{}),
	}

	o.bridge.SetConnectionEnv(persisted.LSAddress, persisted.CSRFToken, persisted.AgentAPIExe)

	if o.conversationID != "" {
		o.agentState = model.AgentListening
		o.agentStatusText = "Linked to Antigravity"
	}

	o.timeline.SetCallback(func(ev model.InteractionEvent) {
		o.mu.Lock()
		cp := ev
		o.recentInteraction = &cp
		o.mu.Unlock()
	})

	return o
}

// SetDrawerToggleCallback registers a native window resize hook when opening/closing the companion drawer.
func (o *Orchestrator) SetDrawerToggleCallback(fn func(expanded bool)) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.onDrawerToggle = fn
}

// SetRegionCallback registers a platform CaptureBox hook invoked when region/visibility/lock state changes.
func (o *Orchestrator) SetRegionCallback(fn func(region model.CaptureRegion)) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.onRegionEvent = fn
}

// SetHotkeysCallback registers a native hook to re-register global OS hotkeys when the user customizes them.
func (o *Orchestrator) SetHotkeysCallback(fn func(cfg model.HotkeyConfig)) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.onHotkeysChange = fn
}

// SetHotkeysSuspendCallback registers a native hook to pause global hotkeys while recording a new shortcut.
func (o *Orchestrator) SetHotkeysSuspendCallback(fn func(suspended bool)) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.onHotkeysSuspend = fn
}

// UpdateHotkeys validates, persists, and dynamically applies custom global hotkeys.
func (o *Orchestrator) UpdateHotkeys(cfg model.HotkeyConfig) error {
	sendHK, err := model.NormalizeHotkeyBinding(cfg.SendTurn)
	if err != nil {
		o.mu.Lock()
		o.lastError = err.Error()
		o.mu.Unlock()
		o.broadcastState()
		return err
	}
	discHK, err := model.NormalizeHotkeyBinding(cfg.DiscardTurn)
	if err != nil {
		o.mu.Lock()
		o.lastError = err.Error()
		o.mu.Unlock()
		o.broadcastState()
		return err
	}
	if sendHK.KeyCode == discHK.KeyCode && sendHK.Modifiers == discHK.Modifiers {
		err = fmt.Errorf("Send and Discard hotkeys cannot be identical (%s)", sendHK.Display)
		o.mu.Lock()
		o.lastError = err.Error()
		o.mu.Unlock()
		o.broadcastState()
		return err
	}

	normalized := model.HotkeyConfig{
		SendTurn:    sendHK,
		DiscardTurn: discHK,
	}

	o.mu.Lock()
	o.hotkeys = normalized
	o.lastError = ""
	o.saveConfigLocked()
	cb := o.onHotkeysChange
	o.mu.Unlock()

	applog.Info("session", "global hotkeys updated: send=%s discard=%s", sendHK.Display, discHK.Display)
	if cb != nil {
		cb(normalized)
	}
	o.broadcastState()
	return nil
}

// ResetHotkeys restores default global hotkeys (⌥↩ and ⌥⌫).
func (o *Orchestrator) ResetHotkeys() {
	_ = o.UpdateHotkeys(model.DefaultHotkeyConfig())
}

// SetHotkeyRecording temporarily suspends/resumes global OS hotkeys while the user records a shortcut.
func (o *Orchestrator) SetHotkeyRecording(active bool) {
	o.mu.RLock()
	cb := o.onHotkeysSuspend
	o.mu.RUnlock()
	if cb != nil {
		cb(active)
	}
}

// BindConversation links XNK Iterate to an existing Google Antigravity conversation ID.
func (o *Orchestrator) BindConversation(convID string) (antigravity.ConversationMeta, error) {
	return o.BindConversationWithEnv(convID, "", "", "", "")
}

// BindConversationWithEnv links XNK Iterate to an existing Google Antigravity conversation ID
// and records the Language Server connection environment (ANTIGRAVITY_LS_ADDRESS, ANTIGRAVITY_CSRF_TOKEN,
// ANTIGRAVITY_AGENTAPI_EXE, ANTIGRAVITY_APP_DATA_DIR) supplied by `/xnk-iterate`.
func (o *Orchestrator) BindConversationWithEnv(convID, lsAddress, csrfToken, agentAPIExe, appDataDir string) (antigravity.ConversationMeta, error) {
	convID = strings.TrimSpace(convID)
	if convID == "" {
		return antigravity.ConversationMeta{}, fmt.Errorf("conversation_id is required")
	}

	o.mu.Lock()
	if strings.TrimSpace(lsAddress) != "" {
		o.lsAddress = strings.TrimSpace(lsAddress)
	}
	if strings.TrimSpace(csrfToken) != "" {
		o.csrfToken = strings.TrimSpace(csrfToken)
	}
	if strings.TrimSpace(agentAPIExe) != "" {
		o.agentAPIExe = strings.TrimSpace(agentAPIExe)
	}
	if strings.TrimSpace(appDataDir) != "" {
		o.appDataDir = strings.TrimSpace(appDataDir)
	}
	o.bridge.SetConnectionEnv(o.lsAddress, o.csrfToken, o.agentAPIExe)
	o.mu.Unlock()

	meta, err := o.bridge.VerifyConversation(context.Background(), convID)
	o.mu.Lock()
	o.conversationID = convID
	o.linkedRepo = meta.RepoName
	o.linkedBranch = meta.BranchName
	o.linkedWorkspace = meta.WorkspacePath
	if err != nil {
		o.lastError = err.Error()
		o.agentState = model.AgentError
		o.agentStatusText = "Link error — re-run /xnk-iterate"
	} else {
		o.lastError = ""
		o.agentState = model.AgentListening
		if meta.RepoName != "" {
			o.agentStatusText = fmt.Sprintf("Linked: %s", meta.RepoName)
		} else {
			o.agentStatusText = fmt.Sprintf("Linked: %s", shortID(convID))
		}
	}
	o.saveConfigLocked()
	o.mu.Unlock()

	o.broadcastState()
	return meta, err
}

// UnbindConversation disconnects from the current Antigravity session.
func (o *Orchestrator) UnbindConversation() {
	o.mu.Lock()
	o.conversationID = ""
	o.linkedRepo = ""
	o.linkedBranch = ""
	o.linkedWorkspace = ""
	o.agentState = model.AgentIdle
	o.agentStatusText = "Run /xnk-iterate in Antigravity to link"
	o.saveConfigLocked()
	o.mu.Unlock()
	o.broadcastState()
}

// UpdateSettings updates conversation ID and silence timeout and persists them.
func (o *Orchestrator) UpdateSettings(convID string, silenceTimeoutMS int) {
	convID = strings.TrimSpace(convID)
	if convID != "" && convID != o.StateSnapshot().ConversationID {
		_, _ = o.BindConversation(convID)
	} else if convID == "" {
		o.UnbindConversation()
	}

	o.mu.Lock()
	if silenceTimeoutMS >= 500 {
		o.silenceTimeoutMS = silenceTimeoutMS
	}
	o.saveConfigLocked()
	o.mu.Unlock()
	o.broadcastState()
}

// UpdateRegion updates the capture drag-box geometry unless locked during active recording.
func (o *Orchestrator) UpdateRegion(r model.CaptureRegion, fromNative bool) {
	o.mu.Lock()
	if o.region.Locked || o.recording {
		o.mu.Unlock()
		return
	}
	if r.Width < 240 {
		r.Width = 240
	}
	if r.Height < 160 {
		r.Height = 160
	}
	r.Locked = false
	r.Detached = true
	if !fromNative {
		r.Visible = true
	} else {
		r.Visible = o.region.Visible
	}
	o.region = r
	o.saveConfigLocked()
	cb := o.onRegionEvent
	o.mu.Unlock()

	if !fromNative && cb != nil {
		cb(r)
	}
	o.broadcastState()
}

// ToggleCaptureBoxVisible shows or hides the detached native Capture Box overlay.
func (o *Orchestrator) ToggleCaptureBoxVisible() bool {
	o.mu.Lock()
	o.region.Visible = !o.region.Visible
	o.region.Detached = true
	r := o.region
	cb := o.onRegionEvent
	o.mu.Unlock()

	applog.Info("session", "capture box visible=%v", r.Visible)
	if cb != nil {
		cb(r)
	}
	o.broadcastState()
	return r.Visible
}

// SetDrawerOpen expands or collapses the Companion HUD's turns/settings drawer.
func (o *Orchestrator) SetDrawerOpen(open bool) {
	o.mu.Lock()
	if o.drawerOpen == open {
		o.mu.Unlock()
		return
	}
	o.drawerOpen = open
	cb := o.onDrawerToggle
	o.mu.Unlock()

	if cb != nil {
		cb(open)
	}
	o.broadcastState()
}

func (o *Orchestrator) saveConfigLocked() {
	reg := o.region
	hk := o.hotkeys
	_ = config.Save("", config.AppConfig{
		ConversationID:   o.conversationID,
		LSAddress:        o.lsAddress,
		CSRFToken:        o.csrfToken,
		AgentAPIExe:      o.agentAPIExe,
		AppDataDir:       o.appDataDir,
		SilenceTimeoutMS: o.silenceTimeoutMS,
		Region:           &reg,
		Hotkeys:          &hk,
	})
}

func (o *Orchestrator) resolveAppDataDirLocked() string {
	if strings.TrimSpace(o.appDataDir) != "" {
		return strings.TrimSpace(o.appDataDir)
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".gemini", "antigravity")
	}
	return filepath.Join(os.TempDir(), "antigravity")
}

// BrainRoot returns the root Antigravity brain directory (~/.gemini/antigravity/brain)
// used for storing conversation-scoped frames that Antigravity can read without prompting.
func (o *Orchestrator) BrainRoot() string {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return filepath.Join(o.resolveAppDataDirLocked(), "brain")
}

// FramesOutputDir dynamically resolves where Trigger/Reaction frames should be written.
// When linked to an Antigravity conversation, frames are written directly into that
// conversation's own brain directory (<appDataDir>/brain/<conversation_id>/xnk-frames/<capture_id>)
// so Antigravity can inspect them natively with zero external-directory permission prompts.
// Switching or unlinking conversations automatically redirects subsequent frames.
func (o *Orchestrator) FramesOutputDir() string {
	o.mu.RLock()
	defer o.mu.RUnlock()
	if convID := strings.TrimSpace(o.conversationID); convID != "" {
		sub := strings.TrimSpace(o.captureID)
		if sub == "" {
			sub = "live"
		}
		return filepath.Join(o.resolveAppDataDirLocked(), "brain", convID, "xnk-frames", sub)
	}
	if o.bundleDir != "" {
		return filepath.Join(o.bundleDir, "frames")
	}
	return filepath.Join(os.TempDir(), "xnk-frames")
}

// Subscribe returns a channel receiving real-time LiveEvents.
func (o *Orchestrator) Subscribe() chan LiveEvent {
	ch := make(chan LiveEvent, 64)
	o.subMu.Lock()
	o.subscribers[ch] = struct{}{}
	o.subMu.Unlock()
	return ch
}

// Unsubscribe removes a subscriber channel.
func (o *Orchestrator) Unsubscribe(ch chan LiveEvent) {
	o.subMu.Lock()
	delete(o.subscribers, ch)
	o.subMu.Unlock()
	close(ch)
}

func (o *Orchestrator) broadcast(ev LiveEvent) {
	o.subMu.RLock()
	defer o.subMu.RUnlock()
	for ch := range o.subscribers {
		select {
		case ch <- ev:
		default:
		}
	}
}

func (o *Orchestrator) broadcastState() {
	o.broadcast(LiveEvent{
		Type:    "state",
		Payload: o.StateSnapshot(),
	})
}

// ListTargets returns available displays and windows for capture.
func (o *Orchestrator) ListTargets() ([]model.CaptureTarget, error) {
	return o.capturer.ListTargets()
}

// Permissions returns the current platform permission and binary readiness report.
func (o *Orchestrator) Permissions() model.PermissionStatus {
	apiPath, apiOK := antigravity.FindAgentAPIBinary()
	return model.PermissionStatus{
		Platform:        runtime.GOOS + "/" + runtime.GOARCH,
		ScreenRecording: o.capturer.ScreenRecordingStatus(),
		Microphone:      o.speechEng.MicrophoneStatus(),
		Speech:          o.speechEng.SpeechStatus(),
		Accessibility:   o.tracker.AccessibilityStatus(),
		FFmpegAvailable: capture.FFmpegAvailable(),
		AgyAvailable:    apiOK,
		AgyPath:         apiPath,
	}
}

// ElapsedSeconds returns active recording elapsed time excluding paused intervals.
func (o *Orchestrator) ElapsedSeconds() float64 {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.elapsedLocked()
}

func (o *Orchestrator) elapsedLocked() float64 {
	if !o.recording {
		return 0
	}
	now := time.Now()
	if o.paused {
		now = o.pausedAt
	}
	d := now.Sub(o.startedAt) - o.pausedAccum
	if d < 0 {
		return 0
	}
	return d.Seconds()
}

// StateSnapshot builds a thread-safe view of the current companion state for the HUD.
func (o *Orchestrator) StateSnapshot() model.SessionStateSnapshot {
	o.mu.RLock()
	defer o.mu.RUnlock()

	utts := make([]model.Utterance, len(o.utterances))
	copy(utts, o.utterances)

	return model.SessionStateSnapshot{
		CaptureID:         o.captureID,
		Recording:         o.recording,
		Paused:            o.paused,
		ElapsedSeconds:    o.elapsedLocked(),
		Target:            o.target,
		Region:            o.region,
		Hotkeys:           o.hotkeys,
		ConversationID:    o.conversationID,
		LinkedRepo:        o.linkedRepo,
		LinkedBranch:      o.linkedBranch,
		LinkedWorkspace:   o.linkedWorkspace,
		SilenceTimeoutMS:  o.silenceTimeoutMS,
		AgentState:        o.agentState,
		AgentStatusText:   o.agentStatusText,
		InjectedCount:     o.injectedCount,
		LastError:         o.lastError,
		LiveTranscript:    o.liveTranscript,
		AudioLevel:        o.audioLevel,
		Utterances:        utts,
		Permissions:       o.Permissions(),
		LastBundlePath:    o.lastBundlePath,
		LogFilePath:       applog.Path(),
		DrawerOpen:        o.drawerOpen,
		RecentInteraction: o.recentInteraction,
	}
}

// StartSession begins screen capture, interaction tracking, and live speech-to-text,
// locking the Capture Box and streaming utterances directly into the linked Antigravity session.
func (o *Orchestrator) StartSession(opts StartOptions) error {
	o.mu.Lock()
	if o.recording {
		o.mu.Unlock()
		return nil
	}

	targets, _ := o.capturer.ListTargets()
	var selected *model.CaptureTarget
	for i := range targets {
		if targets[i].ID == opts.TargetID {
			cp := targets[i]
			selected = &cp
			break
		}
	}
	if selected == nil && len(targets) > 0 {
		cp := targets[0]
		selected = &cp
	}
	if selected == nil {
		selected = &model.CaptureTarget{
			ID:         "display:0",
			Type:       model.TargetDisplay,
			Title:      "Primary Display",
			Resolution: model.Size{Width: 1920, Height: 1080},
		}
	}

	if strings.TrimSpace(opts.ConversationID) != "" {
		o.conversationID = strings.TrimSpace(opts.ConversationID)
	}

	capID := fmt.Sprintf("cap_%d", time.Now().Unix())
	bDir := filepath.Join(o.storageRoot, fmt.Sprintf("capture_%s", capID))

	ctx, cancel := context.WithCancel(context.Background())
	o.captureID = capID
	o.bundleDir = bDir
	o.recording = true
	o.paused = false
	o.startedAt = time.Now()
	o.pausedAccum = 0
	o.utteranceSeq = 0
	o.injectedCount = 0
	o.lastError = ""
	o.target = selected
	o.utterances = nil
	o.frames = nil
	o.liveTranscript = ""
	o.region.Detached = true
	o.region.Visible = true
	o.region.Locked = true
	lockedRegion := o.region
	regionCB := o.onRegionEvent
	o.sessionCancel = cancel
	o.ringBuf.Clear()
	o.timeline.Clear()
	o.mu.Unlock()

	applog.Info("session", "StartSession id=%s target=%s region=(%d,%d %dx%d disp=%d) conv=%s",
		capID, selected.Title, lockedRegion.X, lockedRegion.Y, lockedRegion.Width, lockedRegion.Height, lockedRegion.DisplayIndex, o.conversationID)

	o.broadcastState()
	if regionCB != nil {
		regionCB(lockedRegion)
	}

	_ = o.capturer.Start(ctx, capture.Config{
		Target:     *selected,
		Region:     &lockedRegion,
		OutputDir:  bDir,
		VideoFile:  "recording.mp4",
		FPS:        30,
		RingBuffer: o.ringBuf,
		ElapsedFn:  o.ElapsedSeconds,
	})
	_ = o.tracker.Start(o.ElapsedSeconds, o.timeline)
	if err := o.speechEng.Start(ctx, o.ElapsedSeconds, o.handleSpeechEvent); err != nil {
		applog.Error("session", "speech engine start failed: %v", err)
	}

	return nil
}

// StopSession stops capture, speech, and interaction tracking, unlocks the Capture Box,
// and seals the archive bundle on disk.
func (o *Orchestrator) StopSession() (string, error) {
	o.mu.Lock()
	if !o.recording {
		last := o.lastBundlePath
		o.mu.Unlock()
		return last, nil
	}
	duration := o.elapsedLocked()
	o.recording = false
	o.paused = false
	o.region.Locked = false
	o.liveTranscript = ""
	o.audioLevel = 0
	unlockedRegion := o.region
	regionCB := o.onRegionEvent
	cancel := o.sessionCancel
	o.sessionCancel = nil

	manifest := model.CaptureManifest{
		CaptureID:       o.captureID,
		CreatedAt:       o.startedAt,
		DurationSeconds: duration,
		WorkspacePath:   o.linkedWorkspace,
		ConversationID:  o.conversationID,
		VideoFile:       "recording.mp4",
		Utterances:      append([]model.Utterance(nil), o.utterances...),
		Interactions:    o.timeline.All(),
		Frames:          append([]model.FrameSnapshot(nil), o.frames...),
	}
	if o.target != nil {
		manifest.Target = *o.target
	}
	bDir := o.bundleDir
	o.mu.Unlock()

	applog.Info("session", "StopSession triggered (id=%s duration=%.1fs)", manifest.CaptureID, duration)

	if regionCB != nil {
		regionCB(unlockedRegion)
	}
	o.broadcastState()

	if cancel != nil {
		cancel()
	}
	_ = o.speechEng.Stop()
	o.tracker.Stop()
	_ = o.capturer.Stop()

	exportedPath, err := bundle.Export(bDir, manifest)
	if err != nil {
		applog.Error("session", "bundle export failed: %v", err)
	} else {
		applog.Info("session", "bundle exported to %s", exportedPath)
		o.mu.Lock()
		o.lastBundlePath = exportedPath
		o.mu.Unlock()
	}

	o.broadcastState()
	return exportedPath, err
}

// TogglePause pauses or resumes recording and utterance dispatch.
func (o *Orchestrator) TogglePause() bool {
	o.mu.Lock()
	if !o.recording {
		o.mu.Unlock()
		return false
	}
	if o.paused {
		o.pausedAccum += time.Since(o.pausedAt)
		o.paused = false
	} else {
		o.paused = true
		o.pausedAt = time.Now()
	}
	paused := o.paused
	o.mu.Unlock()
	o.broadcastState()
	return paused
}

// SendCurrentTurn flushes the accumulated voice buffer into Antigravity (triggered by global hotkey or Send button).
func (o *Orchestrator) SendCurrentTurn() {
	o.mu.RLock()
	draft := strings.TrimSpace(o.liveTranscript)
	seqBefore := o.utteranceSeq
	recording := o.recording
	o.mu.RUnlock()

	applog.Info("session", "SendCurrentTurn triggered (recording=%v draft_len=%d)", recording, len(draft))
	if err := o.speechEng.FlushTurn(); err != nil {
		applog.Warn("session", "speechEng.FlushTurn error: %v", err)
	}

	// Safety fallback: if the HUD already had buffered speech text and the native helper
	// doesn't emit a final event within 250ms, submit the buffered draft directly so no turn is lost.
	if draft != "" {
		go func(expectedSeq int, fallbackText string) {
			time.Sleep(250 * time.Millisecond)
			o.mu.RLock()
			stillPending := o.utteranceSeq == expectedSeq && strings.TrimSpace(o.liveTranscript) != ""
			o.mu.RUnlock()
			if stillPending {
				applog.Info("session", "SendCurrentTurn fallback submitting buffered draft (%d chars)", len(fallbackText))
				o.SubmitUtterance(fallbackText, 0.9)
			}
		}(seqBefore, draft)
	}
}

// DiscardCurrentTurn clears the currently buffered voice transcript without sending (triggered by global hotkey or Discard button).
func (o *Orchestrator) DiscardCurrentTurn() {
	applog.Info("session", "DiscardCurrentTurn triggered (hotkey/button)")
	if err := o.speechEng.DiscardTurn(); err != nil {
		applog.Warn("session", "speechEng.DiscardTurn error: %v", err)
	}
	o.mu.Lock()
	o.liveTranscript = ""
	o.speechStartedAt = 0
	o.mu.Unlock()
	o.broadcastState()
}

// SubmitUtterance accepts a finalized utterance (from native STT, Web Speech API, or manual input).
func (o *Orchestrator) SubmitUtterance(text string, confidence float32) {
	o.submitUtteranceWithOffset(text, confidence, 0)
}

func (o *Orchestrator) submitUtteranceWithOffset(text string, confidence float32, silenceOffsetSec float64) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}

	o.mu.RLock()
	paused := o.paused
	recording := o.recording
	o.mu.RUnlock()
	if paused {
		return
	}

	nowTS := o.ElapsedSeconds() - silenceOffsetSec
	if nowTS < 0 {
		nowTS = 0
	}

	o.mu.Lock()
	speechStart := o.speechStartedAt
	if speechStart <= 0 || speechStart > nowTS {
		speechStart = nowTS - 2.5
		if speechStart < 0 {
			speechStart = 0
		}
	}
	o.speechStartedAt = 0
	o.utteranceSeq++
	uttID := o.utteranceSeq
	bDir := o.bundleDir
	o.liveTranscript = ""
	o.mu.Unlock()

	res := o.lookbackEng.Calculate(speechStart, nowTS, o.timeline)

	var trigPath, reactPath string
	var carouselPaths []string
	var snaps []model.FrameSnapshot
	if recording && bDir != "" {
		framesDir := o.FramesOutputDir()
		trigPath, reactPath, carouselPaths, snaps, _ = o.lookbackEng.ExtractFrames(uttID, res, o.ringBuf, framesDir)
	}

	utt := model.Utterance{
		ID:                uttID,
		Transcript:        text,
		Confidence:        confidence,
		SpeechStart:       speechStart,
		SpeechEnd:         nowTS,
		LookbackWindow:    res.Range,
		TriggerAction:     res.AnchorAction,
		TriggerFramePath:  trigPath,
		ReactionFramePath: reactPath,
		CarouselPaths:     carouselPaths,
		Dispatched:        true,
	}

	o.mu.Lock()
	o.utterances = append(o.utterances, utt)
	o.frames = append(o.frames, snaps...)
	o.mu.Unlock()

	o.dispatchBatchToAntigravity([]model.Utterance{utt})
	o.broadcastState()
}

func (o *Orchestrator) handleSpeechEvent(ev speech.Event) {
	switch ev.Type {
	case speech.EventAudioLevel:
		o.mu.Lock()
		o.audioLevel = ev.Level
		o.mu.Unlock()
		o.broadcast(LiveEvent{
			Type:    "audio_level",
			Payload: map[string]any{"level": ev.Level},
		})
	case speech.EventVADStart:
		o.mu.Lock()
		o.speechStartedAt = ev.Timestamp
		o.mu.Unlock()
		applog.Info("speech", "vad_start at %.2fs", ev.Timestamp)
	case speech.EventPartial:
		o.mu.Lock()
		wasEmpty := o.liveTranscript == ""
		o.liveTranscript = ev.Text
		o.mu.Unlock()
		if wasEmpty && ev.Text != "" {
			applog.Info("speech", "first partial transcript received (%d chars)", len(ev.Text))
		}
		o.broadcastState()
	case speech.EventFinal:
		applog.Info("speech", "final transcript received (%d chars, silence_offset=%.2fs)", len(ev.Text), ev.SilenceOffset)
		o.submitUtteranceWithOffset(ev.Text, ev.Confidence, ev.SilenceOffset)
	case speech.EventVADEnd:
		o.mu.Lock()
		o.liveTranscript = ""
		o.mu.Unlock()
		o.broadcastState()
	case speech.EventStatus:
		applog.Info("speech", "status: %s", ev.Message)
	case speech.EventError:
		applog.Error("speech", "error: %s", ev.Message)
		o.mu.Lock()
		o.lastError = "Speech: " + ev.Message
		o.mu.Unlock()
		o.broadcastState()
	}
}

func (o *Orchestrator) dispatchBatchToAntigravity(batch []model.Utterance) {
	if len(batch) == 0 {
		return
	}
	o.mu.RLock()
	convID := o.conversationID
	target := o.target
	o.mu.RUnlock()

	if convID == "" {
		applog.Warn("session", "utterance recorded without linked Antigravity conversation (saved to bundle only)")
		return
	}

	go func() {
		err := o.bridge.SendUtterances(context.Background(), convID, batch, target)
		o.mu.Lock()
		if err != nil {
			o.lastError = err.Error()
			o.agentState = model.AgentError
			o.agentStatusText = "Injection error — check Settings/Logs"
		} else {
			o.lastError = ""
			o.injectedCount += len(batch)
			o.agentState = model.AgentListening
			if o.linkedRepo != "" {
				o.agentStatusText = fmt.Sprintf("Linked: %s (%d sent)", o.linkedRepo, o.injectedCount)
			} else {
				o.agentStatusText = fmt.Sprintf("Linked: %s (%d sent)", shortID(convID), o.injectedCount)
			}
		}
		o.mu.Unlock()
		o.broadcastState()
	}()
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// --- mcp.Provider implementation ---

func (o *Orchestrator) GrabFrame(ts *float64) (string, float64, error) {
	var f ringbuffer.Frame
	var ok bool
	if ts != nil {
		f, ok = o.ringBuf.Nearest(*ts)
	} else {
		f, ok = o.ringBuf.Latest()
	}
	if !ok {
		return "", 0, fmt.Errorf("no frames in ring buffer")
	}

	framesDir := o.FramesOutputDir()
	if err := os.MkdirAll(framesDir, 0o755); err != nil {
		return "", 0, err
	}
	ext := f.Format
	if ext == "" {
		ext = "jpg"
	}
	name := fmt.Sprintf("frame_ondemand_%.2fs.%s", f.Timestamp, ext)
	outPath := filepath.Join(framesDir, name)
	if err := os.WriteFile(outPath, f.Data, 0o644); err != nil {
		return "", 0, err
	}
	return outPath, f.Timestamp, nil
}

func (o *Orchestrator) GetClip(startSec, endSec float64) (string, error) {
	if !capture.FFmpegAvailable() {
		return "", fmt.Errorf("ffmpeg is not available")
	}
	o.mu.RLock()
	bDir := o.bundleDir
	o.mu.RUnlock()
	if bDir == "" {
		return "", fmt.Errorf("no active recording")
	}
	src := filepath.Join(bDir, "recording.mp4")
	if _, err := os.Stat(src); err != nil {
		return "", fmt.Errorf("recording.mp4 not yet available")
	}
	clipPath := filepath.Join(bDir, fmt.Sprintf("clip_%.1f_%.1f.mp4", startSec, endSec))
	duration := endSec - startSec
	if duration <= 0 {
		duration = 5.0
	}
	cmd := exec.Command("ffmpeg", "-y", "-ss", fmt.Sprintf("%.2f", startSec), "-t", fmt.Sprintf("%.2f", duration), "-i", src, "-c", "copy", clipPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("ffmpeg clip failed: %w: %s", err, string(out))
	}
	return clipPath, nil
}

func (o *Orchestrator) SessionStatus() map[string]any {
	st := o.StateSnapshot()
	return map[string]any{
		"capture_id":      st.CaptureID,
		"recording":       st.Recording,
		"paused":          st.Paused,
		"batch_mode":      st.BatchMode,
		"elapsed_seconds": st.ElapsedSeconds,
		"conversation_id": st.ConversationID,
		"utterance_count": len(st.Utterances),
	}
}

func (o *Orchestrator) StorageRoot() string {
	return o.storageRoot
}
