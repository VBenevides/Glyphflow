//go:build workerui

package main

import (
	"bytes"
	"context"
	_ "embed" // Registers go:embed support for the bundled worker icon.
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"fyne.io/systray"
	"gioui.org/app"
	"gioui.org/font"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

//go:embed assets/glyphflow.png
var gioWorkerIcon []byte

type gioWorkerUI struct {
	window *app.Window
	theme  *material.Theme
	logo   paint.ImageOp
	logs   widget.List
	all    widget.Clickable
	stderr widget.Clickable

	mu         sync.RWMutex
	snapshot   Snapshot
	entries    []LogEntry
	stderrOnly bool
}

func newGioWorkerUI(window *app.Window) *gioWorkerUI {
	theme := material.NewTheme()
	theme.Palette = material.Palette{
		Bg:         color.NRGBA{R: 243, G: 240, B: 255, A: 255},
		Fg:         color.NRGBA{R: 50, G: 43, B: 69, A: 255},
		ContrastBg: color.NRGBA{R: 118, G: 84, B: 214, A: 255},
		ContrastFg: color.NRGBA{R: 255, G: 255, B: 255, A: 255},
	}
	img, err := png.Decode(bytes.NewReader(gioWorkerIcon))
	if err != nil {
		img = image.NewRGBA(image.Rect(0, 0, 1, 1))
	}
	logs := widget.List{List: layout.List{Axis: layout.Vertical}}
	return &gioWorkerUI{window: window, theme: theme, logo: paint.NewImageOp(img), logs: logs}
}

func (ui *gioWorkerUI) update(snapshot Snapshot, entries []LogEntry) {
	ui.mu.Lock()
	ui.snapshot = snapshot
	ui.entries = append(ui.entries[:0], entries...)
	ui.mu.Unlock()
	ui.window.Invalidate()
}

func (ui *gioWorkerUI) setFilter(stderrOnly bool) {
	ui.mu.Lock()
	ui.stderrOnly = stderrOnly
	ui.mu.Unlock()
}

func (ui *gioWorkerUI) layout(gtx layout.Context) layout.Dimensions {
	if ui.all.Clicked(gtx) {
		ui.setFilter(false)
	}
	if ui.stderr.Clicked(gtx) {
		ui.setFilter(true)
	}
	ui.mu.RLock()
	snapshot := ui.snapshot
	entries := append([]LogEntry(nil), ui.entries...)
	stderrOnly := ui.stderrOnly
	ui.mu.RUnlock()
	visibleEntries := make([]LogEntry, 0, len(entries))
	for _, entry := range entries {
		if !stderrOnly || entry.Stream == "stderr" {
			visibleEntries = append(visibleEntries, entry)
		}
	}

	header := func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Gap: gtx.Dp(unit.Dp(12))}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(unit.Dp(36)), gtx.Dp(unit.Dp(36))))
				return widget.Image{Src: ui.logo, Fit: widget.Contain}.Layout(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(unit.Dp(3))}.Layout(gtx,
					layout.Rigid(gioLabel(ui.theme, "Glyphflow Worker", 20, gioText, true)),
					layout.Rigid(gioLabel(ui.theme, "Runs in the system tray", 14, gioMuted, false)),
				)
			}),
		)
	}
	filters := func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Gap: gtx.Dp(unit.Dp(6))}.Layout(gtx,
			layout.Rigid(gioFilterButton(ui.theme, &ui.all, "All", !stderrOnly)),
			layout.Rigid(gioFilterButton(ui.theme, &ui.stderr, "Stderr", stderrOnly)),
		)
	}
	logHeader := func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceBetween}.Layout(gtx,
			layout.Rigid(gioLabel(ui.theme, "Logs", 14, gioMuted, false)),
			layout.Rigid(filters),
		)
	}

	return layout.Background{}.Layout(gtx, gioFill(gioBackground), func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(unit.Dp(24)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(unit.Dp(14))}.Layout(gtx,
				layout.Rigid(header),
				layout.Rigid(gioCard(gioInfo(ui.theme, "Runner ID", gioDisplayValue(snapshot.RunnerID)))),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Gap: gtx.Dp(unit.Dp(14))}.Layout(gtx,
						layout.Flexed(2, gioCard(gioInfo(ui.theme, "NATS JetStream endpoint", gioDisplayValue(snapshot.NATSEndpoint)))),
						layout.Flexed(1, gioCard(gioInfo(ui.theme, "Current executions", fmt.Sprintf("%d", snapshot.RunningExecutions)))),
						layout.Flexed(1, gioCard(gioInfo(ui.theme, "Parallel executions capacity", fmt.Sprintf("%d", snapshot.ParallelExecutions)))),
					)
				}),
				layout.Flexed(1, gioCard(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(logHeader),
						layout.Rigid(gioDivider),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return ui.logs.Layout(gtx, len(visibleEntries), func(gtx layout.Context, index int) layout.Dimensions {
								entry := visibleEntries[index]
								line := material.Label(ui.theme, 12, fmt.Sprintf("%s %s", entry.Timestamp, entry.Text))
								line.Font.Typeface = "monospace"
								line.WrapPolicy = text.WrapWords
								if entry.Stream == "stderr" {
									line.Color = gioStderr
								}
								return line.Layout(gtx)
							})
						}),
					)
				})),
			)
		})
	})
}

