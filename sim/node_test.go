package sim

import (
	"math"
	"testing"
)

// parked puts a vehicle in a circular orbit with a stage that still has
// propellant in it, which is the state a manoeuvre node is for.
func parked(alt float64, nodes ...Node) *Sim {
	sys := withAtmoTop(earthMoon(), 0, 140000)
	cfg := Config{
		System: sys,
		Rocket: Rocket{
			Payload: 1000, Cd: 0.3, Diameter: 2,
			Stages: []Stage{{
				DryMass: 1000, PropMass: 8000,
				ThrustVac: 60000, IspVac: 420, IspSL: 420, Throttle: 1,
			}},
		},
		Nodes:   nodes,
		MaxTime: 1e9,
	}
	s := New(cfg)

	r := sys.Bodies[0].Radius + alt
	s.St.Pos = Vec2{r, 0}
	s.St.Vel = Vec2{0, math.Sqrt(sys.Bodies[0].Mu / r)}
	s.St.Landed = false
	s.St.Phase = PhaseCoast
	s.WarpRate = 1e9
	return s
}

func runFor(s *Sim, dt float64) {
	end := s.St.T + dt
	for !s.St.Done && s.St.T < end {
		if s.advanceOne(s.plannedStep()) <= 0 {
			break
		}
	}
}

// A node has to light on time and deliver what it was asked for. The delta-v is
// the thing being commanded, so a hundred metres a second means a hundred, not a
// step's worth more.
func TestNodeDeliversItsDeltaV(t *testing.T) {
	s := parked(300000, Node{T: 500, Frame: BurnPrograde, DeltaV: 100})
	before := s.St.DeltaV

	runFor(s, 2000)

	close(t, "delta-v spent", s.St.DeltaV-before, 100, 1e-6)
	if s.St.Node >= 0 {
		t.Error("the node is still running")
	}
	if s.St.Phase != PhaseCoast {
		t.Errorf("phase = %v after the burn, want coast", s.St.Phase)
	}
	if !hasEvent(s.Events, EvIgnition) || !hasEvent(s.Events, EvCutoff) {
		t.Error("the burn left no ignition and cutoff on the timeline")
	}
}

// The burn has to happen at the time asked for, not whenever a long coast step
// happens to notice it.
func TestNodeLightsOnTime(t *testing.T) {
	s := parked(300000, Node{T: 4000, Frame: BurnPrograde, DeltaV: 50})
	runFor(s, 6000)

	var lit float64 = -1
	for _, e := range s.Events {
		if e.Kind == EvIgnition {
			lit = e.T
		}
	}
	if lit < 0 {
		t.Fatal("never lit")
	}
	// Inside one fixed step of the scheduled instant, with coast steps of up to
	// ten minutes running right up to it.
	if math.Abs(lit-4000) > FixedStep {
		t.Errorf("lit at T+%.3f s, scheduled for T+4000", lit)
	}
}

// A prograde burn at a circular orbit's altitude raises the far side by exactly
// what vis-viva says, which is the check that the direction means what it says.
func TestProgradeNodeRaisesApoapsis(t *testing.T) {
	const alt = 300000
	s := parked(alt, Node{T: 100, Frame: BurnPrograde, DeltaV: 200})
	mu := s.Cfg.Body.Mu
	r := s.Cfg.Body.Radius + alt

	v0 := math.Sqrt(mu / r)
	v1 := v0 + 200
	a := 1 / (2/r - v1*v1/mu)
	wantApo := 2*a - r

	runFor(s, 1000)

	o := ComputeOrbit(s.St.Pos, s.St.Vel, mu)
	close(t, "apoapsis after the burn", o.Apoapsis, wantApo, 2e-3)
	close(t, "periapsis after the burn", o.Periapsis, r, 2e-3)
}

// Radial burns rotate an orbit rather than resize it, and retrograde undoes what
// prograde did. Both are direction checks that a sign error would fail.
func TestNodeDirections(t *testing.T) {
	r := earthMoon().Bodies[0].Radius + 300000
	mu := earthMoon().Bodies[0].Mu

	retro := parked(300000, Node{T: 100, Frame: BurnRetrograde, DeltaV: 200})
	runFor(retro, 1000)
	o := ComputeOrbit(retro.St.Pos, retro.St.Vel, mu)
	if o.Apoapsis > r+1000 {
		t.Errorf("a retrograde burn raised the apoapsis to %.0f km", o.Apoapsis/1000)
	}
	close(t, "apoapsis after retrograde", o.Apoapsis, r, 2e-3)

	radial := parked(300000, Node{T: 100, Frame: BurnRadialOut, DeltaV: 200})
	e0 := 0.0
	runFor(radial, 1000)
	o = ComputeOrbit(radial.St.Pos, radial.St.Vel, mu)
	if o.Eccentricity <= e0+1e-4 {
		t.Errorf("a radial burn left the orbit circular (e = %g)", o.Eccentricity)
	}
	// Radial thrust does no work at the moment it is applied, so the energy —
	// and with it the semi-major axis — barely moves.
	close(t, "semi-major axis after radial", o.SemiMajor, r, 0.02)
}

