//go:build workerui_tui && !workerui && !workerui_fyne && !workerui_gio && windows

package main

import (
	_ "embed"
	"runtime"
	"sync"
	"syscall"
	"time"

	"fyne.io/systray"
)

//go:embed assets/glyphflow.ico
var tuiTrayIcon []byte

var (
	tuiKernel32         = syscall.NewLazyDLL("kernel32.dll")
	tuiGetConsoleWindow = tuiKernel32.NewProc("GetConsoleWindow")
	tuiUser32           = syscall.NewLazyDLL("user32.dll")
	tuiIsIconic         = tuiUser32.NewProc("IsIconic")
	tuiSetForeground    = tuiUser32.NewProc("SetForegroundWindow")
	tuiShowWindow       = tuiUser32.NewProc("ShowWindow")
)

func startTUITray(onExit func()) func() {
	hwnd, _, _ := tuiGetConsoleWindow.Call()
	systray.SetOnTapped(func() { showTUIConsole(hwnd) })
	go func() {
		runtime.LockOSThread()
		systray.Run(func() {
			systray.SetIcon(tuiTrayIcon)
			systray.SetTooltip(trayTooltip(Snapshot{}))
			open := systray.AddMenuItem("Open", "Show Glyphflow Worker")
			exit := systray.AddMenuItem("Exit", "Exit Glyphflow Worker")
			go func() {
				for {
					select {
					case <-open.ClickedCh:
						showTUIConsole(hwnd)
					case <-exit.ClickedCh:
						onExit()
						return
					}
				}
			}()
		}, nil)
	}()

	stop := make(chan struct{})
	go watchTUIConsole(hwnd, stop)
	var once sync.Once
	return func() {
		once.Do(func() {
			close(stop)
			systray.Quit()
		})
	}
}

func setTUITrayTooltip(snapshot Snapshot) {
	systray.SetTooltip(trayTooltip(snapshot))
}

func showTUIConsole(hwnd uintptr) {
	if hwnd == 0 {
		hwnd, _, _ = tuiGetConsoleWindow.Call()
	}
	if hwnd == 0 {
		return
	}
	tuiShowWindow.Call(hwnd, 9) // SW_RESTORE
	tuiSetForeground.Call(hwnd)
}

func watchTUIConsole(hwnd uintptr, stop <-chan struct{}) {
	if hwnd == 0 {
		return
	}
	ticker := time.NewTicker(150 * time.Millisecond)
	defer ticker.Stop()
	hidden := false
	for {
		select {
		case <-ticker.C:
			minimized, _, _ := tuiIsIconic.Call(hwnd)
			if minimized != 0 && !hidden {
				tuiShowWindow.Call(hwnd, 0) // SW_HIDE
				hidden = true
			} else if minimized == 0 {
				hidden = false
			}
		case <-stop:
			return
		}
	}
}
