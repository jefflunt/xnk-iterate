package session

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/jefflunt/xnk-iterate/internal/antigravity"
	"github.com/jefflunt/xnk-iterate/internal/batcher"
	"github.com/jefflunt/xnk-iterate/internal/bundle"
	"github.com/jefflunt/xnk-iterate/internal/capture"
	"github.com/jefflunt/xnk-iterate/internal/interaction"
	"github.com/jefflunt/xnk-iterate/internal/lookback"
	"github.com/jefflunt/xnk-iterate/internal/model"
	"github.com/jefflunt/xnk-iterate/internal/ringbuffer"
	"github.com/jefflunt/xnk-iterate/internal/speech"
	"github.com/jefflunt/xnk-iterate/internal/voice"
)

// StartOptions configures a new live director capture + Antigravity session.
type StartOptions struct {
	TargetID        string `json:"target_id"`
	WorkspacePath   string `json:"workspace_path"`
	ConversationID  string `json:"conversation_id,omitempty"`
	ContinueSession bool   `json:"continue_session"`
	Model           string `json:"model,omitempty"`
	Mode            string `json:"mode,omitempty"`
}

// LiveEvent is broadcast to connected UI clients over WebSocket.
type LiveEvent struct {
	Type    string `json:"type"` // "state", "audio_level", "live_frame", "toast"
	Payload any    `json:"payload"`
}

// Orchestrator owns the full lifecycle of a live XNK Director session.
type Orchestrator struct {
	mu sync.RWMutex

	storageRoot string
	capturer    capture.Capturer
	tracker     interaction.Tracker
	speechEng   speech.Engine
	speaker     voice.Speaker
	lookbackEng *lookback.Engine
	batcher     *batcher.Dispatcher
	agyCtrl     *antigravity.Controller
	ringBuf     *ringbuffer.FrameRingBuffer
	timeline    *interaction.Timeline
	chunker     voice.SentenceChunker

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
	workspacePath     string
	conversationID    string
	continueSession   bool
	agentState        model.AgentState
	agentStatusText   string
	liveTranscript    string
	audioLevel        float64
	pendingApproval   *model.PermissionRequest
	utterances        []model.Utterance
	frames            []model.FrameSnapshot
	chatHistory       []model.ChatEntry
	lastBundlePath    string
	hudCompactMode    bool
	recentInteraction *model.InteractionEvent
	sessionCancel     context.CancelFunc
	lastLiveFrameAt   time.Time

	subMu         sync.RWMutex
	subscribers   map[chan LiveEvent]struct{}
	onHUDToggle   func(compact bool)
	onRegionEvent func(region model.CaptureRegion)
}

