//go:build workerui && windows

package main

import (
	_ "embed"
	"sync/atomic"
	"syscall"
	"time"

	"gioui.org/app"
	"gioui.org/io/system"
)

//go:embed assets/glyphflow.ico
var gioTrayIcon []byte

var (
	gioUser32        = syscall.NewLazyDLL("user32.dll")
	gioIsIconic      = gioUser32.NewProc("IsIconic")
	gioShowWindow    = gioUser32.NewProc("ShowWindow")
	gioSetForeground = gioUser32.NewProc("SetForegroundWindow")
	gioWindowStarted bool
	gioWindowHWND    atomic.Uintptr
)

func handleGioNativeEvent(raw any) {
	event, ok := raw.(app.Win32ViewEvent)
	if !ok || event.HWND == 0 || gioWindowStarted {
		return
	}
	gioWindowStarted = true
	gioWindowHWND.Store(event.HWND)
	go func(hwnd uintptr) {
		ticker := time.NewTicker(150 * time.Millisecond)
		defer ticker.Stop()
		for range ticker.C {
			minimized, _, _ := gioIsIconic.Call(hwnd)
			if minimized != 0 {
				gioShowWindow.Call(hwnd, 0)
				return
			}
		}
	}(event.HWND)
}

func raiseGioWindow(window *app.Window) {
	if hwnd := gioWindowHWND.Load(); hwnd != 0 {
		gioShowWindow.Call(hwnd, 9) // SW_RESTORE
		gioSetForeground.Call(hwnd)
	}
	window.Perform(system.ActionRaise)
}
