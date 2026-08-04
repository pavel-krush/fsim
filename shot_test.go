package main

import (
	"testing"

	"github.com/pavel-krush/fsim/sim"
)

// The capture script names moments instead of counting seconds, so the thing that
// can go wrong is a step resolving to an instant the flight has already passed:
// FastForward does not run backwards, so that step would quietly capture the
// previous one's frame under its own name. Every preset has to keep the script in
// order, and the presets differ by five orders of magnitude in length.
func TestShotStepsAreInOrderForEveryPreset(t *testing.T) {
	for _, p := range sim.Presets() {
		t.Run(p.Name, func(t *testing.T) {
			sr := newShotRunner("", p.Cfg)
			prev, prevName := 0.0, "liftoff"
			resolved := 0
			for _, st := range sr.steps {
				if st.at == nil {
					continue
				}
				at, ok := sr.tl.resolve(st)
				if !ok {
					continue
				}
				resolved++
				if at < prev {
					t.Errorf("%s resolves to T+%.0f s, before %s at T+%.0f s",
						st.name, at, prevName, prev)
				}
				prev, prevName = at, st.name
			}
			// A preset with nothing resolvable would silently capture one frame over
			// and over, which is the failure this whole mechanism exists to avoid.
			if resolved < 4 {
				t.Errorf("only %d of the script's moments exist in this flight", resolved)
			}
			t.Logf("%d moments, last at T+%.0f s, going to body %d", resolved, prev, sr.tl.crossing)
		})
	}
}
