package sim

import (
	"fmt"
	"math"
	"testing"
)

func close(t *testing.T, name string, got, want, tol float64) {
	t.Helper()
	if want == 0 {
		if math.Abs(got) > tol {
			t.Errorf("%s = %g, want 0 +/- %g", name, got, tol)
		}
		return
	}
	if rel := math.Abs(got-want) / math.Abs(want); rel > tol {
		t.Errorf("%s = %g, want %g (off by %.3f%%, tolerance %.3f%%)",
			name, got, want, rel*100, tol*100)
	}
}

func earthAtmo() *Atmosphere {
	a := earthFalcon().Cfg.Body.Atmo
	a.Prepare(9.80665)
	return &a
}

// The layered model must reproduce the published ISA table. These are the
// reference values every atmosphere table agrees on, so anything off by more
// than a fraction of a percent means the barometric chain is wrong.
func TestISATable(t *testing.T) {
	a := earthAtmo()

	cases := []struct {
		alt, temp, press, rho float64
	}{
		{0, 288.15, 101325, 1.2250},
		{5000, 255.65, 54019, 0.73612},
		{11000, 216.65, 22632, 0.36391},
		{20000, 216.65, 5474.9, 0.088035},
		{32000, 228.65, 868.02, 0.013225},
		{47000, 270.65, 110.91, 0.0014275},
	}
	for _, c := range cases {
		st := a.State(c.alt)
		close(t, "T", st.Temp, c.temp, 0.001)
		close(t, "P", st.Pressure, c.press, 0.005)
		close(t, "rho", st.Density, c.rho, 0.005)
	}
}

func TestAtmosphereMixture(t *testing.T) {
	a := earthAtmo()
	close(t, "molar mass", a.MolarMass(), 0.0289644, 0.001)
	close(t, "gamma", a.Gamma(), 1.4, 0.005)
	close(t, "speed of sound", a.State(0).Sound, 340.29, 0.002)
	close(t, "speed of sound at 11 km", a.State(11000).Sound, 295.07, 0.002)
}

// Above the declared top of the atmosphere there must be no air at all,
// otherwise orbits would decay forever.
func TestAtmosphereVacuumAbove(t *testing.T) {
	a := earthAtmo()
	if d := a.State(200000).Density; d != 0 {
		t.Errorf("density at 200 km = %g, want 0", d)
	}
	empty := Atmosphere{}
	empty.Prepare(9.81)
	if !empty.IsVacuum() {
		t.Error("an atmosphere with no layers should be a vacuum")
	}
	if d := empty.State(0).Density; d != 0 {
		t.Errorf("vacuum density = %g, want 0", d)
	}
}

func TestBodyDerivations(t *testing.T) {
	b := Body{Radius: 6371000, MassSource: FromMass, Mass: 5.97237e24}
	b.Normalize()
	close(t, "g0", b.SurfaceG, 9.8196, 0.001)
	close(t, "density", b.Density, 5514, 0.001)
	close(t, "mu", b.Mu, 3.986e14, 0.001)

	// Entering the same planet through surface gravity must recover the mass.
	c := Body{Radius: 6371000, MassSource: FromSurfaceG, SurfaceG: b.SurfaceG}
	c.Normalize()
	close(t, "mass from g", c.Mass, b.Mass, 1e-9)

	d := Body{Radius: 6371000, MassSource: FromDensity, Density: b.Density}
	d.Normalize()
	close(t, "mass from density", d.Mass, b.Mass, 1e-9)
}

func TestOrbitCircular(t *testing.T) {
	mu := 3.986004418e14
	r := 6771000.0
	v := math.Sqrt(mu / r)
	o := ComputeOrbit(Vec2{r, 0}, Vec2{0, v}, mu)

	close(t, "semi-major axis", o.SemiMajor, r, 1e-9)
	if o.Eccentricity > 1e-12 {
		t.Errorf("eccentricity = %g, want 0", o.Eccentricity)
	}
	close(t, "apoapsis", o.Apoapsis, r, 1e-9)
	close(t, "periapsis", o.Periapsis, r, 1e-9)
	// A 400 km circular orbit takes about 92.5 minutes.
	close(t, "period", o.Period, 5546, 0.01)
}

