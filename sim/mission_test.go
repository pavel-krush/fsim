package sim

import (
	"math"
	"testing"
)

// The solar system's data has to be the real data. These are the figures every
// table agrees on, and they are what makes a transfer take the time it takes.
func TestSolarSystemData(t *testing.T) {
	sys := SolarSystem()

	if got := len(sys.Bodies); got < 17 {
		t.Errorf("%d bodies, expected the Sun, eight planets and the major moons", got)
	}
	if sys.Bodies[0].Name != "sun" {
		t.Errorf("root is %q, want the Sun", sys.Bodies[0].Name)
	}

	// Every child is defined after its parent, which is the invariant the whole
	// tree rests on, and every one of them has somewhere to be.
	for i := range sys.Bodies {
		b := &sys.Bodies[i]
		if i == 0 {
			continue
		}
		if b.Parent >= i {
			t.Errorf("%s has parent %d, which is not defined before it", b.Name, b.Parent)
		}
		if b.SemiMajor <= 0 {
			t.Errorf("%s has no orbit", b.Name)
		}
		if b.SOI <= 0 || b.SOI > b.SemiMajor {
			t.Errorf("%s has a sphere of influence of %g m against an orbit of %g", b.Name, b.SOI, b.SemiMajor)
		}
	}

	// Years and months, from the rails rather than from a table.
	period := func(name string) float64 {
		i := sys.IndexOf(name)
		b := &sys.Bodies[i]
		mu := sys.Bodies[b.Parent].Mu
		return 2 * math.Pi * math.Sqrt(b.SemiMajor*b.SemiMajor*b.SemiMajor/mu) / 86400
	}
	close(t, "Earth's year, days", period("earth"), 365.26, 2e-3)
	close(t, "Mars's year, days", period("mars"), 686.98, 3e-3)
	close(t, "Jupiter's year, days", period("jupiter"), 4332.6, 3e-3)
	// The Moon's month is the model month: the rails run on the parent's mu
	// alone, which puts it 0.47% long against the real 27.32 days.
	close(t, "the Moon's month, days", period("moon"), 27.44, 2e-3)
	close(t, "Io's month, days", period("io"), 1.769, 5e-3)
	close(t, "Titan's month, days", period("titan"), 15.945, 5e-3)

	close(t, "the lunar sphere of influence", sys.Bodies[sys.IndexOf("moon")].SOI, 6.61e7, 0.02)
}

// The flagship. Apollo's preset is not just an ascent: it has a translunar
// injection on the plan, and the whole point of the rails, the spheres of
// influence, the adaptive step and the nodes is that flying it arrives somewhere.
func TestApolloPresetReachesTheMoon(t *testing.T) {
	s := New(apolloSaturn().Cfg)
	moon := s.Cfg.System.IndexOf("moon")

	periselene := math.Inf(1)
	for !s.St.Done && s.St.T < 5*86400 {
		if s.advanceOne(s.plannedStepUncapped()) <= 0 {
			break
		}
		if s.St.Center == moon {
			periselene = math.Min(periselene, s.Altitude())
		}
	}

	var arrived, left float64
	for _, e := range s.Events {
		if e.Body != moon {
			continue
		}
		switch e.Kind {
		case EvSOIEnter:
			arrived = e.T
		case EvSOIExit:
			left = e.T
		}
	}
	if arrived == 0 {
		t.Fatalf("never reached the Moon: closest approach was in %s's frame",
			s.Cfg.System.Bodies[s.St.Center].Name)
	}
	t.Logf("lunar sphere entered at T+%.2f days, periselene %.0f km, left at T+%.2f days",
		arrived/86400, periselene/1000, left/86400)

	if arrived/86400 < 2 || arrived/86400 > 4 {
		t.Errorf("arrived at T+%.2f days; a transfer this size takes two to four", arrived/86400)
	}
	// Wide, because a translunar injection is famously sharp: two metres a second
	// of delta-v moves the closest approach by a couple of thousand kilometres.
	// The preset aims for 1800 km, well clear of the surface on purpose.
	if periselene < 300000 || periselene > 6000000 {
		t.Errorf("periselene %.0f km, expected a pass of a few thousand", periselene/1000)
	}
	// It is a flyby, and it has to be: capturing into lunar orbit from here needs
	// some 670 m/s and the S-IVB has 540 left, which is the historical reason
	// Apollo carried a service module with its own engine.
	if left == 0 {
		t.Error("no exit from the lunar sphere: this vehicle cannot capture and should not appear to")
	}
	// And the launch's own verdict is not overwritten by the trip.
	if s.St.Outcome != OutcomeOrbit {
		t.Errorf("outcome = %d, want the orbit the launch achieved", s.St.Outcome)
	}
}

