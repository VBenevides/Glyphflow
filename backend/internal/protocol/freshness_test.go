package protocol

import (
	"testing"
	"time"
)

func TestOrderFreshnessBindsSessionLeaseFenceDigestAndRecipient(t *testing.T) {
	now := time.Now()
	order := OrderPayload{RunnerID: "runner", RunID: "run", Attempt: 1, LeaseToken: "lease", RunnerSessionID: "session", Recipient: "runner", FencingToken: 2, LeaseNotAfter: now.Add(time.Minute), ExecutionSpecDigest: "digest"}
	ctx := FreshnessContext{RunnerID: "runner", SessionID: "session", RunID: "run", Attempt: 1, LeaseToken: "lease", Recipient: "runner", FencingToken: 2, LeaseNotAfter: order.LeaseNotAfter, ExecutionSpecDigest: "digest"}
	if err := order.ValidateFreshness(ctx, now); err != nil {
		t.Fatal(err)
	}
	ctx.SessionID = "old-session"
	if err := order.ValidateFreshness(ctx, now); err == nil {
		t.Fatal("stale session accepted")
	}
}
