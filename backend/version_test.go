package backend

import "testing"

func TestVersionIsEmbedded(t *testing.T) {
	if Version != "0.1.0" {
		t.Fatalf("Version = %q, want 0.1.0", Version)
	}
}
