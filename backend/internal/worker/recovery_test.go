package worker

import "testing"

func TestOrderRecoveryMarksOlderBootOrdersUnknown(t *testing.T) {
	recovery := NewOrderRecovery("boot-2")
	if err := recovery.Claim("order-1"); err != nil {
		t.Fatal(err)
	}
	recovery.orders["order-old"] = "boot-1"
	unknown := recovery.Recover("boot-1")
	if len(unknown) != 1 || unknown[0] != "order-old" {
		t.Fatalf("unexpected recovery result: %#v", unknown)
	}
}
