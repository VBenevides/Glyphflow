package backend

import (
	"os"
	"strings"
	"testing"
)

func TestVersionUsesRepositoryVersion(t *testing.T) {
	raw, err := os.ReadFile("../VERSION")
	if err != nil {
		t.Fatal(err)
	}
	if Version != strings.TrimSpace(string(raw)) {
		t.Fatalf("Version = %q, want %q", Version, strings.TrimSpace(string(raw)))
	}
}