func TestOrbitHyperbolic(t *testing.T) {
	mu := 3.986004418e14
	r := 6771000.0
	v := 1.2 * math.Sqrt(2*mu/r)
	o := ComputeOrbit(Vec2{r, 0}, Vec2{0, v}, mu)
	if o.Bound() {
		t.Error("a trajectory above escape speed must not be bound")
	}
	if o.Eccentricity <= 1 {
		t.Errorf("eccentricity = %g, want > 1", o.Eccentricity)
	}
}

// A coasting circular orbit must come back to where it started after one
// period, and its shape must not drift. This is the integrator's accuracy test.
func TestCircularOrbitIsStable(t *testing.T) {
	b := Body{Radius: 6371000, MassSource: FromMass, Mass: 5.97237e24}
	b.Normalize()

	// A vacuum whose nominal top is above the orbit, so the run does not stop
	// the instant it notices we are already in orbit.
	b.Atmo = Atmosphere{Top: 1e9}
	cfg := Config{
		Body:    b,
		Rocket:  Rocket{Payload: 1000, Diameter: 1},
		MaxTime: 1e9,
	}
	s := New(cfg)

	r := b.Radius + 400000
	s.St.Pos = Vec2{r, 0}
	s.St.Vel = Vec2{0, math.Sqrt(b.Mu / r)}
	s.St.Landed = false
	s.St.Phase = PhaseCoast

	o0 := ComputeOrbit(s.St.Pos, s.St.Vel, b.Mu)
	for n := int(o0.Period / FixedStep); n > 0; n-- {
		s.Step(FixedStep)
		if s.St.Done {
			t.Fatalf("coasting orbit terminated early at t=%.1f: %v", s.St.T, s.St.Outcome)
		}
	}

	o1 := ComputeOrbit(s.St.Pos, s.St.Vel, b.Mu)
	close(t, "radius after one orbit", s.St.Pos.Len(), r, 1e-6)
	close(t, "semi-major axis after one orbit", o1.SemiMajor, o0.SemiMajor, 1e-8)
	if o1.Eccentricity > 1e-6 {
		t.Errorf("eccentricity drifted to %g after one orbit", o1.Eccentricity)
	}
}

// Dropping a mass in a uniform-ish field must match s = g*t^2/2.
func TestFreeFall(t *testing.T) {
	b := Body{Radius: 6371000, MassSource: FromMass, Mass: 5.97237e24}
	b.Normalize()

	cfg := Config{Body: b, Rocket: Rocket{Payload: 1000, Diameter: 1}, MaxTime: 1e9}
	s := New(cfg)

	h0 := 1000.0
	s.St.Pos = Vec2{b.Radius + h0, 0}
	s.St.Vel = Vec2{}
	s.St.Landed = false
	s.St.Phase = PhaseCoast

	for n := 500; n > 0; n-- {
		s.Step(FixedStep)
	}

	dur := s.St.T
	fallen := h0 - s.Altitude()
	// The field strengthens slightly during the drop, so the uniform-gravity
	// answer is evaluated at the midpoint of the fall rather than at release.
	want := b.GravityAt(b.Radius+h0-fallen/2) * dur * dur / 2
	close(t, "elapsed time", dur, 10, 1e-12)
	close(t, "distance fallen", fallen, want, 1e-4)
}

// A burn in a vacuum with no gravity must deliver exactly the Tsiolkovsky
// delta-v. This pins the propellant bookkeeping and the thrust model together.
func TestTsiolkovsky(t *testing.T) {
	b := Body{Radius: 1e12, MassSource: FromMass, Mass: 1e-6}
	b.Normalize()

	stage := Stage{
		DryMass: 1000, PropMass: 9000,
		ThrustVac: 100000, IspVac: 300, IspSL: 300,
	}
	cfg := Config{
		Body:    b,
		Rocket:  Rocket{Payload: 0, Cd: 0, Diameter: 1, Stages: []Stage{stage}},
		Program: Program{Keys: []Keyframe{{Time: 0, Pitch: 90}}},
		MaxTime: 1e9,
	}
	s := New(cfg)
	s.St.Pos = Vec2{b.Radius + 10000, 0}
	s.St.Landed = false

	want := 300 * G0 * math.Log(10000.0/1000.0)
	for !s.St.Done && s.St.Phase == PhaseBurn {
		s.Step(FixedStep)
	}

	close(t, "achieved speed", s.St.Vel.Sub(Vec2{0, 0}).Len(), want, 1e-4)
	close(t, "accounted delta-v", s.St.DeltaV, want, 1e-4)
	close(t, "final mass", s.Mass(), 1000, 1e-9)
	close(t, "burn time", s.St.T, stage.BurnTime(), 1e-6)
}

