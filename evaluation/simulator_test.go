package evaluation

import (
	"context"
	"testing"
)

// TestSimulatorHarness answers every case in everyday language and checks the
// app's invariants: no fabricated finding, lost or wrong warning, stale
// assessment or wrong contradiction.
func TestSimulatorHarness(t *testing.T) {
	cases, err := Cases()
	if err != nil {
		t.Fatal(err)
	}
	changed, runs := 0, 0
	for _, c := range cases {
		for seed := int64(1); seed <= 3; seed++ {
			sim := NewNatural(c, seed)
			r := RunSimulated(context.Background(), c, Fixture{}, sim, 8)
			runs++
			changed += sim.Changed
			if r.Failed() {
				t.Errorf("%s seed %d: fabricated %v, warnings lost %v, alerts %v, stale %v, contradictions %v, errors %v", c.ID, seed, r.FabricatedFindings, r.LostWarnings, r.AlertErrors, r.StaleAssessments, r.ContradictionErrors, r.Errors)
			}
		}
	}
	if changed == 0 {
		t.Fatal("the simulator never rephrased an answer")
	}
	t.Logf("SIMULATOR runs=%d rephrased_answers=%d", runs, changed)
}
