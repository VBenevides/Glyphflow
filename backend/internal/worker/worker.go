package worker

import (
	"context"
	"errors"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/platform"
	"github.com/shirou/gopsutil/v4/process"
)

var ErrOutputLimit = errors.New("command output exceeds configured limit")

type Executor struct {
	Roots           []string
	AllowedCommands map[string]bool
	MaxOutputBytes  int
	Environment     map[string]string
	Metrics         *MemoryStats
}

type MemoryStats struct {
	MaxBytes     uint64
	AverageBytes uint64
	samples      uint64
	average      float64
}

func (m *MemoryStats) Sample(pid int32) {
	if m == nil {
		return
	}
	processInfo, err := process.NewProcess(pid)
	if err != nil {
		return
	}
	memory, err := processInfo.MemoryInfo()
	if err != nil {
		return
	}
	if memory.RSS > m.MaxBytes {
		m.MaxBytes = memory.RSS
	}
	m.samples++
	m.average += (float64(memory.RSS) - m.average) / float64(m.samples)
	m.AverageBytes = uint64(math.Round(m.average))
}

func (e Executor) Run(ctx context.Context, args []string, dir string) ([]byte, error) {
	output, _, err := e.RunStreamingWithExitCode(ctx, args, dir, 0, nil)
	return output, err
}

func (e Executor) RunStreaming(ctx context.Context, args []string, dir string, flushInterval time.Duration, onChunk func(string, []byte) error) ([]byte, error) {
	output, _, err := e.RunStreamingWithExitCode(ctx, args, dir, flushInterval, onChunk)
	return output, err
}

func (e Executor) RunStreamingWithExitCode(ctx context.Context, args []string, dir string, flushInterval time.Duration, onChunk func(string, []byte) error) ([]byte, *int, error) {
	if err := e.validateExecution(ctx, args, dir); err != nil {
		return nil, nil, err
	}
	clean, err := filepath.Abs(dir)
	if err != nil {
		return nil, nil, err
	}
	return e.runCommand(ctx, args, clean, flushInterval, onChunk)
}

func (e Executor) validateExecution(ctx context.Context, args []string, dir string) error {
	if len(args) == 0 {
		return &ValidationError{"command is required"}
	}
	if _, ok := ctx.Deadline(); !ok {
		return &ValidationError{"execution deadline is required"}
	}
	if e.MaxOutputBytes <= 0 {
		return &ValidationError{"maximum output bytes must be greater than zero"}
	}
	if len(e.AllowedCommands) > 0 && !e.AllowedCommands[args[0]] {
		return &ValidationError{"executable is not allowed"}
	}
	clean, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	allowed := false
	for _, root := range e.Roots {
		if platform.AllowedPath(root, clean) {
			allowed = true
			break
		}
	}
	if !allowed {
		return &ValidationError{"working directory is outside configured roots"}
	}
	_ = clean
	return nil
}

func (e Executor) runCommand(ctx context.Context, args []string, clean string, flushInterval time.Duration, onChunk func(string, []byte) error) ([]byte, *int, error) {
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(runCtx, args[0], args[1:]...)
	cmd.Dir = clean
	applyCommandEnvironment(cmd, e.Environment)
	configureCommand(cmd)
	chunks := make(chan executorOutput, 32)
	stopped := &atomic.Bool{}
	cmd.Stdout = executorStreamWriter{stream: "stdout", chunks: chunks, stopped: stopped}
	cmd.Stderr = executorStreamWriter{stream: "stderr", chunks: chunks, stopped: stopped}
	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}
	wait := make(chan error, 1)
	go func() { wait <- cmd.Wait() }()
	var memoryTicker *time.Ticker
	var memoryTick <-chan time.Time
	if e.Metrics != nil {
		e.Metrics.Sample(int32(cmd.Process.Pid))
		memoryTicker = time.NewTicker(time.Second)
		defer memoryTicker.Stop()
		memoryTick = memoryTicker.C
	}

	var output boundedBuffer
	output.limit = e.MaxOutputBytes
	buffers := map[string][]byte{"stdout": nil, "stderr": nil}
	var callbackErr error

	var ticker *time.Ticker
	var tick <-chan time.Time
	if flushInterval > 0 {
		ticker = time.NewTicker(flushInterval)
		defer ticker.Stop()
		tick = ticker.C
	}
	for {
		select {
		case chunk := <-chunks:
			processExecutorChunk(chunk, &output, buffers, stopped, cancel)
		case <-tick:
			flushExecutorBuffers(buffers, &callbackErr, stopped, cancel, onChunk)
		case <-memoryTick:
			e.Metrics.Sample(int32(cmd.Process.Pid))
		case err := <-wait:
			if e.Metrics != nil {
				e.Metrics.Sample(int32(cmd.Process.Pid))
			}
			var exitCode *int
			if cmd.ProcessState != nil {
				code := cmd.ProcessState.ExitCode()
				exitCode = &code
			}
			return drainExecutorChunks(executorDrainState{
				chunks:      chunks,
				output:      &output,
				buffers:     buffers,
				stopped:     stopped,
				cancel:      cancel,
				callbackErr: &callbackErr,
				onChunk:     onChunk,
				exitCode:    exitCode,
				waitErr:     err,
			})
		}
	}
}

