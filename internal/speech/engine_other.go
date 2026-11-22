//go:build !darwin

package speech

import "context"

type fallbackEngine struct{}

func NewPlatformEngine() Engine {
	return &fallbackEngine{}
}

func (f *fallbackEngine) MicrophoneStatus() string { return "unknown" }
func (f *fallbackEngine) SpeechStatus() string     { return "unknown" }
func (f *fallbackEngine) Start(ctx context.Context, elapsedFn func() float64, onEvent func(Event)) error {
	if onEvent != nil {
		onEvent(Event{Type: EventStatus, Message: "using_ui_speech_bridge"})
	}
	return nil
}
func (f *fallbackEngine) FlushTurn() error   { return nil }
func (f *fallbackEngine) DiscardTurn() error { return nil }
func (f *fallbackEngine) Stop() error        { return nil }