func TestStageDeltaVBookkeeping(t *testing.T) {
	r := earthFalcon().Cfg.Rocket
	close(t, "liftoff mass", r.LiftoffMass(), 536970, 1e-9)
	// Both stages together must be able to reach low Earth orbit with margin.
	if dv := r.TotalDeltaV(); dv < 9000 || dv > 12000 {
		t.Errorf("total ideal delta-v = %.0f m/s, expected a launcher-sized 9-12 km/s", dv)
	}
	if twr := r.LiftoffTWR(101325, 9.8196); twr < 1.3 || twr > 1.6 {
		t.Errorf("liftoff TWR = %.2f, expected roughly 1.4", twr)
	}
}

// A rocket that cannot lift its own weight has to stay put and waste its
// propellant, not sink through the planet.
func TestUnderpoweredStaysOnPad(t *testing.T) {
	cfg := earthFalcon().Cfg
	cfg.Rocket.Stages[0].ThrustVac = 1e6 // far below the 5.2 MN it weighs
	s := New(cfg)

	for i := 0; i < 500; i++ {
		s.Step(FixedStep)
	}
	if !s.St.Landed {
		t.Error("an underpowered rocket should still be on the pad")
	}
	if alt := s.Altitude(); math.Abs(alt) > 1e-6 {
		t.Errorf("altitude = %g m, want 0", alt)
	}
	if s.St.Prop[0] >= cfg.Rocket.Stages[0].PropMass {
		t.Error("the engine should still be consuming propellant on the pad")
	}
}

// Advance is what the flight screen calls every frame, so it must land on
// exactly the same state as an equivalent run of fixed steps. A leftover
// fraction of a step at the end of a call must not disturb the burn.
func TestAdvanceMatchesFixedSteps(t *testing.T) {
	ref := New(earthFalcon().Cfg)
	for n := 900; n > 0; n-- {
		ref.Step(FixedStep)
	}

	// Same 18 seconds, but fed in ragged chunks the way frame times arrive.
	got := New(earthFalcon().Cfg)
	for _, chunk := range []float64{5, 0.0166, 7.3, 0.4, 5.2834} {
		got.Advance(chunk)
	}

	close(t, "time", got.St.T, ref.St.T, 1e-9)
	close(t, "propellant", got.St.Prop[0], ref.St.Prop[0], 1e-9)
	close(t, "altitude", got.Altitude(), ref.Altitude(), 1e-9)
	if got.St.Phase != ref.St.Phase {
		t.Errorf("phase = %v, want %v", got.St.Phase, ref.St.Phase)
	}
	if got.St.Prop[0] <= 0 {
		t.Error("the first stage should still have propellant 18 seconds in")
	}
}

// The peak dynamic pressure is only knowable in hindsight, but the marker has
// to show up during the flight rather than only on the graph screen — and it
// has to land on the right instant and in the right place in the timeline.
func TestMaxQMarkerAppearsDuringFlight(t *testing.T) {
	s := New(earthFalcon().Cfg)

	marked := -1.0
	for !s.St.Done && s.St.T < 600 {
		s.Step(FixedStep)
		if marked < 0 && hasEvent(s.Events, EvMaxQ) {
			marked = s.St.T
		}
	}
	if marked < 0 {
		t.Fatal("the max-q marker never appeared")
	}

	peak, alt := s.MaxQ()
	var ev Event
	for _, e := range s.Events {
		if e.Kind == EvMaxQ {
			ev = e
		}
	}
	close(t, "marker time", ev.T, s.MaxQTime(), 1e-9)
	close(t, "peak pressure", peak, 43200, 0.02)
	close(t, "peak altitude", alt, 11000, 0.1)

	if marked <= ev.T {
		t.Error("the marker cannot be emitted before the peak it marks has passed")
	}
	if marked > ev.T+60 {
		t.Errorf("marker emitted at T+%.0f for a peak at T+%.0f — too late to be useful", marked, ev.T)
	}
	// Emitting it late must not leave the timeline out of order.
	for i := 1; i < len(s.Events); i++ {
		if s.Events[i].T < s.Events[i-1].T {
			t.Fatalf("events out of order: kind %d at %.1f after kind %d at %.1f",
				s.Events[i].Kind, s.Events[i].T, s.Events[i-1].Kind, s.Events[i-1].T)
		}
	}
	// MarkMaxQ is the end-of-flight fallback and must not double up.
	before := len(s.Events)
	s.MarkMaxQ()
	if len(s.Events) != before {
		t.Error("MarkMaxQ added a second marker")
	}
}

