package sim

import (
	"math"
	"testing"
)

// earthMoon is the smallest system that exercises everything a tree has to do:
// a root with a satellite heavy enough — 1/81 of it — that ignoring the details
// shows up in the numbers.
func earthMoon() System {
	sys := System{Bodies: []Body{
		{
			Name: "Earth", Radius: 6371000,
			MassSource: FromMass, Mass: 5.97237e24, RotationPeriod: 86164.1,
		},
		{
			Name: "Moon", Radius: 1737400,
			MassSource: FromMass, Mass: 7.342e22, RotationPeriod: 2360591,
			Parent: 0, SemiMajor: 3.844e8,
		},
	}}
	sys.Normalize()
	return sys
}

// A circular rail must produce exactly circular motion: constant radius, the
// circular speed at that radius, and a position that comes back to where it
// started after one period.
func TestCircularRailsStayCircular(t *testing.T) {
	sys := earthMoon()
	moon := &sys.Bodies[1]
	mu := sys.Bodies[0].Mu
	a := moon.SemiMajor
	period := 2 * math.Pi * math.Sqrt(a*a*a/mu)

	for _, f := range []float64{0, 0.1, 0.25, 0.5, 0.75, 0.999} {
		p, v := sys.StateAt(1, f*period)
		close(t, "radius", p.Len(), a, 1e-12)
		close(t, "speed", v.Len(), math.Sqrt(mu/a), 1e-12)
		// Circular motion has velocity square to the radius throughout.
		close(t, "radial velocity", p.Unit().Dot(v), 0, 1e-6)
	}

	p0, v0 := sys.StateAt(1, 0)
	p1, v1 := sys.StateAt(1, period)
	close(t, "position after one period", p1.Sub(p0).Len(), 0, 1e-3)
	close(t, "velocity after one period", v1.Sub(v0).Len(), 0, 1e-9)
}

// The Moon's month comes out of the rails, and it comes out 0.47% long on
// purpose: the rails run on the parent's mu alone, which is the price of the
// frame agreeing exactly with the parent's pull at the Moon's centre. The real
// sidereal month is 27.32 days.
func TestMoonRailsRunTheModelMonth(t *testing.T) {
	sys := earthMoon()
	a := sys.Bodies[1].SemiMajor
	period := 2 * math.Pi * math.Sqrt(a*a*a/sys.Bodies[0].Mu)

	close(t, "sidereal month", period/86400, 27.44, 1e-3)
	if real := 27.321661; math.Abs(period/86400-real)/real > 0.006 {
		t.Errorf("month = %.3f days, further than 0.6%% from the real %.3f",
			period/86400, real)
	}
	// The sphere of influence follows from the mass ratio: 66,100 km in the
	// tables, and the tables are using the same formula.
	close(t, "lunar sphere of influence", sys.Bodies[1].SOI, 6.61e7, 0.02)
	if !math.IsInf(sys.Bodies[0].SOI, 1) {
		t.Errorf("the root's sphere of influence is %g, want unbounded", sys.Bodies[0].SOI)
	}
}

// An eccentric rail has to conserve what a two-body orbit conserves. This is
// what catches a wrong velocity formula, which a circular test cannot: at e = 0
// several wrong expressions agree with the right one.
func TestEccentricRailsConserveEnergyAndMomentum(t *testing.T) {
	sys := earthMoon()
	sys.Bodies[1].Ecc = 0.5
	sys.Bodies[1].ArgPeri = 0.7
	sys.Normalize()

	mu := sys.Bodies[0].Mu
	a, e := sys.Bodies[1].SemiMajor, sys.Bodies[1].Ecc
	period := 2 * math.Pi * math.Sqrt(a*a*a/mu)

	wantEnergy := -mu / (2 * a)
	wantMomentum := math.Sqrt(mu * a * (1 - e*e))
	var rMin, rMax float64 = math.Inf(1), 0

	for i := 0; i <= 400; i++ {
		p, v := sys.StateAt(1, period*float64(i)/400)
		r, s := p.Len(), v.Len()
		close(t, "specific energy", s*s/2-mu/r, wantEnergy, 1e-9)
		close(t, "specific angular momentum", math.Abs(p.Cross(v)), wantMomentum, 1e-9)
		rMin, rMax = math.Min(rMin, r), math.Max(rMax, r)
	}
	close(t, "periapsis", rMin, a*(1-e), 1e-4)
	close(t, "apoapsis", rMax, a*(1+e), 1e-4)

	// Periapsis has to point where ArgPeri says it does.
	p, _ := sys.StateAt(1, 0)
	close(t, "periapsis direction", p.Angle(), 0.7, 1e-9)
}

