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
	addr := flag.String("addr", "127.0.0.1:0", "local HTTP/WebSocket bind address (use 127.0.0.1:9292 for fixed port)")
	storageDir := flag.String("storage", "", "directory for exported director bundles (defaults to ./storage/recordings)")
	headless := flag.Bool("headless", false, "run local HTTP/WS/MCP server without opening the native desktop window")
	flag.Parse()

	logFile := applog.Init("")
	applog.Info("main", "starting XNK Iterate (log=%s)", logFile)

	orch := session.NewOrchestrator(*storageDir)
	srv := server.New(orch, ui.AssetsFS())
	srv.SetFolderPicker(ui.PickFolder)

	url, err := srv.Start(*addr)
	if err != nil {
		applog.Error("main", "failed to start local server: %v", err)
		log.Fatalf("failed to start local server: %v", err)
	}
	applog.Info("main", "local server listening at %s", url)
	defer func() {
		_, _ = orch.StopSession()
		_ = srv.Close()
	}()

	if *headless {
		fmt.Printf("XNK Iterate server running at %s (headless mode, log: %s)\n", url, logFile)
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
		<-sig
		return
	}

	app := ui.NewDesktopApp("XNK Iterate — Voice + Vision Antigravity Director", url)
	captureBox := ui.NewPlatformCaptureBox()
	defer captureBox.Destroy()

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
	orch.SetHUDToggleCallback(func(compact bool) {
		app.SetCompactHUD(compact)
	})

	// Capture box is always detached — show it immediately across monitors on launch
	captureBox.Show(orch.StateSnapshot().Region)

	if err := app.Run(); err != nil {
		applog.Error("main", "desktop UI error: %v", err)
		log.Fatalf("desktop UI error: %v", err)
	}
}
