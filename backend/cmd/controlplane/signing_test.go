package main

import (
	"bytes"
	"os"
	"testing"
)

func TestLoadControlPlaneSigningKeyPersistsGeneratedKey(t *testing.T) {
	path := t.TempDir() + "/control-plane-signing.key"
	first, err := loadControlPlaneSigningKey("", path)
	if err != nil {
		t.Fatal(err)
	}
	second, err := loadControlPlaneSigningKey("", path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.Public.PublicKey, second.Public.PublicKey) {
		t.Fatal("generated control-plane key changed between loads")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("key permissions = %o, want 600", info.Mode().Perm())
	}
}