// The sphere of influence decides which body the state is measured from, and
// nothing else. Deepest containing body wins.
func TestFrameFollowsTheSpheresOfInfluence(t *testing.T) {
	sys := earthMoon()
	moonPos, _ := sys.StateAt(1, 0)
	soi := sys.Bodies[1].SOI

	cases := []struct {
		name string
		pos  Vec2
		want int
	}{
		{"on the pad", Vec2{6371000, 0}, 0},
		{"in low orbit", Vec2{6871000, 0}, 0},
		{"most of the way to the Moon", moonPos.Scale(0.8), 0},
		{"just outside the lunar sphere", moonPos.Scale(1 - 1.01*soi/moonPos.Len()), 0},
		{"just inside it", moonPos.Scale(1 - 0.99*soi/moonPos.Len()), 1},
		{"in low lunar orbit", moonPos.Add(Vec2{0, 1.9e6}), 1},
		{"beyond the Moon entirely", moonPos.Scale(4), 0},
	}
	for _, c := range cases {
		if got := sys.Frame(c.pos, 0); got != c.want {
			t.Errorf("%s: frame = %d (%s), want %d (%s)",
				c.name, got, sys.Bodies[got].Name, c.want, sys.Bodies[c.want].Name)
		}
	}
}

// Changing frame must not move the vehicle. The state is the same point and the
// same velocity, written down from a different centre.
func TestRefocusIsExact(t *testing.T) {
	cfg := Config{
		System: earthMoon(),
		Rocket: Rocket{Payload: 1000, Diameter: 1},
	}
	s := New(cfg)

	moonPos, moonVel := s.Cfg.System.StateAt(1, 0)
	// A hundred kilometres inside the lunar sphere, drifting past the Moon.
	s.St.Pos = moonPos.Sub(moonPos.Unit().Scale(s.Cfg.System.Bodies[1].SOI - 1e5))
	s.St.Vel = Vec2{200, 900}
	s.St.Landed = false
	s.St.Phase = PhaseCoast

	beforePos := s.RootPos()
	_, cv := s.Cfg.System.StateAt(s.St.Center, s.St.T)
	beforeVel := cv.Add(s.St.Vel)

	s.refocus()
	if s.St.Center != 1 {
		t.Fatalf("still centred on %s", s.Center().Name)
	}
	afterPos := s.RootPos()
	_, cv = s.Cfg.System.StateAt(s.St.Center, s.St.T)
	afterVel := cv.Add(s.St.Vel)

	close(t, "root position across the change", afterPos.Sub(beforePos).Len(), 0, 1e-6)
	close(t, "root velocity across the change", afterVel.Sub(beforeVel).Len(), 0, 1e-9)
	// And the new numbers are the ones the Moon would give.
	close(t, "distance from the Moon", s.St.Pos.Len(), s.Cfg.System.Bodies[1].SOI-1e5, 1e-9)
	close(t, "speed relative to the Moon", s.St.Vel.Len(), Vec2{200, 900}.Sub(moonVel).Len(), 1e-9)
}