// A closed orbit around a body that is not the launch body is a capture, and the
// verdict has to say which body it is about.
func TestCapturedVerdictNamesTheBody(t *testing.T) {
	sys := earthMoon()
	s := New(Config{System: sys, Atmo: Atmosphere{Top: 140000},
		Rocket: Rocket{Payload: 1000, Diameter: 1}, MaxTime: 1e9})

	moon := &sys.Bodies[1]
	r := moon.Radius + 100000
	s.St.Center = 1
	s.St.Pos = Vec2{0, r}
	s.St.Vel = Vec2{math.Sqrt(moon.Mu / r), 0}
	s.St.Landed = false
	s.St.Phase = PhaseCoast

	s.Step(FixedStep)

	if s.St.Outcome != OutcomeCaptured {
		t.Fatalf("outcome = %d, want captured", s.St.Outcome)
	}
	if s.St.OutcomeBody != 1 {
		t.Errorf("captured by body %d, want the Moon", s.St.OutcomeBody)
	}
	if !s.Settled() {
		t.Error("a lunar orbit is not a verdict?")
	}
}

// Hitting something that is not home is its own verdict.
func TestImpactVerdictNamesTheBody(t *testing.T) {
	sys := earthMoon()
	s := New(Config{System: sys, Atmo: Atmosphere{Top: 140000},
		Rocket: Rocket{Payload: 1000, Diameter: 1}, MaxTime: 1e9})

	// A hundred kilometres up over the Moon, falling straight down.
	s.St.Center = 1
	s.St.Pos = Vec2{0, sys.Bodies[1].Radius + 100000}
	s.St.Vel = Vec2{}
	s.St.Landed = false
	s.St.Phase = PhaseCoast

	for i := 0; i < 100000 && !s.St.Done; i++ {
		s.Step(FixedStep)
	}

	if s.St.Outcome != OutcomeImpact {
		t.Fatalf("outcome = %d, want an impact", s.St.Outcome)
	}
	if s.St.OutcomeBody != 1 {
		t.Errorf("hit body %d, want the Moon", s.St.OutcomeBody)
	}
}

// Escape means leaving the system. A trajectory that is hyperbolic about a moon
// is doing nothing more remarkable than leaving the moon, which every flyby does.
func TestLeavingAMoonIsNotEscape(t *testing.T) {
	sys := earthMoon()
	s := New(Config{System: sys, Atmo: Atmosphere{Top: 140000},
		Rocket: Rocket{Payload: 1000, Diameter: 1}, MaxTime: 1e9})

	moon := &sys.Bodies[1]
	// Well above lunar escape speed, but nowhere near Earth's.
	s.St.Center = 1
	s.St.Pos = Vec2{0, moon.Radius + 500000}
	s.St.Vel = Vec2{2500, 0}
	s.St.Landed = false
	s.St.Phase = PhaseCoast

	o := ComputeOrbit(s.St.Pos, s.St.Vel, moon.Mu)
	if o.Bound() {
		t.Fatal("the test case is not hyperbolic about the Moon")
	}

	for i := 0; i < 20000 && !s.St.Done; i++ {
		s.Step(FixedStep)
	}
	if s.St.Outcome == OutcomeEscape {
		t.Error("leaving the Moon was reported as leaving the system")
	}
	if s.St.Done {
		t.Errorf("the flight ended with outcome %d", s.St.Outcome)
	}
}