// Opening the graph screen mid-ascent calls MarkMaxQ. That must not plant the
// marker on whatever the running maximum happens to be at that moment — the
// peak has not happened yet, and the flag would stop the automatic detection
// from ever correcting it.
func TestMarkMaxQIgnoredWhileFlying(t *testing.T) {
	s := New(earthFalcon().Cfg)

	// Climb to a point where the pressure is still building towards the peak.
	for s.St.T < 40 {
		s.Step(FixedStep)
	}
	early, _ := s.MaxQ()
	earlyT := s.MaxQTime()

	s.MarkMaxQ()
	if hasEvent(s.Events, EvMaxQ) {
		t.Fatal("the marker was planted before the peak had even happened")
	}

	s.RunToEnd()
	if !hasEvent(s.Events, EvMaxQ) {
		t.Fatal("the marker never appeared after the flight finished")
	}

	peak, _ := s.MaxQ()
	if peak <= early {
		t.Fatalf("the test is not exercising the case: pressure at T+40 (%.0f Pa) "+
			"was already the peak (%.0f Pa)", early, peak)
	}
	for _, e := range s.Events {
		if e.Kind == EvMaxQ {
			close(t, "marker time", e.T, s.MaxQTime(), 1e-9)
			if math.Abs(e.T-earlyT) < 1e-9 {
				t.Error("the marker is stuck on the running maximum from T+40")
			}
		}
	}
}

// A flight cut off while the pressure is still near its peak never gives the
// automatic check the drop-off it waits for, so ending the flight has to plant
// the marker itself.
func TestMaxQMarkedWhenFlightEndsAtThePeak(t *testing.T) {
	cfg := earthFalcon().Cfg
	cfg.MaxTime = 65 // a couple of seconds past the peak, nowhere near the falloff

	s := New(cfg)
	s.RunToEnd()

	if s.St.Outcome != OutcomeTimeout {
		t.Fatalf("expected the run to be cut short, got outcome %d", s.St.Outcome)
	}
	q, _ := s.MaxQ()
	if s.lastQ < q*0.25 {
		t.Fatal("the test is not exercising the fallback: the automatic check would have fired")
	}
	if !hasEvent(s.Events, EvMaxQ) {
		t.Error("a flight that ended at the peak still needs its max-q marker")
	}
	for _, e := range s.Events {
		if e.Kind == EvMaxQ {
			close(t, "marker time", e.T, s.MaxQTime(), 1e-9)
		}
	}
	for i := 1; i < len(s.Events); i++ {
		if s.Events[i].T < s.Events[i-1].T {
			t.Fatalf("events out of order around kind %d at %.1f", s.Events[i].Kind, s.Events[i].T)
		}
	}
}

func hasEvent(events []Event, k EventKind) bool {
	for _, e := range events {
		if e.Kind == k {
			return true
		}
	}
	return false
}

func TestPitchProgramInterpolation(t *testing.T) {
	p := Program{Keys: []Keyframe{
		{Time: 0, Pitch: 90},
		{Time: 100, Pitch: 50},
		{Time: 200, Prograde: true},
	}}
	close(t, "before the first key", p.Pitch(-5, 0), 90, 1e-12)
	close(t, "midway", p.Pitch(50, 0), 70, 1e-12)
	close(t, "on a key", p.Pitch(100, 0), 50, 1e-12)
	// A prograde key resolves to the current flight path angle, and the
	// segment leading up to it blends smoothly into that value.
	close(t, "at the prograde key", p.Pitch(300, 12), 12, 1e-12)
	close(t, "blending into prograde", p.Pitch(150, 10), 30, 1e-12)
}

func TestFlightPathAngle(t *testing.T) {
	up := Vec2{1, 0}
	close(t, "straight up", FlightPathAngle(up, Vec2{100, 0}), 90, 1e-9)
	close(t, "horizontal", FlightPathAngle(up, Vec2{0, 100}), 0, 1e-9)
	close(t, "45 degrees", FlightPathAngle(up, Vec2{100, 100}), 45, 1e-9)
	close(t, "descending", FlightPathAngle(up, Vec2{-100, 0}), -90, 1e-9)
}