// A node with nothing to burn has to be dropped, not retried for ever.
func TestNodeWithoutPropellantIsSpent(t *testing.T) {
	s := parked(300000, Node{T: 100, Frame: BurnPrograde, DeltaV: 50})
	s.St.Prop[0] = 0

	runFor(s, 500)

	if s.St.NodesDone == 0 {
		t.Error("the node is still pending and will be retried every step for ever")
	}
	if s.St.Phase != PhaseCoast {
		t.Errorf("phase = %v, want coast", s.St.Phase)
	}
}

// Several nodes in a plan run in time order, whatever order they were typed in.
func TestNodesRunInTimeOrder(t *testing.T) {
	s := parked(300000,
		Node{T: 3000, Frame: BurnPrograde, DeltaV: 30},
		Node{T: 1000, Frame: BurnPrograde, DeltaV: 10},
		Node{T: 2000, Frame: BurnPrograde, DeltaV: 20},
	)
	runFor(s, 5000)

	var lit []float64
	for _, e := range s.Events {
		if e.Kind == EvIgnition {
			lit = append(lit, e.T)
		}
	}
	if len(lit) != 3 {
		t.Fatalf("%d ignitions, want 3", len(lit))
	}
	for i := 1; i < len(lit); i++ {
		if lit[i] <= lit[i-1] {
			t.Errorf("ignitions out of order: %v", lit)
		}
	}
	close(t, "total delta-v", s.St.DeltaV, 60, 1e-6)
}

// The prediction has to be the flight, not a sketch of it: whatever it draws is
// where the vehicle will actually be, nodes and all.
func TestPredictionMatchesTheFlight(t *testing.T) {
	plan := Node{T: 600, Frame: BurnPrograde, DeltaV: 150}

	s := parked(300000, plan)
	path := s.Predict(4000, 400)
	if len(path) < 50 {
		t.Fatalf("prediction is %d points long", len(path))
	}
	last := path[len(path)-1]

	// The prediction must not have touched the flight it predicted.
	if s.St.T != 0 || s.St.Node >= 0 || s.St.NodesDone != 0 || len(s.Events) != 1 {
		t.Errorf("predicting disturbed the state: T=%g node=%d done=%d events=%d",
			s.St.T, s.St.Node, s.St.NodesDone, len(s.Events))
	}

	// Fly it for real, stopping on the same instant the prediction did — an
	// overshoot of even one coast step is kilometres at orbital speed, and would
	// be measuring the clock rather than the trajectory.
	live := parked(300000, plan)
	for !live.St.Done && live.St.T < last.T {
		h := math.Min(live.plannedStep(), last.T-live.St.T)
		if live.advanceOne(h) <= 0 {
			break
		}
	}

	close(t, "flown time", live.St.T, last.T, 1e-9)
	if last.Center != live.St.Center {
		t.Fatalf("predicted centre %d, flew %d", last.Center, live.St.Center)
	}
	// The two runs cover the coast in different numbers of steps — the prediction
	// steps as coarsely as it draws — so this is the tolerance talking, not a
	// promise of identical arithmetic. Half a kilometre out of a 40,000 km arc.
	if gap := last.Pos.Sub(live.St.Pos).Len(); gap > 500 {
		t.Errorf("prediction ended %.1f m from where the flight did", gap)
	}
}