var (
	gioBackground = color.NRGBA{R: 243, G: 240, B: 255, A: 255}
	gioWhite      = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	gioText       = color.NRGBA{R: 50, G: 43, B: 69, A: 255}
	gioMuted      = color.NRGBA{R: 118, G: 110, B: 135, A: 255}
	gioBorder     = color.NRGBA{R: 222, G: 216, B: 237, A: 255}
	gioButtonLine = color.NRGBA{R: 201, G: 192, B: 222, A: 255}
	gioPrimary    = color.NRGBA{R: 118, G: 84, B: 214, A: 255}
	gioStderr     = color.NRGBA{R: 162, G: 39, B: 102, A: 255}
)

func gioLabel(th *material.Theme, value string, size unit.Sp, color color.NRGBA, bold bool) layout.Widget {
	label := material.Label(th, size, value)
	label.Color = color
	label.WrapPolicy = text.WrapWords
	label.Font.Weight = font.Normal
	if bold {
		label.Font.Weight = font.Bold
	}
	return label.Layout
}

func gioInfo(th *material.Theme, label, value string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(unit.Dp(8))}.Layout(gtx,
			layout.Rigid(gioLabel(th, label, 12, gioMuted, false)),
			layout.Rigid(gioLabel(th, value, 17, gioText, false)),
		)
	}
}

func gioCard(content layout.Widget) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return widget.Border{Color: gioBorder, CornerRadius: 12, Width: 1}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Background{}.Layout(gtx, gioFill(gioWhite), func(gtx layout.Context) layout.Dimensions {
				return layout.UniformInset(unit.Dp(16)).Layout(gtx, content)
			})
		})
	}
}

func gioFill(color color.NRGBA) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		size := gtx.Constraints.Min
		paint.FillShape(gtx.Ops, color, clip.UniformRRect(image.Rectangle{Max: size}, gtx.Dp(unit.Dp(12))).Op(gtx.Ops))
		return layout.Dimensions{Size: size}
	}
}

func gioDivider(gtx layout.Context) layout.Dimensions {
	size := image.Pt(gtx.Constraints.Max.X, gtx.Dp(unit.Dp(1)))
	paint.FillShape(gtx.Ops, gioBorder, clip.Rect{Max: size}.Op())
	return layout.Dimensions{Size: size}
}

