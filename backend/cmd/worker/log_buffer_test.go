package main

import (
	"bytes"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestLogBufferClassifiesStreamsAndJoinsLines(t *testing.T) {
	buffer := NewLogBuffer(nil)
	stdout := buffer.Writer("stdout", nil)
	stderr := buffer.Writer("stderr", nil)
	_, _ = stdout.Write([]byte("first"))
	_, _ = stderr.Write([]byte("warning\r"))
	_, _ = stdout.Write([]byte(" line\n\n"))
	_, _ = stderr.Write([]byte("\n"))

	snapshot := buffer.Snapshot(0)
	if len(snapshot.Entries) != 3 {
		t.Fatalf("entries = %d, want 3", len(snapshot.Entries))
	}
	want := []struct{ stream, text string }{{"stdout", "first line"}, {"stdout", ""}, {"stderr", "warning"}}
	for i, item := range want {
		got := snapshot.Entries[i]
		if got.Stream != item.stream || got.Text != item.text {
			t.Fatalf("entry[%d] = %#v, want %q/%q", i, got, item.stream, item.text)
		}
		if !strings.Contains(got.Timestamp, "T") || !strings.Contains(got.Timestamp, ".") {
			t.Fatalf("timestamp %q is not fractional RFC3339", got.Timestamp)
		}
	}
	if snapshot.Entries[0].Sequence >= snapshot.Entries[1].Sequence || snapshot.Entries[1].Sequence >= snapshot.Entries[2].Sequence {
		t.Fatal("sequences are not chronological")
	}
}

func TestLogBufferBoundsHistoryAndRequestsReset(t *testing.T) {
	buffer := NewLogBuffer(nil)
	writer := buffer.Writer("stdout", nil)
	for i := 0; i < maxWorkerLogEntries+1; i++ {
		_, _ = writer.Write([]byte(fmt.Sprintf("line-%d\n", i)))
	}
	snapshot := buffer.Snapshot(0)
	if !snapshot.Reset || len(snapshot.Entries) != maxWorkerLogEntries {
		t.Fatalf("snapshot reset=%t entries=%d, want true/%d", snapshot.Reset, len(snapshot.Entries), maxWorkerLogEntries)
	}
	if snapshot.Entries[0].Text != "line-1" || snapshot.Entries[len(snapshot.Entries)-1].Text != fmt.Sprintf("line-%d", maxWorkerLogEntries) {
		t.Fatalf("retained range = %q..%q", snapshot.Entries[0].Text, snapshot.Entries[len(snapshot.Entries)-1].Text)
	}
	latest := snapshot.Entries[len(snapshot.Entries)-1].Sequence
	if got := buffer.Snapshot(latest); len(got.Entries) != 0 || got.Reset {
		t.Fatalf("latest snapshot = %#v", got)
	}
}

func TestLogBufferCapacitySourceIsLive(t *testing.T) {
	var capacity atomic.Int64
	capacity.Store(2)
	buffer := NewLogBuffer(&capacity)
	if got := buffer.Snapshot(0).ParallelExecutions; got != 2 {
		t.Fatalf("capacity = %d, want 2", got)
	}
	capacity.Store(7)
	if got := buffer.Snapshot(0).ParallelExecutions; got != 7 {
		t.Fatalf("updated capacity = %d, want 7", got)
	}
}

func TestLogBufferRunningSourceIsLive(t *testing.T) {
	var running atomic.Int64
	running.Store(2)
	buffer := NewLogBuffer(nil)
	buffer.SetRunningSource(running.Load)
	if got := buffer.Snapshot(0).RunningExecutions; got != 2 {
		t.Fatalf("running = %d, want 2", got)
	}
	running.Store(1)
	if got := buffer.Snapshot(0).RunningExecutions; got != 1 {
		t.Fatalf("updated running = %d, want 1", got)
	}
}

func TestLogBufferConcurrentWriters(t *testing.T) {
	buffer := NewLogBuffer(nil)
	var group sync.WaitGroup
	for i := 0; i < 16; i++ {
		group.Add(1)
		stream := "stderr"
		if i%2 == 0 {
			stream = "stdout"
		}
		go func(stream string) {
			defer group.Done()
			writer := buffer.Writer(stream, nil)
			for j := 0; j < 100; j++ {
				_, _ = writer.Write([]byte("line\n"))
			}
		}(stream)
	}
	group.Wait()
	if got := len(buffer.Snapshot(0).Entries); got != maxWorkerLogEntries {
		t.Fatalf("entries = %d, want %d", got, maxWorkerLogEntries)
	}
}

func TestRedactNATSEndpoint(t *testing.T) {
	got, err := redactNATSEndpoint("nats://runner:secret@example.test:4222")
	if err != nil || got != "nats://runner:xxxxx@example.test:4222" {
		t.Fatalf("redacted endpoint = %q, err=%v", got, err)
	}
	if _, err := url.Parse(got); err != nil {
		t.Fatalf("redacted endpoint is invalid: %v", err)
	}
	got, err = redactNATSEndpoint("://bad")
	if err == nil || got != "<invalid NATS endpoint>" {
		t.Fatalf("invalid endpoint = %q, err=%v", got, err)
	}
}

func TestLogWriterMirrorsWithoutChangingBuffer(t *testing.T) {
	var mirror bytes.Buffer
	buffer := NewLogBuffer(nil)
	_, _ = buffer.Writer("stderr", &mirror).Write([]byte("error\n"))
	if mirror.String() != "error\n" || buffer.Snapshot(0).Entries[0].Text != "error" {
		t.Fatalf("mirror=%q snapshot=%#v", mirror.String(), buffer.Snapshot(0))
	}
}
