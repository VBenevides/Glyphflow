package platform

import (
	"sync"
	"testing"
	"time"
)

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
