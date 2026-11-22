//go:build darwin

package speech

import (
	"bufio"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/jefflunt/xnk-iterate/internal/applog"
)

//go:embed helper_darwin.swift
var helperSwiftSource string

type darwinEngine struct {
	mu      sync.Mutex
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	cancel  context.CancelFunc
	running bool
}

// NewPlatformEngine creates the macOS native AVAudioEngine + SFSpeechRecognizer engine.
func NewPlatformEngine() Engine {
	return &darwinEngine{}
}

func (d *darwinEngine) MicrophoneStatus() string {
	if _, err := exec.LookPath("swiftc"); err == nil {
		return "ready"
	}
	return "unknown"
}

func (d *darwinEngine) SpeechStatus() string {
	if _, err := exec.LookPath("swiftc"); err == nil {
		return "ready"
	}
	return "unknown"
}

func ensureSwiftHelperBinary() (string, error) {
	if _, err := exec.LookPath("swiftc"); err != nil {
		return "", fmt.Errorf("swiftc not available: %w", err)
	}
	sum := sha256.Sum256([]byte(helperSwiftSource))
	hash := hex.EncodeToString(sum[:8])

	cacheDir := filepath.Join(os.TempDir(), "xnk-iterate-helpers")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", err
	}
	binPath := filepath.Join(cacheDir, "xnk-speech-helper-"+hash)
	if _, err := os.Stat(binPath); err == nil {
		return binPath, nil
	}

	srcPath := filepath.Join(cacheDir, "xnk-speech-helper-"+hash+".swift")
	if err := os.WriteFile(srcPath, []byte(helperSwiftSource), 0o644); err != nil {
		return "", err
	}
	defer os.Remove(srcPath)

	modCache := filepath.Join(cacheDir, "swift-module-cache")
	_ = os.MkdirAll(modCache, 0o755)
	cmd := exec.Command("swiftc", "-module-cache-path", modCache, "-O", srcPath, "-o", binPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("swiftc compile failed: %w: %s", err, string(out))
	}
	return binPath, nil
}

func (d *darwinEngine) Start(ctx context.Context, elapsedFn func() float64, onEvent func(Event)) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.running {
		return nil
	}

	binPath, err := ensureSwiftHelperBinary()
	if err != nil {
		if onEvent != nil {
			onEvent(Event{Type: EventStatus, Message: "native_speech_unavailable: " + err.Error()})
		}
		return nil
	}

	runCtx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(runCtx, binPath)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return err
	}
	stderr, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		cancel()
		return err
	}
	applog.Info("speech", "native speech helper started (pid=%d bin=%s)", cmd.Process.Pid, binPath)

	d.cmd = cmd
	d.stdin = stdin
	d.cancel = cancel
	d.running = true

	if stderr != nil {
		go func() {
			sc := bufio.NewScanner(stderr)
			logged := 0
			for sc.Scan() {
				line := strings.TrimSpace(sc.Text())
				if line == "" || logged >= 15 {
					continue
				}
				logged++
				applog.Warn("speech-helper", "%s", line)
			}
		}()
	}

	go func() {
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 0, 64*1024), 512*1024)
		for scanner.Scan() {
			var ev Event
			if err := json.Unmarshal(scanner.Bytes(), &ev); err != nil {
				continue
			}
			if elapsedFn != nil {
				ev.Timestamp = elapsedFn()
			}
			if onEvent != nil {
				onEvent(ev)
			}
		}
		_ = cmd.Wait()
	}()

	return nil
}

func (d *darwinEngine) FlushTurn() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.running || d.stdin == nil {
		return nil
	}
	_, err := d.stdin.Write([]byte("flush\n"))
	return err
}

func (d *darwinEngine) DiscardTurn() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.running || d.stdin == nil {
		return nil
	}
	_, err := d.stdin.Write([]byte("discard\n"))
	return err
}

func (d *darwinEngine) Stop() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.stdin != nil {
		_ = d.stdin.Close()
		d.stdin = nil
	}
	if d.cancel != nil {
		d.cancel()
		d.cancel = nil
	}
	if d.cmd != nil && d.cmd.Process != nil {
		_ = d.cmd.Process.Kill()
		d.cmd = nil
	}
	d.running = false
	return nil
}
