package sim

import (
	"math"
	"testing"
)

// Every shipped preset gets audited, because a preset is data and data rots
// quietly: a body added to the system, a node moved past the time limit, a stage
// edited into something that cannot burn. None of that fails to compile and most
// of it does not fail to fly — it just stops meaning what it says.
func TestPresetsAreValid(t *testing.T) {
	for _, p := range Presets() {
		t.Run(p.Name, func(t *testing.T) {
			cfg := p.Cfg
			cfg.EnsureSystem()
			sys := &cfg.System

			if cfg.LaunchBody < 0 || cfg.LaunchBody >= len(sys.Bodies) {
				t.Fatalf("launch body %d of %d", cfg.LaunchBody, len(sys.Bodies))
			}

			for i := range sys.Bodies {
				b := &sys.Bodies[i]
				if b.Mass <= 0 || b.Radius <= 0 {
					t.Errorf("%s: mass %g, radius %g", b.Name, b.Mass, b.Radius)
				}
				if i == 0 {
					continue
				}
				// The invariant the whole tree rests on.
				if b.Parent < 0 || b.Parent >= i {
					t.Errorf("%s has parent %d at index %d", b.Name, b.Parent, i)
				}
				if b.SemiMajor <= 0 {
					t.Errorf("%s has no orbit", b.Name)
				}
				// Past this the Kepler solver's opening guess stops converging.
				if b.Ecc >= 0.95 {
					t.Errorf("%s has eccentricity %g", b.Name, b.Ecc)
				}
			}

			// The air, and the orbit being aimed at through it.
			if n := len(cfg.Body.Atmo.Fractions); n != 0 && n != len(Gases) {
				t.Errorf("%d gas fractions, want %d or none", n, len(Gases))
			}
			if cfg.TargetOrbit <= cfg.Body.Atmo.Top {
				t.Errorf("target orbit %.0f m is inside the atmosphere (%.0f m)",
					cfg.TargetOrbit, cfg.Body.Atmo.Top)
			}

			// A vehicle that cannot lift itself off the pad is not a preset.
			twr := cfg.Rocket.LiftoffTWR(cfg.Body.Atmo.SurfacePressure, cfg.Body.SurfaceG)
			if twr <= 1.05 {
				t.Errorf("liftoff thrust-to-weight is %.2f", twr)
			}
			for i := range cfg.Rocket.Stages {
				st := &cfg.Rocket.Stages[i]
				switch {
				case st.DryMass <= 0 || st.ThrustVac <= 0 || st.IspVac <= 0 || st.IspSL <= 0:
					t.Errorf("stage %d has a zero where it needs a number", i+1)
				case st.IspSL > st.IspVac:
					t.Errorf("stage %d: sea-level Isp %g above the vacuum figure %g",
						i+1, st.IspSL, st.IspVac)
				case st.BurnTime() <= 0:
					t.Errorf("stage %d never burns", i+1)
				}
			}

			// A plan the clock cuts short is a plan that does not happen. It only
			// happens today because a verdict disables the limit, which is an
			// accident and not something to rely on.
			for i, n := range cfg.Nodes {
				// A control point's delta-v is an output, so what it has to carry is a
				// budget to spend and an aim to spend it on.
				if n.Target != TargetNone {
					if n.Limit <= 0 {
						t.Errorf("control point %d has nothing to spend", i)
					}
					if n.Horizon <= 0 {
						t.Errorf("control point %d measures its aim over no time at all", i)
					}
				} else if n.DeltaV <= 0 {
					t.Errorf("node %d asks for no delta-v", i)
				}
				if n.T < 0 {
					t.Errorf("node %d is scheduled before liftoff", i)
				}
				if cfg.MaxTime > 0 && n.T > cfg.MaxTime {
					t.Errorf("node %d fires at T+%.0f s, past the time limit of %.0f",
						i, n.T, cfg.MaxTime)
				}
			}

			// And it has to fly, to roughly where it says it is going. Only as far as
			// its first verdict: what is being checked is the ascent, and a preset
			// whose mission runs for twenty-four years should not cost that here.
			s := flyToVerdict(p.Cfg)
			if s.St.Outcome != OutcomeOrbit {
				t.Fatalf("outcome %d at T+%.0f s", s.St.Outcome, s.St.T)
			}
			apo := s.Telemetry().ApoAlt
			switch {
			case len(cfg.Nodes) > 0:
				// The target belongs to the mission, not to the ascent: a preset with
				// a plan reaches its verdict in a parking orbit and goes on from
				// there. All the ascent owes is an orbit that clears the air.
				if apo <= cfg.Body.Atmo.Top {
					t.Errorf("apoapsis %.0f km is inside the atmosphere", apo/1000)
				}
			case math.Abs(apo-cfg.TargetOrbit) > cfg.TargetOrbit/2:
				t.Errorf("apoapsis %.0f km against a target of %.0f km",
					apo/1000, cfg.TargetOrbit/1000)
			}
		})
	}
}

// Deleting the burn that is currently running has to shut the engine down. The
// panel does it with one button, and the state it leaves behind used to index a
// plan that no longer had that entry in it.
func TestRemoveRunningNode(t *testing.T) {
	s := parked(300000, Node{T: 100, Frame: BurnPrograde, DeltaV: 3000})
	runFor(s, 200)
	if s.St.Node != 0 || s.St.Phase != PhaseBurn {
		t.Fatalf("the node is not running: node %d, phase %v", s.St.Node, s.St.Phase)
	}
	spent := s.St.DeltaV

	s.RemoveNode(0)

	if s.St.Node >= 0 {
		t.Errorf("still running node %d", s.St.Node)
	}
	if s.St.Phase != PhaseCoast {
		t.Errorf("phase %v after the plan was deleted, want coast", s.St.Phase)
	}
	runFor(s, 300)
	if s.St.DeltaV > spent+1e-6 {
		t.Errorf("kept burning for a plan that is gone: %.3f m/s more", s.St.DeltaV-spent)
	}
}

// Deleting an earlier node has to bring the running index down with the slice, or
// the burn in progress starts answering for a different entry.
func TestRemoveEarlierNode(t *testing.T) {
	s := parked(300000,
		Node{T: 100, Frame: BurnPrograde, DeltaV: 10},
		// Long enough that it is still burning when the earlier one is deleted:
		// three thousand metres a second is five hundred seconds of thrust here.
		Node{T: 400, Frame: BurnPrograde, DeltaV: 3000},
	)
	runFor(s, 500)
	if s.St.Node != 1 {
		t.Fatalf("running node is %d, wanted the second", s.St.Node)
	}

	s.RemoveNode(0)

	if s.St.Node != 0 {
		t.Fatalf("running node is %d after the first was deleted, want 0", s.St.Node)
	}
	if s.St.NodesDone&1 == 0 {
		t.Error("the running node is no longer marked as started")
	}
	runFor(s, 900)

	// It finishes the burn it was actually running, not the one it now shares an
	// index with.
	close(t, "total delta-v", s.St.DeltaV, 3010, 1e-6)
}
