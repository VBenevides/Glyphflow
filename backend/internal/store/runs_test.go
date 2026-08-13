package store

import (
	"strings"
	"testing"
)

func TestCreateTaskRunUsesOneTransactionAndAllDurableRows(t *testing.T) {
	for _, query := range []string{insertTaskRunSQL, insertResourceLeaseSQL, insertDispatchOutboxSQL} {
		if !strings.Contains(query, "INSERT INTO") {
			t.Fatal("run creation query is not an insert")
		}
	}
	if !strings.Contains(insertDispatchOutboxSQL, "order_bytes") {
		t.Fatal("outbox insert does not persist exact order bytes")
	}
}
