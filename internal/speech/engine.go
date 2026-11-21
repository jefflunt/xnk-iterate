package speech

import "context"

// EventType enumerates real-time speech & VAD stream events.
type EventType string

const (
	EventAudioLevel EventType = "audio_level"
	EventVADStart   EventType = "vad_start"
	EventPartial    EventType = "partial"
	EventFinal      EventType = "final"
	EventVADEnd     EventType = "vad_end"
	EventStatus     EventType = "status"
	EventError      EventType = "error"
)

// Event represents a single real-time speech/VAD update emitted by the SpeechEngine.
type Event struct {
	Type          EventType `json:"type"`
	Level         float64   `json:"level,omitempty"`          // Normalized RMS [0.0, 1.0]
	Text          string    `json:"text,omitempty"`           // Partial or final transcript
	Confidence    float32   `json:"confidence,omitempty"`     // 0..1 recognition confidence
	Timestamp     float64   `json:"timestamp,omitempty"`      // Recording-relative seconds
	SilenceOffset float64   `json:"silence_offset,omitempty"` // Seconds elapsed since last spoken word
	Message       string    `json:"message,omitempty"`        // Diagnostic/status text
}

// Engine is the cross-platform live microphone VAD + Speech-to-Text interface.
type Engine interface {
	Start(ctx context.Context, silenceTimeoutMS int, elapsedFn func() float64, onEvent func(Event)) error
	Stop() error
	MicrophoneStatus() string
	SpeechStatus() string
}
