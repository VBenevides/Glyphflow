//go:build workerui

package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed ui assets
var workerAssets embed.FS

//go:embed assets/glyphflow.png
var workerIcon []byte

func main() {
	var capacity atomic.Int64
	logs := NewLogBuffer(&capacity)
	stdout := logs.Writer("stdout", os.Stdout)
	stderr := logs.Writer("stderr", os.Stderr)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		if err := runWorker(ctx, stdout, stderr, logs); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
		}
	}()
	trayEnabled := trayAvailable()
	if !trayEnabled {
		_, _ = fmt.Fprintln(stderr, "system tray unavailable; showing a regular worker window")
	}

	app := application.New(application.Options{
		Name:        "Glyphflow Worker",
		Description: "Glyphflow worker tray application",
		Assets:      application.AssetOptions{Handler: workerAssetHandler(logs)},
	})
	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:             "worker-window",
		Title:            "Glyphflow Worker",
		Width:            860,
		Height:           560,
		MinWidth:         640,
		MinHeight:        420,
		Hidden:           trayEnabled,
		BackgroundColour: application.NewRGB(243, 240, 255),
		URL:              "http://wails.localhost/",
	})
	var exitOnce sync.Once
	exit := func() {
		exitOnce.Do(func() {
			cancel()
			app.Quit()
		})
	}
	if trayEnabled {
		tray := app.SystemTray.New()
		tray.SetIcon(workerIcon)
		tray.SetLabel("Glyphflow Worker")
		tray.SetTooltip("Glyphflow Worker")
		open := func() {
			window.UnMinimise()
			window.Show()
			window.Focus()
		}
		menu := app.NewMenu()
		menu.Add("Open").OnClick(func(*application.Context) { open() })
		menu.AddSeparator()
		menu.Add("Exit").OnClick(func(*application.Context) { exit() })
		tray.SetMenu(menu)
		tray.OnClick(open)

		window.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
			select {
			case <-ctx.Done():
				return
			default:
				event.Cancel()
				window.Hide()
			}
		})
		window.OnWindowEvent(events.Common.WindowMinimise, func(*application.WindowEvent) {
			window.UnMinimise()
			window.Hide()
		})
	}
	app.OnShutdown(func() {
		cancel()
		<-workerDone
	})

	if err := app.Run(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
	}
}

func workerAssetHandler(logs *LogBuffer) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/api/snapshot", snapshotHandler(logs))
	mux.Handle("/", application.AssetFileServerFS(workerAssets))
	return mux
}

func snapshotHandler(logs *LogBuffer) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		after := uint64(0)
		if raw := r.URL.Query().Get("after"); raw != "" {
			var err error
			after, err = parseSequence(raw)
			if err != nil {
				http.Error(w, "after must be a non-negative integer", http.StatusBadRequest)
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = jsonEncode(w, logs.Snapshot(after))
	})
}

func parseSequence(raw string) (uint64, error) {
	return strconv.ParseUint(raw, 10, 64)
}

func jsonEncode(w io.Writer, value any) error {
	return json.NewEncoder(w).Encode(value)
}