// In a moon's frame the parent's pull is nearly all cancelled by the frame's own
// acceleration, leaving the tidal difference. Without the rail correction the
// Earth's raw 2.7 mm/s^2 would stand there in full and quietly drag every lunar
// orbit sideways.
func TestMoonFrameLeavesOnlyTheTide(t *testing.T) {
	sys := earthMoon()
	earthPullAtTheMoon := sys.Bodies[0].Mu / (3.844e8 * 3.844e8)

	// The Moon sits on the +X axis at t = 0, so these two offsets are along the
	// Earth-Moon line and across it. The tide is twice as strong along the line
	// as across it, and the two signs differ: that is the whole shape of the
	// tidal field, and getting it out of one formula is what pins the correction.
	const r = 1.8374e6 // a low lunar orbit, 100 km up
	tide := sys.Bodies[0].Mu * r / math.Pow(3.844e8, 3)

	for _, c := range []struct {
		name string
		rel  Vec2
		want float64
	}{
		{"along the Earth-Moon line", Vec2{r, 0}, 2 * tide},
		{"across it", Vec2{0, r}, tide},
	} {
		acc := sys.Gravity(1, c.rel, 0)
		central := c.rel.Unit().Scale(-sys.Bodies[1].Mu / c.rel.Dot(c.rel))
		residual := acc.Sub(central).Len()

		close(t, "tidal residual "+c.name, residual, c.want, 0.02)
		if residual > earthPullAtTheMoon/50 {
			t.Errorf("%s: residual %g m/s^2 is a sizeable part of the Earth's %g, so the rail correction is not doing its job",
				c.name, residual, earthPullAtTheMoon)
		}
	}
}

// The root does not move, so in its frame there is no correction to make and
// gravity is the plain sum of what every body pulls with.
func TestRootFrameGravityIsThePlainSum(t *testing.T) {
	sys := earthMoon()
	pos := Vec2{3e7, 1e7}
	moonPos, _ := sys.StateAt(1, 1e5)

	got := sys.Gravity(0, pos, 1e5)

	r := pos.Len()
	want := pos.Scale(1 / r).Scale(-sys.Bodies[0].Mu / (r * r))
	d := moonPos.Sub(pos)
	dl := d.Len()
	want = want.Add(d.Scale(sys.Bodies[1].Mu / (dl * dl * dl)))

	close(t, "acceleration in the root frame", got.Sub(want).Len(), 0, 1e-12)
}

// A system of one body has to be the single-planet model to the last digit, or
// every ascent in the presets quietly changes.
func TestOneBodySystemIsTheOldModel(t *testing.T) {
	b := Body{Radius: 6371000, MassSource: FromMass, Mass: 5.97237e24}
	sys := System{Bodies: []Body{b}}
	sys.Normalize()

	for _, pos := range []Vec2{{6371000, 0}, {7e6, 3e6}, {-1e8, 4e7}} {
		r := pos.Len()
		want := pos.Scale(1 / r).Scale(-sys.Bodies[0].Mu / (r * r))
		got := sys.Gravity(0, pos, 12345)
		if got != want {
			t.Errorf("gravity at %v = %v, want exactly %v", pos, got, want)
		}
	}
}

// The parent invariant is what makes a cycle impossible, so Normalize has to
// enforce it rather than trust the data.
func TestNormalizeRepairsTheParentInvariant(t *testing.T) {
	sys := System{Bodies: []Body{
		{Name: "Sun", Radius: 7e8, MassSource: FromMass, Mass: 1.989e30, Parent: 3},
		{Name: "forward reference", Radius: 1e6, MassSource: FromMass, Mass: 1e22, Parent: 2, SemiMajor: 1e9},
		{Name: "self reference", Radius: 1e6, MassSource: FromMass, Mass: 1e22, Parent: 2, SemiMajor: 1e9},
		{Name: "fine", Radius: 1e6, MassSource: FromMass, Mass: 1e22, Parent: 1, SemiMajor: 1e8},
	}}
	sys.Normalize()

	if sys.Bodies[0].Parent != -1 {
		t.Errorf("root parent = %d, want -1", sys.Bodies[0].Parent)
	}
	for i, want := range []int{-1, 0, 0, 1} {
		if got := sys.Bodies[i].Parent; got != want {
			t.Errorf("body %d (%s) parent = %d, want %d", i, sys.Bodies[i].Name, got, want)
		}
	}
	// Every walk up the tree now terminates, which is the whole point.
	for i := range sys.Bodies {
		p, _ := sys.StateAt(i, 1000)
		if math.IsNaN(p.X) || math.IsNaN(p.Y) {
			t.Errorf("body %d has no finite position", i)
		}
	}
}

