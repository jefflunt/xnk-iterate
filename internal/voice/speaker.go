package voice

import (
	"strings"
	"sync"
)

// Speaker provides cross-platform spoken TTS output with instant barge-in cancellation.
type Speaker interface {
	Speak(text string)
	StopImmediate()
	IsSpeaking() bool
	SetMuted(muted bool)
	IsMuted() bool
	SetOnStateChange(fn func(speaking bool))
}

// SentenceChunker buffers streaming LLM text deltas and emits complete,
// voice-friendly sentences as soon as sentence punctuation arrives.
type SentenceChunker struct {
	mu  sync.Mutex
	buf strings.Builder
}

// Push appends a text delta and returns zero or more complete sentences ready to speak.
func (c *SentenceChunker) Push(delta string) []string {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.buf.WriteString(delta)
	s := c.buf.String()
	var sentences []string

	for {
		idx := findSentenceBoundary(s)
		if idx < 0 {
			break
		}
		chunk := cleanForSpeech(s[:idx+1])
		if chunk != "" {
			sentences = append(sentences, chunk)
		}
		s = strings.TrimLeft(s[idx+1:], " \t\r\n")
	}

	c.buf.Reset()
	c.buf.WriteString(s)
	return sentences
}

// Flush emits any remaining buffered text.
func (c *SentenceChunker) Flush() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	rem := cleanForSpeech(c.buf.String())
	c.buf.Reset()
	return rem
}

// Reset discards buffered text.
func (c *SentenceChunker) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.buf.Reset()
}

func findSentenceBoundary(s string) int {
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch == '.' || ch == '!' || ch == '?' || ch == '\n' {
			// Require whitespace or end-of-string after punctuation (avoid splitting file.go / 0.1.0)
			if ch == '\n' || i+1 == len(s) || s[i+1] == ' ' || s[i+1] == '\n' || s[i+1] == '\t' {
				if i >= 12 || ch == '!' || ch == '?' || ch == '\n' {
					return i
				}
			}
		}
	}
	return -1
}

// cleanForSpeech strips markdown formatting so TTS sounds natural.
func cleanForSpeech(s string) string {
	s = strings.ReplaceAll(s, "```", " ")
	s = strings.ReplaceAll(s, "`", "")
	s = strings.ReplaceAll(s, "**", "")
	s = strings.ReplaceAll(s, "__", "")
	s = strings.ReplaceAll(s, "##", "")
	s = strings.ReplaceAll(s, "# ", "")
	s = strings.TrimSpace(s)
	return s
}
