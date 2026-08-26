package worker

import (
	"database/sql"
	"testing"
)

func TestLocalStoreCreatesDurableInboxAndOutbox(t *testing.T) {
	store, err := OpenStore(t.TempDir() + "/runner.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	for _, table := range []string{"order_inbox", "event_outbox"} {
		var count int
		if err := store.db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("missing table %s", table)
		}
	}
	var indexCount int
	if err := store.db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type = 'index' AND name = 'event_outbox_pending_idx'").Scan(&indexCount); err != nil {
		t.Fatal(err)
	}
	if indexCount != 1 {
		t.Fatal("missing pending event outbox index")
	}
	var _ *sql.DB
}
