package protocol

import (
	"bytes"
	"testing"
)

func TestDecodeEnvelopeRejectsOversizedInputBeforeParsing(t *testing.T) {
	raw := bytes.Repeat([]byte("{"), 64)
	if _, err := DecodeEnvelopeLimited(raw, 32); err == nil {
		t.Fatal("oversized envelope was accepted")
	}
}