// Kepler's equation has to be inverted for every eccentricity the rails allow,
// and the answer is only right if it puts E back where M came from.
func TestSolveKeplerInverts(t *testing.T) {
	for _, e := range []float64{0, 0.01, 0.2, 0.5, 0.8, 0.95} {
		for i := range 32 {
			m := -math.Pi + 2*math.Pi*float64(i)/31
			E := solveKepler(m, e)
			if got := E - e*math.Sin(E); math.Abs(got-m) > 1e-9 {
				t.Errorf("e=%g, M=%g: E=%g gives M=%g back", e, m, E, got)
			}
		}
	}
}

// The frame change has to happen inside the step loop, not just when asked for
// by hand: a coast that crosses into the Moon's sphere must come out the other
// side measured from the Moon, with the crossing marked by nothing at all.
func TestCoastCrossesIntoTheLunarSphere(t *testing.T) {
	sys := earthMoon()
	s := New(Config{System: sys, Rocket: Rocket{Payload: 1000, Diameter: 1}, MaxTime: 1e9})

	moonPos, moonVel := sys.StateAt(1, 0)
	// Five thousand kilometres short of the sphere, drifting inwards along the
	// Earth-Moon line at the Moon's own pace plus a little.
	toMoon := moonPos.Unit()
	s.St.Pos = moonPos.Sub(toMoon.Scale(sys.Bodies[1].SOI + 5e6))
	s.St.Vel = moonVel.Add(toMoon.Scale(500))
	s.St.Landed = false
	s.St.Phase = PhaseCoast

	if s.St.Center != 0 {
		t.Fatalf("started centred on %s", s.Center().Name)
	}
	for s.St.Center == 0 && s.St.T < 40000 && !s.St.Done {
		s.Step(FixedStep)
	}
	if s.St.Center != 1 {
		t.Fatalf("never entered the lunar sphere: centred on %s at T+%.0f s",
			s.Center().Name, s.St.T)
	}
	// It crossed the boundary, so it is at the boundary — the frame change is a
	// change of description, not a jump.
	close(t, "distance from the Moon at the crossing", s.St.Pos.Len(), sys.Bodies[1].SOI, 1e-4)
}

// A low lunar orbit has to stay one. This is the dynamic version of the tidal
// test: without the rail correction the Earth's 2.7 mm/s^2 would add 19 m/s over
// a single revolution and the orbit would visibly walk away.
func TestLowLunarOrbitHolds(t *testing.T) {
	sys := earthMoon()
	s := New(Config{System: sys, Rocket: Rocket{Payload: 1000, Diameter: 1}, MaxTime: 1e9})

	moon := &sys.Bodies[1]
	r := moon.Radius + 100000
	s.St.Center = 1
	s.St.Pos = Vec2{0, r}
	s.St.Vel = Vec2{math.Sqrt(moon.Mu / r), 0}
	s.St.Landed = false
	s.St.Phase = PhaseCoast

	o0 := ComputeOrbit(s.St.Pos, s.St.Vel, moon.Mu)
	for n := int(o0.Period / FixedStep); n > 0 && !s.St.Done; n-- {
		s.Step(FixedStep)
	}
	if s.St.Center != 1 {
		t.Fatalf("lost the Moon: centred on %s", s.Center().Name)
	}

	o1 := ComputeOrbit(s.St.Pos, s.St.Vel, moon.Mu)
	close(t, "semi-major axis after one lunar revolution", o1.SemiMajor, o0.SemiMajor, 1e-4)
	if o1.Eccentricity > 0.002 {
		t.Errorf("eccentricity walked to %g over one revolution", o1.Eccentricity)
	}
}

