package platform

import "testing"

func TestPlacementSkipsUnavailableAndBalancesEligibleRunners(t *testing.T) {
	p := new(Placer)
	runners := []RunnerCandidate{{ID: "offline", Online: false}, {ID: "r1", SessionID: "s1", Enabled: true, Online: true, Capacity: 2, Capabilities: map[string]string{"os": "linux"}}, {ID: "r2", SessionID: "s2", Enabled: true, Online: true, Capacity: 2, Capabilities: map[string]string{"os": "linux"}}}
	a, err := p.Select(runners, PlacementRequest{Required: map[string]string{"os": "linux"}})
	if err != nil || a.ID != "r1" {
		t.Fatalf("first placement: %#v %v", a, err)
	}
	b, err := p.Select(runners, PlacementRequest{Required: map[string]string{"os": "linux"}})
	if err != nil || b.ID != "r2" {
		t.Fatalf("second placement: %#v %v", b, err)
	}
	if _, err := p.Select(runners, PlacementRequest{PinnedRunner: "offline"}); err == nil {
		t.Fatal("unavailable pinned runner accepted")
	}
}