func gioFilterButton(th *material.Theme, button *widget.Clickable, label string, active bool) layout.Widget {
	style := material.Button(th, button, label)
	style.CornerRadius = 6
	style.Inset = layout.Inset{Top: 6, Bottom: 6, Left: 10, Right: 10}
	if !active {
		style.Background = gioWhite
		style.Color = gioText
	}
	return func(gtx layout.Context) layout.Dimensions {
		return widget.Border{Color: func() color.NRGBA {
			if active {
				return gioPrimary
			}
			return gioButtonLine
		}(), CornerRadius: 6, Width: 1}.Layout(gtx, style.Layout)
	}
}

func updateGioStatus(ctx context.Context, logs *LogBuffer, ui *gioWorkerUI) {
	var after uint64
	entries := make([]LogEntry, 0, maxWorkerLogEntries)
	ticker := time.NewTicker(750 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			snapshot := logs.Snapshot(after)
			if snapshot.Reset {
				entries = append(entries[:0], snapshot.Entries...)
			} else {
				entries = append(entries, snapshot.Entries...)
			}
			if len(entries) > maxWorkerLogEntries {
				entries = entries[len(entries)-maxWorkerLogEntries:]
			}
			if len(snapshot.Entries) > 0 {
				after = snapshot.Entries[len(snapshot.Entries)-1].Sequence
			}
			systray.SetTooltip(trayTooltip(snapshot))
			ui.update(snapshot, entries)
		}
	}
}

func runGioWindow(window *app.Window, ui *gioWorkerUI, onDestroy func()) {
	var ops op.Ops
	for {
		event := window.Event()
		handleGioNativeEvent(event)
		switch event := event.(type) {
		case app.FrameEvent:
			gtx := app.NewContext(&ops, event)
			ui.layout(gtx)
			event.Frame(gtx.Ops)
		case app.DestroyEvent:
			onDestroy()
			return
		}
	}
}

func startGioTray(onReady func()) func() {
	go func() {
		runtime.LockOSThread()
		systray.Run(onReady, nil)
	}()
	return systray.Quit
}

func gioDisplayValue(value string) string {
	if value == "" {
		return "—"
	}
	return value
}

func renderGioLogs(entries []LogEntry, stderrOnly bool) string {
	var result strings.Builder
	for _, entry := range entries {
		if stderrOnly && entry.Stream != "stderr" {
			continue
		}
		fmt.Fprintf(&result, "%s %s\n", entry.Timestamp, entry.Text)
	}
	return result.String()
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var capacity atomic.Int64
	logs := NewLogBuffer(&capacity)
	stdout := logs.Writer("stdout", os.Stdout)
	stderr := logs.Writer("stderr", os.Stderr)
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		if err := runWorker(ctx, stdout, stderr, logs); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
		}
	}()

	window := new(app.Window)
	window.Option(app.Title("Glyphflow Worker"), app.Size(860, 560), app.MinSize(640, 420))
	ui := newGioWorkerUI(window)
	var trayStop func()
	var trayOnce sync.Once
	stopTray := func() {
		trayOnce.Do(func() {
			if trayStop != nil {
				trayStop()
			}
		})
	}
	var exitOnce sync.Once
	exit := func() {
		exitOnce.Do(func() {
			cancel()
			window.Perform(system.ActionClose)
		})
	}
	systray.SetOnTapped(func() { raiseGioWindow(window) })
	trayStop = startGioTray(func() {
		systray.SetIcon(gioTrayIcon)
		systray.SetTooltip(trayTooltip(Snapshot{}))
		open := systray.AddMenuItem("Open", "Show Glyphflow Worker")
		exitItem := systray.AddMenuItem("Exit", "Exit Glyphflow Worker")
		go func() {
			for {
				select {
				case <-open.ClickedCh:
					raiseGioWindow(window)
				case <-exitItem.ClickedCh:
					exit()
					return
				}
			}
		}()
	})

	go updateGioStatus(ctx, logs, ui)
	go runGioWindow(window, ui, func() {
		cancel()
		stopTray()
		<-workerDone
		os.Exit(0)
	})
	app.Main()
}
