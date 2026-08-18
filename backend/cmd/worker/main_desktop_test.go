//go:build workerui

package main

import "testing"

func TestRenderGioLogsFilter(t *testing.T) {
	entries := []LogEntry{
		{Timestamp: "t1", Stream: "stdout", Text: "out"},
		{Timestamp: "t2", Stream: "stderr", Text: "err"},
	}
	if got := renderGioLogs(entries, true); got != "t2 err\n" {
		t.Fatalf("stderr log rendering = %q", got)
	}
}
