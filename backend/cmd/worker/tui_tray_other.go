//go:build workerui_tui && !workerui && !workerui_fyne && !workerui_gio && !windows

package main

func startTUITray(func()) func() {
	return func() {}
}
