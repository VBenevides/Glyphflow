//go:build workerui && !windows

package main

import (
	"gioui.org/app"
	"gioui.org/io/system"
)

var gioTrayIcon = gioWorkerIcon

// handleGioNativeEvent is only needed for Windows window-handle integration.
func handleGioNativeEvent(any) {
	// Non-Windows builds do not expose a native window handle.
}

func raiseGioWindow(window *app.Window) {
	window.Perform(system.ActionRaise)
}