// Removing a body takes everything orbiting it with it. Leaving orphans pointing
// at a slot that now holds something else would be worse than any error message.
func TestRemoveTakesTheSubtreeWithIt(t *testing.T) {
	sys := SolarSystem()
	mars := sys.IndexOf("mars")
	before := len(sys.Bodies)

	remap := sys.Remove(mars)

	if len(sys.Bodies) != before-3 {
		t.Errorf("%d bodies left, want %d: Mars and its two moons should have gone",
			len(sys.Bodies), before-3)
	}
	for _, gone := range []string{"mars", "phobos", "deimos"} {
		if sys.IndexOf(gone) >= 0 {
			t.Errorf("%s survived", gone)
		}
	}
	if remap[mars] != -1 {
		t.Errorf("remap says Mars moved to %d, want -1", remap[mars])
	}
	// What is left has to still be pointing at what it was pointing at.
	for _, pair := range [][2]string{{"moon", "earth"}, {"io", "jupiter"}, {"titan", "saturn"}} {
		i := sys.IndexOf(pair[0])
		if i < 0 {
			t.Fatalf("%s went missing", pair[0])
		}
		if got := sys.Bodies[sys.Bodies[i].Parent].Name; got != pair[1] {
			t.Errorf("%s now orbits %s, want %s", pair[0], got, pair[1])
		}
	}
	// And the invariant that makes the whole tree work still holds.
	for i := range sys.Bodies {
		if i > 0 && sys.Bodies[i].Parent >= i {
			t.Errorf("%s has parent %d at index %d", sys.Bodies[i].Name, sys.Bodies[i].Parent, i)
		}
	}
}

// The root is the frame everything is measured in. It does not go anywhere.
func TestRemoveRefusesTheRoot(t *testing.T) {
	sys := earthMoon()
	remap := sys.Remove(0)

	if len(sys.Bodies) != 2 {
		t.Errorf("%d bodies left, want both", len(sys.Bodies))
	}
	for i, m := range remap {
		if m != i {
			t.Errorf("remap[%d] = %d, want an unchanged system", i, m)
		}
	}
}

// A body added to the end of the slice is after every possible parent, which is
// how the invariant is kept without anyone having to think about it.
func TestAddChildKeepsTheInvariant(t *testing.T) {
	sys := earthMoon()
	i := sys.AddChild(1, Body{
		Name: "custom", Radius: 200000,
		MassSource: FromDensity, Density: 3000, SemiMajor: 2e7,
	})

	if i != 2 || sys.Bodies[i].Parent != 1 {
		t.Fatalf("added at %d with parent %d, want 2 orbiting the Moon", i, sys.Bodies[i].Parent)
	}
	if sys.Bodies[i].Mu <= 0 || sys.Bodies[i].SOI <= 0 {
		t.Error("the new body has no derived quantities: AddChild has to normalize")
	}
	p, _ := sys.StateAt(i, 1000)
	if p.Len() == 0 {
		t.Error("a moon of the Moon sits on the Earth")
	}
}

// A configuration that describes one planet and nothing else has to come out as a
// system of one, because everything downstream now works on the tree.
func TestEnsureSystemFromASinglePlanet(t *testing.T) {
	cfg := Config{Body: Body{Radius: 6371000, MassSource: FromMass, Mass: 5.97237e24}}
	cfg.EnsureSystem()

	if len(cfg.System.Bodies) != 1 {
		t.Fatalf("%d bodies, want one", len(cfg.System.Bodies))
	}
	if cfg.LaunchBody != 0 || cfg.Body.Mu <= 0 {
		t.Errorf("launch body %d, mu %g: the mirror was not filled in", cfg.LaunchBody, cfg.Body.Mu)
	}
	if !math.IsInf(cfg.System.Bodies[0].SOI, 1) {
		t.Error("the only body in a system is its root")
	}
}

