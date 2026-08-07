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
	// OutcomeCaptured is a closed orbit around a body other than the one launched
	// from, and OutcomeImpact is hitting one. Both name the body in
	// State.OutcomeBody, because "captured" without saying by what is not a
	// verdict.
	OutcomeCaptured
	OutcomeImpact
	// OutcomeReturned is coming down on the launch body after having been away
	// from it — out of its sphere of influence, or inside somebody else's. That
	// is a free return, and calling it a crash would be a strange way to
	// describe the only outcome Apollo 13 was hoping for. There is no entry
	// model behind it: the vehicle is flown down through the air it was launched
	// through, and the g-load it pulls on the way is on the graph to be read.
	OutcomeReturned
)

// outcomeRank orders the verdicts a flight can settle into by how much they say.
// A later verdict only replaces an earlier one if it says more: reaching orbit
// and then being captured by a moon is a lunar mission, not a demotion.
func outcomeRank(o Outcome) int {
	switch o {
	case OutcomeDecaying:
		return 1
	case OutcomeOrbit:
		return 2
	case OutcomeCaptured:
		return 3
	default:
		return 0
	}
}

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
	// EvSOIEnter and EvSOIExit are crossings into and out of a body's sphere of
	// influence, and name the body in Event.Body.
	EvSOIEnter
	EvSOIExit
)

// Event is a timestamped marker used by the trajectory view and the graphs.
// It carries no text: what an event is called, and in which language, is a
// presentation decision that does not belong in the physics.
type Event struct {
	T    float64
	Kind EventKind
	// Body is which body the event is about, for the kinds that are about one.
	// Zero is the root, which is meaningless for the rest and harmless.
	Body int
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
	// Nodes are the scheduled burns, in no particular order: the pitch programme
	// flies the ascent and these fly everything after it.
	Nodes []Node

	// TargetOrbit is the altitude the launch is aiming for, m. It only drives
	// the reference ring in the trajectory view.
	TargetOrbit float64
	// MaxTime caps the simulated flight, s.
	MaxTime float64
}

// EnsureSystem fills in whatever the configuration left out, so that everything
// downstream has a tree to work with: a single-planet configuration becomes a
// system of one body, the launch index is brought into range, and the derived
// quantities are filled in.
//
// Body is the launch body's editable face: it is copied *into* the system when it
// has a radius, then mirrored back. Copying the other way made editing the planet
// on a multi-body preset a silent no-op. A caller that fills the system and
// leaves Body empty — every test that builds one by hand — is left alone.
func (c *Config) EnsureSystem() {
	if len(c.System.Bodies) == 0 {
		c.System.Bodies = []Body{c.Body}
	}
	c.System.Normalize()
	if c.LaunchBody < 0 || c.LaunchBody >= len(c.System.Bodies) {
		c.LaunchBody = 0
	}
	if c.Body.Radius > 0 {
		c.System.Bodies[c.LaunchBody] = c.Body
		c.System.Normalize()
	}
	c.Body = c.System.Bodies[c.LaunchBody]
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

	// Node is the manoeuvre being flown, or -1. NodesDone marks the ones already
	// run as a bitmask, so the plan can be edited mid-flight without a cursor
	// into it going stale.
	Node      int
	NodeDV    float64 // m/s spent by the running node so far
	NodesDone uint64

	DeltaV    float64 // m/s, ideal delta-v expended
	GravLoss  float64 // m/s
	DragLoss  float64 // m/s
	SteerLoss float64 // m/s

	Done    bool
	Outcome Outcome
	// OutcomeBody is the body a verdict is about: captured by it, or hit it.
	OutcomeBody int
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
	// Steps counts the integrator steps taken, fixed and adaptive alike. Nothing in
	// the physics reads it: it is there so that the interface can say what a step
	// costs and how many of them a frame is buying, which is the difference between
	// "the simulation is slow" and "the drawing is slow". A prediction runs on a
	// copy, so its steps land in the copy's counter and not in this one.
	Steps int64

	// histThin multiplies the recording interval. It doubles every time the history
	// is halved, so a flight left running for months keeps costing the same rather
	// than being thinned again every few seconds. See maxHist.
	histThin float64

	coastH float64 // step the adaptive propagator wants next, s

	// The ephemeris cache. One Runge-Kutta step asks for gravity at three
	// distinct instants and evaluates four stages, and every answer costs a
	// Kepler solve per body — in a system of eighteen that was most of the cost
	// of a step. The positions are a pure function of the system and the time, so
	// caching them is safe as long as the system does not change mid-flight,
	// which nothing does: the setup screen edits its own copy.
	ephT    [ephSlots]float64
	ephHas  [ephSlots]bool
	ephPos  [ephSlots][]Vec2
	ephNext int

	coastScale   float64 // multiplier on the adaptive step; 0 means one
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
	// leftHome is set once the vehicle has been somewhere other than the sphere
	// of influence it launched into. It is what tells a return from a crash.
	leftHome bool
}

