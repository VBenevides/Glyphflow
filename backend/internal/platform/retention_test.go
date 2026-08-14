package platform

import (
	"testing"
	"time"
)

func TestRetentionWorkerPurgesBoundedNonAuditRows(t *testing.T) {
	now := time.Now()
	w := NewRetentionWorker()
	w.Add(RetentionItem{Kind: "session", ID: "s", At: now.Add(-time.Hour)})
	w.Add(RetentionItem{Kind: "audit", ID: "a", At: now.Add(-time.Hour)})
	w.Add(RetentionItem{Kind: "inbox", ID: "i", At: now})
	deleted := w.Run(now, time.Minute, 10)
	if len(deleted) != 1 || deleted[0].ID != "s" {
		t.Fatalf("unexpected retention result: %#v", deleted)
	}
}
