package sim

import (
	"math"
	"testing"
)

// orbiting sets a vehicle coasting on a closed orbit around the launch body,
// which is the state the adaptive step is for. It starts at periapsis.
func orbiting(peri, apo float64) *Sim {
	sys := earthMoon()
	s := New(Config{System: sys, Atmo: Atmosphere{Top: 140000}, Rocket: Rocket{Payload: 1000, Diameter: 1}, MaxTime: 1e9})

	rp := sys.Bodies[0].Radius + peri
	ra := sys.Bodies[0].Radius + apo
	a := (rp + ra) / 2
	s.St.Pos = Vec2{rp, 0}
	s.St.Vel = Vec2{0, math.Sqrt(2 * sys.Bodies[0].Mu * (1/rp - 1/(2*a)))}
	s.St.Landed = false
	s.St.Phase = PhaseCoast
	return s
}

// falling starts the same orbit at apoapsis instead, on its way down.
func falling(peri, apo float64) *Sim {
	s := orbiting(peri, apo)
	r := s.Cfg.Body.Radius
	rp, ra := r+peri, r+apo
	a := (rp + ra) / 2
	s.St.Pos = Vec2{0, ra}
	s.St.Vel = Vec2{math.Sqrt(2 * s.Cfg.Body.Mu * (1/ra - 1/(2*a))), 0}
	return s
}

// The adaptive step has to agree with the step it replaces. A day of coasting on
// a high ellipse, one run at ten minutes a step and one at 0.02 s, must land in
// the same place — the whole argument for the long step is that it costs nothing
// but time.
func TestCoastAgreesWithTheFixedStep(t *testing.T) {
	const day = 86400.0

	ref := orbiting(400000, 60000000)
	for ref.St.T < day {
		// Landing exactly on the day matters: a step's worth of extra flight is
		// 160 m near perigee, which would swamp what is being measured.
		ref.Step(math.Min(FixedStep, day-ref.St.T))
	}

	fast := orbiting(400000, 60000000)
	fast.WarpRate = 1e6
	steps := 0
	for fast.St.T < day {
		fast.advanceOne(math.Min(fast.plannedStep(), day-fast.St.T))
		steps++
	}

	gap := fast.St.Pos.Sub(ref.St.Pos).Len()
	t.Logf("%d adaptive steps against %.0f fixed ones, %.3f m apart after a day",
		steps, day/FixedStep, gap)
	if gap > 1 {
		t.Errorf("adaptive coast is %.3f m from the fixed-step answer after one day", gap)
	}
	if dv := fast.St.Vel.Sub(ref.St.Vel).Len(); dv > 0.01 {
		t.Errorf("velocity is %.4f m/s off after one day", dv)
	}
	// The point of the exercise: a day of coasting in a few hundred steps.
	if steps > 3000 {
		t.Errorf("took %d steps for one day of coasting, expected a couple of thousand", steps)
	}
}

// At ×1 the cap equals the fixed step, so a real-time flight has to be exactly
// what the simulator produced before the adaptive step existed. Not close: the
// same bits.
func TestWarpRateOneIsTheOldFixedStep(t *testing.T) {
	ref := orbiting(400000, 400000)
	for range 20000 {
		ref.Step(FixedStep)
	}

	live := orbiting(400000, 400000)
	live.WarpRate = 1
	for live.St.T < ref.St.T-1e-9 {
		live.Advance(1.0 / 60)
	}

	if live.St.Pos != ref.St.Pos || live.St.Vel != ref.St.Vel {
		t.Errorf("at warp 1 the state drifted from the fixed-step run:\n got %v %v\nwant %v %v",
			live.St.Pos, live.St.Vel, ref.St.Pos, ref.St.Vel)
	}
}

// Ragged frames must not show up in the trajectory. The step comes from the
// state; the accumulator only decides where a frame stops.
func TestAdvanceIgnoresFrameTimingAtWarp(t *testing.T) {
	const total = 20000.0

	// The same total simulated time, handed over in different slices.
	slice := func(dts ...float64) []float64 {
		var out []float64
		left := total
		for i := 0; left > 0; i++ {
			dt := math.Min(dts[i%len(dts)], left)
			out = append(out, dt)
			left -= dt
		}
		return out
	}

	even := orbiting(400000, 20000000)
	even.WarpRate = 5000
	for _, dt := range slice(100) {
		even.Advance(dt)
	}

	ragged := orbiting(400000, 20000000)
	ragged.WarpRate = 5000
	for _, dt := range slice(7, 231, 0.5, 61, 0.5) {
		ragged.Advance(dt)
	}

	if ragged.St.T != even.St.T || ragged.St.Pos != even.St.Pos {
		t.Errorf("ragged frames changed the trajectory:\n got T=%.6f %v\nwant T=%.6f %v",
			ragged.St.T, ragged.St.Pos, even.St.T, even.St.Pos)
	}
}