// NewOrchestrator wires all subsystems for the current OS platform.
func NewOrchestrator(storageRoot string) *Orchestrator {
	if storageRoot == "" {
		wd, _ := os.Getwd()
		storageRoot = filepath.Join(wd, "storage", "recordings")
	}
	wd, _ := os.Getwd()

	o := &Orchestrator{
		storageRoot:     storageRoot,
		capturer:        capture.NewPlatformCapturer(),
		tracker:         interaction.NewPlatformTracker(),
		speechEng:       speech.NewPlatformEngine(),
		speaker:         voice.NewPlatformSpeaker(),
		lookbackEng:     lookback.New(),
		ringBuf:         ringbuffer.New(30.0, 90),
		timeline:        interaction.NewTimeline(5000),
		region: model.CaptureRegion{
			X:            120,
			Y:            140,
			Width:        1280,
			Height:       720,
			DisplayIndex: 0,
			Detached:     false,
			Locked:       false,
		},
		workspacePath:   wd,
		continueSession: true,
		agentState:      model.AgentIdle,
		agentStatusText: "Ready to start live director session",
		subscribers:     make(map[chan LiveEvent]struct{}),
	}

	o.timeline.SetCallback(func(ev model.InteractionEvent) {
		o.mu.Lock()
		cp := ev
		o.recentInteraction = &cp
		o.mu.Unlock()
	})

	o.speaker.SetOnStateChange(func(speaking bool) {
		o.mu.Lock()
		if speaking && o.recording {
			o.agentState = model.AgentSpeaking
			o.agentStatusText = "Antigravity speaking (speak to interrupt)"
		} else if !speaking && o.recording && o.agentState == model.AgentSpeaking {
			o.agentState = model.AgentListening
			o.agentStatusText = "Listening for director commentary"
		}
		o.mu.Unlock()
		o.broadcastState()
	})

	o.batcher = batcher.New(func(batch []model.Utterance) {
		o.dispatchBatchToAntigravity(batch)
	})

	o.agyCtrl = antigravity.NewController(antigravity.Callbacks{
		OnStateChange: func(state model.AgentState, statusText string) {
			o.mu.Lock()
			o.agentState = state
			o.agentStatusText = statusText
			o.mu.Unlock()
			o.broadcastState()
		},
		OnConversationID: func(convID string) {
			o.mu.Lock()
			o.conversationID = convID
			o.mu.Unlock()
			o.broadcastState()
		},
		OnTextDelta: func(delta string) {
			sentences := o.chunker.Push(delta)
			for _, s := range sentences {
				o.speaker.Speak(s)
			}
		},
		OnAssistantMessage: func(fullText string) {
			if rem := o.chunker.Flush(); rem != "" {
				o.speaker.Speak(rem)
			}
			o.appendChat(model.ChatEntry{
				ID:        fmt.Sprintf("ai_%d", time.Now().UnixNano()),
				Role:      "ai",
				Text:      fullText,
				Timestamp: time.Now(),
			})
		},
		OnToolCall: func(toolName, detail string) {
			msg := toolName
			if detail != "" {
				msg = fmt.Sprintf("%s — %s", toolName, detail)
			}
			o.appendChat(model.ChatEntry{
				ID:        fmt.Sprintf("tool_%d", time.Now().UnixNano()),
				Role:      "tool",
				ToolName:  toolName,
				Text:      msg,
				Timestamp: time.Now(),
			})
		},
		OnPermissionReq: func(req model.PermissionRequest) {
			o.mu.Lock()
			cp := req
			o.pendingApproval = &cp
			o.agentState = model.AgentAwaitingApproval
			o.agentStatusText = fmt.Sprintf("Approval needed: %s", req.ToolName)
			o.mu.Unlock()

			spoken := fmt.Sprintf("Permission requested for %s. Say approve or deny.", req.ToolName)
			if req.Command != "" {
				spoken = fmt.Sprintf("Approve running %s? Say approve or deny.", req.Command)
			}
			o.speaker.StopImmediate()
			o.speaker.Speak(spoken)
			o.broadcastState()
		},
		OnError: func(err error) {
			o.appendChat(model.ChatEntry{
				ID:        fmt.Sprintf("err_%d", time.Now().UnixNano()),
				Role:      "system",
				Text:      "Antigravity notice: " + err.Error(),
				Timestamp: time.Now(),
			})
		},
	})

	return o
}

// SetHUDToggleCallback registers a platform window hook when switching between Studio and Compact HUD mode.
func (o *Orchestrator) SetHUDToggleCallback(fn func(compact bool)) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.onHUDToggle = fn
}

// SetRegionCallback registers a platform CaptureBox hook invoked when region/detach/lock state changes.
func (o *Orchestrator) SetRegionCallback(fn func(region model.CaptureRegion)) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.onRegionEvent = fn
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
	o.region = r
	cb := o.onRegionEvent
	o.mu.Unlock()

	if !fromNative && cb != nil {
		cb(r)
	}
	o.broadcastState()
}

