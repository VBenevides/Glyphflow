package main

import (
	"testing"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/protocol"
	"github.com/VBenevides/Glyphflow/backend/internal/worker"
)

func TestNeedsRunnerEnrollmentForLegacyStore(t *testing.T) {
	bootstrap := &worker.Bootstrap{RunnerID: "runner-1"}
	validKey, err := protocol.GenerateSigningKey("runner:runner-1", time.Now().UTC(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	expiredKey := validKey
	expiredKey.Public.NotAfter = time.Now().UTC().Add(-time.Hour)
	for _, test := range []struct {
		name                      string
		connectionFound, keyFound bool
		key                       protocol.SigningKey
		want                      bool
	}{
		{name: "new store", want: true},
		{name: "legacy store without key", connectionFound: true, want: true},
		{name: "changed key identity", connectionFound: true, keyFound: true, key: protocol.SigningKey{ID: "runner:old"}, want: true},
		{name: "expired key", connectionFound: true, keyFound: true, key: expiredKey, want: true},
		{name: "enrolled store", connectionFound: true, keyFound: true, key: validKey, want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := needsRunnerEnrollment(bootstrap, test.connectionFound, test.keyFound, test.key); got != test.want {
				t.Fatalf("needsRunnerEnrollment() = %t, want %t", got, test.want)
			}
		})
	}
}