func TestThrustDirection(t *testing.T) {
	up, east := Vec2{1, 0}, Vec2{0, 1}
	d := ThrustDirection(up, east, 90)
	close(t, "vertical x", d.X, 1, 1e-12)
	close(t, "vertical y", d.Y, 0, 1e-12)

	d = ThrustDirection(up, east, 0)
	close(t, "horizontal x", d.X, 0, 1e-12)
	close(t, "horizontal y", d.Y, 1, 1e-12)
}

// The headline check: the default Earth configuration must actually make orbit,
// and the delta-v it spends getting there must land in the range real
// launchers need.
func TestEarthPresetReachesOrbit(t *testing.T) {
	s := New(earthFalcon().Cfg)
	s.RunToEnd()

	if s.St.Outcome != OutcomeOrbit {
		tm := s.Telemetry()
		t.Fatalf("outcome = %d at t=%.0fs; apoapsis %.0f km, periapsis %.0f km",
			s.St.Outcome, s.St.T, tm.ApoAlt/1000, tm.PeriAlt/1000)
	}

	tm := s.Telemetry()
	if tm.PeriAlt < 140000 {
		t.Errorf("periapsis = %.0f km, want above the atmosphere", tm.PeriAlt/1000)
	}
	// Real launchers spend 9.3-9.5 km/s reaching low Earth orbit. This one
	// lifts off from the equator and gets the full 465 m/s of rotation for
	// free, so a slightly smaller figure is the correct answer, not a cheat.
	if s.St.DeltaV < 8500 || s.St.DeltaV > 10500 {
		t.Errorf("delta-v spent = %.0f m/s, expected 8.5-10.5 km/s for low Earth orbit", s.St.DeltaV)
	}
	if s.St.GravLoss < 800 || s.St.GravLoss > 2500 {
		t.Errorf("gravity losses = %.0f m/s, expected roughly 1-2 km/s", s.St.GravLoss)
	}
	if s.St.DragLoss < 20 || s.St.DragLoss > 400 {
		t.Errorf("drag losses = %.0f m/s, expected roughly 0.1-0.3 km/s", s.St.DragLoss)
	}

	q, alt := s.MaxQ()
	if q < 15000 || q > 60000 {
		t.Errorf("max q = %.0f Pa, expected roughly 30 kPa", q)
	}
	if alt < 6000 || alt > 20000 {
		t.Errorf("max q at %.0f m, expected roughly 11-13 km", alt)
	}
}

// Reaching orbit is a verdict, not the end of the run: the vehicle has to keep
// going round, and running out of clock later must not overwrite the verdict
// with a timeout.
func TestOrbitDoesNotStopTheSimulation(t *testing.T) {
	s := New(earthFalcon().Cfg)

	// Fly until the verdict lands.
	for !s.Settled() && s.St.T < s.Cfg.MaxTime {
		s.Step(FixedStep)
	}
	if s.St.Outcome != OutcomeOrbit {
		t.Fatalf("expected orbit, got outcome %d at T+%.0f", s.St.Outcome, s.St.T)
	}
	if s.St.Done {
		t.Fatal("the run stopped the moment orbit was reached")
	}

	settledAt := s.St.T
	before := ComputeOrbit(s.St.Pos, s.St.Vel, s.Cfg.Body.Mu)

	// Keep flying for a full revolution and a bit.
	for n := int(before.Period * 1.2 / FixedStep); n > 0 && !s.St.Done; n-- {
		s.Step(FixedStep)
	}
	if s.St.T <= settledAt {
		t.Fatal("time did not advance after the verdict")
	}

	// A stable orbit must still be the same orbit a revolution later.
	after := ComputeOrbit(s.St.Pos, s.St.Vel, s.Cfg.Body.Mu)
	close(t, "semi-major axis", after.SemiMajor, before.SemiMajor, 1e-6)
	close(t, "eccentricity", after.Eccentricity, before.Eccentricity, 1e-3)

	// And the time limit does not apply to a flight that got where it was
	// going: it exists to cut short the ones that did not.
	for s.St.T < s.Cfg.MaxTime*2 && !s.St.Done {
		s.Step(FixedStep)
	}
	if s.St.Done {
		t.Errorf("the run stopped at T+%.0f, past a time limit that should no longer apply", s.St.T)
	}
	if s.St.Outcome != OutcomeOrbit {
		t.Errorf("outcome became %d; nothing should overrule a reached orbit", s.St.Outcome)
	}
	if !hasEvent(s.Events, EvOrbit) {
		t.Error("no orbit marker on the timeline")
	}
}

