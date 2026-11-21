//go:build darwin

package voice

import (
	"os/exec"
	"sync"
)

type darwinSpeaker struct {
	mu            sync.Mutex
	muted         bool
	speaking      bool
	cmd           *exec.Cmd
	queue         []string
	onStateChange func(speaking bool)
}

// NewPlatformSpeaker returns a macOS native TTS speaker using `/usr/bin/say`
// with instant process-kill barge-in interruption (<5ms).
func NewPlatformSpeaker() Speaker {
	return &darwinSpeaker{}
}

func (d *darwinSpeaker) SetOnStateChange(fn func(speaking bool)) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.onStateChange = fn
}

func (d *darwinSpeaker) SetMuted(muted bool) {
	d.mu.Lock()
	d.muted = muted
	d.mu.Unlock()
	if muted {
		d.StopImmediate()
	}
}

func (d *darwinSpeaker) IsMuted() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.muted
}

func (d *darwinSpeaker) IsSpeaking() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.speaking
}

func (d *darwinSpeaker) Speak(text string) {
	text = cleanForSpeech(text)
	if text == "" {
		return
	}

	d.mu.Lock()
	if d.muted {
		d.mu.Unlock()
		return
	}
	d.queue = append(d.queue, text)
	if !d.speaking {
		d.speaking = true
		cb := d.onStateChange
		d.mu.Unlock()
		if cb != nil {
			cb(true)
		}
		go d.drainQueue()
		return
	}
	d.mu.Unlock()
}

func (d *darwinSpeaker) drainQueue() {
	for {
		d.mu.Lock()
		if d.muted || len(d.queue) == 0 {
			d.speaking = false
			d.cmd = nil
			cb := d.onStateChange
			d.mu.Unlock()
			if cb != nil {
				cb(false)
			}
			return
		}
		next := d.queue[0]
		d.queue = d.queue[1:]
		cmd := exec.Command("/usr/bin/say", "-r", "205", next)
		d.cmd = cmd
		d.mu.Unlock()

		_ = cmd.Run()
	}
}

func (d *darwinSpeaker) StopImmediate() {
	d.mu.Lock()
	d.queue = d.queue[:0]
	cmd := d.cmd
	wasSpeaking := d.speaking
	d.speaking = false
	d.cmd = nil
	cb := d.onStateChange
	d.mu.Unlock()

	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	if wasSpeaking && cb != nil {
		cb(false)
	}
}