// ToggleDetachCaptureBox switches the capture region between docked in Studio and a detached multi-monitor native overlay.
func (o *Orchestrator) ToggleDetachCaptureBox() bool {
	o.mu.Lock()
	if o.recording {
		// Prevent detaching/re-docking while recording is locked
		detached := o.region.Detached
		o.mu.Unlock()
		return detached
	}
	o.region.Detached = !o.region.Detached
	r := o.region
	cb := o.onRegionEvent
	o.mu.Unlock()

	if cb != nil {
		cb(r)
	}
	o.broadcastState()
	return r.Detached
}

// UpdateSettings updates workspace, conversation ID, and continue flag from the Settings drawer.
func (o *Orchestrator) UpdateSettings(workspacePath, convID string, continueSession bool) {
	o.mu.Lock()
	if strings.TrimSpace(workspacePath) != "" {
		o.workspacePath = strings.TrimSpace(workspacePath)
	}
	o.conversationID = strings.TrimSpace(convID)
	o.continueSession = continueSession
	o.mu.Unlock()
	o.broadcastState()
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
			// Drop slow-consumer tick rather than blocking capture pipeline.
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
	agyPath, agyOK := antigravity.FindAgyBinary()
	return model.PermissionStatus{
		Platform:        runtime.GOOS + "/" + runtime.GOARCH,
		ScreenRecording: o.capturer.ScreenRecordingStatus(),
		Microphone:      o.speechEng.MicrophoneStatus(),
		Speech:          o.speechEng.SpeechStatus(),
		Accessibility:   o.tracker.AccessibilityStatus(),
		FFmpegAvailable: capture.FFmpegAvailable(),
		AgyAvailable:    agyOK,
		AgyPath:         agyPath,
	}
}