// A flight with no end must not record itself without bound.
func TestHistoryStaysBoundedInOrbit(t *testing.T) {
	s := New(earthFalcon().Cfg)
	for !s.Settled() && s.St.T < s.Cfg.MaxTime {
		s.Step(FixedStep)
	}
	if !s.Settled() {
		t.Fatal("never reached orbit")
	}
	ascent := len(s.Hist)

	// Six hours of coasting, twenty times the ascent.
	for target := s.St.T + 6*3600; s.St.T < target && !s.St.Done; {
		s.Step(FixedStep)
	}

	added := len(s.Hist) - ascent
	full := int(6 * 3600 / s.HistInterval)
	if added >= full/10 {
		t.Errorf("six hours of orbit added %d samples; at full rate it would be %d, "+
			"so the record is barely being thinned", added, full)
	}
	if added == 0 {
		t.Error("nothing was recorded at all during the coast")
	}
}

// The Apollo preset is the one place the simulation can be checked against a
// flight that actually happened, so it is worth pinning to the real numbers
// rather than to "it got there". Apollo 11 staged the S-IC at T+161 s, cut the
// S-IVB at T+11:39 and ended up in a 185.9 x 183.2 km parking orbit.
func TestApolloPresetMatchesTheRealAscent(t *testing.T) {
	s := New(apolloSaturn().Cfg)
	s.RunToEnd()
	tm := s.Telemetry()

	if s.St.Outcome != OutcomeOrbit {
		t.Fatalf("outcome = %d at t=%.0fs; apoapsis %.0f km, periapsis %.0f km",
			s.St.Outcome, s.St.T, tm.ApoAlt/1000, tm.PeriAlt/1000)
	}
	// A parking orbit is meant to be round. Within 15 km of the target at both
	// ends is as close as a time-based pitch programme gets.
	for _, c := range []struct {
		name string
		got  float64
	}{{"apoapsis", tm.ApoAlt}, {"periapsis", tm.PeriAlt}} {
		if math.Abs(c.got-185000) > 15000 {
			t.Errorf("%s = %.1f km, want a 185 km parking orbit", c.name, c.got/1000)
		}
	}
	// Real vehicle: 2,930 t on the pad at a thrust-to-weight of 1.16.
	r := &s.Cfg.Rocket
	close(t, "liftoff mass", r.LiftoffMass(), 2857400, 1e-9)
	if twr := r.LiftoffTWR(101325, s.Cfg.Body.SurfaceG); twr < 1.1 || twr > 1.3 {
		t.Errorf("liftoff TWR = %.2f, expected roughly 1.2", twr)
	}
	// Saturn V lost only about 40 m/s to drag: it is a big vehicle, but a heavy
	// one, and it leaves the thick air early.
	if s.St.DragLoss > 150 {
		t.Errorf("drag losses = %.0f m/s, expected under 150", s.St.DragLoss)
	}
	if s.St.T < 480 || s.St.T > 780 {
		t.Errorf("insertion at T+%.0f s, expected roughly the real T+700", s.St.T)
	}
	// The S-IVB keeps what it would have burned for translunar injection: this
	// simulation has one central body and nowhere to send it.
	if left := s.St.Prop[2] / r.Stages[2].PropMass; left < 0.6 {
		t.Errorf("S-IVB has %.0f%% of its propellant left, expected most of it", left*100)
	}
}

// stack builds a vehicle of n identical stages in a vacuum, for exercising the
// staging machine at counts the presets do not cover.
func stack(n int) Config {
	b := Body{Radius: 600000, MassSource: FromMass, Mass: 5.2915158e22}
	b.Normalize()

	stages := make([]Stage, n)
	for i := range stages {
		stages[i] = Stage{
			DryMass: 200, PropMass: 800,
			ThrustVac: 60000, IspVac: 320, IspSL: 320,
			Throttle: 1, SepDelay: 2,
		}
	}
	b.Atmo = Atmosphere{Top: 1e9}
	return Config{
		Body:    b,
		Rocket:  Rocket{Payload: 100, Cd: 0, Diameter: 1, Stages: stages},
		Program: Program{Keys: []Keyframe{{Time: 0, Pitch: 90}}},
		MaxTime: 1e9,
	}
}

