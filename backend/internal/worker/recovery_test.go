package worker

import (
	"testing"
	"time"
)

func TestOrderRecoveryMarksOlderBootOrdersUnknown(t *testing.T) {
	recovery := NewOrderRecovery("boot-2")
	if err := recovery.Claim("order-1"); err != nil {
		t.Fatal(err)
	}
	recovery.orders["order-old"] = "boot-1"
	unknown := recovery.Recover("boot-1")
	if len(unknown) != 1 || unknown[0] != "order-old" {
		t.Fatalf("unexpected recovery result: %#v", unknown)
	}
}

func TestDurableRecoveryMarksSQLiteClaimsUnknown(t *testing.T) {
	store, err := OpenStore(t.TempDir() + "/runner.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.PutOrder(InboxOrder{OrderID: "o", ExecutionAttemptID: "a", RunID: "r", TaskVersionID: "t", RunnerID: "runner", RunnerSessionID: "session", Envelope: "order", LeaseToken: "lease", LeaseNotAfter: time.Now().Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if err := store.ClaimOrder("o", "old-boot", 1); err != nil {
		t.Fatal(err)
	}
	ids, err := RecoverDurable(store, "old-boot")
	if err != nil || len(ids) != 1 {
		t.Fatalf("durable recovery: %#v %v", ids, err)
	}
	events, err := store.PendingEvents(10)
	if err != nil || len(events) != 1 || events[0].EventType != "unknown" || events[0].State != "PENDING" {
		t.Fatalf("durable recovery event: %#v %v", events, err)
	}
}