// ElapsedSeconds returns the active recording elapsed time excluding paused intervals.
func (o *Orchestrator) ElapsedSeconds() float64 {
	o.mu.RLock()
	defer o.mu.RUnlock()
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

// StateSnapshot builds a thread-safe view of the current session state for the UI.
func (o *Orchestrator) StateSnapshot() model.SessionStateSnapshot {
	o.mu.RLock()
	defer o.mu.RUnlock()

	utts := make([]model.Utterance, len(o.utterances))
	copy(utts, o.utterances)
	chat := make([]model.ChatEntry, len(o.chatHistory))
	copy(chat, o.chatHistory)

	var elapsed float64
	if o.recording {
		now := time.Now()
		if o.paused {
			now = o.pausedAt
		}
		d := now.Sub(o.startedAt) - o.pausedAccum
		if d > 0 {
			elapsed = d.Seconds()
		}
	}

	return model.SessionStateSnapshot{
		CaptureID:         o.captureID,
		Recording:         o.recording,
		Paused:            o.paused,
		BatchMode:         o.batcher.IsBatchMode(),
		BatchedCount:      o.batcher.PendingCount(),
		ElapsedSeconds:    elapsed,
		Target:            o.target,
		Region:            o.region,
		WorkspacePath:     o.workspacePath,
		ConversationID:    o.conversationID,
		ContinueSession:   o.continueSession,
		VoiceOutputMuted:  o.speaker.IsMuted(),
		AgentState:        o.agentState,
		AgentStatusText:   o.agentStatusText,
		LiveTranscript:    o.liveTranscript,
		AudioLevel:        o.audioLevel,
		PendingApproval:   o.pendingApproval,
		Utterances:        utts,
		ChatHistory:       chat,
		Permissions:       o.Permissions(),
		LastBundlePath:    o.lastBundlePath,
		HUDCompactMode:    o.hudCompactMode,
		RecentInteraction: o.recentInteraction,
	}
}

// StartSession begins screen capture, interaction tracking, live speech/VAD, and spawns the owned Antigravity session.
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

	wsPath := strings.TrimSpace(opts.WorkspacePath)
	if wsPath == "" {
		wsPath = o.workspacePath
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
	o.target = selected
	o.workspacePath = wsPath
	o.conversationID = opts.ConversationID
	o.continueSession = opts.ContinueSession
	o.utterances = nil
	o.frames = nil
	o.pendingApproval = nil
	o.liveTranscript = ""
	o.region.Locked = true
	lockedRegion := o.region
	regionCB := o.onRegionEvent
	o.sessionCancel = cancel
	o.ringBuf.Clear()
	o.timeline.Clear()
	o.batcher.Clear()
	o.chunker.Reset()
	o.mu.Unlock()

	if regionCB != nil {
		regionCB(lockedRegion)
	}

	// 1. Start Capture (fMP4 + rolling ring buffer cropped to locked CaptureRegion)
	_ = o.capturer.Start(ctx, capture.Config{
		Target:     *selected,
		Region:     &lockedRegion,
		OutputDir:  bDir,
		VideoFile:  "recording.mp4",
		FPS:        30,
		RingBuffer: o.ringBuf,
		ElapsedFn:  o.ElapsedSeconds,
		OnLiveFrame: func(ts float64, format string, data []byte) {
			o.mu.Lock()
			if time.Since(o.lastLiveFrameAt) < 800*time.Millisecond {
				o.mu.Unlock()
				return
			}
			o.lastLiveFrameAt = time.Now()
			o.mu.Unlock()

			mime := "image/jpeg"
			if format == "png" {
				mime = "image/png"
			}
			o.broadcast(LiveEvent{
				Type: "live_frame",
				Payload: map[string]any{
					"timestamp": ts,
					"data_url":  "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data),
				},
			})
		},
	})

	// 2. Start Interaction Tracker
	_ = o.tracker.Start(o.ElapsedSeconds, o.timeline)

	// 3. Start Native Speech / VAD Engine
	_ = o.speechEng.Start(ctx, o.ElapsedSeconds, o.handleSpeechEvent)

	// 4. Spawn Owned Google Antigravity (`agy`) Session
	err := o.agyCtrl.Start(ctx, antigravity.SessionConfig{
		WorkspacePath:   wsPath,
		ConversationID:  opts.ConversationID,
		ContinueSession: opts.ContinueSession,
		Model:           opts.Model,
		Mode:            opts.Mode,
	})
	if err != nil {
		o.appendChat(model.ChatEntry{
			ID:        fmt.Sprintf("sys_%d", time.Now().UnixNano()),
			Role:      "system",
			Text:      fmt.Sprintf("Recording active. Antigravity CLI notice: %v (utterances will still be captured and bundled).", err),
			Timestamp: time.Now(),
		})
	} else {
		o.appendChat(model.ChatEntry{
			ID:        fmt.Sprintf("sys_%d", time.Now().UnixNano()),
			Role:      "system",
			Text:      fmt.Sprintf("Live Director Session started — capturing %s and streaming to Google Antigravity in %s.", selected.Title, wsPath),
			Timestamp: time.Now(),
		})
	}

	o.broadcastState()
	return nil
}

// StopSession stops capture, speech, interaction tracking, and Antigravity, then seals the archive bundle.
func (o *Orchestrator) StopSession() (string, error) {
	o.mu.Lock()
	if !o.recording {
		o.mu.Unlock()
		return o.lastBundlePath, nil
	}
	duration := o.ElapsedSeconds()
	o.recording = false
	o.paused = false
	o.region.Locked = false
	unlockedRegion := o.region
	regionCB := o.onRegionEvent
	cancel := o.sessionCancel
	o.sessionCancel = nil

	manifest := model.CaptureManifest{
		CaptureID:       o.captureID,
		CreatedAt:       o.startedAt,
		DurationSeconds: duration,
		WorkspacePath:   o.workspacePath,
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

	// Flush any remaining batched utterances before shutdown
	o.batcher.Flush()

	if regionCB != nil {
		regionCB(unlockedRegion)
	}

	o.speaker.StopImmediate()
	_ = o.speechEng.Stop()
	o.tracker.Stop()
	_ = o.capturer.Stop()
	o.agyCtrl.Stop()
	if cancel != nil {
		cancel()
	}

	exportedPath, err := bundle.Export(bDir, manifest)
	if err == nil {
		o.mu.Lock()
		o.lastBundlePath = exportedPath
		o.liveTranscript = ""
		o.audioLevel = 0
		o.pendingApproval = nil
		o.mu.Unlock()
		o.appendChat(model.ChatEntry{
			ID:        fmt.Sprintf("sys_%d", time.Now().UnixNano()),
			Role:      "system",
			Text:      fmt.Sprintf("Session sealed and exported to %s", exportedPath),
			Timestamp: time.Now(),
		})
	}

	o.broadcastState()
	return exportedPath, err
}

// TogglePause pauses or resumes recording and speech dispatch.
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

	if paused {
		o.speaker.StopImmediate()
	}
	o.broadcastState()
	return paused
}

// ToggleBatchMode toggles Batch-Hold mode (`⌥ + B`). Disabling flushes pending utterances.
func (o *Orchestrator) ToggleBatchMode() bool {
	next := o.batcher.ToggleBatchMode()
	o.broadcastState()
	return next
}

// FlushBatch dispatches buffered utterances immediately.
func (o *Orchestrator) FlushBatch() int {
	n := o.batcher.Flush()
	o.broadcastState()
	return n
}

// DiscardLastUtterance purges the most recent utterance (removing from pending batch if buffered).
func (o *Orchestrator) DiscardLastUtterance() bool {
	o.batcher.DiscardLastPending()
	o.mu.Lock()
	defer func() {
		o.mu.Unlock()
		o.broadcastState()
	}()
	for i := len(o.utterances) - 1; i >= 0; i-- {
		if !o.utterances[i].Discarded {
			o.utterances[i].Discarded = true
			return true
		}
	}
	return false
}

// ToggleVoiceMute mutes or unmutes Antigravity spoken TTS responses.
func (o *Orchestrator) ToggleVoiceMute() bool {
	next := !o.speaker.IsMuted()
	o.speaker.SetMuted(next)
	o.broadcastState()
	return next
}

// ToggleHUDCompactMode switches between full Director Studio and Compact Floating HUD.
func (o *Orchestrator) ToggleHUDCompactMode() bool {
	o.mu.Lock()
	o.hudCompactMode = !o.hudCompactMode
	compact := o.hudCompactMode
	cb := o.onHUDToggle
	o.mu.Unlock()

	if cb != nil {
		cb(compact)
	}
	o.broadcastState()
	return compact
}

// ResolveApproval sends a user approve/deny decision to the pending Antigravity permission request.
func (o *Orchestrator) ResolveApproval(requestID string, decision antigravity.ApprovalDecision) error {
	o.mu.Lock()
	pending := o.pendingApproval
	if pending != nil && (requestID == "" || pending.ID == requestID) {
		requestID = pending.ID
		o.pendingApproval = nil
	}
	o.mu.Unlock()

	o.speaker.StopImmediate()
	if requestID == "" {
		o.broadcastState()
		return nil
	}
	err := o.agyCtrl.ResolvePermission(requestID, decision)
	o.appendChat(model.ChatEntry{
		ID:        fmt.Sprintf("sys_%d", time.Now().UnixNano()),
		Role:      "system",
		Text:      fmt.Sprintf("Permission %s: %sd", requestID, decision),
		Timestamp: time.Now(),
	})
	o.broadcastState()
	return err
}

// SubmitUtterance allows injecting a finalized director utterance (from native STT, Web Speech API, or manual input).
func (o *Orchestrator) SubmitUtterance(text string, confidence float32) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}

	// If a permission approval is pending, check for voice approval intent first!
	o.mu.RLock()
	pending := o.pendingApproval
	paused := o.paused
	recording := o.recording
	o.mu.RUnlock()

	if pending != nil {
		if decision := antigravity.ParseVoiceApprovalIntent(text); decision != antigravity.DecisionNone {
			_ = o.ResolveApproval(pending.ID, decision)
			return
		}
	}

	if paused {
		return
	}

	nowTS := o.ElapsedSeconds()
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
		framesDir := filepath.Join(bDir, "frames")
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
	}

	dispatched := o.batcher.Submit(utt)
	utt.Dispatched = dispatched

	o.mu.Lock()
	o.utterances = append(o.utterances, utt)
	o.frames = append(o.frames, snaps...)
	o.mu.Unlock()

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
		// Hardware AEC barge-in: immediately interrupt agent TTS playback!
		if o.speaker.IsSpeaking() {
			o.speaker.StopImmediate()
		}
		o.mu.Lock()
		o.speechStartedAt = ev.Timestamp
		o.mu.Unlock()

	case speech.EventPartial:
		o.mu.Lock()
		o.liveTranscript = ev.Text
		o.mu.Unlock()
		o.broadcastState()

	case speech.EventFinal:
		o.SubmitUtterance(ev.Text, ev.Confidence)

	case speech.EventVADEnd:
		o.mu.Lock()
		o.liveTranscript = ""
		o.mu.Unlock()
		o.broadcastState()
	}
}

