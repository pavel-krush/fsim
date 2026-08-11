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

// A resonant return, which is the thing control points were built for. A correction on the way
// to Venus, aimed at the *period the pass will leave* rather than at a delta-v, puts the vehicle
// on an orbit commensurate with Venus's year — and four of those orbits later it is back where
// Venus is.
//
// The two aims a chain wants, a safe pass and a particular period, are held by two nodes rather
// than by one: the early one sets the energy and a later one trims the aim. One knob holds one
// target, and a Newton over both at once does not converge through a close flyby.
func TestAResonantReturnComesBack(t *testing.T) {
	base := parkerSolar().Cfg
	ven := base.System.IndexOf("venus")
	soi := base.System.Bodies[ven].SOI
	const venusYear = 224.701 * 86400
	const want = venusYear * 3 / 4 // four vehicle orbits to three of Venus's

	cfg := base
	cfg.Nodes = []Node{
		base.Nodes[0],
		// Trimmed so there is propellant left to steer with, which is all a correction
		// needs: the flyby does the reshaping.
		{T: base.Nodes[1].T, Frame: BurnPrograde, DeltaV: 3300},
		{T: 20 * 86400, Frame: BurnPrograde, Target: TargetPeriodAfterFlyby,
			TargetBody: ven, TargetValue: want, Limit: 120, Horizon: 120 * 86400},
	}
	cfg.MaxTime = 4 * 365.25 * 86400

	s := New(cfg)
	var passes []float64
	best, in := math.Inf(1), false
	for s.St.T < 800*86400 && !s.St.Done {
		step := 7200.0
		if s.St.Center == ven {
			step = 60
		}
		s.FastForward(s.St.T + step)
		bp, _ := s.Cfg.System.StateAt(ven, s.St.T)
		d := s.RootPos().Sub(bp).Len()
		switch near := d < 4*soi; {
		case near && d < best:
			best, in = d, true
		case !near && in:
			passes, best, in = append(passes, best), math.Inf(1), false
		}
	}
	if in {
		passes = append(passes, best)
	}

	n := s.Cfg.Nodes[2]
	if !n.Solved || n.Missed {
		t.Fatalf("the energy correction solved %v, missed %v (%g m/s)", n.Solved, n.Missed, n.DeltaV)
	}
	if n.DeltaV > 120 {
		t.Errorf("spent %g m/s against a limit of 120", n.DeltaV)
	}
	if len(passes) < 2 {
		t.Fatalf("%d Venus approaches in 800 days: the resonance did not bring it back",
			len(passes))
	}
	// Both passes clear of the planet, and the second one is the return: four orbits of
	// three quarters of a Venus year later.
	for i, d := range passes[:2] {
		if d < base.System.Bodies[ven].Radius {
			t.Errorf("approach %d went through Venus", i+1)
		}
	}
}

// A solve is spread over as many calls as the interface has frames, and the answer must not
// depend on how many candidates each of them ran. Otherwise the trajectory would depend on the
// frame rate, which is the one thing this simulator refuses to let anything depend on.
//
// A control point costs twenty-five flights of the mission ahead — six seconds of them on
// Parker's third correction — so doing it inside one frame was a window that had stopped
// answering, with no way to draw a word of explanation until the frame ended.
func TestASolveSpreadOverFramesGivesTheSameAnswer(t *testing.T) {
	fly := func(budget int) (float64, float64, [4]float64) {
		s := New(kerbinMun().Cfg)
		mun := s.Cfg.System.IndexOf("mun")
		s.Cfg.Nodes = []Node{s.Cfg.Nodes[0],
			{T: 4000, Frame: BurnPrograde, Target: TargetFlybyPeriapsis,
				TargetBody: mun, TargetValue: 700e3, Limit: 200, Horizon: 6 * 86400}}
		// One frame at a time, as the flight screen drives it, rather than in one jump.
		for s.St.T < 2*86400 && !s.St.Done {
			if _, _, solving := s.Solving(); solving {
				s.PumpSolve(budget)
				continue
			}
			s.WarpRate = 10000
			s.Advance(1.0 / 60)
		}
		n := s.Cfg.Nodes[1]
		return n.DeltaV, s.St.T, [4]float64{s.St.Pos.X, s.St.Pos.Y, s.St.Vel.X, s.St.Vel.Y}
	}

	dv1, t1, st1 := fly(1)
	dv2, t2, st2 := fly(4)
	if dv1 != dv2 || t1 != t2 || st1 != st2 {
		t.Errorf("one candidate a frame and four gave different flights:\n %g at %g %v\n %g at %g %v",
			dv1, t1, st1, dv2, t2, st2)
	}
	if dv1 <= 0 {
		t.Errorf("the correction solved to %g m/s", dv1)
	}
}

