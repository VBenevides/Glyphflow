package controlplane

import (
	"context"
	"errors"
	"testing"
)

func TestPlaneRunCoversEmptyAndFailingComponents(t *testing.T) {
	if err := New().Run(context.Background()); err != nil {
		t.Fatalf("empty plane returned %v", err)
	}
	want := errors.New("component failed")
	if err := New(func(context.Context) error { return want }).Run(context.Background()); !errors.Is(err, want) {
		t.Fatalf("failing plane returned %v", err)
	}
}
