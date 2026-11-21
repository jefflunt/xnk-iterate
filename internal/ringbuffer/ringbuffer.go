package ringbuffer

import (
	"math"
	"sync"
)

// Frame stores an encoded video frame (PNG or JPEG) with its recording timestamp in seconds.
type Frame struct {
	Timestamp float64
	Format    string // "png" or "jpeg"
	Data      []byte
}

// FrameRingBuffer maintains a bounded, thread-safe rolling buffer of recent video frames
// for sub-15ms Trigger and Reaction snapshot extraction without touching disk.
type FrameRingBuffer struct {
	mu            sync.RWMutex
	maxDuration   float64
	maxFrames     int
	frames        []Frame
}

// New creates a FrameRingBuffer retaining up to maxDurationSeconds of frames (capped at maxFrames).
func New(maxDurationSeconds float64, maxFrames int) *FrameRingBuffer {
	if maxDurationSeconds <= 0 {
		maxDurationSeconds = 30.0
	}
	if maxFrames <= 0 {
		maxFrames = 180
	}
	return &FrameRingBuffer{
		maxDuration: maxDurationSeconds,
		maxFrames:   maxFrames,
		frames:      make([]Frame, 0, min(maxFrames, 64)),
	}
}

// Push appends a new frame and evicts frames older than the rolling window or beyond maxFrames.
func (b *FrameRingBuffer) Push(ts float64, format string, data []byte) {
	cp := make([]byte, len(data))
	copy(cp, data)

	b.mu.Lock()
	defer b.mu.Unlock()

	b.frames = append(b.frames, Frame{
		Timestamp: ts,
		Format:    format,
		Data:      cp,
	})

	cutoff := ts - b.maxDuration
	trimIdx := 0
	for trimIdx < len(b.frames) && b.frames[trimIdx].Timestamp < cutoff {
		trimIdx++
	}
	if len(b.frames)-trimIdx > b.maxFrames {
		trimIdx = len(b.frames) - b.maxFrames
	}
	if trimIdx > 0 {
		remaining := make([]Frame, len(b.frames)-trimIdx)
		copy(remaining, b.frames[trimIdx:])
		b.frames = remaining
	}
}

// Nearest returns the frame closest to targetTimestamp, or false if the buffer is empty.
func (b *FrameRingBuffer) Nearest(targetTimestamp float64) (Frame, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if len(b.frames) == 0 {
		return Frame{}, false
	}

	bestIdx := 0
	bestDist := math.Abs(b.frames[0].Timestamp - targetTimestamp)
	for i := 1; i < len(b.frames); i++ {
		d := math.Abs(b.frames[i].Timestamp - targetTimestamp)
		if d < bestDist {
			bestDist = d
			bestIdx = i
		}
	}

	f := b.frames[bestIdx]
	cp := make([]byte, len(f.Data))
	copy(cp, f.Data)
	f.Data = cp
	return f, true
}

// Latest returns the most recently pushed frame.
func (b *FrameRingBuffer) Latest() (Frame, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if len(b.frames) == 0 {
		return Frame{}, false
	}
	f := b.frames[len(b.frames)-1]
	cp := make([]byte, len(f.Data))
	copy(cp, f.Data)
	f.Data = cp
	return f, true
}

// Len returns the number of frames currently retained in memory.
func (b *FrameRingBuffer) Len() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.frames)
}

// Clear empties the ring buffer.
func (b *FrameRingBuffer) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.frames = b.frames[:0]
}