func (o *Orchestrator) dispatchBatchToAntigravity(batch []model.Utterance) {
	if len(batch) == 0 {
		return
	}

	var combinedText []string
	var framePaths []string
	for _, u := range batch {
		combinedText = append(combinedText, u.Transcript)
		if u.TriggerFramePath != "" {
			framePaths = append(framePaths, u.TriggerFramePath)
		}
		if u.ReactionFramePath != "" && u.ReactionFramePath != u.TriggerFramePath {
			framePaths = append(framePaths, u.ReactionFramePath)
		}
	}

	o.appendChat(model.ChatEntry{
		ID:         fmt.Sprintf("usr_%d", time.Now().UnixNano()),
		Role:       "user",
		Text:       strings.Join(combinedText, " "),
		FramePaths: framePaths,
		Timestamp:  time.Now(),
	})

	o.mu.RLock()
	target := o.target
	o.mu.RUnlock()

	if o.agyCtrl.IsRunning() {
		if err := o.agyCtrl.SendUtterances(batch, target); err != nil {
			o.appendChat(model.ChatEntry{
				ID:        fmt.Sprintf("err_%d", time.Now().UnixNano()),
				Role:      "system",
				Text:      "Failed to dispatch to Antigravity: " + err.Error(),
				Timestamp: time.Now(),
			})
		}
	}
}

