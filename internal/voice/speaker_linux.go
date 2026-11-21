//go:build linux

package voice

import (
	"os/exec"
	"sync"
)

type linuxSpeaker struct {
	mu            sync.Mutex
	muted         bool
	speaking      bool
	cmd           *exec.Cmd
	queue         []string
	onStateChange func(speaking bool)
}

// NewPlatformSpeaker returns a Linux espeak-ng / spd-say TTS speaker with instant barge-in.
func NewPlatformSpeaker() Speaker {
	return &linuxSpeaker{}
}

func (l *linuxSpeaker) SetOnStateChange(fn func(speaking bool)) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.onStateChange = fn
}

func (l *linuxSpeaker) SetMuted(muted bool) {
	l.mu.Lock()
	l.muted = muted
	l.mu.Unlock()
	if muted {
		l.StopImmediate()
	}
}

func (l *linuxSpeaker) IsMuted() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.muted
}

func (l *linuxSpeaker) IsSpeaking() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.speaking
}

func (l *linuxSpeaker) Speak(text string) {
	text = cleanForSpeech(text)
	if text == "" {
		return
	}
	l.mu.Lock()
	if l.muted {
		l.mu.Unlock()
		return
	}
	l.queue = append(l.queue, text)
	if !l.speaking {
		l.speaking = true
		cb := l.onStateChange
		l.mu.Unlock()
		if cb != nil {
			cb(true)
		}
		go l.drainQueue()
		return
	}
	l.mu.Unlock()
}

func (l *linuxSpeaker) drainQueue() {
	bin := "espeak-ng"
	if _, err := exec.LookPath(bin); err != nil {
		bin = "spd-say"
	}
	for {
		l.mu.Lock()
		if l.muted || len(l.queue) == 0 {
			l.speaking = false
			l.cmd = nil
			cb := l.onStateChange
			l.mu.Unlock()
			if cb != nil {
				cb(false)
			}
			return
		}
		next := l.queue[0]
		l.queue = l.queue[1:]
		cmd := exec.Command(bin, next)
		l.cmd = cmd
		l.mu.Unlock()
		_ = cmd.Run()
	}
}

func (l *linuxSpeaker) StopImmediate() {
	l.mu.Lock()
	l.queue = l.queue[:0]
	cmd := l.cmd
	wasSpeaking := l.speaking
	l.speaking = false
	l.cmd = nil
	cb := l.onStateChange
	l.mu.Unlock()
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	if wasSpeaking && cb != nil {
		cb(false)
	}
}
