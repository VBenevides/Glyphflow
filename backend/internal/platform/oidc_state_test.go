package platform

import (
	"crypto/rand"
	"errors"
	"sync"
	"testing"
	"time"
)

type failingRandomReader struct{}

func (failingRandomReader) Read([]byte) (int, error) { return 0, errors.New("entropy unavailable") }

func TestAuthorizationStateStoreFailsClosedWithoutEntropy(t *testing.T) {
	original := rand.Reader
	rand.Reader = failingRandomReader{}
	defer func() { rand.Reader = original }()
	defer func() {
		if recover() == nil {
			t.Fatal("expected constructor to fail closed")
		}
	}()
	NewAuthorizationStateStore()
}

func TestAuthorizationStateIsSingleUseUnderConcurrentCallbacks(t *testing.T) {
	store := NewAuthorizationStateStore()
	now := time.Now()
	state, nonce, err := store.Create("provider", "login", now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- store.Consume(state, nonce, "provider", "login", now)
		}()
	}
	wg.Wait()
	close(results)
	var success int
	for err := range results {
		if err == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("expected one callback, got %d", success)
	}
}

func TestAuthorizationChallengeBindsCallbackAndPKCEVerifier(t *testing.T) {
	store := NewAuthorizationStateStore()
	now := time.Now()
	state, nonce, err := store.CreateChallenge("provider", "login", "https://app.example/callback", "verifier", now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ConsumeChallenge(state, nonce, "provider", "login", "https://app.example/callback", "verifier", now); err != nil {
		t.Fatal(err)
	}
	if err := store.ConsumeChallenge(state, nonce, "provider", "login", "https://app.example/callback", "verifier", now); err == nil {
		t.Fatal("challenge replay accepted")
	}
}

func TestAuthorizationStateStoreRemovesConsumedAndExpiredEntries(t *testing.T) {
	store := NewAuthorizationStateStore()
	now := time.Now()
	state, nonce, err := store.Create("provider", "login", now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Consume(state, nonce, "provider", "login", now); err != nil {
		t.Fatal(err)
	}
	if len(store.states) != 0 {
		t.Fatalf("consumed states = %d, want 0", len(store.states))
	}
	if _, _, err := store.Create("provider", "login", now.Add(-time.Minute), time.Second); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Create("provider", "login", now, time.Minute); err != nil {
		t.Fatal(err)
	}
	if len(store.states) != 1 {
		t.Fatalf("expired states retained = %d, want 1 active state", len(store.states))
	}
}
