package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/jefflunt/xnk-iterate/internal/applog"
	"github.com/jefflunt/xnk-iterate/internal/model"
	"github.com/jefflunt/xnk-iterate/internal/server"
	"github.com/jefflunt/xnk-iterate/internal/session"
	"github.com/jefflunt/xnk-iterate/internal/ui"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:9292", "local HTTP/WebSocket/MCP bind address (falls back to ephemeral if busy)")
	storageDir := flag.String("storage", "", "directory for exported director bundles")
	headless := flag.Bool("headless", false, "run local HTTP/WS/MCP server without opening the native HUD window")
	flag.Parse()

	logFile := applog.Init("")
	applog.Info("main", "starting XNK Iterate Companion (log=%s)", logFile)

	orch := session.NewOrchestrator(*storageDir)
	srv := server.New(orch, ui.AssetsFS())

	url, err := srv.Start(*addr)
	if err != nil {
		applog.Error("main", "failed to start local server: %v", err)
		log.Fatalf("failed to start local server: %v", err)
	}
	applog.Info("main", "local companion bridge listening at %s", url)
	defer func() {
		_, _ = orch.StopSession()
		_ = srv.Close()
	}()

	if *headless {
		fmt.Printf("XNK Iterate Companion running at %s (headless mode, log: %s)\n", url, logFile)
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
		<-sig
		return
	}

	app := ui.NewDesktopApp("XNK Iterate", url)
	captureBox := ui.NewPlatformCaptureBox()
	defer captureBox.Destroy()

	ui.SetOnTerminate(func() {
		applog.Info("main", "application terminating — unregistering hotkeys, stopping session, destroying Capture Box")
		ui.UnregisterGlobalHotkeys()
		captureBox.Destroy()
		_, _ = orch.StopSession()
		_ = srv.Close()
	})

	// Register system-wide macOS Carbon hotkeys (customizable in Settings; defaults ⌥↩ and ⌥⌫)
	ui.RegisterGlobalHotkeys(orch.StateSnapshot().Hotkeys, func(hotkeyID int) {
		switch hotkeyID {
		case ui.HotkeySendTurn:
			orch.SendCurrentTurn()
		case ui.HotkeyDiscardTurn:
			orch.DiscardCurrentTurn()
		}
	})
	orch.SetHotkeysCallback(func(cfg model.HotkeyConfig) {
		ui.UpdateGlobalHotkeys(cfg)
	})
	orch.SetHotkeysSuspendCallback(func(suspended bool) {
		ui.SuspendGlobalHotkeys(suspended)
	})

	captureBox.SetOnRegionChange(func(r model.CaptureRegion) {
		orch.UpdateRegion(r, true)
	})
	orch.SetRegionCallback(func(r model.CaptureRegion) {
		if r.Visible {
			captureBox.Show(r)
			captureBox.SetLocked(r.Locked)
		} else {
			captureBox.Hide()
		}
	})
	orch.SetDrawerToggleCallback(func(expanded bool) {
		app.SetDrawerExpanded(expanded)
	})

	// Detached multi-monitor Capture Box opens immediately on launch
	captureBox.Show(orch.StateSnapshot().Region)

	if err := app.Run(); err != nil {
		applog.Error("main", "desktop UI error: %v", err)
		log.Fatalf("desktop UI error: %v", err)
	}
}