func (o *Orchestrator) appendChat(entry model.ChatEntry) {
	o.mu.Lock()
	o.chatHistory = append(o.chatHistory, entry)
	if len(o.chatHistory) > 200 {
		o.chatHistory = o.chatHistory[len(o.chatHistory)-200:]
	}
	o.mu.Unlock()
	o.broadcastState()
}

// --- mcp.Provider implementation ---

// GrabFrame extracts a frame on demand from the active ring buffer for Antigravity's MCP tool.
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

	o.mu.RLock()
	bDir := o.bundleDir
	o.mu.RUnlock()
	if bDir == "" {
		bDir = os.TempDir()
	}
	framesDir := filepath.Join(bDir, "frames")
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

// GetClip slices a sub-clip from the active recording.mp4 using ffmpeg.
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

// SessionStatus returns a compact status map for the xnk_session_status MCP tool.
func (o *Orchestrator) SessionStatus() map[string]any {
	st := o.StateSnapshot()
	return map[string]any{
		"capture_id":      st.CaptureID,
		"recording":       st.Recording,
		"paused":          st.Paused,
		"batch_mode":      st.BatchMode,
		"elapsed_seconds": st.ElapsedSeconds,
		"agent_state":     st.AgentState,
		"utterance_count": len(st.Utterances),
		"workspace_path":  st.WorkspacePath,
	}
}

// StorageRoot returns the root directory where recordings are stored.
func (o *Orchestrator) StorageRoot() string {
	return o.storageRoot
}
