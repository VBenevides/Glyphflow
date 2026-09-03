//go:build workerui_tui && !workerui && !workerui_fyne && !workerui_gio && !windows

package main

// startTUITray has no platform tray implementation outside Windows.
func startTUITray(func()) func() {
	return func() {}
}

// setTUITrayTooltip has no platform tray implementation outside Windows.
func setTUITrayTooltip(Snapshot) {}
