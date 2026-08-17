//go:build windows

package worker

import "os/exec"

func configureCommand(cmd *exec.Cmd) {
	cmd.Cancel = func() error { return cmd.Process.Kill() }
}