// Bodies off the frame's own chain are left out of the sum, and that is a
// correction as much as a saving: the rails give a body the motion of its own
// chain and nothing else, so a distant planet pulls the vehicle without pulling
// the centre it is measured from. Including Jupiter in an Earth-centred flight
// buys three hundred nanometres a second squared of pull that the Earth never
// feels — twenty kilometres of error over four days, against a true differential
// effect of about 1e-11.
func TestDistantPlanetsAreLeftOut(t *testing.T) {
	sys := SolarSystem()
	earth, moon := sys.IndexOf("earth"), sys.IndexOf("moon")
	rel := Vec2{sys.Bodies[earth].Radius + 300000, 0}

	if !sys.Contributes(sys.IndexOf("sun"), earth) {
		t.Error("the Sun is up the Earth's chain and has to count")
	}
	if !sys.Contributes(moon, earth) {
		t.Error("the Moon orbits the Earth and has to count")
	}
	for _, name := range []string{"jupiter", "venus", "io", "titan", "phobos"} {
		if sys.Contributes(sys.IndexOf(name), earth) {
			t.Errorf("%s is off the Earth's chain and should not count", name)
		}
	}

	base := sys.Gravity(earth, rel, 1000)

	// Doubling Jupiter changes nothing near the Earth...
	heavy := SolarSystem()
	j := heavy.IndexOf("jupiter")
	heavy.Bodies[j].Mass *= 2
	heavy.Normalize()
	if got := heavy.Gravity(earth, rel, 1000); got != base {
		t.Errorf("doubling Jupiter moved the acceleration near the Earth by %g m/s^2",
			got.Sub(base).Len())
	}

	// ...but doubling the Moon certainly does, or the sum is not summing.
	heavy = SolarSystem()
	heavy.Bodies[heavy.IndexOf("moon")].Mass *= 2
	heavy.Normalize()
	if got := heavy.Gravity(earth, rel, 1000); got == base {
		t.Error("doubling the Moon changed nothing: the perturbers are not being summed")
	}

	// When the root is the frame, nothing is dropped: it does not move, so every
	// pull on the vehicle is an honest one.
	for i := range sys.Bodies {
		if !sys.Contributes(i, 0) {
			t.Errorf("%s was dropped in the root's own frame", sys.Bodies[i].Name)
		}
	}
}

// The second Apollo preset brakes into lunar orbit instead of passing the Moon.
// This is the whole chain end to end: rails, spheres of influence, the adaptive
// step, staging, two nodes and a verdict that names the body it is about.
func TestApolloLunarPresetEntersLunarOrbit(t *testing.T) {
	s := New(apolloLunar().Cfg)
	moon := s.Cfg.System.IndexOf("moon")

	for !s.St.Done && s.St.T < 5*86400 {
		if s.advanceOne(s.plannedStepUncapped()) <= 0 {
			break
		}
	}

	if s.St.Center != moon {
		t.Fatalf("ended in %s's frame at T+%.2f days", s.Center().Name, s.St.T/86400)
	}
	o := ComputeOrbit(s.St.Pos, s.St.Vel, s.Center().Mu)
	peri, apo := o.PeriapsisAlt(s.Center().Radius), o.ApoapsisAlt(s.Center().Radius)
	t.Logf("lunar orbit %.0f x %.0f km, e %.4f, period %.0f min, service module %.0f%% full",
		peri/1000, apo/1000, o.Eccentricity, o.Period/60,
		s.St.Prop[3]/s.Cfg.Rocket.Stages[3].PropMass*100)

	if !o.Bound() {
		t.Fatal("the orbit around the Moon is not closed")
	}
	if peri < 500000 {
		t.Errorf("periapsis %.0f km: too close to call this an orbit", peri/1000)
	}
	if o.Eccentricity > 0.15 {
		t.Errorf("eccentricity %.3f: the insertion burn is not sized for this approach", o.Eccentricity)
	}
	if s.St.Outcome != OutcomeCaptured || s.St.OutcomeBody != moon {
		t.Errorf("outcome %d about body %d, want captured by the Moon", s.St.Outcome, s.St.OutcomeBody)
	}
	// The S-IVB went overboard with the burn that no longer needed it, and the
	// service module is what is left.
	if s.St.Stage != 3 {
		t.Errorf("flying stage %d, want the fourth", s.St.Stage+1)
	}
	if s.St.Prop[3] <= 0 {
		t.Error("the service module has nothing left, so it cannot have braked with it")
	}
}

// FastForward has to land on a scheduled burn like every other way of advancing.
// It used to roll its own step — "fixed, or the coast target" — which left out the
// clamp to the next node: a ten-minute step sailed up to ten minutes past the
// ignition, and on a lunar insertion that is the difference between an orbit and a
// crater. The screenshots showed it before any test did.
func TestFastForwardLandsOnScheduledBurns(t *testing.T) {
	const at = 40000.0
	s := parked(300000, Node{T: at, Frame: BurnPrograde, DeltaV: 50})

	s.FastForward(at + 5000)

	var lit float64 = -1
	for _, e := range s.Events {
		if e.Kind == EvIgnition {
			lit = e.T
		}
	}
	if lit < 0 {
		t.Fatal("the burn never happened")
	}
	if math.Abs(lit-at) > FixedStep {
		t.Errorf("lit at T+%.3f s, scheduled for T+%.0f — %.1f s late", lit, at, lit-at)
	}
	close(t, "delta-v spent", s.St.DeltaV, 50, 1e-6)
}
