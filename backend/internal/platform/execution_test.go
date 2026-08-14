package platform

import "testing"

func TestExecutionDigestAndLogBound(t *testing.T) {
	spec := ExecutionSpec{TaskVersion: "v1", Command: []string{"echo", "ok"}, WorkingDir: "/tmp", Timeout: 10, MaxOutput: 4}
	digest, err := ExecutionDigest(spec)
	if err != nil || len(digest) != 64 {
		t.Fatalf("digest failed: %q %v", digest, err)
	}
	if other, _ := ExecutionDigest(ExecutionSpec{TaskVersion: "v1", Command: []string{"echo", "different"}, WorkingDir: "/tmp", Timeout: 10, MaxOutput: 4}); digest == other {
		t.Fatal("different specs have the same digest")
	}
	chunk, truncated, err := BoundLogChunk([]byte("abcdef"), 4)
	if err != nil || !truncated || string(chunk) != "abcd" {
		t.Fatalf("log bound failed: %q %v %v", chunk, truncated, err)
	}
}
