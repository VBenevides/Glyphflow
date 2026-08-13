package protocol

import "testing"

func TestOrderTypes(t *testing.T) {
	for _, kind := range []OrderType{OrderExecute, OrderCancel} {
		if !kind.Valid() {
			t.Fatalf("expected order type %q to be valid", kind)
		}
	}
	if OrderType("pause").Valid() {
		t.Fatal("unsupported order type was accepted")
	}
}
