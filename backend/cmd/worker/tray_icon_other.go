//go:build workerui && !windows

package main

import (
	"gioui.org/app"
	"gioui.org/io/system"
)

var gioTrayIcon = gioWorkerIcon

func handleGioNativeEvent(any) {}

func raiseGioWindow(window *app.Window) {
	window.Perform(system.ActionRaise)
}
