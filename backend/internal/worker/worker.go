package worker

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"path/filepath"

	"github.com/VBenevides/Glyphflow/backend/internal/platform"
)

var ErrOutputLimit = errors.New("command output exceeds configured limit")

type Executor struct {
	Roots           []string
	AllowedCommands map[string]bool
	MaxOutputBytes  int
}

func (e Executor) Run(ctx context.Context, args []string, dir string) ([]byte, error) {
	if len(args) == 0 {
		return nil, &ValidationError{"command is required"}
	}
	if _, ok := ctx.Deadline(); !ok {
		return nil, &ValidationError{"execution deadline is required"}
	}
	if e.MaxOutputBytes <= 0 {
		return nil, &ValidationError{"maximum output bytes must be greater than zero"}
	}
	if len(e.AllowedCommands) > 0 && !e.AllowedCommands[args[0]] {
		return nil, &ValidationError{"executable is not allowed"}
	}
	clean, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	allowed := false
	for _, root := range e.Roots {
		if platform.AllowedPath(root, clean) {
			allowed = true
			break
		}
	}
	if !allowed {
		return nil, &ValidationError{"working directory is outside configured roots"}
	}
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	configureCommand(cmd)
	var output boundedBuffer
	output.limit = e.MaxOutputBytes
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Run(); err != nil {
		if output.exceeded {
			return output.Bytes(), ErrOutputLimit
		}
		return output.Bytes(), err
	}
	return output.Bytes(), nil
}

type boundedBuffer struct {
	data     []byte
	limit    int
	exceeded bool
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	remaining := b.limit - len(b.data)
	if remaining <= 0 {
		b.exceeded = true
		return 0, ErrOutputLimit
	}
	if len(p) > remaining {
		b.data = append(b.data, p[:remaining]...)
		b.exceeded = true
		return remaining, io.ErrShortWrite
	}
	b.data = append(b.data, p...)
	return len(p), nil
}

func (b *boundedBuffer) Bytes() []byte { return b.data }

type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }
