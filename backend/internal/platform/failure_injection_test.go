package platform

import "testing"

func TestFailureInjectionKeepsOneLogicalOccurrenceAcrossRetry(t *testing.T) {
	f := NewFailureInjector()
	f.FailNext("after-run-commit")
	count := 0
	commit := func() error { count++; return nil }
	if err := f.RunOccurrence("occurrence", "after-run-commit", commit); err == nil {
		t.Fatal("injected failure was ignored")
	}
	if err := f.RunOccurrence("occurrence", "retry", commit); err != nil {
		t.Fatal(err)
	}
	if err := f.RunOccurrence("occurrence", "retry", commit); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("unexpected commit count: %d", count)
	}
}