// The staging machine has to walk any stack, not just the two-stage one the
// presets ship: one stage means going straight from burnout to coast without
// looking for a stage that is not there, and four means three separations.
func TestStagingWalksAnyStageCount(t *testing.T) {
	for _, n := range []int{1, 2, 3, 4} {
		t.Run(fmt.Sprintf("%d-stage", n), func(t *testing.T) {
			s := New(stack(n))

			for i := 0; i < 400000 && s.St.Phase != PhaseCoast && !s.St.Done; i++ {
				s.Step(FixedStep)
			}
			if s.St.Phase != PhaseCoast {
				t.Fatalf("never reached coast: phase %v at t=%.1f, outcome %d",
					s.St.Phase, s.St.T, s.St.Outcome)
			}
			if s.St.Stage != n-1 {
				t.Errorf("finished on stage %d, want the top one (%d)", s.St.Stage+1, n)
			}

			seps := 0
			for _, e := range s.Events {
				if e.Kind == EvSeparation {
					seps++
				}
			}
			if seps != n-1 {
				t.Errorf("%d separations, want %d", seps, n-1)
			}
			// Every stage but the last one has to have been emptied on the way
			// up, and the ideal delta-v has to account for all of them.
			for i := 0; i < n-1; i++ {
				if s.St.Prop[i] > 1e-6 {
					t.Errorf("stage %d kept %.3f kg of propellant", i+1, s.St.Prop[i])
				}
			}
			if want := s.Cfg.Rocket.TotalDeltaV(); s.St.DeltaV < want*0.99 {
				t.Errorf("expended delta-v %.0f m/s against an ideal %.0f", s.St.DeltaV, want)
			}
		})
	}
}

// Every shipped preset has to be flyable, otherwise the setup screen offers
// broken starting points.
func TestAllPresetsReachOrbit(t *testing.T) {
	for _, p := range Presets() {
		t.Run(p.Name, func(t *testing.T) {
			s := New(p.Cfg)
			s.RunToEnd()
			tm := s.Telemetry()
			if s.St.Outcome != OutcomeOrbit {
				t.Errorf("outcome = %d; apoapsis %.0f km, periapsis %.0f km, dv %.0f m/s",
					s.St.Outcome, tm.ApoAlt/1000, tm.PeriAlt/1000, s.St.DeltaV)
			}
		})
	}
}

// A g is 9.80665 m/s² wherever the vehicle is. Both figures used to be divided by
// the surface gravity of whatever body the state was measured from, which made
// them mean "local surface gravities" — 0.68 g of Titan ascent reported as 4.9 —
// and, worse, made them step discontinuously when the frame changed: kerbin-mun
// showed 22 g for a burn pulling 3.7 of them, because the divisor became the Mun's.
func TestGLoadIsInStandardGravities(t *testing.T) {
	s := New(kerbinMun().Cfg)

	// Mid-ascent, where there is thrust and drag to account for.
	runFor(s, 40)
	tm := s.Telemetry()
	want := math.Hypot(tm.Thrust, 0) // thrust and drag are along the same line here
	if tm.Drag > 0 {
		want = tm.Thrust - tm.Drag
	}
	close(t, "acceleration", tm.AccelG, math.Abs(want)/tm.Mass/G0, 0.02)

	// And over the whole flight, which crosses into the Mun's sphere of influence
	// and burns there. The peak belongs to the ascent either way; what must not
	// happen is the number jumping by the ratio of the two surface gravities.
	s = New(kerbinMun().Cfg)
	s.FastForward(s.Cfg.MaxTime)
	if g := s.MaxG(); g < 3 || g > 5 {
		t.Errorf("peak %.2f g over the whole mission, want the ascent's three and a half", g)
	}
	// The Mun's gravity is a sixth of Kerbin's, so the old bug showed as a factor
	// of six on anything burning out there.
	for _, h := range s.Hist {
		if h.Center != 0 && h.AccelG > 6 {
			t.Fatalf("%.2f g at T+%.0f s in body %d's frame: the divisor is following the frame",
				h.AccelG, h.T, h.Center)
		}
	}
}
