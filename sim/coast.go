package sim

import "math"

// The adaptive coast step. Three days of coasting to the Moon is 13 million
// fixed steps, which no amount of time warp can chew through; the same coast at
// ten minutes a step is nine thousand. So a vehicle that is only falling —
// engine off, out of the air, off the ground — gets an error-controlled step,
// and everything else stays on the fixed 0.02 s that the ascent was tuned on.
//
// What this costs, and it is worth being explicit about it: the trajectory now
// depends on the time warp, because the warp caps how far a single step may
// reach. It is bounded by the tolerance below rather than free, and at ×1 the
// cap equals the fixed step so nothing changes at all. The old promise — that
// the trajectory depended on neither the frame rate nor the warp — survives only
// in its first half. Frame rate still cannot matter: the step is a function of
// the state, and the accumulator decides where a frame stops, never how far a
// step goes.

// coastTol is the local position error a coast step is allowed, as a fraction of
// the distance to the centre. At 1e-11 a step out by the Moon is good to a
// centimetre, so a whole transfer lands inside a metre of the fixed-step answer.
const coastTol = 1e-11

// maxCoastStep is the longest step allowed however quiet the trajectory is.
// Beyond ten minutes the events found by watching for them between steps —
// apoapsis, a verdict, a sphere of influence crossed — get too coarse to place.
const maxCoastStep = 600.0

// maxStepsPerAdvance bounds the work in one call. Time warp asks for a fixed
// amount of simulated time and there are regimes that cannot be bought at any
// price: a million times real time inside the atmosphere is fifty million fixed
// steps a second. Past the cap the debt is dropped rather than queued, the
// simulation simply runs slower than asked, and WarpLimited says so.
const maxStepsPerAdvance = 20000

// coastState is the integrated pair for the coast propagator.
type coastState struct{ P, V Vec2 }

func (y coastState) add(k coastState, h float64) coastState {
	return coastState{y.P.Add(k.P.Scale(h)), y.V.Add(k.V.Scale(h))}
}

// coasting reports whether the vehicle is doing nothing but falling. Only then
// may the step grow.
//
// PhaseSepWait and PhaseIgnitionWait are excluded even though no engine is
// running: both have a timer that has to land on its exact instant, and a
// ten-minute step would sail straight past it. PhaseCoast is the only phase with
// nothing left to wait for — nothing can turn it back into a burn — so a long
// step cannot find itself thrusting halfway through.
func (s *Sim) coasting() bool {
	if s.St.Landed || s.St.Phase != PhaseCoast {
		return false
	}
	return s.Altitude() > s.atmoTop()
}

// coastFactor sets the target step as a fraction of the local timescale. It is
// conservative on purpose: the controller below can shrink a step it does not
// like, but it remembers nothing between steps, so a target that is usually
// accepted first time is worth more than an aggressive one.
const coastFactor = 0.01

// coastTarget is the step the state asks for, before any cap: a small fraction
// of the local orbital timescale, or of the time to fall the current distance,
// whichever is shorter. The second term is what keeps a fast flyby from
// proposing a step it will only have to reject.
//
// Deliberately carrying no state between steps. An adaptive controller normally
// remembers the step it settled on, which makes the trajectory depend on the
// history of the controller as well as on the state; this way the step is a pure
// function of where the vehicle is and how fast it is going, and the frame-rate
// promise needs no arguing about.
func (s *Sim) coastTarget() float64 {
	r := s.St.Pos.Len()
	mu := s.Center().Mu
	if r <= 0 || mu <= 0 {
		return FixedStep
	}
	orbital := math.Sqrt(r * r * r / mu)
	crossing := r / math.Max(s.St.Vel.Len(), 1)
	return clampStep(coastFactor * math.Min(orbital, crossing))
}

func clampStep(h float64) float64 {
	return math.Min(math.Max(h, FixedStep), maxCoastStep)
}

