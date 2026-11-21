//go:build !darwin

package speech

import "context"

type fallbackEngine struct{}

// NewPlatformEngine returns a cross-platform speech engine fallback.
// Windows (SAPI / Whisper) and Linux (Whisper / Vosk) providers plug in here;
// the WebView UI also supports Web Speech API and manual director input.
func NewPlatformEngine() Engine {
	return &fallbackEngine{}
}

func (f *fallbackEngine) MicrophoneStatus() string { return "unknown" }
func (f *fallbackEngine) SpeechStatus() string     { return "unknown" }
func (f *fallbackEngine) Start(ctx context.Context, silenceTimeoutMS int, elapsedFn func() float64, onEvent func(Event)) error {
	if onEvent != nil {
		onEvent(Event{Type: EventStatus, Message: "using_ui_speech_bridge"})
	}
	return nil
}
func (f *fallbackEngine) Stop() error { return nil }
