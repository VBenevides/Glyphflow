package protocol

import (
	"testing"
	"time"
)

func TestValidationEdgeBranches(t *testing.T) {
	now := time.Now().UTC()
	valid := OrderPayload{OrderID: "order", RunID: "run", RunnerID: "runner", RunnerSessionID: "session", LeaseToken: "lease", Attempt: 1, WorkingDir: ".", DurationSeconds: 1, Command: []string{"true"}, IssuedAt: now, NotBefore: now, ExpiresAt: now.Add(time.Minute), FencingToken: 1, ExecutionSpecDigest: "digest", Recipient: "runner", LeaseNotAfter: now.Add(time.Minute)}
	for _, order := range []OrderPayload{{}, {OrderID: "order", RunID: "run", RunnerID: "runner", RunnerSessionID: "session", LeaseToken: "lease", WorkingDir: ".", DurationSeconds: 1, Command: []string{""}}, validWithSecret("BAD-NAME", "ref"), validWithSecret("TOKEN", "../ref"), validWithSecret("TOKEN", "bad ref")} {
		if err := order.ValidateExecution(); err == nil {
			t.Fatalf("invalid execution accepted: %#v", order)
		}
	}
	for _, order := range []OrderPayload{
		{},
		{IssuedAt: now.Add(time.Minute), NotBefore: now, ExpiresAt: now.Add(2 * time.Minute)},
		{IssuedAt: now, NotBefore: now.Add(-2 * time.Minute), ExpiresAt: now.Add(time.Minute)},
		{IssuedAt: now, NotBefore: now, ExpiresAt: now},
		{IssuedAt: now, NotBefore: now.Add(time.Minute), ExpiresAt: now.Add(2 * time.Minute)},
		{IssuedAt: now.Add(-2 * time.Minute), NotBefore: now.Add(-2 * time.Minute), ExpiresAt: now.Add(-time.Minute)},
	} {
		if err := order.ValidateTime(now, time.Second); err == nil {
			t.Fatalf("invalid order time accepted: %#v", order)
		}
	}
	if err := valid.ValidateTime(now, -time.Second); err == nil {
		t.Fatal("negative time tolerance accepted")
	}
	event := EventPayload{ObservedAt: now}
	for _, candidate := range []EventPayload{{}, {ObservedAt: now.Add(time.Minute)}} {
		if err := candidate.ValidateTime(now, time.Second); err == nil {
			t.Fatal("invalid event time accepted")
		}
	}
	if err := event.ValidateTime(now, -time.Second); err == nil {
		t.Fatal("negative event tolerance accepted")
	}
	context := FreshnessContext{RunnerID: "runner", SessionID: "session", RunID: "run", Recipient: "runner", LeaseToken: "lease", ExecutionSpecDigest: "digest", Attempt: 1, FencingToken: 1, LeaseNotAfter: now.Add(time.Minute)}
	for _, candidate := range []FreshnessContext{{}, context} {
		if candidate == context {
			continue
		}
		if err := valid.ValidateFreshness(candidate, now); err == nil {
			t.Fatal("incomplete freshness context accepted")
		}
	}
	if err := valid.ValidateFreshness(context, now); err != nil {
		t.Fatal(err)
	}
	valid.RunnerSessionID = "other"
	if err := valid.ValidateFreshness(context, now); err == nil {
		t.Fatal("mismatched freshness accepted")
	}
	valid.RunnerSessionID = context.SessionID
	valid.LeaseNotAfter = now.Add(-time.Minute)
	if err := valid.ValidateFreshness(context, now); err == nil {
		t.Fatal("expired lease accepted")
	}
	for _, sequence := range []uint64{0, 2} {
		if err := (EventPayload{Sequence: sequence}).ValidateSequence(1); err == nil {
			t.Fatalf("sequence %d accepted", sequence)
		}
	}
	if err := (EventPayload{Sequence: 1}).ValidateSequence(1); err != nil {
		t.Fatal(err)
	}
	if err := (EventPayload{}).ValidateIdentity("runner", "run", 1, "lease"); err == nil {
		t.Fatal("empty event identity accepted")
	}
}

func validWithSecret(name, ref string) OrderPayload {
	return OrderPayload{OrderID: "order", RunID: "run", RunnerID: "runner", RunnerSessionID: "session", LeaseToken: "lease", WorkingDir: ".", DurationSeconds: 1, Command: []string{"true"}, SecretRefs: map[string]string{name: ref}}
}