// plannedStep is how far the next step intends to go. It is a function of the
// state and of the warp rate, and of nothing else — in particular not of how
// long the last frame took.
func (s *Sim) plannedStep() float64 {
	if !s.coasting() {
		return FixedStep
	}

	h := math.Min(s.coastTarget(), s.stepCap())

	// Land exactly on a scheduled burn. Without this a ten-minute coast step
	// would notice the ignition ten minutes late, which for a three-second
	// correction is the difference between a transfer and a miss.
	if next := s.nextNodeTime(); next > s.St.T && next-s.St.T < h {
		h = next - s.St.T
	}

	// Never reach the air inside one step. The decision to take a long step is
	// made at the start of it, so without this a descending vehicle would come
	// out of a ten-minute step somewhere underground and report a crash from a
	// place it never flew through. Half the time to the boundary, because it is
	// accelerating downwards and will arrive sooner than this says.
	if vr := -s.St.Pos.Unit().Dot(s.St.Vel); vr > 0 {
		if room := s.Altitude() - s.atmoTop(); room > 0 {
			h = math.Min(h, 0.5*room/vr)
		}
	}
	return math.Max(h, FixedStep)
}

// plannedStepUncapped is plannedStep with no warp cap, which is what an instant
// run wants: RunToEnd, Predict and the tuners all step as far as the state allows.
func (s *Sim) plannedStepUncapped() float64 {
	rate := s.WarpRate
	s.WarpRate = math.Inf(1)
	h := s.plannedStep()
	s.WarpRate = rate
	return h
}

// stepCap is how far a single step may reach at the current warp rate. At ×1 it
// is the fixed step, which is what keeps a real-time flight identical to what
// the simulator has always produced.
func (s *Sim) stepCap() float64 {
	rate := s.WarpRate
	if rate < 1 || math.IsNaN(rate) {
		rate = 1
	}
	return math.Min(FixedStep*rate, maxCoastStep)
}

// advanceOne takes the next step and reports how much simulated time it
// consumed. A step no longer than the fixed one goes through the original
// integrator, so at ×1 — where the cap is the fixed step — a real-time flight is
// exactly what the simulator has always produced, to the last bit.
func (s *Sim) advanceOne(h float64) float64 {
	if h <= FixedStep || !s.coasting() {
		s.Step(h)
		return h
	}
	return s.coastStep(h)
}

// coastStep integrates one adaptive step of at most h seconds and returns how
// far it got. It cannot fail: a step whose error estimate is too large is
// retried shorter, down to the fixed step, which needs no estimate.
func (s *Sim) coastStep(h float64) float64 {
	s.checkPhase()
	if s.St.Done {
		return 0
	}
	if !s.coasting() {
		// The phase machine lit something — a node whose time had come. Hand it
		// to the fixed integrator, which is the only one that knows about
		// thrust; it runs checkPhase again, harmlessly.
		s.Step(FixedStep)
		return FixedStep
	}

	y := coastState{s.St.Pos, s.St.Vel}
	lossBefore := s.gravLossRate(s.St.Pos, s.St.Vel, s.St.T)

	for {
		fifth, fourth := s.cashKarp(y, s.St.T, h)
		err := fifth.P.Sub(fourth.P).Len()
		tol := coastTol * math.Max(y.P.Len(), 1)

		if err > tol && h > FixedStep {
			h = math.Max(FixedStep, h*shrinkFactor(err, tol))
			continue
		}

		s.St.Pos, s.St.Vel = fifth.P, fifth.V

		// Gravity losses over a long coast are integrated as a trapezoid, not as
		// the left-hand rectangle the fixed path can get away with: over ten
		// minutes the flight path angle turns through a lot, and one end of the
		// step is nothing like an average of it.
		after := s.gravLossRate(s.St.Pos, s.St.Vel, s.St.T+h)
		s.St.GravLoss += (lossBefore + after) / 2 * h

		s.St.T += h
		s.St.PhaseT += h
		s.postStep()
		return h
	}
}

// shrinkFactor is how much smaller a rejected step should be, for a fifth-order
// method. Floored so that one wild estimate cannot collapse the step to nothing.
func shrinkFactor(err, tol float64) float64 {
	if err <= 0 {
		return 1
	}
	return math.Max(0.2, 0.9*math.Pow(tol/err, 0.2))
}