// Body is a read-back mirror of the launch body, not a second place to edit it. The
// editor writes through the tree — it has to, for the other seventeen bodies — and the
// mirror used to be copied back over the top on the next call, which silently undid
// every edit made to the planet the pad is on.
func TestTheLaunchBodyIsEditedThroughTheTree(t *testing.T) {
	cfg := Config{System: earthMoon(), LaunchBody: 0}
	cfg.EnsureSystem()

	cfg.System.Bodies[0].Radius = 3000000 // as the diameter field writes it
	cfg.EnsureSystem()

	if got := cfg.System.Bodies[0].Radius; got != 3000000 {
		t.Errorf("the edit was undone: radius %g, want 3e6", got)
	}
	if cfg.Body.Radius != 3000000 {
		t.Errorf("the mirror reads %g, want the edited 3e6", cfg.Body.Radius)
	}
	// And the Moon, which nobody touched, is still where it was.
	if got := cfg.System.Bodies[1].SemiMajor; got != 3.844e8 {
		t.Errorf("the Moon's orbit changed to %g", got)
	}
}

// A configuration with no system at all is the one case where Body is an input: that
// is what a single-planet setup is, and what every test that writes one by hand does.
func TestBodyBuildsTheSystemWhenThereIsNone(t *testing.T) {
	cfg := Config{Body: Body{Name: "somewhere", Radius: 1e6, Mass: 1e22}}
	cfg.EnsureSystem()

	if len(cfg.System.Bodies) != 1 || cfg.System.Bodies[0].Radius != 1e6 {
		t.Fatalf("the system is %+v", cfg.System.Bodies)
	}
	if cfg.Body.Mu <= 0 {
		t.Error("the mirror came back without its derived values")
	}
}

// The editor's two operations, run in a long enough sequence to catch a fixup that
// only works the first time. Whatever it does, the tree has to stay walkable: that
// is what everything from gravity to the camera assumes.
func TestEditingKeepsTheTreeWalkable(t *testing.T) {
	sys := SolarSystem()

	for round := range 6 {
		// Hang something new on a body a little further down each time.
		parent := (round * 3) % len(sys.Bodies)
		sys.AddChild(parent, Body{
			Name: "custom", Radius: 300000,
			MassSource: FromDensity, Density: 3000,
			SemiMajor: sys.Bodies[parent].Radius * 12,
		})
		// And take out something that has children of its own.
		if victim := sys.IndexOf("jupiter"); victim > 0 && round == 2 {
			sys.Remove(victim)
			if sys.IndexOf("io") >= 0 {
				t.Error("Io outlived Jupiter")
			}
		}
		if victim := sys.IndexOf("earth"); victim > 0 && round == 4 {
			sys.Remove(victim)
			if sys.IndexOf("moon") >= 0 {
				t.Error("the Moon outlived the Earth")
			}
		}

		for i := range sys.Bodies {
			b := &sys.Bodies[i]
			if i == 0 {
				if b.Parent != -1 {
					t.Fatalf("round %d: the root has parent %d", round, b.Parent)
				}
				continue
			}
			if b.Parent < 0 || b.Parent >= i {
				t.Fatalf("round %d: %s has parent %d at index %d", round, b.Name, b.Parent, i)
			}
			// Which is to say: every walk up the tree terminates, and every body
			// has a position and a sphere of influence to be found in.
			p, _ := sys.StateAt(i, 12345)
			if math.IsNaN(p.X) || math.IsNaN(p.Y) || b.SOI <= 0 {
				t.Fatalf("round %d: %s is at %v with an SOI of %g", round, b.Name, p, b.SOI)
			}
			if g := sys.Gravity(i, Vec2{X: b.Radius * 2}, 12345); math.IsNaN(g.X) {
				t.Fatalf("round %d: gravity at %s came out NaN", round, b.Name)
			}
		}
	}
}