func applyCommandEnvironment(cmd *exec.Cmd, values map[string]string) {
	if len(values) == 0 {
		return
	}
	environment := os.Environ()
	positions := make(map[string]int, len(environment))
	for index, entry := range environment {
		if split := strings.IndexByte(entry, '='); split > 0 {
			positions[entry[:split]] = index
		}
	}
	for name, value := range values {
		entry := name + "=" + value
		if index, ok := positions[name]; ok {
			environment[index] = entry
		} else {
			environment = append(environment, entry)
		}
	}
	cmd.Env = environment
}

func flushExecutorBuffer(buffers map[string][]byte, stream string, callbackErr *error, stopped *atomic.Bool, cancel context.CancelFunc, onChunk func(string, []byte) error) {
	if *callbackErr != nil || len(buffers[stream]) == 0 {
		return
	}
	chunk := append([]byte(nil), buffers[stream]...)
	buffers[stream] = nil
	if onChunk == nil {
		return
	}
	if err := onChunk(stream, chunk); err != nil {
		*callbackErr = err
		stopped.Store(true)
		cancel()
	}
}

func flushExecutorBuffers(buffers map[string][]byte, callbackErr *error, stopped *atomic.Bool, cancel context.CancelFunc, onChunk func(string, []byte) error) {
	flushExecutorBuffer(buffers, "stdout", callbackErr, stopped, cancel, onChunk)
	flushExecutorBuffer(buffers, "stderr", callbackErr, stopped, cancel, onChunk)
}

func processExecutorChunk(chunk executorOutput, output *boundedBuffer, buffers map[string][]byte, stopped *atomic.Bool, cancel context.CancelFunc) {
	if stopped.Load() {
		return
	}
	remaining := output.limit - len(output.data)
	if remaining <= 0 {
		output.exceeded = true
		stopped.Store(true)
		cancel()
		return
	}
	accepted := chunk.data
	if len(accepted) > remaining {
		accepted = accepted[:remaining]
		output.exceeded = true
		stopped.Store(true)
		cancel()
	}
	output.data = append(output.data, accepted...)
	buffers[chunk.stream] = append(buffers[chunk.stream], accepted...)
}

type executorDrainState struct {
	chunks      <-chan executorOutput
	output      *boundedBuffer
	buffers     map[string][]byte
	stopped     *atomic.Bool
	cancel      context.CancelFunc
	callbackErr *error
	onChunk     func(string, []byte) error
	exitCode    *int
	waitErr     error
}

func drainExecutorChunks(state executorDrainState) ([]byte, *int, error) {
	for {
		select {
		case chunk := <-state.chunks:
			processExecutorChunk(chunk, state.output, state.buffers, state.stopped, state.cancel)
		default:
			flushExecutorBuffers(state.buffers, state.callbackErr, state.stopped, state.cancel, state.onChunk)
			if *state.callbackErr != nil {
				return state.output.Bytes(), state.exitCode, *state.callbackErr
			}
			if state.output.exceeded {
				return state.output.Bytes(), state.exitCode, ErrOutputLimit
			}
			return state.output.Bytes(), state.exitCode, state.waitErr
		}
	}
}

type executorOutput struct {
	stream string
	data   []byte
}

type executorStreamWriter struct {
	stream  string
	chunks  chan<- executorOutput
	stopped *atomic.Bool
}

func (w executorStreamWriter) Write(p []byte) (int, error) {
	if w.stopped.Load() {
		return 0, ErrOutputLimit
	}
	w.chunks <- executorOutput{stream: w.stream, data: append([]byte(nil), p...)}
	return len(p), nil
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
