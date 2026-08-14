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

func TestLogAccumulatorBoundsTotalOutput(t *testing.T) {
	accumulator, err := NewLogAccumulator(5)
	if err != nil {
		t.Fatal(err)
	}
	if got, truncated := accumulator.Append([]byte("abc")); truncated || string(got) != "abc" {
		t.Fatal("first chunk was truncated")
	}
	if got, truncated := accumulator.Append([]byte("def")); !truncated || string(got) != "de" {
		t.Fatalf("total bound failed: %q %v", got, truncated)
	}
	if got, truncated := accumulator.Append([]byte("x")); !truncated || len(got) != 0 {
		t.Fatal("post-limit output accepted")
	}
	if data, truncated := accumulator.Bytes(); string(data) != "abcde" || !truncated {
		t.Fatalf("stored output: %q %v", data, truncated)
	}
}