// No mission time passes while a correction is being worked out, and the burn still starts at
// the instant the plan asked for. Out beyond the air a coast step is minutes long, so a flight
// that kept stepping through a solve would have the engine light minutes late — and solve the
// correction for a state it had already flown out of.
func TestNoTimePassesWhileSolving(t *testing.T) {
	s := New(kerbinMun().Cfg)
	mun := s.Cfg.System.IndexOf("mun")
	const when = 4000.0
	s.Cfg.Nodes = []Node{s.Cfg.Nodes[0],
		{T: when, Frame: BurnPrograde, Target: TargetFlybyPeriapsis,
			TargetBody: mun, TargetValue: 700e3, Limit: 200, Horizon: 6 * 86400}}

	s.WarpRate = 10000
	solving := false
	for s.St.T < when+3600 && !s.St.Done && !solving {
		s.Advance(1.0 / 60)
		_, _, solving = s.Solving()
	}
	if !solving {
		t.Fatalf("no solve started by T+%g s", s.St.T)
	}
	// Not before the node's own time. It can be after: the plan's earlier burn was still
	// running at T+4000, and the sequence it is in the middle of comes first.
	held := s.St.T
	if held < when-1e-6 {
		t.Errorf("the solve began at T+%g s, before the T+%g the plan asked for", held, when)
	}
	for range 5 {
		s.Advance(1.0 / 60)
		if s.St.T != held {
			t.Fatalf("the clock moved to %g while a correction was being solved", s.St.T)
		}
	}
	for {
		if _, _, solving := s.Solving(); !solving {
			break
		}
		s.PumpSolve(1)
	}
	if s.St.T != held {
		t.Errorf("finishing the solve moved the clock to %g", s.St.T)
	}
	if s.St.Phase != PhaseBurn || s.St.Node != 1 {
		t.Errorf("the burn did not start when the solve finished: phase %v, node %d",
			s.St.Phase, s.St.Node)
	}
}

// A prediction must not solve a control point, and this is the expensive half of "a copy never
// solves". A drawn path that solved the corrections in the plan cost twenty-seven flights of the
// mission ahead every time it was recomputed — 2.4 seconds of them once Parker's first control
// point came inside the ten-day horizon, several times over during the coast to Venus. It was
// reported as the simulation freezing, and it was.
//
// What is drawn instead is the path with the pending correction left out, which is the honest
// answer: nobody knows its size until the moment arrives. So the predicted curve has to match,
// point for point, the one a plan with that burn set to nothing produces.
func TestAPredictionDoesNotSolveAControlPoint(t *testing.T) {
	base := parkerSolar().Cfg
	ven := base.System.IndexOf("venus")

	aimed := New(base)
	aimed.FastForward(15 * 86400) // inside the first control point's horizon, before its time

	// The same flight with the control point reduced to a burn of nothing, which is what a
	// pending correction is worth to anybody drawing a path.
	plain := New(base)
	plain.FastForward(15 * 86400)
	for i := range plain.Cfg.Nodes {
		if n := &plain.Cfg.Nodes[i]; n.Target != TargetNone {
			n.Target, n.DeltaV, n.Limit = TargetNone, 0, 0
		}
	}

	got := aimed.Predict(10*86400, 400)
	want := plain.Predict(10*86400, 400)
	if len(got) != len(want) || len(got) < 10 {
		t.Fatalf("%d points against %d", len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("point %d differs: %+v against %+v — the prediction solved the correction",
				i, got[i], want[i])
		}
	}

	// And the flight's own plan is untouched: still unsolved, still aiming at Venus.
	for i, n := range aimed.Cfg.Nodes {
		if n.Target == TargetNone {
			continue
		}
		if n.Solved || n.DeltaV != 0 || n.TargetBody != ven {
			t.Errorf("node %d came back as %+v", i, n)
		}
	}
}
