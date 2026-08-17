package platform

import "testing"

func TestRefreshFamilyRevokesOnReplay(t *testing.T) {
	f := NewRefreshFamily()
	f.Issue("f", "a")
	if err := f.Rotate("f", "a", "b"); err != nil {
		t.Fatal(err)
	}
	if err := f.Rotate("f", "a", "c"); err == nil {
		t.Fatal("replayed family token accepted")
	}
	if err := f.Rotate("f", "b", "d"); err == nil {
		t.Fatal("revoked family remained usable")
	}
}
