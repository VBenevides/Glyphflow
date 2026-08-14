package platform

import "testing"

func TestArgon2idPasswordHashingAndUpgrade(t *testing.T) {
	hasher := PasswordHasher{Time: 1, Memory: 8 * 1024, Threads: 1, KeyLen: 32, SaltLen: 16, Pepper: []byte("pepper")}
	hash, err := hasher.Hash("correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := hasher.Verify(hash, "correct horse"); err != nil || !ok {
		t.Fatalf("valid password rejected: %v %v", ok, err)
	}
	if ok, err := hasher.Verify(hash, "wrong horse"); err != nil || ok {
		t.Fatalf("invalid password accepted: %v %v", ok, err)
	}
	if !(PasswordHasher{Time: 2, Memory: 8 * 1024, Threads: 1, KeyLen: 32, SaltLen: 16}).NeedsRehash(hash) {
		t.Fatal("parameter upgrade was not detected")
	}
	if _, err := hasher.Verify("bad", "correct horse"); err == nil {
		t.Fatal("malformed hash accepted")
	}
}
