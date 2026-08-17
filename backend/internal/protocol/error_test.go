package protocol

import (
	"strings"
	"testing"
)

func TestEventErrorIsBounded(t *testing.T) {
	valid := EventPayload{Error: strings.Repeat("e", MaxEventErrorBytes)}
	if err := valid.ValidateError(); err != nil {
		t.Fatal(err)
	}
	tooLarge := EventPayload{Error: strings.Repeat("e", MaxEventErrorBytes+1)}
	if err := tooLarge.ValidateError(); err == nil {
		t.Fatal("oversized event error was accepted")
	}
}