// A long step must never carry the vehicle into the air, let alone through the
// ground: the decision to take it is made at the start, so it has to leave room.
func TestCoastCannotStepIntoTheAir(t *testing.T) {
	s := falling(-100000, 8000000) // a periapsis well underground, seen from the top
	s.WarpRate = 1e6

	top := s.Cfg.Atmo.Top
	for !s.St.Done && s.St.T < 40000 {
		alt := s.Altitude()
		h := s.plannedStep()
		s.advanceOne(h)

		// Above the air, one step may not cross the boundary by more than the
		// air is deep. Below it, the fixed step is in charge again.
		if alt > top && s.Altitude() < 0 {
			t.Fatalf("stepped from %.0f m straight to %.0f m: %.1f s in one go",
				alt, s.Altitude(), h)
		}
	}
	if s.St.Outcome != OutcomeCrashed && s.St.Outcome != OutcomeSuborbital {
		t.Fatalf("outcome = %d, expected the ground", s.St.Outcome)
	}
	// The impact is found where the ground is, not somewhere inside the planet.
	if alt := s.Altitude(); alt < -2000 {
		t.Errorf("crash recorded at %.0f m, which is well inside the planet", alt)
	}
}

// Some regimes cannot be bought at any warp. Inside the air the step is fixed at
// 0.02 s, so a million times real time is fifty million steps a second: the call
// has to give up, say so, and still make progress.
func TestWarpIsLimitedWhereTheStepCannotGrow(t *testing.T) {
	s := New(Config{
		System:  earthMoon(),
		Atmo:    Atmosphere{Top: 140000},
		Rocket:  Rocket{Payload: 1000, Diameter: 1},
		MaxTime: 1e9,
	})
	// Inside the nominal atmosphere, so the step cannot grow, but on a circular
	// orbit in a configuration with no air pressure — nothing to slow it down, so
	// the run cannot end before the budget does.
	r := s.Cfg.Body.Radius + 20000
	s.St.Pos = Vec2{r, 0}
	s.St.Vel = Vec2{0, math.Sqrt(s.Cfg.Body.Mu / r)}
	s.St.Landed = false
	s.St.Phase = PhaseCoast
	s.WarpRate = 1e6

	before := s.St.T
	s.Advance(1e6 / 60)

	if !s.WarpLimited {
		t.Error("a million times real time inside the atmosphere went unnoticed")
	}
	if s.St.T <= before {
		t.Error("gave up without advancing at all")
	}
	if got := s.St.T - before; got > maxStepsPerAdvance*FixedStep+1e-9 {
		t.Errorf("advanced %.1f s, more than the step budget allows", got)
	}
}

// The capstone: three days to the Moon has to be a thing that can be run at all.
// Thirteen million fixed steps is not, and this is the whole reason the adaptive
// step exists.
func TestTranslunarCoastArrivesAtTheMoon(t *testing.T) {
	sys := earthMoon()
	moonR := sys.Bodies[1].SemiMajor
	rate := math.Sqrt(sys.Bodies[0].Mu / (moonR * moonR * moonR)) // the Moon's mean motion

	// A transfer whose apogee reaches the Moon's orbit, from a 185 km parking
	// orbit. Where the Moon has to be for the two to meet is not a thing to
	// guess: fly it once to find out when the vehicle gets there, then put the
	// Moon in that place.
	fly := func(meanAnom0 float64) *Sim {
		s := sys
		s.Bodies = append([]Body(nil), sys.Bodies...)
		s.Bodies[1].MeanAnom0 = meanAnom0
		s.Normalize()

		sim := New(Config{System: s, Atmo: Atmosphere{Top: 140000},
			Rocket: Rocket{Payload: 1000, Diameter: 1}, MaxTime: 1e9})
		rp := sys.Bodies[0].Radius + 185000
		a := (rp + moonR) / 2
		sim.St.Pos = Vec2{rp, 0}
		sim.St.Vel = Vec2{0, math.Sqrt(2 * sys.Bodies[0].Mu * (1/rp - 1/(2*a)))}
		sim.St.Landed = false
		sim.St.Phase = PhaseCoast
		sim.WarpRate = 1e9 // as high as the interface goes, and then some
		return sim
	}

	// Pass one, with the Moon parked out of the way behind the launch point.
	scout := fly(math.Pi)
	steps := 0
	for scout.St.T < 12*86400 && !scout.St.Done {
		if scout.RootPos().Len() >= moonR {
			break
		}
		scout.advanceOne(scout.plannedStep())
		steps++
	}
	arrival, where := scout.St.T, scout.RootPos().Angle()
	t.Logf("reached the Moon's orbit at T+%.2f days after %d steps, %.1f degrees round",
		arrival/86400, steps, where*180/math.Pi)

	// Pass two, with the Moon where the vehicle will be.
	s := fly(where - rate*arrival)
	steps = 0
	for s.St.Center == 0 && s.St.T < 12*86400 && !s.St.Done {
		s.advanceOne(s.plannedStep())
		steps++
	}
	if s.St.Center != 1 {
		t.Fatalf("no capture: still centred on %s at T+%.2f days, %.0f km out",
			s.Center().Name, s.St.T/86400, s.RootPos().Len()/1000)
	}

	o := ComputeOrbit(s.St.Pos, s.St.Vel, s.Center().Mu)
	t.Logf("entered the lunar sphere at T+%.2f days after %d steps, periselene %.0f km",
		s.St.T/86400, steps, o.PeriapsisAlt(s.Center().Radius)/1000)

	if steps > 5000 {
		t.Errorf("%d steps to the Moon; the fixed step would have taken %.0f, so this is no better",
			steps, s.St.T/FixedStep)
	}
	if s.St.T < 3*86400 || s.St.T > 7*86400 {
		t.Errorf("arrived at T+%.2f days, expected the three-to-six-day range of a real transfer",
			s.St.T/86400)
	}
}
