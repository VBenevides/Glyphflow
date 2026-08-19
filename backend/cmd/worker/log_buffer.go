package main

import (
	"fmt"
	"io"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const maxWorkerLogEntries = 500

// ponytail: retain only 500 lines; use a persistent log store if operators later need longer history.
type LogEntry struct {
	Sequence  uint64 `json:"sequence"`
	Timestamp string `json:"timestamp"`
	Stream    string `json:"stream"`
	Text      string `json:"text"`
}

type Snapshot struct {
	RunnerID           string     `json:"runnerId"`
	NATSEndpoint       string     `json:"natsEndpoint"`
	ParallelExecutions int64      `json:"parallelExecutions"`
	RunningExecutions  int64      `json:"runningExecutions"`
	Entries            []LogEntry `json:"entries"`
	Reset              bool       `json:"reset"`
}

type StatusSink interface {
	SetRunnerID(string)
	SetNATSEndpoint(string)
	SetCapacitySource(*atomic.Int64)
	SetRunningSource(func() int64)
}

type LogBuffer struct {
	mu       sync.Mutex
	entries  []LogEntry
	next     uint64
	runnerID string
	endpoint string
	capacity *atomic.Int64
	running  func() int64
	partial  map[string]string
}

func (b *LogBuffer) SetRunnerID(runnerID string) {
	b.mu.Lock()
	b.runnerID = runnerID
	b.mu.Unlock()
}

func NewLogBuffer(capacity *atomic.Int64) *LogBuffer {
	return &LogBuffer{capacity: capacity, partial: map[string]string{"stdout": "", "stderr": ""}}
}

func (b *LogBuffer) SetNATSEndpoint(endpoint string) {
	b.mu.Lock()
	b.endpoint = endpoint
	b.mu.Unlock()
}

func (b *LogBuffer) SetCapacitySource(capacity *atomic.Int64) {
	b.mu.Lock()
	b.capacity = capacity
	b.mu.Unlock()
}

func (b *LogBuffer) SetRunningSource(running func() int64) {
	b.mu.Lock()
	b.running = running
	b.mu.Unlock()
}

func (b *LogBuffer) SetParallelExecutions(value int64) {
	b.mu.Lock()
	if b.capacity == nil {
		b.capacity = &atomic.Int64{}
	}
	b.capacity.Store(value)
	b.mu.Unlock()
}

func (b *LogBuffer) Snapshot(after uint64) Snapshot {
	b.mu.Lock()
	defer b.mu.Unlock()
	var capacity int64
	if b.capacity != nil {
		capacity = b.capacity.Load()
	}
	running := int64(0)
	if b.running != nil {
		running = b.running()
	}
	result := Snapshot{RunnerID: b.runnerID, NATSEndpoint: b.endpoint, ParallelExecutions: capacity, RunningExecutions: running}
	if len(b.entries) == 0 {
		return result
	}
	oldest := b.entries[0].Sequence
	if after < oldest-1 {
		result.Reset = true
	}
	for _, entry := range b.entries {
		if entry.Sequence > after {
			result.Entries = append(result.Entries, entry)
		}
	}
	return result
}

func trayTooltip(snapshot Snapshot) string {
	return fmt.Sprintf("Capacity: %d/%d", snapshot.RunningExecutions, snapshot.ParallelExecutions)
}

func (b *LogBuffer) Writer(stream string, mirror io.Writer) io.Writer {
	if stream != "stdout" && stream != "stderr" {
		return io.Discard
	}
	return &logWriter{buffer: b, stream: stream, mirror: mirror}
}

type logWriter struct {
	buffer *LogBuffer
	stream string
	mirror io.Writer
}

func (w *logWriter) Write(data []byte) (int, error) {
	if w.mirror != nil {
		_, _ = w.mirror.Write(data)
	}
	w.buffer.mu.Lock()
	defer w.buffer.mu.Unlock()
	pending := w.buffer.partial[w.stream] + string(data)
	for {
		lineEnd := strings.IndexByte(pending, '\n')
		if lineEnd < 0 {
			break
		}
		line := pending[:lineEnd]
		if strings.HasSuffix(line, "\r") {
			line = strings.TrimSuffix(line, "\r")
		}
		w.buffer.appendLocked(w.stream, line)
		pending = pending[lineEnd+1:]
	}
	w.buffer.partial[w.stream] = pending
	return len(data), nil
}

func (b *LogBuffer) appendLocked(stream, text string) {
	b.next++
	b.entries = append(b.entries, LogEntry{
		Sequence:  b.next,
		Timestamp: time.Now().UTC().Format("2006-01-02T15:04:05.000000000Z07:00"),
		Stream:    stream,
		Text:      text,
	})
	if len(b.entries) > maxWorkerLogEntries {
		b.entries = b.entries[len(b.entries)-maxWorkerLogEntries:]
	}
}

func redactNATSEndpoint(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "<invalid NATS endpoint>", err
	}
	return parsed.Redacted(), nil
}