// withAtmoTop puts a ceiling on a body without putting any air under it, which is what
// most of these tests want out of an atmosphere: somewhere for the coast logic and the
// step planner to stop, with no drag to change the trajectory they are measuring.
func withAtmoTop(sys System, i int, top float64) System {
	sys.Bodies[i].Atmo = Atmosphere{Top: top}
	sys.Normalize()
	return sys
}

// Air belongs to the body, and the vehicle flies through whatever it is next to. Until
// this moved, only the launch body had an atmosphere at all: a descent to Mars on a
// preset launched from Earth was a descent through nothing, and the drag that should
// have been there was simply absent.
func TestTheVehicleFliesThroughTheAirOfWhateverItIsAt(t *testing.T) {
	sys := SolarSystem()
	mars := sys.IndexOf("mars")
	if sys.Bodies[mars].Atmo.IsVacuum() {
		t.Fatal("Mars has no air, so this test cannot be about anything")
	}

	s := New(Config{
		System: sys, LaunchBody: sys.IndexOf("earth"),
		Rocket:  Rocket{Payload: 1000, Cd: 0.4, Diameter: 2},
		MaxTime: 1e9,
	})

	// Put the vehicle thirty kilometres over Mars, going fast, and ask what it feels.
	s.St.Center = mars
	b := &sys.Bodies[mars]
	s.St.Pos = Vec2{b.Radius + 30000, 0}
	s.St.Vel = Vec2{0, 4000}
	s.St.Landed, s.St.Phase = false, PhaseCoast

	tm := s.Telemetry()
	if tm.Density <= 0 {
		t.Errorf("no air over Mars: density %g", tm.Density)
	}
	if tm.Drag <= 0 {
		t.Errorf("no drag over Mars: %g N", tm.Drag)
	}
	if got := s.AtmoTop(); got != b.Atmo.Top {
		t.Errorf("the ceiling reads %g, want Mars's %g", got, b.Atmo.Top)
	}

	// And over an airless one it feels nothing, whatever the launch body had.
	s.St.Center = sys.IndexOf("moon")
	mb := &sys.Bodies[s.St.Center]
	s.St.Pos = Vec2{mb.Radius + 30000, 0}
	tm = s.Telemetry()
	if tm.Density != 0 || tm.Drag != 0 || tm.Mach != 0 {
		t.Errorf("the Moon has weather: density %g, drag %g, Mach %g",
			tm.Density, tm.Drag, tm.Mach)
	}
	if s.AtmoTop() != 0 {
		t.Errorf("the Moon has a ceiling at %g", s.AtmoTop())
	}
}

// Every body's air is derived with that body's own gravity, which is the whole reason
// it cannot be prepared once for the configuration: the same gas at the same surface
// pressure thins out at a different rate under a different g.
func TestEachAtmosphereIsPreparedWithItsOwnGravity(t *testing.T) {
	sys := SolarSystem()
	titan := sys.IndexOf("titan")
	g := sys.Bodies[titan].SurfaceG

	// What the tree produced, against the same air prepared by hand with Titan's own
	// gravity, and against it prepared with Earth's.
	right := TitanAir()
	right.Prepare(g)
	wrong := TitanAir()
	wrong.Prepare(9.80665)

	const h = 100000.0
	got := sys.Bodies[titan].Atmo.State(h).Pressure
	if got != right.State(h).Pressure {
		t.Errorf("at %g km the tree gives %.4g Pa and Titan's own gravity %.4g Pa",
			h/1000, got, right.State(h).Pressure)
	}
	if got == wrong.State(h).Pressure {
		t.Errorf("Titan's air reads the same under Earth's gravity, so nothing about " +
			"the body reached the profile")
	}
	// And it is thicker up there than Earth's g would leave it, since a seventh of the
	// pull holds the column up over seven times the height.
	if got <= wrong.State(h).Pressure {
		t.Errorf("%.4g Pa under g = %.3f against %.4g Pa under 9.81", got, g,
			wrong.State(h).Pressure)
	}
}