// gravLossRate is the instantaneous gravity loss, m/s per second: the component
// of gravity along the flight path, measured against the ground-relative
// velocity for the same reason the fixed path does it — in the inertial frame a
// launch starts out horizontal and the loss would come out zero.
func (s *Sim) gravLossRate(pos, vel Vec2, t float64) float64 {
	vRel := vel.Sub(pos.Perp().Scale(s.Center().AngularVelocity()))
	gamma := FlightPathAngle(pos, vRel)
	g := s.Cfg.System.Gravity(s.St.Center, pos, t).Len()
	return g * math.Sin(gamma*math.Pi/180)
}

// cashKarp performs one embedded Runge-Kutta step, returning the fifth-order
// result and the fourth-order one it is compared against. Gravity does not
// depend on velocity — there is no air out here, by the definition of a coast —
// so the derivative is a plain function of position and time.
func (s *Sim) cashKarp(y coastState, t, h float64) (fifth, fourth coastState) {
	f := func(dt float64, y coastState) coastState {
		return coastState{y.V, s.Cfg.System.Gravity(s.St.Center, y.P, t+dt)}
	}

	k1 := f(0, y)
	k2 := f(h/5, y.add(k1, h/5))
	k3 := f(3*h/10, coastState{
		y.P.Add(k1.P.Scale(3 * h / 40)).Add(k2.P.Scale(9 * h / 40)),
		y.V.Add(k1.V.Scale(3 * h / 40)).Add(k2.V.Scale(9 * h / 40)),
	})
	k4 := f(3*h/5, coastState{
		y.P.Add(k1.P.Scale(3 * h / 10)).Sub(k2.P.Scale(9 * h / 10)).Add(k3.P.Scale(6 * h / 5)),
		y.V.Add(k1.V.Scale(3 * h / 10)).Sub(k2.V.Scale(9 * h / 10)).Add(k3.V.Scale(6 * h / 5)),
	})
	k5 := f(h, coastState{
		y.P.Sub(k1.P.Scale(11 * h / 54)).Add(k2.P.Scale(5 * h / 2)).Sub(k3.P.Scale(70 * h / 27)).Add(k4.P.Scale(35 * h / 27)),
		y.V.Sub(k1.V.Scale(11 * h / 54)).Add(k2.V.Scale(5 * h / 2)).Sub(k3.V.Scale(70 * h / 27)).Add(k4.V.Scale(35 * h / 27)),
	})
	k6 := f(7*h/8, coastState{
		y.P.Add(k1.P.Scale(1631 * h / 55296)).Add(k2.P.Scale(175 * h / 512)).Add(k3.P.Scale(575 * h / 13824)).Add(k4.P.Scale(44275 * h / 110592)).Add(k5.P.Scale(253 * h / 4096)),
		y.V.Add(k1.V.Scale(1631 * h / 55296)).Add(k2.V.Scale(175 * h / 512)).Add(k3.V.Scale(575 * h / 13824)).Add(k4.V.Scale(44275 * h / 110592)).Add(k5.V.Scale(253 * h / 4096)),
	})

	fifth = coastState{
		y.P.Add(k1.P.Scale(37 * h / 378)).Add(k3.P.Scale(250 * h / 621)).Add(k4.P.Scale(125 * h / 594)).Add(k6.P.Scale(512 * h / 1771)),
		y.V.Add(k1.V.Scale(37 * h / 378)).Add(k3.V.Scale(250 * h / 621)).Add(k4.V.Scale(125 * h / 594)).Add(k6.V.Scale(512 * h / 1771)),
	}
	fourth = coastState{
		y.P.Add(k1.P.Scale(2825 * h / 27648)).Add(k3.P.Scale(18575 * h / 48384)).Add(k4.P.Scale(13525 * h / 55296)).Add(k5.P.Scale(277 * h / 14336)).Add(k6.P.Scale(h / 4)),
		y.V.Add(k1.V.Scale(2825 * h / 27648)).Add(k3.V.Scale(18575 * h / 48384)).Add(k4.V.Scale(13525 * h / 55296)).Add(k5.V.Scale(277 * h / 14336)).Add(k6.V.Scale(h / 4)),
	}
	return fifth, fourth
}
