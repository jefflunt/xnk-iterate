package model

import "time"

// TargetType identifies whether a capture targets a display, window, or application.
type TargetType string

const (
	TargetDisplay     TargetType = "display"
	TargetWindow      TargetType = "window"
	TargetApplication TargetType = "application"
)

// Size represents a 2D pixel resolution.
type Size struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

// Rect represents a screen-space rectangle.
type Rect struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

// Point represents a 2D screen coordinate.
type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// CaptureRegion defines the user's draggable/resizable screen-space capture rectangle.
// Coordinates are in top-left global screen space across all connected monitors.
type CaptureRegion struct {
	X            int  `json:"x"`
	Y            int  `json:"y"`
	Width        int  `json:"width"`
	Height       int  `json:"height"`
	DisplayIndex int  `json:"display_index"`
	Detached     bool `json:"detached"`
	Locked       bool `json:"locked"`
}

// CaptureTarget describes an available or selected display/window capture source.
type CaptureTarget struct {
	ID               string     `json:"id"`
	Type             TargetType `json:"type"`
	Title            string     `json:"title"`
	AppName          string     `json:"app_name,omitempty"`
	BundleIdentifier string     `json:"bundle_id,omitempty"`
	Resolution       Size       `json:"resolution"`
	Bounds           Rect       `json:"bounds"`
	DisplayIndex     int        `json:"display_index"`
}

// InteractionType enumerates OS-level user interaction events.
type InteractionType string

const (
	InteractionMouseDown  InteractionType = "mouseDown"
	InteractionMouseUp    InteractionType = "mouseUp"
	InteractionRightClick InteractionType = "rightClick"
	InteractionKeyPress   InteractionType = "keyPress"
	InteractionHotkey     InteractionType = "hotkey"
	InteractionAppSwitch  InteractionType = "appSwitch"
)

// InteractionEvent represents an OS-level input or focus event captured during recording.
type InteractionEvent struct {
	Timestamp  float64         `json:"timestamp"`
	Type       InteractionType `json:"type"`
	Location   *Point          `json:"location,omitempty"`
	KeyCommand string          `json:"key_command,omitempty"`
	FocusedApp string          `json:"focused_app,omitempty"`
}

// LookbackRange defines the dynamically computed visual context window for an utterance.
type LookbackRange struct {
	StartSeconds   float64 `json:"start_seconds"`
	EndSeconds     float64 `json:"end_seconds"`
	TriggerSeconds float64 `json:"trigger_seconds"`
	Duration       float64 `json:"duration"`
}

// FrameRole identifies the semantic role of an extracted video snapshot.
type FrameRole string

const (
	FrameRoleTrigger  FrameRole = "trigger"
	FrameRoleReaction FrameRole = "reaction"
	FrameRoleCarousel FrameRole = "carousel"
	FrameRoleOnDemand FrameRole = "ondemand"
)

// FrameSnapshot represents an extracted visual frame saved to disk.
type FrameSnapshot struct {
	Timestamp    float64   `json:"timestamp"`
	RelativePath string    `json:"relative_path"`
	AbsolutePath string    `json:"absolute_path,omitempty"`
	Role         FrameRole `json:"role"`
	UtteranceID  int       `json:"utterance_id"`
}

// Utterance represents a finalized director voice comment correlated with screen frames.
type Utterance struct {
	ID                int               `json:"id"`
	Transcript        string            `json:"transcript"`
	Confidence        float32           `json:"confidence"`
	SpeechStart       float64           `json:"speech_start"`
	SpeechEnd         float64           `json:"speech_end"`
	LookbackWindow    LookbackRange     `json:"lookback_window"`
	TriggerAction     *InteractionEvent `json:"trigger_action,omitempty"`
	TriggerFramePath  string            `json:"trigger_frame_path"`
	ReactionFramePath string            `json:"reaction_frame_path"`
	CarouselPaths     []string          `json:"carousel_paths,omitempty"`
	Dispatched        bool              `json:"dispatched"`
	Discarded         bool              `json:"discarded,omitempty"`
}

