//go:build windows

package worker

import (
	"os/exec"
	"testing"
)

func TestConfigureCommandHidesWindowsWindow(t *testing.T) {
	command := exec.Command("cmd.exe")
	configureCommand(command)
	if command.SysProcAttr == nil || !command.SysProcAttr.HideWindow {
		t.Fatal("Windows command window is not hidden")
	}
}
