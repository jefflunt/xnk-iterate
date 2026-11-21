package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

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

	orch := session.NewOrchestrator(*storageDir)
	srv := server.New(orch, ui.AssetsFS())

	url, err := srv.Start(*addr)
	if err != nil {
		log.Fatalf("failed to start local server: %v", err)
	}
	defer func() {
		_, _ = orch.StopSession()
		_ = srv.Close()
	}()

	if *headless {
		fmt.Printf("XNK Iterate server running at %s (headless mode)\n", url)
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
		if r.Detached {
			captureBox.Show(r)
			captureBox.SetLocked(r.Locked)
		} else {
			captureBox.Hide()
		}
	})
	orch.SetHUDToggleCallback(func(compact bool) {
		app.SetCompactHUD(compact)
	})

	if err := app.Run(); err != nil {
		log.Fatalf("desktop UI error: %v", err)
	}
}