// New builds a simulation ready to run from the given configuration. The
// configuration is copied, so the caller can keep editing its own.
func New(cfg Config) *Sim {
	cfg.EnsureSystem()

	cfg.Atmo.Prepare(cfg.Body.SurfaceG)
	cfg.Program.Sort()

	s := &Sim{
		Cfg:          cfg,
		HistInterval: 0.1,
		histThin:     1,
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
		// No node running. The zero value would mean "node zero is burning",
		// which is a lively way to start a flight.
		Node: -1,
	}
	st.OutcomeBody = s.Cfg.LaunchBody
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
	s.leftHome = false
	s.Steps = 0
	s.histThin = 1
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

// ephSlots is how many instants the ephemeris cache holds. A step asks for t,
// t+h/2 twice and t+h, and the next step reuses its own t, so four is enough to
// catch every repeat without a map's overhead.
const ephSlots = 4

// gravity is Gravity with the ephemeris cache in front of it.
func (s *Sim) gravity(center int, rel Vec2, t float64) Vec2 {
	if len(s.Cfg.System.Bodies) == 1 {
		return s.Cfg.System.GravityFrom(center, rel, nil)
	}
	return s.Cfg.System.GravityFrom(center, rel, s.ephemeris(t))
}

// ephemeris returns the contributing bodies' root-frame positions at t, from the
// cache when that instant has been asked for before. The cache is keyed on time
// alone, so it is dropped when the frame changes: which bodies are in it depends
// on the centre.
func (s *Sim) ephemeris(t float64) []Vec2 {
	for i := range s.ephT {
		if s.ephHas[i] && s.ephT[i] == t {
			return s.ephPos[i]
		}
	}
	i := s.ephNext
	s.ephNext = (s.ephNext + 1) % ephSlots
	s.ephPos[i] = s.Cfg.System.Positions(s.St.Center, t, s.ephPos[i])
	s.ephT[i], s.ephHas[i] = t, true
	return s.ephPos[i]
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

// RootVel is the vehicle's velocity relative to the system's root.
func (s *Sim) RootVel() Vec2 {
	_, v := s.Cfg.System.StateAt(s.St.Center, s.St.T)
	return v.Add(s.St.Vel)
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
	s.ephHas = [ephSlots]bool{} // a different centre means a different set of bodies

	// Which way the crossing went: into a body's sphere if the one being left is
	// an ancestor of the one being entered, out of it otherwise.
	if sys.isAncestor(s.St.Center, want) {
		s.markBody(EvSOIEnter, want)
	} else {
		s.markBody(EvSOIExit, s.St.Center)
	}
	s.St.Center = want
	if want != s.Cfg.LaunchBody {
		s.leftHome = true
	}
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
		// The same step planner the live flight uses, minus the warp cap. Rolling
		// its own — "fixed step, or the coast target if coasting" — left out both
		// of the guards that live in it: a ten-minute step sailed past a scheduled
		// burn by up to ten minutes, which on a lunar insertion is the difference
		// between an orbit and a crater, and a descending vehicle could step
		// straight through the atmosphere.
		h := s.plannedStepUncapped()
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
		if s.advanceOne(s.plannedStepUncapped()) <= 0 {
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
			if s.St.Node < 0 && stg.CutoffTime > 0 {
				if c := stg.CutoffTime - s.St.StageBurnT; c < left {
					left = c
				}
			}
			if s.St.Node >= 0 {
				// Solve for the instant the node's delta-v lands, so that a
				// three metre a second correction is not overshot by a whole
				// step's worth of thrust.
				p := s.Cfg.Atmo.State(s.Altitude()).Pressure
				if c := s.nodeBurnLeft(stg.Thrust(p, s.surfaceP), ctx.mdot, ctx.m0); c < left {
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
	f.Grav = s.gravity(s.St.Center, pos, t)

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
		dir := ThrustDirection(up, east, s.Cfg.Program.Pitch(t, FlightPathAngle(pos, vel)))
		if s.St.Node >= 0 {
			// A node holds a direction rather than an angle: prograde is
			// whichever way the vehicle happens to be going, and writing that as
			// a pitch would go wrong the moment the orbit runs the other way.
			dir = s.Cfg.Nodes[s.St.Node].Direction(pos, vel)
		}
		// The pitch readout is what the thrust is actually doing, measured the
		// usual way: above the local horizon.
		f.Pitch = math.Atan2(dir.Dot(up), dir.Dot(east)) * 180 / math.Pi
		f.ThrustMag = ctx.stage.Thrust(f.Atmo.Pressure, s.surfaceP)
		f.Thrust = dir.Scale(f.ThrustMag / f.Mass)
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
	if s.St.Node >= 0 {
		s.St.NodeDV += dv
	}
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
	// Divided by standard gravity, not by the local surface value. A g is 9.80665
	// m/s^2 wherever the vehicle is: dividing by the body underneath made the
	// figure mean "local surface gravities", which read as six on a moon where the
	// crew would have felt one — and it stepped discontinuously the moment the
	// frame changed, because the divisor changed with it. The interface's own
	// thresholds are human tolerances, so they only ever meant real g.
	if ag := f.total().Sub(f.Grav).Len() / G0; ag > s.maxG {
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
		// A node's own delta-v is its cutoff; the stage's timer belongs to the
		// ascent and has already had its turn by the time a node can run.
		cut := s.St.Node < 0 && stg.CutoffTime > 0 && s.St.StageBurnT >= stg.CutoffTime-1e-9
		spent := s.St.Node >= 0 && s.St.NodeDV >= s.Cfg.Nodes[s.St.Node].DeltaV-1e-12
		switch {
		case s.St.Node >= 0 && (out || spent):
			// A node burn does not stage on its own: it uses what is attached, and
			// what is attached stays attached — unless the node says to drop it.
			drop := s.Cfg.Nodes[s.St.Node].Separate && s.St.Stage < len(stages)-1
			s.endNode()
			s.mark(EvCutoff)
			if drop {
				// Straight to separation, skipping the ignition-wait machinery:
				// that belongs to the ascent sequence, and a flight flown from a
				// plan lights its next engine when the plan says so.
				s.St.Stage++
				s.mark(EvSeparation)
			}
			s.setPhase(PhaseCoast)
		case out || cut:
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

	s.checkNodes()
	s.checkEnd()
}

// endBurn shuts the current stage down and decides what comes next.
func (s *Sim) endBurn() {
	next := s.St.Stage + 1
	// Nothing to hand over to: either there is no stage above, or the one above is
	// lit by the flight plan rather than by the sequence. In both cases the vehicle
	// coasts with what it has — including the stage that has just finished, which
	// is exactly what a spacecraft does with a spent upper stage it is not ready to
	// throw away yet.
	if next >= len(s.Cfg.Rocket.Stages) || s.Cfg.Rocket.Stages[next].Ignition == IgniteOnNode {
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
	case IgniteOnNode:
		// Never by itself. Belt and braces: endBurn does not hand over to a stage
		// like this in the first place.
		return false
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
		switch {
		case s.St.Center != s.Cfg.LaunchBody:
			// Hitting something that is not home is a different kind of news, and
			// which body it was is most of the news.
			s.St.OutcomeBody = s.St.Center
			s.finish(OutcomeImpact)
		case s.leftHome:
			// It went away and came back. Whether anything aboard would have
			// survived the entry is a question this simulator does not ask, but
			// arriving at the planet it left is not a crash.
			s.finish(OutcomeReturned)
		case outcomeRank(s.St.Outcome) >= outcomeRank(OutcomeOrbit):
			// It orbited and then came down. Calling that suborbital would be a
			// claim that it never got there.
			s.finish(OutcomeCrashed)
		case s.reachedSpace:
			s.finish(OutcomeSuborbital)
		default:
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

	// Settled is not the end of the story any more: a flight that made orbit can
	// still go on to be captured somewhere else, and settle only takes a verdict
	// that says more than the one it has.
	if s.St.Phase != PhaseCoast {
		return
	}

	// Escape is about leaving the system, so it is measured against the root — and
	// only asked at all when the root is what holds the vehicle. Inside a moon's
	// sphere of influence the question is meaningless: a craft in low lunar orbit
	// is moving faster than Earth escape at that distance, and is going nowhere.
	// The moon is on a rail, and the vehicle is attached to the moon.
	if s.St.Center == 0 {
		root := &s.Cfg.System.Bodies[0]
		if ro := ComputeOrbit(s.RootPos(), s.RootVel(), root.Mu); ro.Energy >= 0 {
			s.finish(OutcomeEscape)
			return
		}
	}

	o := ComputeOrbit(s.St.Pos, s.St.Vel, b.Mu)
	top := s.atmoTop()
	peri := o.PeriapsisAlt(b.Radius)
	switch {
	case s.St.Center == 0 && s.Cfg.LaunchBody != 0:
		// Out between the planets. A heliocentric orbit is not a capture — every
		// rock in the system is in one — and saying so would outrank the verdict
		// the flight actually earned. The first interplanetary preset reported
		// "in orbit around the Sun" from the moment it left the Earth behind.
	case s.St.Center != s.Cfg.LaunchBody:
		// Around something else entirely. A closed orbit that clears the surface
		// is a capture; a hyperbolic pass is just a visit, and says nothing until
		// it is over.
		if o.Bound() && peri >= 0 {
			s.St.OutcomeBody = s.St.Center
			s.settle(OutcomeCaptured)
		}
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
	if outcomeRank(o) <= outcomeRank(s.St.Outcome) {
		return
	}
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

// markBody appends an event about a particular body.
func (s *Sim) markBody(k EventKind, body int) {
	for i := range s.Events {
		if s.Events[i].Kind == k && s.Events[i].Body == body &&
			math.Abs(s.Events[i].T-s.St.T) < 1e-6 {
			return
		}
	}
	s.Events = append(s.Events, Event{T: s.St.T, Kind: k, Body: body})
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

// maxHist is how many samples the history holds before it starts throwing every
// second one away. A flight has no end once it has a verdict, so without a ceiling
// this is a leak with a physics engine attached: 93,000 samples and 43 MB of heap
// at T+600 days on the Mars preset, growing linearly for as long as it is left
// running, and every sample carries a PropFrac slice of its own for the collector
// to walk.
//
// Twenty thousand is about eight megabytes and more points than any plot has pixels
// for. Note that thinning the *rate* alone would not do it: during a coast a sample
// is written per integrator step and the steps are minutes long, so the interval is
// not what is binding — halving the stored history is.
const maxHist = 20000

// record appends a history sample if enough simulated time has passed.
func (s *Sim) record() {
	interval := s.HistInterval * s.histThin
	if s.Settled() {
		interval *= coastRecordFactor
	}
	if s.lastRecord >= 0 && s.St.T-s.lastRecord < interval && !s.St.Done {
		return
	}
	s.lastRecord = s.St.T
	t := s.Telemetry()
	s.Hist = append(s.Hist, t.Sample)
	if len(s.Hist) >= maxHist {
		s.thinHistory()
	}
}

// thinHistory halves the history, and halves the rate the rest of the flight is
// recorded at along with it.
//
// Keeping every second sample is a coarser record of the whole flight rather than a
// complete record of the recent part: an ascent five days back is still on the graph,
// at half the resolution it had. The alternative — dropping the oldest — would throw
// the launch away, which is the one part of a flight everybody wants to look at.
//
// The last sample is always kept. It is the one the interface reads as "now", and an
// even-length history would otherwise lose it.
func (s *Sim) thinHistory() {
	w := 0
	for i := 0; i < len(s.Hist); i += 2 {
		s.Hist[w] = s.Hist[i]
		w++
	}
	if last := len(s.Hist) - 1; last%2 != 0 {
		s.Hist[w] = s.Hist[last]
		w++
	}
	s.Hist = s.Hist[:w]
	s.histThin *= 2
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
	t.AccelG = f.Thrust.Add(f.Drag).Len() / G0
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