// A control point aims rather than burns: it says where the trajectory should end up and the
// delta-v is solved for when the moment arrives. This is the Mun flyby aimed by hand in the
// kerbin-mun preset, expressed as an aim instead of a number.
func TestAControlPointAimsAFlyby(t *testing.T) {
	base := kerbinMun().Cfg
	mun := base.System.IndexOf("mun")

	for _, want := range []float64{700e3, 1200e3, -900e3} {
		cfg := base
		cfg.Nodes = []Node{
			base.Nodes[0],
			{T: 4000, Frame: BurnPrograde, Target: TargetFlybyPeriapsis,
				TargetBody: mun, TargetValue: want, Limit: 200, Horizon: 6 * 86400},
		}

		s := New(cfg)
		s.FastForward(6 * 86400)
		n := s.Cfg.Nodes[1]
		if !n.Solved || n.Missed {
			t.Errorf("aiming %+.0f km: solved %v, missed %v", want/1000, n.Solved, n.Missed)
			continue
		}
		if n.DeltaV <= 0 || n.DeltaV > 200 {
			t.Errorf("aiming %+.0f km: solved to %g m/s", want/1000, n.DeltaV)
		}

		// And the flight it flies passes where it was aimed. Ten kilometres of tolerance
		// on seven hundred: what is left is the chaos of the approach itself, which is
		// what a second, later correction is for.
		s2 := New(cfg)
		best := math.Inf(1)
		for s2.St.T < 6*86400 && !s2.St.Done {
			step := 600.0
			if s2.St.Center != 0 {
				step = 2
			}
			s2.FastForward(s2.St.T + step)
			bp, _ := s2.Cfg.System.StateAt(mun, s2.St.T)
			if d := s2.RootPos().Sub(bp).Len(); d < best {
				best = d
			}
		}
		if off := math.Abs(best - math.Abs(want)); off > 12e3 {
			t.Errorf("aimed %+.0f km, passed %.0f km: off by %.0f km",
				want/1000, best/1000, off/1000)
		}
	}
}

// An aim that cannot be reached has to say so and fly anyway. Silently spending the limit and
// calling it a solution is how a plan lies about what it did.
func TestAControlPointSaysWhenItCannot(t *testing.T) {
	base := kerbinMun().Cfg
	mun := base.System.IndexOf("mun")
	cfg := base
	cfg.Nodes = []Node{
		base.Nodes[0],
		// A metre a second cannot move a flyby by a hundred thousand kilometres.
		{T: 4000, Frame: BurnPrograde, Target: TargetFlybyPeriapsis,
			TargetBody: mun, TargetValue: 100e6, Limit: 1, Horizon: 6 * 86400},
	}

	s := New(cfg)
	s.FastForward(6 * 86400)
	n := s.Cfg.Nodes[1]
	if !n.Solved || !n.Missed {
		t.Errorf("solved %v, missed %v: an unreachable aim has to be marked", n.Solved, n.Missed)
	}
	if n.DeltaV > 1 {
		t.Errorf("spent %g m/s against a limit of 1", n.DeltaV)
	}
	// Where the flight ends up is not the point and not asserted: a vehicle that failed to
	// aim past something may well hit it, which is the honest consequence of the miss.
}

// The solver may not depend on anything but the state. A fixed number of flights, never a time
// budget: a solver that stopped when it ran out of milliseconds would make the trajectory
// depend on how busy the machine was.
func TestSolvingIsDeterministic(t *testing.T) {
	base := kerbinMun().Cfg
	mun := base.System.IndexOf("mun")
	cfg := base
	cfg.Nodes = []Node{
		base.Nodes[0],
		{T: 4000, Frame: BurnPrograde, Target: TargetFlybyPeriapsis,
			TargetBody: mun, TargetValue: 900e3, Limit: 200, Horizon: 6 * 86400},
	}

	var got [2]Node
	for i := range got {
		s := New(cfg)
		s.FastForward(5000)
		got[i] = s.Cfg.Nodes[1]
	}
	if got[0].DeltaV != got[1].DeltaV || got[0].Frame != got[1].Frame {
		t.Errorf("two runs solved to %v and %v", got[0], got[1])
	}
}

// A prediction must not solve the flight's own plan. The plan is a slice, so a copy of the Sim
// shares it — harmless while every node was a fixed number, and a way to have the drawn path
// rewrite the mission once a control point writes its answer back.
func TestAPredictionLeavesThePlanAlone(t *testing.T) {
	base := kerbinMun().Cfg
	mun := base.System.IndexOf("mun")
	cfg := base
	cfg.Nodes = []Node{
		base.Nodes[0],
		{T: 20000, Frame: BurnPrograde, Target: TargetFlybyPeriapsis,
			TargetBody: mun, TargetValue: 900e3, Limit: 200, Horizon: 6 * 86400},
	}

	s := New(cfg)
	s.FastForward(3000)
	s.Predict(4*86400, 200)

	if n := s.Cfg.Nodes[1]; n.Solved || n.DeltaV != 0 {
		t.Errorf("the prediction wrote %+v into the live plan", n)
	}
}
