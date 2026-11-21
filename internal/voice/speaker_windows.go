//go:build windows

package voice

import (
	"fmt"
	"os/exec"
	"strings"
	"sync"
)

type windowsSpeaker struct {
	mu            sync.Mutex
	muted         bool
	speaking      bool
	cmd           *exec.Cmd
	queue         []string
	onStateChange func(speaking bool)
}

// NewPlatformSpeaker returns a Windows System.Speech TTS speaker with instant barge-in.
func NewPlatformSpeaker() Speaker {
	return &windowsSpeaker{}
}

func (w *windowsSpeaker) SetOnStateChange(fn func(speaking bool)) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.onStateChange = fn
}

func (w *windowsSpeaker) SetMuted(muted bool) {
	w.mu.Lock()
	w.muted = muted
	w.mu.Unlock()
	if muted {
		w.StopImmediate()
	}
}

func (w *windowsSpeaker) IsMuted() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.muted
}

func (w *windowsSpeaker) IsSpeaking() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.speaking
}

func (w *windowsSpeaker) Speak(text string) {
	text = cleanForSpeech(text)
	if text == "" {
		return
	}
	w.mu.Lock()
	if w.muted {
		w.mu.Unlock()
		return
	}
	w.queue = append(w.queue, text)
	if !w.speaking {
		w.speaking = true
		cb := w.onStateChange
		w.mu.Unlock()
		if cb != nil {
			cb(true)
		}
		go w.drainQueue()
		return
	}
	w.mu.Unlock()
}

func (w *windowsSpeaker) drainQueue() {
	for {
		w.mu.Lock()
		if w.muted || len(w.queue) == 0 {
			w.speaking = false
			w.cmd = nil
			cb := w.onStateChange
			w.mu.Unlock()
			if cb != nil {
				cb(false)
			}
			return
		}
		next := strings.ReplaceAll(w.queue[0], "'", "''")
		w.queue = w.queue[1:]
		ps := fmt.Sprintf("Add-Type -AssemblyName System.Speech; (New-Object System.Speech.Synthesis.SpeechSynthesizer).Speak('%s')", next)
		cmd := exec.Command("powershell", "-NoProfile", "-Command", ps)
		w.cmd = cmd
		w.mu.Unlock()
		_ = cmd.Run()
	}
}

func (w *windowsSpeaker) StopImmediate() {
	w.mu.Lock()
	w.queue = w.queue[:0]
	cmd := w.cmd
	wasSpeaking := w.speaking
	w.speaking = false
	w.cmd = nil
	cb := w.onStateChange
	w.mu.Unlock()
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	if wasSpeaking && cb != nil {
		cb(false)
	}
}
