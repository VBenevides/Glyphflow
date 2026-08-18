//go:build workerui && windows

package main

import (
	_ "embed"
	"syscall"
	"time"

	"gioui.org/app"
)

//go:embed assets/glyphflow.ico
var gioTrayIcon []byte

var (
	gioUser32        = syscall.NewLazyDLL("user32.dll")
	gioIsIconic      = gioUser32.NewProc("IsIconic")
	gioShowWindow    = gioUser32.NewProc("ShowWindow")
	gioWindowStarted bool
)

func handleGioNativeEvent(raw any) {
	event, ok := raw.(app.Win32ViewEvent)
	if !ok || event.HWND == 0 || gioWindowStarted {
		return
	}
	gioWindowStarted = true
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
