package applog

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const maxLogBytes = 5 * 1024 * 1024 // 5 MB rotation threshold

var (
	mu       sync.Mutex
	logFile  *os.File
	logPath  string
	ring     []string
	maxRing  = 250
	inited   bool
)

// Init initializes the persistent diagnostic log file at ~/Library/Logs/XNK Iterate/xnk-iterate.log
// (or a custom path if provided, falling back to os.TempDir() if needed).
func Init(customPath string) string {
	mu.Lock()
	defer mu.Unlock()
	if inited && customPath == "" {
		return logPath
	}
	if logFile != nil {
		_ = logFile.Close()
		logFile = nil
	}

	target := customPath
	if target == "" {
		if home, err := os.UserHomeDir(); err == nil {
			target = filepath.Join(home, "Library", "Logs", "XNK Iterate", "xnk-iterate.log")
		} else {
			target = filepath.Join(os.TempDir(), "xnk-iterate.log")
		}
	}

	f, resolved := openOrFallback(target)
	logFile = f
	logPath = resolved
	inited = true
	return logPath
}

func openOrFallback(target string) (*os.File, string) {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err == nil {
		rotateIfLarge(target)
		if f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
			return f, target
		}
	}
	fallback := filepath.Join(os.TempDir(), "xnk-iterate.log")
	rotateIfLarge(fallback)
	f, _ := os.OpenFile(fallback, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	return f, fallback
}

func rotateIfLarge(path string) {
	if st, err := os.Stat(path); err == nil && st.Size() > maxLogBytes {
		_ = os.Rename(path, path+".1")
	}
}

// Path returns the active log file path on disk.
func Path() string {
	mu.Lock()
	defer mu.Unlock()
	if !inited {
		mu.Unlock()
		p := Init("")
		mu.Lock()
		return p
	}
	return logPath
}

// Log writes a timestamped diagnostic entry to the log file and in-memory tail buffer.
func Log(level, component, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	ts := time.Now().Format("2006-01-02T15:04:05.000-07:00")
	line := fmt.Sprintf("%s [%s] [%s] %s", ts, level, component, msg)

	mu.Lock()
	if !inited {
		mu.Unlock()
		Init("")
		mu.Lock()
	}
	ring = append(ring, line)
	if len(ring) > maxRing {
		ring = ring[len(ring)-maxRing:]
	}
	if logFile != nil {
		_, _ = fmt.Fprintln(logFile, line)
	}
	mu.Unlock()
}

// Info logs an informational diagnostic event.
func Info(component, format string, args ...any) {
	Log("INFO", component, format, args...)
}

// Warn logs a warning diagnostic event.
func Warn(component, format string, args ...any) {
	Log("WARN", component, format, args...)
}

// Error logs an error diagnostic event.
func Error(component, format string, args ...any) {
	Log("ERROR", component, format, args...)
}

// Recent returns up to n most recent log lines from memory.
func Recent(n int) []string {
	mu.Lock()
	defer mu.Unlock()
	if n <= 0 || n > len(ring) {
		n = len(ring)
	}
	out := make([]string, n)
	copy(out, ring[len(ring)-n:])
	return out
}
