package worker

import "testing"

func TestBootstrapRoundTrip(t *testing.T) {
	input := Bootstrap{Token: "token", RunnerID: "runner-1", ControlPlaneURL: "https://control.example", NATSURL: "nats://localhost:4222", MaxMessageBytes: 1024}
	raw, err := PackBootstrap([]byte("binary"), input)
	if err != nil {
		t.Fatal(err)
	}
	output, err := UnpackBootstrap(raw)
	if err != nil || output == nil || *output != input {
		t.Fatalf("bootstrap = %#v, err=%v", output, err)
	}
	plain, err := UnpackBootstrap([]byte("binary"))
	if err != nil || plain != nil {
		t.Fatalf("plain binary = %#v, err=%v", plain, err)
	}
}
