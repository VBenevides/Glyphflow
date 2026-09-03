//go:build workerui_tui && !workerui && !workerui_fyne && !workerui_gio && !windows

package main

// startTUITray has no platform tray implementation outside Windows.
func startTUITray(func()) func() {
	return func() {
		// TUI builds outside Windows have no tray lifecycle to stop.
	}
}

// setTUITrayTooltip has no platform tray implementation outside Windows.
func setTUITrayTooltip(Snapshot) {
	// TUI builds outside Windows have no tray tooltip.
}