// CaptureManifest is the root metadata schema written to manifest.json in an archive bundle.
type CaptureManifest struct {
	CaptureID       string             `json:"capture_id"`
	CreatedAt       time.Time          `json:"created_at"`
	DurationSeconds float64            `json:"duration_seconds"`
	Target          CaptureTarget      `json:"target"`
	WorkspacePath   string             `json:"workspace_path,omitempty"`
	ConversationID  string             `json:"conversation_id,omitempty"`
	VideoFile       string             `json:"video_file"`
	Utterances      []Utterance        `json:"utterances"`
	Interactions    []InteractionEvent `json:"interactions"`
	Frames          []FrameSnapshot    `json:"frames"`
}

// AgentState represents the lifecycle state of the embedded Google Antigravity session.
type AgentState string

const (
	AgentIdle             AgentState = "idle"
	AgentStarting         AgentState = "starting"
	AgentListening        AgentState = "listening"
	AgentThinking         AgentState = "thinking"
	AgentWorking          AgentState = "working"
	AgentSpeaking         AgentState = "speaking"
	AgentAwaitingApproval AgentState = "awaiting_approval"
	AgentError            AgentState = "error"
)

// PermissionRequest represents a pending tool approval emitted by the owned agy session.
type PermissionRequest struct {
	ID          string    `json:"id"`
	ToolName    string    `json:"tool_name"`
	Description string    `json:"description"`
	Command     string    `json:"command,omitempty"`
	RequestedAt time.Time `json:"requested_at"`
}

// ChatEntry represents a message in the embedded Antigravity live conversation feed.
type ChatEntry struct {
	ID         string    `json:"id"`
	Role       string    `json:"role"` // "user", "ai", "system", "tool"
	Text       string    `json:"text"`
	ToolName   string    `json:"tool_name,omitempty"`
	FramePaths []string  `json:"frame_paths,omitempty"`
	Timestamp  time.Time `json:"timestamp"`
}

// PermissionStatus reports OS-level permission readiness for the UI.
type PermissionStatus struct {
	Platform        string `json:"platform"`
	ScreenRecording string `json:"screen_recording"` // "granted", "unknown", "denied"
	Microphone      string `json:"microphone"`
	Speech          string `json:"speech"`
	Accessibility   string `json:"accessibility"`
	FFmpegAvailable bool   `json:"ffmpeg_available"`
	AgyAvailable    bool   `json:"agy_available"`
	AgyPath         string `json:"agy_path,omitempty"`
}

// SessionStateSnapshot is broadcast to the UI over WebSocket on state changes.
type SessionStateSnapshot struct {
	CaptureID         string             `json:"capture_id,omitempty"`
	Recording         bool               `json:"recording"`
	Paused            bool               `json:"paused"`
	BatchMode         bool               `json:"batch_mode"`
	BatchedCount      int                `json:"batched_count"`
	ElapsedSeconds    float64            `json:"elapsed_seconds"`
	Target            *CaptureTarget     `json:"target,omitempty"`
	Region            CaptureRegion      `json:"region"`
	WorkspacePath     string             `json:"workspace_path"`
	ConversationID    string             `json:"conversation_id,omitempty"`
	ContinueSession   bool               `json:"continue_session"`
	VoiceOutputMuted  bool               `json:"voice_output_muted"`
	AgentState        AgentState         `json:"agent_state"`
	AgentStatusText   string             `json:"agent_status_text"`
	LiveTranscript    string             `json:"live_transcript"`
	AudioLevel        float64            `json:"audio_level"`
	PendingApproval   *PermissionRequest `json:"pending_approval,omitempty"`
	Utterances        []Utterance        `json:"utterances"`
	ChatHistory       []ChatEntry        `json:"chat_history"`
	Permissions       PermissionStatus   `json:"permissions"`
	LastBundlePath    string             `json:"last_bundle_path,omitempty"`
	HUDCompactMode    bool               `json:"hud_compact_mode"`
	RecentInteraction *InteractionEvent  `json:"recent_interaction,omitempty"`
}