// vacuumDensity is where a body's air is taken to have ended: the density Earth's own
// ceiling cuts off at. It is the convention every atmosphere here is measured against,
// and it exists as a number because it used to exist only as an intention.
const vacuumDensity = 1e-9 // kg/m^3

// Top has to mean the same thing on every body. It did not: each was set to whatever
// suited the preset launched from it, so Mars stopped at 90 km with the profile still
// at 7.8e-7 kg/m³ — 2600 times Earth's cutoff — and Titan at 500 km with 660 times.
// That mattered beyond tidiness, because Top is also the line an orbit has to clear to
// count as one: both presets were reaching "orbit" with their periapsis a few kilometres
// above a cliff edge drawn just under it.
func TestEveryCeilingIsAtTheSameDensity(t *testing.T) {
	sys := SolarSystem()
	sys.Normalize()

	for i := range sys.Bodies {
		b := &sys.Bodies[i]
		if b.Atmo.IsVacuum() {
			continue
		}
		at := &b.Atmo
		rho := at.DensityAt(at.Top)
		if rho > 10*vacuumDensity {
			t.Errorf("%s: the air is cut off at %.0f km where the profile still gives "+
				"%.3g kg/m³, %.0f times the %g it should be",
				b.Name, at.Top/1000, rho, rho/vacuumDensity, vacuumDensity)
		}
		// And not absurdly deep either, which is a hundred kilometres of nothing that
		// an orbit still has to clear: Venus was cut off at 1e-19.
		if rho < vacuumDensity/1000 {
			t.Errorf("%s: cut off at %.0f km where the profile gives %.3g kg/m³, far "+
				"below the %g it should be", b.Name, at.Top/1000, rho, vacuumDensity)
		}
	}
}

// Kerbin is the exception, on purpose. It is a game's planet and in that game the air
// ends abruptly at 70 km, where a real profile would still have a millionth of a
// kilogram in it — so the cutoff is a cliff, and it is the source's cliff. Anything
// that made it consistent with the real bodies would stop it being Kerbin.
func TestKerbinKeepsItsCliff(t *testing.T) {
	b := kerbinSystem().Bodies[0]
	if b.Atmo.Top != 70000 {
		t.Errorf("Kerbin's air ends at %.0f km, and the game says 70", b.Atmo.Top/1000)
	}
	if rho := b.Atmo.DensityAt(b.Atmo.Top); rho < 100*vacuumDensity {
		t.Errorf("Kerbin's cutoff is at %.3g kg/m³, which is no longer the cliff this "+
			"test is about", rho)
	}
}

// Every preset has to reach an orbit that clears its own body's air by a margin, not by
// a rounding error. Two of them used to clear it by two and four kilometres, which is
// how the shallow ceilings stayed unnoticed.
func TestEveryPresetClearsTheAirItFliesIn(t *testing.T) {
	for _, p := range Presets() {
		t.Run(p.Name, func(t *testing.T) {
			s := flyToVerdict(p.Cfg)
			top := s.AtmoTop()
			if top <= 0 {
				return // an airless body has nothing to clear
			}
			peri := s.Telemetry().PeriAlt
			if peri < top*1.1 {
				t.Errorf("periapsis %.0f km against a ceiling at %.0f km: %.0f km of "+
					"margin, which is not a margin", peri/1000, top/1000, (peri-top)/1000)
			}
			if s.Cfg.TargetOrbit <= top {
				t.Errorf("the target orbit is %.0f km, inside the air it launches "+
					"through (%.0f km)", s.Cfg.TargetOrbit/1000, top/1000)
			}
		})
	}
}

// flyToVerdict flies a configuration until it has one — which for every preset here is
// the orbit its ascent reaches, minutes into the flight. What comes after is the mission,
// and an audit of the ascent has no business flying it: the grand tour takes twenty-four
// years and thirty seconds of wall clock to finish.
func flyToVerdict(cfg Config) *Sim {
	s := New(cfg)
	for !s.Settled() && !s.St.Done && s.St.T < 6*3600 {
		s.Step(FixedStep)
	}
	return s
}
