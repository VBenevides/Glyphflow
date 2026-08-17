package protocol

import (
	"crypto/ed25519"
	"embed"
	"encoding/base64"
	"encoding/json"
	"testing"
)

//go:embed testdata/order-v1.json
var goldenFiles embed.FS

func TestGoldenOrderVector(t *testing.T) {
	raw, err := goldenFiles.ReadFile("testdata/order-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var vector struct {
		Envelope
		PublicKey string `json:"public_key"`
	}
	if err := json.Unmarshal(raw, &vector); err != nil {
		t.Fatal(err)
	}
	publicKey, err := base64.StdEncoding.DecodeString(vector.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := vector.Envelope.VerifyOrder(ed25519.PublicKey(publicKey)); err != nil {
		t.Fatal(err)
	}
}
