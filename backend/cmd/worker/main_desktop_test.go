//go:build workerui

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestSnapshotHandler(t *testing.T) {
	var capacity atomic.Int64
	capacity.Store(4)
	logs := NewLogBuffer(&capacity)
	logs.SetNATSEndpoint("nats://example.test:4222")
	_, _ = logs.Writer("stderr", nil).Write([]byte("warning\n"))

	req := httptest.NewRequest(http.MethodGet, "/api/snapshot?after=0", nil)
	recorder := httptest.NewRecorder()
	snapshotHandler(logs).ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	var snapshot Snapshot
	if err := json.NewDecoder(recorder.Body).Decode(&snapshot); err != nil {
		t.Fatalf("decode snapshot: %v", err)
	}
	if snapshot.NATSEndpoint != "nats://example.test:4222" || snapshot.ParallelExecutions != 4 || len(snapshot.Entries) != 1 {
		t.Fatalf("snapshot = %#v", snapshot)
	}

	bad := httptest.NewRecorder()
	snapshotHandler(logs).ServeHTTP(bad, httptest.NewRequest(http.MethodGet, "/api/snapshot?after=bad", nil))
	if bad.Code != http.StatusBadRequest || !strings.Contains(bad.Body.String(), "non-negative integer") {
		t.Fatalf("invalid sequence response = %d %q", bad.Code, bad.Body.String())
	}

	method := httptest.NewRecorder()
	snapshotHandler(logs).ServeHTTP(method, httptest.NewRequest(http.MethodPost, "/api/snapshot", nil))
	if method.Code != http.StatusMethodNotAllowed {
		t.Fatalf("method status = %d, want 405", method.Code)
	}
}
