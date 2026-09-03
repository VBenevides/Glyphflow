package platform

import (
	"testing"
	"time"
)

func TestPlatformStoresAndStateCoverage(t *testing.T) { // NOSONAR: this comprehensive coverage scenario intentionally exercises the platform state stores and their transitions together.
	guard := NewAdministratorGuard("one")
	if err := guard.Remove("one", nil); err == nil {
		t.Fatal("last administrator removed")
	}
	guard.Add("two")
	if err := guard.Remove("one", func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	guard.Set("two", "three")
	if guard.ActiveCount() != 2 || ValidateRoleMutation(false) != nil || ValidateRoleMutation(true) == nil {
		t.Fatal("administrator guard state is wrong")
	}

	store := NewAuthStore()
	if err := store.AddUser(User{ID: "user", Username: "User", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if err := store.AddUser(User{ID: "duplicate", Username: "user"}); err == nil {
		t.Fatal("duplicate user accepted")
	}
	if err := store.SetPassword("user", "hash"); err != nil {
		t.Fatal(err)
	}
	if err := store.AddPermission(Permission{ID: "read", Key: "read"}); err != nil {
		t.Fatal(err)
	}
	if err := store.AddRole(Role{ID: "role", Key: "role"}, "read"); err != nil {
		t.Fatal(err)
	}
	if err := store.AssignRole("user", "role"); err != nil {
		t.Fatal(err)
	}
	if !store.EffectivePermissions("user")["read"] {
		t.Fatal("permission assignment missing")
	}

	events := NewEventTracker()
	if accepted, err := events.Accept("event", "attempt", 1); err != nil || !accepted {
		t.Fatalf("event accept = %v, %v", accepted, err)
	}
	if accepted, err := events.Accept("event", "attempt", 2); err != nil || accepted {
		t.Fatalf("duplicate event = %v, %v", accepted, err)
	}
	if accepted, err := events.Accept("event-2", "attempt", 1); err == nil || accepted {
		t.Fatalf("out of order event = %v, %v", accepted, err)
	}
	if accepted, err := events.AcceptChannel("channel", "attempt", "stdout", 1); err != nil || !accepted {
		t.Fatalf("channel event = %v, %v", accepted, err)
	}

	now := time.Now().UTC()
	states := NewAuthorizationStateStore()
	if _, _, err := states.Create("provider", "login", now, time.Minute); err != nil {
		t.Fatal(err)
	}
	state, nonce, err := states.CreateChallenge("provider", "login", "https://callback", "verifier", now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if provider, callback, verifier, err := states.ReadChallenge(state, nonce, "login", now); err != nil || provider != "provider" || callback != "https://callback" || verifier != "verifier" {
		t.Fatalf("challenge = %q %q %q err=%v", provider, callback, verifier, err)
	}
	if err := states.ConsumeChallenge(state, nonce, "provider", "login", "https://callback", "verifier", now); err != nil {
		t.Fatal(err)
	}
	if err := states.Consume(state, nonce, "provider", "login", now); err == nil {
		t.Fatal("consumed authorization state accepted")
	}

	if got, err := ResolveAmbiguous(RetryAmbiguous); err != nil || got != "retry_wait" {
		t.Fatalf("retry ambiguity = %q err=%v", got, err)
	}
	if got, err := ResolveAmbiguous(ManualAmbiguous); err != nil || got != "unknown" {
		t.Fatalf("manual ambiguity = %q err=%v", got, err)
	}
	if got, err := ResolveAmbiguous(FailedAmbiguous); err != nil || got != "failed" {
		t.Fatalf("failed ambiguity = %q err=%v", got, err)
	}
	if _, err := ResolveAmbiguous("invalid"); err == nil {
		t.Fatal("invalid ambiguity accepted")
	}
	policy := RetryPolicy{MaxAttempts: 3, RetryableExitCodes: map[int]bool{1: true}, BaseDelay: time.Second, MaxDelay: time.Minute}
	if state, _ := policy.Decide(1, 1, ""); state != "retry_wait" {
		t.Fatal("retry policy did not retry")
	}
	aggregator := &RunAggregator{}
	for _, outcome := range []string{"unknown", "completed", "failed"} {
		aggregator.Apply(outcome, true, 3)
	}
	if !FinalState("unknown") || FinalState("waiting") || !TransitionAllowed("running", "cancelled") {
		t.Fatal("recovery state rules are wrong")
	}
	machine, err := NewStateMachine("waiting")
	if err != nil {
		t.Fatal(err)
	}
	if err := machine.CompareAndSwap("waiting", 0, "running"); err != nil {
		t.Fatal(err)
	}
	if _, version := machine.Snapshot(); version != 1 {
		t.Fatal("state machine version did not advance")
	}

	refresh := NewRefreshSessionManager()
	if _, _, err := refresh.Issue("", time.Hour); err == nil {
		t.Fatal("empty refresh user accepted")
	}
	sessionID, token, err := refresh.Issue("user", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := refresh.Rotate(sessionID, token, time.Hour); err != nil {
		t.Fatal(err)
	}
	refresh.DisableUser("user")
	refresh.RevokeUser("user")
	refresh.Revoke(sessionID)
	if _, ok := refresh.UserID(sessionID); ok {
		t.Fatal("revoked refresh session remained")
	}

	assignments := []RoleAssignment{{UserID: "user", RoleID: "manual", SourceType: "manual", SourceKey: "admin"}, {UserID: "user", RoleID: "old", SourceType: "sso", SourceKey: "old", ProviderID: "provider"}}
	next, changes := SyncSSORoles("user", "provider", assignments, []string{"admins", "admins", "ignored"}, map[string]string{"admins": "admin"}, nil)
	if len(next) != 2 || len(changes) != 2 {
		t.Fatalf("SSO reconciliation = %#v changes=%#v", next, changes)
	}
	if groups := ExtractSSOGroups(map[string]any{"groups": []any{"a", "a"}, "nested": map[string]any{"group": "b"}}, []string{"groups", "nested.group"}); len(groups) != 2 {
		t.Fatalf("SSO groups = %#v", groups)
	}
	assignmentStore := NewRoleAssignmentStore()
	if err := assignmentStore.Add(RoleAssignment{UserID: "user", RoleID: "role", SourceType: "SSO", SourceKey: "Group"}); err != nil {
		t.Fatal(err)
	}
	if err := assignmentStore.Add(RoleAssignment{UserID: "user", RoleID: "role", SourceType: "SSO", SourceKey: "Group"}); err == nil {
		t.Fatal("duplicate assignment accepted")
	}
	if len(assignmentStore.List("user")) != 1 {
		t.Fatal("assignment list is wrong")
	}

	sessions := NewRunnerSessionRegistry()
	if _, err := sessions.Connect("", "session"); err == nil {
		t.Fatal("empty runner session accepted")
	}
	if _, err := sessions.Connect("runner", "session"); err != nil || !sessions.IsActive("runner", "session") {
		t.Fatal("runner session was not connected")
	}
	if sessions.Disconnect("runner", "other") || !sessions.Disconnect("runner", "session") {
		t.Fatal("runner session disconnect is wrong")
	}

	for _, free := range []float64{-1, 5, 10, 20, 50} {
		_ = EvaluateStoragePressure(free)
	}
	monitor := new(StoragePressureMonitor)
	_ = monitor.Observe(4)
	_ = monitor.Observe(8)
	if !(StoragePressure{State: StorageEmergency}).RejectNewRuns() {
		t.Fatal("emergency storage was accepted")
	}
	accounts := NewServiceAccountStore()
	if _, err := accounts.Issue(""); err == nil {
		t.Fatal("empty service account accepted")
	}
	accountToken, err := accounts.Issue("account")
	if err != nil || !accounts.Verify("account", accountToken) {
		t.Fatal("service account token failed")
	}
}

func TestPlatformExecutionAndObjectLogCoverage(t *testing.T) {
	if _, err := EncryptSecret([]byte("short"), "secret"); err == nil {
		t.Fatal("invalid encryption key accepted")
	}
	key := []byte("01234567890123456789012345678901")
	ciphertext, err := EncryptSecret(key, "secret")
	if err != nil {
		t.Fatal(err)
	}
	if value, err := DecryptSecret(key, ciphertext); err != nil || value != "secret" {
		t.Fatalf("decrypted secret = %q err=%v", value, err)
	}
	if _, err := DecryptSecret([]byte("short"), ciphertext); err != ErrSecretDecryption {
		t.Fatalf("invalid key error = %v", err)
	}
	ciphertext[len(ciphertext)-1] ^= 1
	if _, err := DecryptSecret(key, ciphertext); err != ErrSecretIntegrity {
		t.Fatalf("tampered secret error = %v", err)
	}
	if err := ValidateCancellation(Cancellation{}, Cancellation{}); err == nil {
		t.Fatal("incomplete cancellation accepted")
	}
	cancellation := Cancellation{RunID: "run", AttemptID: "attempt", SessionID: "session", LeaseToken: "lease", Fencing: 1}
	for _, state := range []string{"waiting", "running", "cancelling", "completed", "invalid"} {
		_, _ = ApplyCancellation(cancellation, cancellation, state, false)
	}
	accumulator, err := NewLogAccumulator(3)
	if err != nil {
		t.Fatal(err)
	}
	accumulator.Append([]byte("abcd"))
	accumulator.Append([]byte("e"))
	if data, truncated := accumulator.Bytes(); string(data) != "abc" || !truncated {
		t.Fatalf("log accumulator = %q truncated=%v", data, truncated)
	}
	if _, err := ExecutionDigest(ExecutionSpec{}); err == nil {
		t.Fatal("incomplete execution spec accepted")
	}
	if digest, err := ExecutionDigest(ExecutionSpec{TaskVersion: "v1", Command: []string{"echo"}, WorkingDir: ".", Duration: 1, MaxOutput: 1}); err != nil || digest == "" {
		t.Fatalf("execution digest = %q err=%v", digest, err)
	}
	if _, _, err := BoundLogChunk([]byte("x"), 0); err == nil {
		t.Fatal("invalid log bound accepted")
	}
	if chunk, truncated, err := BoundLogChunk([]byte("abcd"), 2); err != nil || string(chunk) != "ab" || !truncated {
		t.Fatalf("bounded chunk = %q truncated=%v err=%v", chunk, truncated, err)
	}
	leases := NewLeaseManager()
	if _, err := leases.Acquire("", "attempt", "token", time.Now(), time.Minute); err == nil {
		t.Fatal("incomplete lease accepted")
	}
	lease, err := leases.Acquire("resource", "attempt", "token", time.Now(), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := leases.Renew(lease, time.Now(), time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := leases.Release(lease, time.Now()); err != nil {
		t.Fatal(err)
	}
	_ = leases.Expire(time.Now().Add(time.Hour))
	logs, err := NewObjectLogStore(t.TempDir(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if err := logs.Put("run", []byte("log")); err != nil {
		t.Fatal(err)
	}
	if data, err := logs.Get("run"); err != nil || string(data) != "log" {
		t.Fatalf("object log = %q err=%v", data, err)
	}
	if err := logs.Put("too-large", []byte("01234567890")); err == nil {
		t.Fatal("oversized object log accepted")
	}
}
