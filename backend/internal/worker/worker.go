package worker

import (
	"context"
	"os/exec"
	"path/filepath"
)

type Executor struct{ Roots []string }

func (e Executor) Run(ctx context.Context, args []string, dir string) ([]byte, error) {
	if len(args) == 0 {
		return nil, &ValidationError{"command is required"}
	}
	clean, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	allowed := false
	for _, root := range e.Roots {
		base, _ := filepath.Abs(root)
		if clean == base || len(clean) > len(base) && clean[:len(base)] == base && clean[len(base)] == filepath.Separator {
			allowed = true
			break
		}
	}
	if !allowed {
		return nil, &ValidationError{"working directory is outside configured roots"}
	}
	return exec.CommandContext(ctx, args[0], args[1:]...).CombinedOutput()
}

type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }
