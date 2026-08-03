package sim

import "math"

// FixedStep is the integrator step in seconds of simulated time. Everything —
// real-time playback and instant runs alike — advances in multiples of this,
// so the trajectory does not depend on the frame rate or the time warp.
const FixedStep = 0.02

// Phase is where the vehicle is in its staging sequence.
type Phase int

const (
	PhaseBurn         Phase = iota // current stage's engine is running
	PhaseSepWait                   // engine shut down, waiting to separate
	PhaseIgnitionWait              // separated, waiting for the ignition cue
	PhaseCoast                     // nothing left to burn
)

// Outcome is the verdict once the flight is over.
type Outcome int

const (
	OutcomeFlying Outcome = iota
	OutcomeOrbit
	OutcomeDecaying // closed orbit, but the periapsis is inside the atmosphere
	OutcomeSuborbital
	OutcomeCrashed
	OutcomeEscape
	OutcomeTimeout
)

// EventKind marks a notable moment on the timeline.
type EventKind int

const (
	EvLiftoff EventKind = iota
	EvMaxQ
	EvCutoff
	EvSeparation
	EvIgnition
	EvApoapsis
	EvOrbit
	EvEnd
)

// Event is a timestamped marker used by the trajectory view and the graphs.
// It carries no text: what an event is called, and in which language, is a
// presentation decision that does not belong in the physics.
type Event struct {
	T    float64
	Kind EventKind
}

// Config is everything the user typed in on the setup screen.
type Config struct {
	// System is the world flown in. Leaving it empty means a system of one
	// body, built from Body — which is what every single-planet configuration
	// is, and what keeps the setup screen editing one planet.
	System System
	// LaunchBody indexes the body launched from, and Body is a copy of it.
	// New fills Body in from the system, so after construction it is a mirror
	// and not an input.
	LaunchBody int
	LaunchLon  float64 // rad, the launch site's angle on that body at t = 0

	Body    Body
	Atmo    Atmosphere
	Rocket  Rocket
	Program Program

	// TargetOrbit is the altitude the launch is aiming for, m. It only drives
	// the reference ring in the trajectory view.
	TargetOrbit float64
	// MaxTime caps the simulated flight, s.
	MaxTime float64
}

// State is the integrated state of the vehicle.
type State struct {
	T float64
	// Pos and Vel are measured from Center, the body whose sphere of influence
	// the vehicle is in, in a frame that does not rotate with it. Integrating
	// close to the nearest centre rather than out at the root is what keeps the
	// coordinates six digits shorter than the float they live in.
	Pos    Vec2
	Vel    Vec2
	Center int

	Prop  []float64 // remaining propellant per stage, kg
	Stage int       // index of the stage currently attached at the bottom

	Phase      Phase
	PhaseT     float64 // time spent in the current phase, s
	StageBurnT float64 // time the current stage has been burning, s
	Landed     bool

	DeltaV    float64 // m/s, ideal delta-v expended
	GravLoss  float64 // m/s
	DragLoss  float64 // m/s
	SteerLoss float64 // m/s

	Done    bool
	Outcome Outcome
}

// Sample is one recorded point of telemetry.
type Sample struct {
	T float64
	// Pos is measured from Center, as in State: a recorded track only means
	// something alongside the body it was measured from.
	Pos    Vec2
	Center int
	Alt    float64
	// Speed is measured in the inertial frame — the one that matters for
	// orbit. SurfSpeed, VertSpeed and HorizSpeed are relative to the rotating
	// ground, which is what the vehicle actually feels during the ascent.
	Speed      float64
	SurfSpeed  float64
	VertSpeed  float64
	HorizSpeed float64
	Mach       float64
	Q          float64
	Mass       float64
	AccelG     float64
	Pitch      float64
	ApoAlt     float64
	PeriAlt    float64
	Ecc        float64
	Density    float64
	Thrust     float64
	Drag       float64
	DeltaV     float64
	GravLoss   float64
	DragLoss   float64
	SteerLoss  float64
	PropFrac   []float64
}

// Telemetry is the instantaneous readout shown during flight.
type Telemetry struct {
	Sample
	Pressure  float64
	Temp      float64
	Sound     float64
	TWR       float64
	Orbit     Orbit
	Phase     Phase
	Stage     int
	Burning   bool
	PropLeft  []float64
	Downrange float64
}

// Sim owns a configuration, the live state and the recorded history.
type Sim struct {
	Cfg Config
	St  State

	Hist   []Sample
	Events []Event

	// HistInterval is the recording period in seconds of simulated time.
	HistInterval float64

	// WarpRate is the playback rate the caller is advancing at, and it caps how
	// far one coast step may reach: a step longer than a frame's worth of
	// simulated time would stutter instead of playing. At 1 the cap is the fixed
	// step, which is what keeps a real-time flight identical to a fixed-step run.
	WarpRate float64
	// WarpLimited says the last Advance ran out of its step budget before it
	// delivered the time asked for, so the flight is going slower than the warp
	// setting claims.
	WarpLimited bool

	coastH float64 // step the adaptive propagator wants next, s

	accum        float64 // leftover real time not yet turned into a fixed step
	surfaceP     float64
	launchAngle  float64
	maxQ         float64
	maxQAlt      float64
	maxQT        float64
	maxQMarked   bool
	lastQ        float64
	maxG         float64
	maxAlt       float64
	lastRecord   float64
	prevRadialV  float64
	reachedSpace bool
}

// New builds a simulation ready to run from the given configuration. The
// configuration is copied, so the caller can keep editing its own.
func New(cfg Config) *Sim {
	if len(cfg.System.Bodies) == 0 {
		cfg.System.Bodies = []Body{cfg.Body}
	}
	cfg.System.Normalize()
	if cfg.LaunchBody < 0 || cfg.LaunchBody >= len(cfg.System.Bodies) {
		cfg.LaunchBody = 0
	}
	// From here on the system is the truth and Body is its mirror, so that
	// everything reading Cfg.Body still gets the planet being launched from.
	cfg.Body = cfg.System.Bodies[cfg.LaunchBody]

	cfg.Atmo.Prepare(cfg.Body.SurfaceG)
	cfg.Program.Sort()

	s := &Sim{
		Cfg:          cfg,
		HistInterval: 0.1,
		WarpRate:     1,
		surfaceP:     cfg.Atmo.SurfacePressure,
	}
	s.Reset()
	return s
}

// Reset returns the vehicle to the pad.
func (s *Sim) Reset() {
	b := &s.Cfg.System.Bodies[s.Cfg.LaunchBody]
	w := b.AngularVelocity()

	st := State{
		Pos:    Vec2{math.Cos(s.Cfg.LaunchLon), math.Sin(s.Cfg.LaunchLon)}.Scale(b.Radius),
		Center: s.Cfg.LaunchBody,
		Prop:   make([]float64, len(s.Cfg.Rocket.Stages)),
		Landed: true,
	}
	st.Vel = st.Pos.Perp().Scale(w)
	s.launchAngle = st.Pos.Angle()
	for i := range s.Cfg.Rocket.Stages {
		st.Prop[i] = s.Cfg.Rocket.Stages[i].PropMass
	}
	if len(s.Cfg.Rocket.Stages) == 0 {
		st.Phase = PhaseCoast
	}

	s.St = st
	s.Hist = s.Hist[:0]
	s.Events = s.Events[:0]
	s.maxQ, s.maxQAlt, s.maxG, s.maxAlt = 0, 0, 0, 0
	s.maxQT, s.maxQMarked, s.lastQ = 0, false, 0
	s.accum = 0
	s.lastRecord = -1
	s.prevRadialV = 0
	s.reachedSpace = false
	s.mark(EvLiftoff)
	s.record()
}

// Mass of the vehicle right now, kg.
func (s *Sim) Mass() float64 {
	m := s.Cfg.Rocket.Payload
	for i := s.St.Stage; i < len(s.Cfg.Rocket.Stages); i++ {
		m += s.Cfg.Rocket.Stages[i].DryMass + s.St.Prop[i]
	}
	return m
}

// Center is the body the state is currently measured from.
func (s *Sim) Center() *Body { return &s.Cfg.System.Bodies[s.St.Center] }

// RootPos is the vehicle's position relative to the system's root. It is the
// one frame every body agrees on, so anything comparing across bodies goes
// through it.
func (s *Sim) RootPos() Vec2 {
	p, _ := s.Cfg.System.StateAt(s.St.Center, s.St.T)
	return p.Add(s.St.Pos)
}

// atmoTop is the top of the air around the body the state is measured from.
// Only the launch body has an atmosphere for now: air on every body needs the
// setup screen to be able to describe it first.
func (s *Sim) atmoTop() float64 {
	if s.St.Center == s.Cfg.LaunchBody {
		return s.Cfg.Atmo.Top
	}
	return 0
}

// refocus moves the state into the frame of whichever body now holds it. The
// transformation is exact — the same point expressed from a different centre —
// so nothing about the trajectory changes, only the numbers describing it.
func (s *Sim) refocus() {
	sys := &s.Cfg.System
	if len(sys.Bodies) == 1 {
		return
	}
	want := sys.Frame(s.RootPos(), s.St.T)
	if want == s.St.Center {
		return
	}
	dp, dv := sys.RelState(s.St.Center, want, s.St.T)
	s.St.Pos, s.St.Vel = s.St.Pos.Add(dp), s.St.Vel.Add(dv)
	s.St.Center = want
}

// Altitude above the surface of the body the state is measured from, m.
func (s *Sim) Altitude() float64 { return s.St.Pos.Len() - s.Center().Radius }

// MaxQ returns the peak dynamic pressure seen so far and the altitude it
// happened at.
func (s *Sim) MaxQ() (q, alt float64) { return s.maxQ, s.maxQAlt }

// MaxQTime is when the peak dynamic pressure happened, in seconds.
func (s *Sim) MaxQTime() float64 { return s.maxQT }

// MaxG is the peak acceleration in local surface gravities.
func (s *Sim) MaxG() float64 { return s.maxG }

// MaxAlt is the highest altitude reached, m.
func (s *Sim) MaxAlt() float64 { return s.maxAlt }

// Advance runs the simulation forward by dt seconds of simulated time. The
// leftover is carried into the next call, so a caller feeding it irregular frame
// times still gets exactly the same trajectory as an even one: the step is
// chosen from the state, and the accumulator only decides where a frame stops.
func (s *Sim) Advance(dt float64) {
	if dt <= 0 {
		return
	}
	s.accum += dt
	s.WarpLimited = false

	for n := 0; !s.St.Done; n++ {
		if n >= maxStepsPerAdvance {
			// Out of budget. Drop the debt rather than queue it: catching up
			// later would spend the next frames replaying this one.
			s.WarpLimited = true
			s.accum = 0
			return
		}
		h := s.plannedStep()
		if s.accum < h {
			return
		}
		took := s.advanceOne(h)
		if took <= 0 {
			return
		}
		s.accum -= took
	}
}

// FastForward runs the simulation up to time target with no regard for playback:
// no warp cap on the step and no budget on the work, so it takes as long as it
// takes and lands exactly on the time asked for.
//
// This is what a scripted jump wants. Advance is the interactive path and gives
// up when a frame's worth of work runs out, which is right for a frame and wrong
// for "show me the state four hours in".
func (s *Sim) FastForward(target float64) {
	for !s.St.Done && s.St.T < target {
		h := FixedStep
		if s.coasting() {
			h = s.coastTarget()
		}
		if h > target-s.St.T {
			h = target - s.St.T
		}
		if s.advanceOne(h) <= 0 {
			break
		}
	}
}

// RunToEnd integrates until the flight has a verdict, or the clock runs out
// without one. It stops at the verdict rather than at the end of the flight
// because an orbit does not have an end.
func (s *Sim) RunToEnd() {
	limit := s.Cfg.MaxTime
	if limit <= 0 {
		limit = 6 * 3600
	}
	// No warp cap here: an instant run has no playback to keep smooth, so the
	// step is whatever the state can carry. That is what makes a three-day
	// mission finish in the time it takes to ask for it.
	for !s.St.Done && !s.Settled() && s.St.T < limit {
		h := FixedStep
		if s.coasting() {
			h = s.coastTarget()
		}
		if s.advanceOne(h) <= 0 {
			break
		}
	}
	if !s.St.Done && !s.Settled() {
		s.stop(OutcomeTimeout)
	}
}

// burnContext freezes what the engine is doing for the duration of one
// integrator step, so that mass stays an exact linear function of time inside
// the Runge-Kutta stages.
type burnContext struct {
	on    bool
	mdot  float64
	m0    float64
	t0    float64
	stage *Stage
}

func (c burnContext) massAt(t float64) float64 {
	m := c.m0
	if c.on {
		m -= c.mdot * (t - c.t0)
	}
	if m < 1 {
		return 1
	}
	return m
}

// Step advances the state by at most dt seconds. The step is shortened if a
// staging event falls inside it, so discontinuities land exactly on a step
// boundary instead of being smeared across one.
func (s *Sim) Step(dt float64) {
	if s.St.Done || dt <= 0 {
		return
	}

	s.checkPhase()
	if s.St.Done {
		return
	}

	ctx := burnContext{m0: s.Mass(), t0: s.St.T}
	if s.St.Phase == PhaseBurn {
		stg := &s.Cfg.Rocket.Stages[s.St.Stage]
		ctx.on = true
		ctx.mdot = stg.MassFlow()
		ctx.stage = stg
		if ctx.mdot > 0 {
			// Time left in this burn, whichever ends it first. The step is
			// shortened to land exactly on that boundary — but only ever
			// shortened to a positive value, never to zero, or a short step
			// requested by the caller would look like a burnout.
			left := s.St.Prop[s.St.Stage] / ctx.mdot
			if stg.CutoffTime > 0 {
				if c := stg.CutoffTime - s.St.StageBurnT; c < left {
					left = c
				}
			}
			if left <= 0 {
				// The burn is over at this exact instant. checkPhase shuts the
				// engine down on the next call.
				s.St.Prop[s.St.Stage] = 0
				return
			}
			if left < dt {
				dt = left
			}
		}
	}

	f := s.forces(s.St.T, s.St.Pos, s.St.Vel, ctx)
	s.accumulate(f, ctx, dt)

	if s.St.Landed {
		if f.total().Dot(s.St.Pos.Unit()) <= 0 {
			s.holdOnPad(dt, ctx)
			return
		}
		s.St.Landed = false
	}

	s.integrate(dt, ctx)
	s.advanceClocks(dt, ctx)
	s.postStep()
}

// holdOnPad keeps the vehicle clamped to the ground while the engine cannot
// lift it. Propellant still burns — a rocket with a thrust-to-weight below one
// just sits there wasting it.
func (s *Sim) holdOnPad(dt float64, ctx burnContext) {
	w := s.Cfg.Body.AngularVelocity()
	s.St.Pos = s.St.Pos.Rotate(w * dt)
	s.St.Vel = s.St.Pos.Perp().Scale(w)
	s.advanceClocks(dt, ctx)
	s.postStep()
}

// integrate performs one classical Runge-Kutta 4 step on position and velocity.
func (s *Sim) integrate(dt float64, ctx burnContext) {
	t := s.St.T
	p0, v0 := s.St.Pos, s.St.Vel

	a1 := s.forces(t, p0, v0, ctx).total()
	p2 := p0.Add(v0.Scale(dt / 2))
	v2 := v0.Add(a1.Scale(dt / 2))

	a2 := s.forces(t+dt/2, p2, v2, ctx).total()
	p3 := p0.Add(v2.Scale(dt / 2))
	v3 := v0.Add(a2.Scale(dt / 2))

	a3 := s.forces(t+dt/2, p3, v3, ctx).total()
	p4 := p0.Add(v3.Scale(dt))
	v4 := v0.Add(a3.Scale(dt))

	a4 := s.forces(t+dt, p4, v4, ctx).total()

	dp := v0.Add(v2.Scale(2)).Add(v3.Scale(2)).Add(v4).Scale(dt / 6)
	dv := a1.Add(a2.Scale(2)).Add(a3.Scale(2)).Add(a4).Scale(dt / 6)

	s.St.Pos = p0.Add(dp)
	s.St.Vel = v0.Add(dv)
}

// advanceClocks moves time forward and burns the propellant consumed.
func (s *Sim) advanceClocks(dt float64, ctx burnContext) {
	s.St.T += dt
	s.St.PhaseT += dt
	if ctx.on {
		s.St.StageBurnT += dt
		i := s.St.Stage
		s.St.Prop[i] -= ctx.mdot * dt
		if s.St.Prop[i] < 0 {
			s.St.Prop[i] = 0
		}
	}
}

// forceSet is the breakdown of accelerations at one point in the step.
type forceSet struct {
	Grav   Vec2
	Thrust Vec2
	Drag   Vec2

	Mass      float64
	ThrustMag float64
	DragMag   float64
	Q         float64
	Pitch     float64
	Atmo      AtmoState
	RelSpeed  float64
	RelFPA    float64
}

func (f forceSet) total() Vec2 { return f.Grav.Add(f.Thrust).Add(f.Drag) }

// forces evaluates gravity, thrust and drag for a trial state inside a step.
func (s *Sim) forces(t float64, pos, vel Vec2, ctx burnContext) forceSet {
	var f forceSet
	b := s.Center()

	r := pos.Len()
	if r < 1 {
		r = 1
	}
	up := pos.Scale(1 / r)
	east := up.Perp()
	h := r - b.Radius

	f.Mass = ctx.massAt(t)
	if s.St.Center == s.Cfg.LaunchBody {
		f.Atmo = s.Cfg.Atmo.State(h)
	}
	f.Grav = s.Cfg.System.Gravity(s.St.Center, pos, t)

	// Velocity relative to the rotating atmosphere: what the airframe feels,
	// and the frame the pitch programme is judged against.
	vRel := vel.Sub(pos.Perp().Scale(b.AngularVelocity()))
	f.RelSpeed = vRel.Len()
	f.RelFPA = FlightPathAngle(pos, vRel)

	if f.Atmo.Density > 0 && f.RelSpeed > 0 {
		f.Q = 0.5 * f.Atmo.Density * f.RelSpeed * f.RelSpeed
		f.DragMag = f.Q * s.Cfg.Rocket.Cd * s.Cfg.Rocket.Area()
		f.Drag = vRel.Unit().Scale(-f.DragMag / f.Mass)
	}

	if ctx.on && ctx.stage != nil {
		fpa := FlightPathAngle(pos, vel)
		f.Pitch = s.Cfg.Program.Pitch(t, fpa)
		f.ThrustMag = ctx.stage.Thrust(f.Atmo.Pressure, s.surfaceP)
		f.Thrust = ThrustDirection(up, east, f.Pitch).Scale(f.ThrustMag / f.Mass)
	} else {
		f.Pitch = FlightPathAngle(pos, vel)
	}
	return f
}

// accumulate integrates the delta-v budget over one step using the forces at
// the start of the step. The classic decomposition: what the engine produced,
// minus what gravity, drag and off-prograde steering took away.
func (s *Sim) accumulate(f forceSet, ctx burnContext, dt float64) {
	at := f.ThrustMag / f.Mass

	// The ideal delta-v over the step is integrated in closed form rather than
	// by a left-hand rectangle: with a constant mass flow the mass drops
	// through the step, so the rectangle systematically undershoots.
	dv := at * dt
	if ctx.on && ctx.mdot > 0 && f.ThrustMag > 0 {
		if m1 := f.Mass - ctx.mdot*dt; m1 > 0 {
			dv = f.ThrustMag / ctx.mdot * math.Log(f.Mass/m1)
		}
	}
	s.St.DeltaV += dv
	s.St.DragLoss += f.DragMag / f.Mass * dt

	// Flight path angle relative to the ground: at liftoff this is 90 degrees,
	// so the full weight counts as a gravity loss, which is what it is.
	gamma := f.RelFPA
	if f.RelSpeed < 1 {
		gamma = f.Pitch
	}
	g := f.Grav.Len()
	s.St.GravLoss += g * math.Sin(gamma*math.Pi/180) * dt

	if at > 0 && f.RelSpeed >= 1 {
		alpha := (f.Pitch - gamma) * math.Pi / 180
		s.St.SteerLoss += dv * (1 - math.Cos(alpha))
	}

	s.lastQ = f.Q
	if f.Q > s.maxQ {
		s.maxQ = f.Q
		s.maxQAlt = s.Altitude()
		s.maxQT = s.St.T
	}
	if ag := f.total().Sub(f.Grav).Len() / s.Center().SurfaceG; ag > s.maxG {
		s.maxG = ag
	}
}

// checkPhase runs the staging state machine and the end-of-flight checks
// before each step.
func (s *Sim) checkPhase() {
	stages := s.Cfg.Rocket.Stages

	switch s.St.Phase {
	case PhaseBurn:
		stg := &stages[s.St.Stage]
		out := s.St.Prop[s.St.Stage] <= 1e-9
		cut := stg.CutoffTime > 0 && s.St.StageBurnT >= stg.CutoffTime-1e-9
		if out || cut {
			s.endBurn()
		}

	case PhaseSepWait:
		if s.St.PhaseT >= stages[s.St.Stage].SepDelay-1e-9 {
			s.St.Stage++
			s.setPhase(PhaseIgnitionWait)
			s.mark(EvSeparation)
		}

	case PhaseIgnitionWait:
		if s.readyToIgnite() {
			s.St.StageBurnT = 0
			s.setPhase(PhaseBurn)
			s.mark(EvIgnition)
		}
	}

	s.checkEnd()
}

// endBurn shuts the current stage down and decides what comes next.
func (s *Sim) endBurn() {
	last := s.St.Stage >= len(s.Cfg.Rocket.Stages)-1
	if last {
		s.setPhase(PhaseCoast)
		s.mark(EvCutoff)
		return
	}
	s.setPhase(PhaseSepWait)
	s.mark(EvCutoff)
}

// readyToIgnite tests the ignition cue of the stage now at the bottom.
func (s *Sim) readyToIgnite() bool {
	stg := &s.Cfg.Rocket.Stages[s.St.Stage]
	if s.St.Prop[s.St.Stage] <= 0 {
		return false
	}
	switch stg.Ignition {
	case IgniteAfterDelay:
		return s.St.PhaseT >= stg.IgnitionDelay
	case IgniteAtApoapsis:
		// Apoapsis is where the radial velocity flips from climbing to
		// falling. If we are already descending we have missed it, so light
		// the engine immediately rather than waiting for the next orbit.
		return s.St.Pos.Dot(s.St.Vel) <= 0
	default:
		return true
	}
}

func (s *Sim) setPhase(p Phase) {
	s.St.Phase = p
	s.St.PhaseT = 0
}

// checkEnd evaluates the termination conditions.
func (s *Sim) checkEnd() {
	if s.St.Done {
		return
	}
	b := s.Center()
	alt := s.Altitude()

	// Strictly below the surface: a vehicle still clamped to the pad sits at
	// exactly zero altitude and must not count as a crash.
	if !s.St.Landed && alt < 0 {
		if s.reachedSpace {
			s.finish(OutcomeSuborbital)
		} else {
			s.finish(OutcomeCrashed)
		}
		return
	}

	// The clock is there to cut short a flight that is going nowhere. A
	// vehicle that made orbit has somewhere to be, and gets to stay there.
	if s.Cfg.MaxTime > 0 && s.St.T >= s.Cfg.MaxTime && !s.Settled() {
		s.stop(OutcomeTimeout)
		return
	}

	if s.St.Phase != PhaseCoast || s.Settled() {
		return
	}

	o := ComputeOrbit(s.St.Pos, s.St.Vel, b.Mu)
	if o.Energy >= 0 {
		s.finish(OutcomeEscape)
		return
	}

	top := s.atmoTop()
	peri := o.PeriapsisAlt(b.Radius)
	switch {
	case peri >= top:
		s.settle(OutcomeOrbit)
	case peri >= 0 && alt > top:
		// The vehicle is above the air but its low point is still inside it:
		// a real orbit for a few revolutions, then it comes down.
		s.settle(OutcomeDecaying)
	}
}

// Settled reports whether the flight has a verdict yet. It is not the same as
// finished: reaching orbit is a result, not a reason to stop the clock.
func (s *Sim) Settled() bool { return s.St.Outcome != OutcomeFlying }

// settle records the verdict and lets the vehicle carry on flying. Watching
// the thing actually go round is most of the reward for getting it up there.
func (s *Sim) settle(o Outcome) {
	s.St.Outcome = o
	s.emitMaxQ()
	s.mark(EvOrbit)
}

// stop ends the run without overruling a verdict already reached. Running out
// of clock while in a perfectly good orbit is not a timeout, it is just the
// end of the recording.
func (s *Sim) stop(fallback Outcome) {
	if !s.Settled() {
		s.St.Outcome = fallback
	}
	s.St.Done = true
	s.emitMaxQ()
	s.mark(EvEnd)
	s.record()
}

func (s *Sim) finish(o Outcome) {
	s.St.Done = true
	s.St.Outcome = o
	s.emitMaxQ()
	s.mark(EvEnd)
	s.record()
}

// postStep hands the state to whichever body now holds it, updates the running
// maxima, detects apoapsis and records history.
func (s *Sim) postStep() {
	// First, because every reading below is relative to the centre. A frame
	// change only ever happens on a step boundary, so nothing is ever half in
	// one frame and half in another.
	s.refocus()

	alt := s.Altitude()
	if alt > s.maxAlt {
		s.maxAlt = alt
	}
	if !s.reachedSpace && alt > s.atmoTop() {
		s.reachedSpace = true
	}

	// Apoapsis is worth marking on the way up. In orbit it comes round every
	// revolution for ever, which is neither news nor a bounded list.
	radial := s.St.Pos.Unit().Dot(s.St.Vel)
	if s.prevRadialV > 0 && radial <= 0 && alt > 1000 && !s.Settled() {
		s.mark(EvApoapsis)
	}
	s.prevRadialV = radial

	s.checkMaxQPassed()
	s.record()
	s.checkEnd()
}

// mark appends a timeline event, collapsing duplicates at the same instant.
func (s *Sim) mark(k EventKind) {
	for i := range s.Events {
		if s.Events[i].Kind == k && math.Abs(s.Events[i].T-s.St.T) < 1e-6 {
			return
		}
	}
	s.Events = append(s.Events, Event{T: s.St.T, Kind: k})
}

// markAt inserts an event at a time that has already passed, keeping the list
// in chronological order.
func (s *Sim) markAt(t float64, k EventKind) {
	e := Event{T: t, Kind: k}
	i := len(s.Events)
	for i > 0 && s.Events[i-1].T > t {
		i--
	}
	s.Events = append(s.Events, Event{})
	copy(s.Events[i+1:], s.Events[i:])
	s.Events[i] = e
}

// emitMaxQ places the marker on the recorded peak, at most once. The flag it
// sets also stops the automatic detection, so nothing may call this before the
// peak is genuinely settled.
func (s *Sim) emitMaxQ() {
	if s.maxQ <= 0 || s.maxQMarked {
		return
	}
	s.maxQMarked = true
	s.markAt(s.maxQT, EvMaxQ)
}

// MarkMaxQ is the fallback for a flight that ended before the peak could be
// established by the automatic check — a launch cut short inside the
// atmosphere, say. It deliberately does nothing while the flight is still
// running: opening the graph screen mid-ascent used to call this and pin the
// marker permanently to whatever the running maximum happened to be.
func (s *Sim) MarkMaxQ() {
	if !s.St.Done {
		return
	}
	s.emitMaxQ()
}

// checkMaxQPassed emits the max-q marker as soon as the dynamic pressure has
// dropped far enough below the peak that no later peak is plausible. Waiting
// for the flight to finish would mean the marker never shows up during the
// part of the ascent where it is interesting.
func (s *Sim) checkMaxQPassed() {
	if s.maxQMarked || s.maxQ <= 0 {
		return
	}
	if s.lastQ < s.maxQ*0.25 && s.St.T > s.maxQT+2 {
		s.emitMaxQ()
	}
}

// coastRecordFactor is how much more sparsely the history is written once the
// flight has a verdict. Nothing changes in a stable orbit, and the flight now
// has no end, so recording it at full rate would grow without bound.
const coastRecordFactor = 50

// record appends a history sample if enough simulated time has passed.
func (s *Sim) record() {
	interval := s.HistInterval
	if s.Settled() {
		interval *= coastRecordFactor
	}
	if s.lastRecord >= 0 && s.St.T-s.lastRecord < interval && !s.St.Done {
		return
	}
	s.lastRecord = s.St.T
	t := s.Telemetry()
	s.Hist = append(s.Hist, t.Sample)
}

