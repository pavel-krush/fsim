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
			// A step that took no time is either the end of the flight or a control point
			// that has just come due — the solve holds the clock until it is finished, and
			// breaking out here would leave the mission stopped at its own correction.
			if s.job != nil {
				s.finishSolve()
				continue
			}
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
	// Tight, because the pass is *held* rather than hoped for: a control point aims it and
	// solves the delta-v when the moment arrives. That is what lets this preset fly where
	// Apollo flew — 200 km — instead of standing off at 1800: a translunar injection is
	// famously sharp, two metres a second move the closest approach by a couple of thousand
	// kilometres, and a fixed number that close to the surface would be a crater waiting for
	// the tenth digit to change.
	if periselene < 150000 || periselene > 260000 {
		t.Errorf("periselene %.0f km, and the aim says 200", periselene/1000)
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
	s := New(Config{System: withAtmoTop(sys, 0, 140000),
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
	s := New(Config{System: withAtmoTop(sys, 0, 140000),
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
	s := New(Config{System: withAtmoTop(sys, 0, 140000),
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
			// A step that took no time is either the end of the flight or a control point
			// that has just come due — the solve holds the clock until it is finished, and
			// breaking out here would leave the mission stopped at its own correction.
			if s.job != nil {
				s.finishSolve()
				continue
			}
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

// The Proton preset: nineteen tonnes to an elliptical orbit, made round by what the
// third stage has left, and the module dropped off with its own tanks still full.
func TestProtonPresetDeliversToTheStationsAltitude(t *testing.T) {
	s := New(protonZvezda().Cfg)

	// The launcher's own job first.
	s.RunToEnd()
	if s.St.Outcome != OutcomeOrbit {
		t.Fatalf("outcome %d at T+%.0f s", s.St.Outcome, s.St.T)
	}
	tm := s.Telemetry()
	t.Logf("insertion at T+%.0f s into %.0f x %.0f km", s.St.T, tm.ApoAlt/1000, tm.PeriAlt/1000)
	if tm.PeriAlt < 200000 {
		t.Errorf("periapsis %.0f km, which is not an orbit anyone would leave a module in", tm.PeriAlt/1000)
	}

	// Then the plan.
	s.FastForward(5000)
	tm = s.Telemetry()
	t.Logf("after circularising: %.0f x %.0f km, e %.4f", tm.ApoAlt/1000, tm.PeriAlt/1000, tm.Ecc)

	if tm.Ecc > 0.02 {
		t.Errorf("eccentricity %.4f: the circularisation burn did not do its job", tm.Ecc)
	}
	// The station is at 408 km and the low side of the orbit is what has to reach it.
	if d := tm.PeriAlt - 408000; d < -60000 || d > 60000 {
		t.Errorf("periapsis %.0f km, %.0f km off the station's altitude", tm.PeriAlt/1000, d/1000)
	}
	// The module is flying alone, with its own propellant untouched.
	if s.St.Stage != 3 {
		t.Errorf("flying stage %d, want the module", s.St.Stage+1)
	}
	if left := s.St.Prop[3]; left < 859 {
		t.Errorf("the module has spent %.0f kg of its own propellant", 860-left)
	}
	if m := s.Mass(); m < 18500 || m > 19500 {
		t.Errorf("mass in orbit %.0f kg, want the module's nineteen tonnes", m)
	}
}

// The geostationary preset: three burns, five and a half hours of coasting, and the
// measure of success is the period rather than the altitude — a belt satellite is
// one whose day matches the planet's.
func TestProtonGeoPresetReachesTheBelt(t *testing.T) {
	s := New(protonGeo().Cfg)
	s.FastForward(30000)

	tm := s.Telemetry()
	const sidereal = 23.9345 * 3600
	t.Logf("%.0f x %.0f km, e %.4f, period %.3f h against the sidereal day's 23.934",
		tm.ApoAlt/1000, tm.PeriAlt/1000, tm.Ecc, tm.Orbit.Period/3600)

	if !tm.Orbit.Bound() {
		t.Fatal("nothing closed")
	}
	// Half a per cent of a day is a drift a satellite can be nudged back from; a
	// preset that misses by more has stopped being geostationary.
	if d := math.Abs(tm.Orbit.Period-sidereal) / sidereal; d > 0.005 {
		t.Errorf("period is %.2f%% off the sidereal day", d*100)
	}
	if tm.Ecc > 0.02 {
		t.Errorf("eccentricity %.4f: the belt is a circle", tm.Ecc)
	}
	// The launcher's third stage went overboard at the first periapsis, and Blok DM
	// did the rest with propellant to spare.
	if s.St.Stage != 3 {
		t.Errorf("flying stage %d, want Blok DM", s.St.Stage+1)
	}
	if left := s.St.Prop[3]; left < 1000 {
		t.Errorf("Blok DM has %.0f kg left: no margin at all", left)
	}
}

// Titan is the awkward one: four times Earth's surface density under a seventh of
// its gravity, and an atmosphere deep enough that a closed orbit starts at 600 km.
// The preset exists to prove the atmosphere model holds up somewhere that is nothing
// like Earth, so the test checks the things that make it strange.
func TestTitanPresetReachesOrbit(t *testing.T) {
	cfg := titanAscent().Cfg
	cfg.EnsureSystem()
	// EnsureSystem derived it already: the air belongs to the body now, and its
	// profile is prepared with that body's own surface gravity.
	at := &cfg.Body.Atmo

	// The air first, because everything else follows from it.
	st := at.State(0)
	close(t, "surface density", st.Density, 5.14, 0.02)
	close(t, "speed of sound", st.Sound, 199, 0.02)
	close(t, "surface gravity", cfg.Body.SurfaceG, 1.354, 0.01)
	if st.Density < 4*1.225 {
		t.Errorf("surface density %.2f kg/m^3 is not four times Earth's", st.Density)
	}

	s := New(titanAscent().Cfg)
	s.RunToEnd()
	tm := s.Telemetry()
	t.Logf("orbit %.0f x %.0f km at T+%.0f s, drag %.0f m/s, gravity %.0f m/s, circular %.0f m/s",
		tm.ApoAlt/1000, tm.PeriAlt/1000, s.St.T, s.St.DragLoss, s.St.GravLoss,
		cfg.Body.CircularSpeed(600000))

	if s.St.Outcome != OutcomeOrbit {
		t.Fatalf("outcome %d: %.0f x %.0f km", s.St.Outcome, tm.ApoAlt/1000, tm.PeriAlt/1000)
	}
	if tm.Ecc > 0.05 {
		t.Errorf("eccentricity %.3f, want something near circular", tm.Ecc)
	}
	// Drag is the whole difficulty here, and a tenth of the budget going into it is
	// the sign that the trajectory is still the tuned one. Ten times that is what
	// happens when the turn comes early.
	if s.St.DragLoss > 1200 {
		t.Errorf("drag losses %.0f m/s: the turn is happening in the thick air", s.St.DragLoss)
	}
	// A seventh of Earth's gravity makes the vertical climb cheap in thrust and
	// expensive in time: seven and a half minutes of it before the turn.
	if s.St.T < 1200 || s.St.T > 3000 {
		t.Errorf("insertion at T+%.0f s, expected the twenty-odd minutes this takes", s.St.T)
	}
}

// The free return: round the Moon and back into the atmosphere on the injection
// burn alone, which is the one thing a trajectory can do that no engine can be
// asked to fix. Nothing fires after T+15295 s, so everything about the arrival
// eight days later was decided by a five-and-a-half-minute burn on the first
// morning — which is why the test checks the arrival and not just the departure.
func TestApolloReturnPresetComesHome(t *testing.T) {
	s := New(apolloReturn().Cfg)
	moon := s.Cfg.System.IndexOf("moon")
	top := s.AtmoTop()

	periSel := math.Inf(1)
	var entryV, entryAng float64
	for !s.St.Done {
		if s.advanceOne(s.plannedStepUncapped()) <= 0 {
			// A step that took no time is either the end of the flight or a control point
			// that has just come due — the solve holds the clock until it is finished, and
			// breaking out here would leave the mission stopped at its own correction.
			if s.job != nil {
				s.finishSolve()
				continue
			}
			break
		}
		if s.St.Center == moon {
			periSel = math.Min(periSel, s.Altitude())
			continue
		}
		// The entry interface, taken the first time it comes back down through the
		// top of the air. The flight path angle here is the whole difference
		// between an entry and an impact.
		if entryV == 0 && s.St.T > 3600 && s.Altitude() < top {
			r, v := s.St.Pos, s.St.Vel
			entryV = v.Len()
			sin := -(r.X*v.X + r.Y*v.Y) / (r.Len() * v.Len())
			entryAng = math.Asin(math.Max(-1, math.Min(1, sin))) * 180 / math.Pi
		}
	}

	t.Logf("past the Moon at %.0f km, home at T+%.2f days, entry %.0f m/s at %.1f deg, peak %.1f g",
		periSel/1000, s.St.T/86400, entryV, entryAng, s.MaxG())

	if s.St.Outcome != OutcomeReturned {
		t.Fatalf("outcome %d at T+%.2f days, want a return", s.St.Outcome, s.St.T/86400)
	}
	if math.IsInf(periSel, 1) {
		t.Fatal("never entered the Moon's sphere of influence")
	}
	if periSel < 0 {
		t.Errorf("periselene %.0f km: that is not a flyby", periSel/1000)
	}
	// Nothing is fired after the injection, and the plan has nothing else in it.
	// A free return that spent propellant on the way would be a different mission.
	if len(s.Cfg.Nodes) != 1 {
		t.Fatalf("%d nodes on the plan, want the injection alone", len(s.Cfg.Nodes))
	}
	if want := s.Cfg.Nodes[0].DeltaV; math.Abs(s.St.DeltaV-(8930+want)) > 400 {
		t.Errorf("spent %.0f m/s in total, want the ascent plus the %.0f m/s injection",
			s.St.DeltaV, want)
	}
	// The corridor. Apollo came in at 6.5 degrees below the horizontal; steeper
	// than about fifteen and the deceleration runs to hundreds of g, shallower
	// than about five and it skips back out and takes another week over it.
	if entryAng < 5 || entryAng > 12 {
		t.Errorf("entry at %.1f degrees below the horizontal, outside the corridor", entryAng)
	}
	if entryV < 10000 || entryV > 11500 {
		t.Errorf("entry speed %.0f m/s, want the escape-speed arrival of a lunar return", entryV)
	}
	// A ballistic dive down that corridor, with no lift anywhere in the model,
	// is about fifteen g. Three hundred means it came in nose-first.
	if g := s.MaxG(); g > 25 {
		t.Errorf("peak %.1f g: this is an impact, not an entry", g)
	}
}

// Mars, which is the whole machine at once: a hundred and eighty-six days, two
// burns two hundred million kilometres apart, and a verdict about a body the
// vehicle was never in the same sphere of influence as when it launched.
//
// It is also the preset that found the last verdict bug. The moment the vehicle
// left the Earth behind, its centre became the Sun — which is not the launch body,
// and a heliocentric orbit is bound and clears the Sun's surface, so it settled as
// a capture. "IN ORBIT AROUND THE SUN", outranking the one it had earned, for the
// six months of the coast.
func TestApolloMarsPresetEntersMarsOrbit(t *testing.T) {
	s := New(apolloMars().Cfg)
	mars := s.Cfg.System.IndexOf("mars")

	var enteredSOI, leftEarth float64
	for !s.St.Done && s.St.T < 200*86400 {
		if s.advanceOne(s.plannedStepUncapped()) <= 0 {
			// A step that took no time is either the end of the flight or a control point
			// that has just come due — the solve holds the clock until it is finished, and
			// breaking out here would leave the mission stopped at its own correction.
			if s.job != nil {
				s.finishSolve()
				continue
			}
			break
		}
		if leftEarth == 0 && s.St.Center == 0 {
			leftEarth = s.St.T
		}
		if enteredSOI == 0 && s.St.Center == mars {
			enteredSOI = s.St.T
		}
		// The heliocentric leg is not a capture, and must never say it is.
		if s.St.Center == 0 && s.St.Outcome == OutcomeCaptured {
			t.Fatalf("settled as a capture by %s at T+%.1f days",
				s.Cfg.System.Bodies[s.St.OutcomeBody].Name, s.St.T/86400)
		}
	}

	if s.St.Center != mars {
		t.Fatalf("ended in %s's frame", s.Center().Name)
	}
	o := ComputeOrbit(s.St.Pos, s.St.Vel, s.Center().Mu)
	peri, apo := o.PeriapsisAlt(s.Center().Radius), o.ApoapsisAlt(s.Center().Radius)
	t.Logf("left the Earth at T+%.2f d, inside Mars's sphere at T+%.2f d, orbit %.0f x %.0f km, e %.4f, period %.1f d, %.0f kg left",
		leftEarth/86400, enteredSOI/86400, peri/1000, apo/1000, o.Eccentricity, o.Period/86400, s.St.Prop[3])

	if s.St.Outcome != OutcomeCaptured || s.St.OutcomeBody != mars {
		t.Fatalf("outcome %d about body %d, want captured by Mars", s.St.Outcome, s.St.OutcomeBody)
	}
	if !o.Bound() {
		t.Fatal("the orbit around Mars is not closed")
	}
	// High, and it has to be: the vehicle arrives with 3 km/s of hyperbolic excess
	// and the service module can only take 2410 m/s off it. What matters is that
	// it is comfortably inside the sphere of influence rather than scraping the
	// edge of it, where the Sun would take it back.
	if soi := s.Cfg.System.Bodies[mars].SOI; apo > soi/2 {
		t.Errorf("apoapsis %.0f km against a sphere of influence of %.0f km", apo/1000, soi/1000)
	}
	if o.Eccentricity > 0.1 {
		t.Errorf("eccentricity %.3f: the braking burn is not sized for this approach", o.Eccentricity)
	}
	if s.St.Prop[3] <= 0 {
		t.Error("the service module is dry, so it cannot have braked with what it had")
	}
	// The S-IVB was dropped with the injection, so the vehicle at Mars is the
	// spacecraft alone.
	if s.St.Stage != 3 {
		t.Errorf("flying stage %d, want the fourth", s.St.Stage+1)
	}
	if enteredSOI < 150*86400 || enteredSOI > 200*86400 {
		t.Errorf("arrival at T+%.1f days, want the hundred and eighty-six the transfer takes",
			enteredSOI/86400)
	}
}

// The launch window is data: Mars's mean anomaly at t = 0 is what puts it where the
// transfer crosses its orbit. Everything else about the system's phasing is a
// picture choice, so this one is worth stating out loud.
func TestMarsWindowPhasing(t *testing.T) {
	sys := SolarSystem()
	m := &sys.Bodies[sys.IndexOf("mars")]
	if math.Abs(m.MeanAnom0-5.9975) > 1e-9 {
		t.Errorf("Mars's mean anomaly is %.4f: the apollo-mars transfer is tuned to 5.9975", m.MeanAnom0)
	}
}

// The Mun: the same machinery on a system nobody has to look up, and the smallest
// target in the collection. Two hundred kilometres of radius and a sphere of
// influence twelve radii wide, which is what makes the aiming interesting.
func TestKerbinMunPresetEntersMunOrbit(t *testing.T) {
	// The invented system first, because the mission is only as good as its data.
	sys := kerbinSystem()
	mun := &sys.Bodies[1]
	close(t, "Kerbin's surface gravity", sys.Bodies[0].SurfaceG, 9.81, 0.001)
	close(t, "the Mun's surface gravity", mun.SurfaceG, 1.6285, 0.001)
	close(t, "the Mun's sphere of influence", mun.SOI, 2430000, 0.01)
	close(t, "the Mun's orbital period", sys.Period(1), 138982, 0.01)

	s := New(kerbinMun().Cfg)
	var enter float64
	peri := math.Inf(1)
	for !s.St.Done && s.St.T < 12*3600 {
		if s.advanceOne(s.plannedStepUncapped()) <= 0 {
			// A step that took no time is either the end of the flight or a control point
			// that has just come due — the solve holds the clock until it is finished, and
			// breaking out here would leave the mission stopped at its own correction.
			if s.job != nil {
				s.finishSolve()
				continue
			}
			break
		}
		if s.St.Center == 1 {
			if enter == 0 {
				enter = s.St.T
			}
			peri = math.Min(peri, s.Altitude())
		}
	}

	o := ComputeOrbit(s.St.Pos, s.St.Vel, s.Center().Mu)
	pa, aa := o.PeriapsisAlt(s.Center().Radius), o.ApoapsisAlt(s.Center().Radius)
	t.Logf("inside the Mun's sphere at T+%.0f s, closest %.0f km, orbit %.0f x %.0f km, e %.4f, period %.0f min, %.0f kg left",
		enter, peri/1000, pa/1000, aa/1000, o.Eccentricity, o.Period/60, s.St.Prop[1])

	if s.St.Outcome != OutcomeCaptured || s.St.OutcomeBody != 1 {
		t.Fatalf("outcome %d about body %d, want captured by the Mun", s.St.Outcome, s.St.OutcomeBody)
	}
	if !o.Bound() || pa < 20000 {
		t.Errorf("orbit %.0f x %.0f km: too close to call this an orbit", pa/1000, aa/1000)
	}
	if o.Eccentricity > 0.08 {
		t.Errorf("eccentricity %.3f: the braking burn is not sized for this approach", o.Eccentricity)
	}
	// It has to fit inside the sphere of influence with room to spare, which on a
	// body this small is the constraint that bites.
	if aa+mun.Radius > mun.SOI/2 {
		t.Errorf("apoapsis %.0f km from the centre against a sphere of influence of %.0f km",
			(aa+mun.Radius)/1000, mun.SOI/1000)
	}
	if s.St.Prop[1] <= 0 {
		t.Error("the second stage is dry, so it cannot have braked with what it had")
	}
}

// The single-planet Kerbin stays single-planet. Its figures are quoted, and a Mun
// three thousand kilometres away with a gravity of its own would move them.
func TestPlainKerbinHasNoMun(t *testing.T) {
	cfg := kerbinAscent().Cfg
	cfg.EnsureSystem()
	if n := len(cfg.System.Bodies); n != 1 {
		t.Errorf("the kerbin preset flies in a system of %d bodies", n)
	}
}

// Io: the smallest sphere of influence anything launches into here. Four and a
// third radii, so an orbit round it is a thing there is barely room for, and
// leaving is 750 m/s — after which Jupiter has the vehicle and the verdict says so.
func TestIoPresetLeavesForJupiter(t *testing.T) {
	s := New(ioJupiter().Cfg)
	sys := &s.Cfg.System
	io, jupiter := sys.IndexOf("io"), sys.IndexOf("jupiter")

	// What makes it awkward, stated as data.
	b := &sys.Bodies[io]
	close(t, "Io's circular speed at the surface", b.CircularSpeed(0), 1809, 0.01)
	if r := b.SOI / b.Radius; r < 4 || r > 4.6 {
		t.Errorf("Io's sphere of influence is %.2f radii wide", r)
	}

	// The parking orbit has to fit inside it with room to spare, which is the
	// constraint that does not exist anywhere else in the collection.
	s.RunToEnd()
	tm := s.Telemetry()
	if s.St.Outcome != OutcomeOrbit {
		t.Fatalf("the ascent ends with outcome %d", s.St.Outcome)
	}
	if tm.ApoAlt+b.Radius > b.SOI/4 {
		t.Errorf("apoapsis %.0f km from the centre against a sphere of influence of %.0f km",
			(tm.ApoAlt+b.Radius)/1000, b.SOI/1000)
	}

	// And then the departure. A month, because the thing worth checking is that it
	// does not wander back in: the orbit it ends up in crosses Io's own, and the
	// neighbouring values of the burn come back through the sphere of influence.
	s = New(ioJupiter().Cfg)
	var leftIo float64
	crossings := 0
	last := s.St.Center
	for !s.St.Done && s.St.T < 30*86400 {
		if s.advanceOne(s.plannedStepUncapped()) <= 0 {
			// A step that took no time is either the end of the flight or a control point
			// that has just come due — the solve holds the clock until it is finished, and
			// breaking out here would leave the mission stopped at its own correction.
			if s.job != nil {
				s.finishSolve()
				continue
			}
			break
		}
		if s.St.Center != last {
			crossings++
			last = s.St.Center
		}
		if leftIo == 0 && s.St.Center != io {
			leftIo = s.St.T
		}
	}
	o := ComputeOrbit(s.St.Pos, s.St.Vel, s.Center().Mu)
	t.Logf("left Io at T+%.0f s, %d frame changes in thirty days, orbit round %s %.0f x %.0f Mm at e %.3f",
		leftIo, crossings, s.Center().Name, o.PeriapsisAlt(s.Center().Radius)/1e6,
		o.ApoapsisAlt(s.Center().Radius)/1e6, o.Eccentricity)

	if s.St.Outcome != OutcomeCaptured || s.St.OutcomeBody != jupiter {
		t.Fatalf("outcome %d about body %d, want captured by Jupiter", s.St.Outcome, s.St.OutcomeBody)
	}
	if s.St.Center != jupiter {
		t.Errorf("ended in %s's frame", s.Center().Name)
	}
	if crossings != 1 {
		t.Errorf("%d frame changes: the vehicle went back through Io's sphere of influence", crossings)
	}
	if !o.Bound() {
		t.Error("the orbit round Jupiter is not closed")
	}
	// Outwards, not inwards. The same burn earlier in the parking orbit drops the
	// vehicle to 342 Mm, inside Io's orbit, which is the wrong way to leave.
	if apo := o.ApoapsisAlt(s.Center().Radius) + s.Center().Radius; apo < sys.Bodies[io].SemiMajor {
		t.Errorf("apoapsis %.0f Mm, inside Io's own orbit at %.0f Mm",
			apo/1e6, sys.Bodies[io].SemiMajor/1e6)
	}
}

// The grand tour: four gravity assists on one injection, each planet put where the
// flight crosses its orbit. What this guards is the *chain* — a preset whose windows have
// drifted loses the tour at the first planet and sails on through empty space, which is
// exactly what one rounded pitch keyframe did while this was being found.
//
// Flown in a single FastForward and read out of the events afterwards. Polling for the
// encounters costs six times as much: asking for the state every two days holds the
// adaptive step down to two days, where left alone it grows to months and the error
// control is what keeps it honest.
func TestVoyagerTourFliesPastFourPlanets(t *testing.T) {
	p := voyagerTour()
	s := New(p.Cfg)
	s.FastForward(p.Cfg.MaxTime)

	want := []struct {
		name             string
		earliest, latest float64 // years
	}{
		{"jupiter", 1.5, 2.5},
		{"saturn", 4.0, 5.0},
		{"uranus", 11.5, 13.0},
		{"neptune", 20.5, 22.5},
	}

	// The encounters are now *held* by a control point apiece rather than left to the chain,
	// which is what makes them worth asserting tightly. Without them this tour is only true at
	// the step size it was tuned on: integrated finely it passes Uranus at 167 radii on the
	// wrong side and misses Neptune by 894 million kilometres. See TestTheTourHoldsHowever-
	// ItIsAdvanced, which is the claim in its own right.

	// Planets only. The departure threads the Moon's sphere of influence on the way out,
	// which is a real encounter and not one the tour was aimed at.
	planet := func(i int) bool { return s.Cfg.System.Bodies[i].Parent == 0 }

	got := 0
	for _, e := range s.Events {
		if e.Kind != EvSOIEnter || !planet(e.Body) {
			continue
		}
		name := s.Cfg.System.Bodies[e.Body].Name
		if got >= len(want) {
			t.Fatalf("an extra encounter with %s at T+%.2f y", name, e.T/(365.25*86400))
		}
		w := want[got]
		y := e.T / (365.25 * 86400)
		if name != w.name {
			t.Fatalf("encounter %d is %s at T+%.2f y, expected %s", got+1, name, y, w.name)
		}
		if y < w.earliest || y > w.latest {
			t.Errorf("%s at T+%.2f y, expected between %.1f and %.1f y",
				w.name, y, w.earliest, w.latest)
		}
		got++
	}
	if got != len(want) {
		t.Fatalf("%d of the four planets were met", got)
	}

	// Each pass has to be a pass and not a collision, and close enough to be one at
	// all. The history is coarse inside a flyby, so this is a bound and not a
	// measurement: the numbers it is bounding are 41, 161, 10 and 127 radii.
	for _, e := range s.Events {
		if e.Kind != EvSOIEnter || !planet(e.Body) {
			continue
		}
		b := &s.Cfg.System.Bodies[e.Body]
		best := math.Inf(1)
		for _, h := range s.Hist {
			if h.Center == e.Body {
				best = math.Min(best, h.Pos.Len())
			}
		}
		switch {
		case best < b.Radius:
			t.Errorf("%s: the vehicle went through it", b.Name)
		case best > 400*b.Radius:
			t.Errorf("%s passed at %.0f radii, which is not a flyby", b.Name, best/b.Radius)
		}
	}
}

// Parker is the fastest thing here, the only one that spends its energy going down, and the one
// whose mission is a chain rather than a trajectory: two Venus flybys walk the perihelion from
// 39 solar radii to 29.
//
// The flybys are aims rather than numbers — control points that re-solve when the moment arrives
// — and what that buys is measured in TestTheParkerChainHoldsHoweverItIsAdvanced: the mission's
// own figures come out the same whether the flight is flown in hourly steps or in one jump.
func TestParkerReachesTheCorona(t *testing.T) {
	p := parkerSolar()
	s := New(p.Cfg)
	// Flown in one jump and read out of the events afterwards, which is both cheaper and
	// stricter than polling: the encounters are where refocus said they were.
	s.FastForward(p.Cfg.MaxTime)

	ven := s.Cfg.System.IndexOf("venus")
	var venusPasses []float64
	for _, e := range s.Events {
		if e.Kind == EvSOIEnter && e.Body == ven {
			venusPasses = append(venusPasses, e.T/86400)
		}
	}
	if len(venusPasses) != 2 {
		t.Fatalf("%d Venus encounters at %v days, expected two", len(venusPasses), venusPasses)
	}
	for i, want := range []float64{45, 495} {
		if d := venusPasses[i]; math.Abs(d-want) > 10 {
			t.Errorf("flyby %d at T+%.0f d, expected T+%.0f", i+1, d, want)
		}
	}

	// Both aims reached, and inside what the hydrazine holds.
	total := 0.0
	for i, n := range s.Cfg.Nodes {
		if n.Target == TargetNone {
			continue
		}
		if !n.Solved || n.Missed {
			t.Errorf("aim %d solved %v, missed %v", i, n.Solved, n.Missed)
		}
		total += n.DeltaV
	}
	if total > 178 {
		t.Errorf("the corrections spent %.0f m/s of 178 aboard", total)
	}

	sun := &s.Cfg.System.Bodies[0]
	best, fastest := math.Inf(1), 0.0
	for _, h := range s.Hist {
		if h.Center != 0 {
			continue
		}
		best = math.Min(best, h.Pos.Len())
		fastest = math.Max(fastest, h.Speed)
	}
	if r := best / sun.Radius; r > 30 {
		t.Errorf("closest approach %.1f solar radii: two flybys should reach 29.2", r)
	}
	if fastest < 105000 {
		t.Errorf("fastest %.0f m/s, and 29 radii is worth 107 km/s", fastest)
	}
	// Inside Mercury's orbit, which is the point of the mission.
	if merc := s.Cfg.System.Bodies[s.Cfg.System.IndexOf("mercury")]; best > merc.SemiMajor {
		t.Errorf("closest approach %.3g m does not reach inside Mercury's orbit (%.3g m)",
			best, merc.SemiMajor)
	}
}

// What the corrections are *for*: a mission whose figures do not depend on how the flight was
// advanced. A chain of flybys amplifies everything, and the arithmetic of a coast is something to
// amplify — the same Parker flight advanced in hourly steps and in twelve-hourly ones arrives at
// Venus 11 radii apart. With an aim on each pass the mission comes out the same anyway: both
// encounters, the same perihelion, the same speed.
//
// Before the aims this was not true of either chained preset, and for the grand tour it was not
// true at all: finely integrated it lost Uranus and Neptune entirely.
func TestTheParkerChainHoldsHoweverItIsAdvanced(t *testing.T) {
	type result struct {
		encounters         int
		periRadii, fastKps float64
	}
	run := func(poll float64) result {
		s := New(parkerSolar().Cfg)
		ven := s.Cfg.System.IndexOf("venus")
		peri, fastest := math.Inf(1), 0.0
		for s.St.T < s.Cfg.MaxTime && !s.St.Done {
			if poll <= 0 {
				s.FastForward(s.Cfg.MaxTime)
			} else {
				s.FastForward(s.St.T + poll)
			}
			if s.St.Center == 0 {
				peri = math.Min(peri, s.RootPos().Len())
				fastest = math.Max(fastest, s.St.Vel.Len())
			}
			if poll <= 0 {
				break
			}
		}
		if poll <= 0 {
			for _, h := range s.Hist {
				if h.Center == 0 {
					peri = math.Min(peri, h.Pos.Len())
					fastest = math.Max(fastest, h.Speed)
				}
			}
		}
		n := 0
		for _, e := range s.Events {
			if e.Kind == EvSOIEnter && e.Body == ven {
				n++
			}
		}
		return result{n, peri / s.Cfg.System.Bodies[0].Radius, fastest / 1000}
	}

	// One jump, hourly, and a couple of days at a time. The last is what a flight at high warp
	// actually takes.
	want := run(0)
	if want.encounters != 2 {
		t.Fatalf("one jump gave %d Venus encounters", want.encounters)
	}
	for _, poll := range []float64{3600, 12 * 3600, 2 * 86400} {
		got := run(poll)
		// A radius of perihelion and a couple of kilometres a second: what is being asserted
		// is that the mission is the same mission, not that two integrations agree bit for
		// bit — they cannot, and the corrections are what makes that stop mattering.
		switch {
		case got.encounters != want.encounters:
			t.Errorf("advanced %.0f s at a time: %d Venus encounters, against %d in one jump",
				poll, got.encounters, want.encounters)
		case math.Abs(got.periRadii-want.periRadii) > 1:
			t.Errorf("advanced %.0f s at a time: perihelion %.1f solar radii, against %.1f",
				poll, got.periRadii, want.periRadii)
		case math.Abs(got.fastKps-want.fastKps) > 2:
			t.Errorf("advanced %.0f s at a time: %.0f km/s, against %.0f",
				poll, got.fastKps, want.fastKps)
		}
	}
}