// Telemetry snapshots everything worth displaying about the current state.
func (s *Sim) Telemetry() Telemetry {
	b := s.Center()
	var t Telemetry

	ctx := burnContext{m0: s.Mass(), t0: s.St.T}
	if s.St.Phase == PhaseBurn && s.St.Stage < len(s.Cfg.Rocket.Stages) {
		stg := &s.Cfg.Rocket.Stages[s.St.Stage]
		ctx.on = true
		ctx.mdot = stg.MassFlow()
		ctx.stage = stg
	}
	f := s.forces(s.St.T, s.St.Pos, s.St.Vel, ctx)

	up := s.St.Pos.Unit()
	vRel := s.St.Vel.Sub(s.St.Pos.Perp().Scale(b.AngularVelocity()))

	t.T = s.St.T
	t.Pos = s.St.Pos
	t.Center = s.St.Center
	t.Alt = s.Altitude()
	t.Speed = s.St.Vel.Len()
	t.SurfSpeed = f.RelSpeed
	t.VertSpeed = up.Dot(vRel)
	t.HorizSpeed = vRel.Sub(up.Scale(t.VertSpeed)).Len()
	t.Mass = f.Mass
	t.Q = f.Q
	t.Density = f.Atmo.Density
	t.Pressure = f.Atmo.Pressure
	t.Temp = f.Atmo.Temp
	t.Sound = f.Atmo.Sound
	if f.Atmo.Sound > 0 {
		t.Mach = f.RelSpeed / f.Atmo.Sound
	}
	t.Thrust = f.ThrustMag
	t.Drag = f.DragMag
	t.Pitch = f.Pitch
	t.AccelG = f.Thrust.Add(f.Drag).Len() / b.SurfaceG
	t.DeltaV = s.St.DeltaV
	t.GravLoss = s.St.GravLoss
	t.DragLoss = s.St.DragLoss
	t.SteerLoss = s.St.SteerLoss

	t.Orbit = ComputeOrbit(s.St.Pos, s.St.Vel, b.Mu)
	t.ApoAlt = t.Orbit.ApoapsisAlt(b.Radius)
	t.PeriAlt = t.Orbit.PeriapsisAlt(b.Radius)
	t.Ecc = t.Orbit.Eccentricity

	if w := f.Mass * b.SurfaceG; w > 0 {
		t.TWR = f.ThrustMag / w
	}
	t.Phase = s.St.Phase
	t.Stage = s.St.Stage
	t.Burning = ctx.on
	t.PropLeft = s.St.Prop

	t.PropFrac = make([]float64, len(s.St.Prop))
	for i := range s.St.Prop {
		if m := s.Cfg.Rocket.Stages[i].PropMass; m > 0 {
			t.PropFrac[i] = s.St.Prop[i] / m
		}
	}

	// Downrange distance along the surface from the launch pad, following the
	// ground rather than the chord. The pad moves: measuring against its fixed
	// inertial angle would report the planet's own rotation as downrange, so
	// the launch site's angle is advanced by omega*t first. It is measured on
	// the launch body, which is not necessarily the one the state is centred
	// on any more.
	lb := &s.Cfg.System.Bodies[s.Cfg.LaunchBody]
	rel := s.St.Pos
	if s.St.Center != s.Cfg.LaunchBody {
		d, _ := s.Cfg.System.RelState(s.St.Center, s.Cfg.LaunchBody, s.St.T)
		rel = rel.Add(d)
	}
	ang := rel.Angle() - (s.launchAngle + lb.AngularVelocity()*s.St.T)
	for ang > math.Pi {
		ang -= 2 * math.Pi
	}
	for ang < -math.Pi {
		ang += 2 * math.Pi
	}
	t.Downrange = math.Abs(ang) * lb.Radius

	return t
}

// PadPos is where the launch site is right now, in the same frame as the state:
// measured from whichever body currently holds the vehicle. It rides around with
// its planet, so it is only under the vehicle at T+0.
func (s *Sim) PadPos() Vec2 {
	lb := &s.Cfg.System.Bodies[s.Cfg.LaunchBody]
	a := s.launchAngle + lb.AngularVelocity()*s.St.T
	pad := Vec2{math.Cos(a), math.Sin(a)}.Scale(lb.Radius)
	if s.St.Center != s.Cfg.LaunchBody {
		d, _ := s.Cfg.System.RelState(s.Cfg.LaunchBody, s.St.Center, s.St.T)
		pad = pad.Add(d)
	}
	return pad
}

// GroundFrame maps an inertial position recorded at time t into the frame that
// rotates with the planet, expressed as of the current instant. The trajectory
// view draws the flown path through this so that a launch reads as a climb
// straight off the pad instead of a 6 km sideways drift — the vehicle carries
// the launch site's eastward velocity, which is real but unhelpful to look at.
func (s *Sim) GroundFrame(p Vec2, t float64) Vec2 {
	w := s.Cfg.System.Bodies[s.Cfg.LaunchBody].AngularVelocity()
	if w == 0 {
		return p
	}
	return p.Rotate(w * (s.St.T - t))
}
