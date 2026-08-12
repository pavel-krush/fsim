package sim

import "math"

// Manoeuvre nodes. The pitch programme is a schedule of angles against the clock
// and it is the right tool for an ascent, but it cannot express "two days from
// now, add three metres a second along the velocity" — which is the whole of
// what flying anywhere beyond the launch body consists of. A node is that
// sentence: a time, a direction to hold, and how much delta-v to spend.

// BurnFrame is what a node's thrust direction is measured against. All of them
// resolve to a direction in the frame of the body the vehicle is at, because
// that is the body the manoeuvre is about.
type BurnFrame int

const (
	// BurnPrograde holds the thrust along the velocity, which raises the far
	// side of the orbit. Retrograde lowers it.
	BurnPrograde BurnFrame = iota
	BurnRetrograde
	// BurnRadialOut points straight away from the body, radial in towards it.
	// Neither changes the orbit's size much; they rotate it.
	BurnRadialOut
	BurnRadialIn
	// BurnPitch holds a fixed angle above the local horizon, the same convention
	// the pitch programme uses.
	BurnPitch
)

// predCoastScale is how much coarser a predicted coast may step than a flown one.
const predCoastScale = 5

// predBurnStep is the step a predicted burn is integrated at, in seconds. See the
// loop below for why it is not the fixed step.
const predBurnStep = 1.0

// maxPredSteps bounds the work one prediction may do. A burn runs at the fixed
// step, and a long one is tens of thousands of them; the prediction is recomputed
// several times a second, so it needs a ceiling more than it needs to be complete.
const maxPredSteps = 20000

// MaxNodes is how many manoeuvres a flight plan may hold. The executed ones are
// tracked as a bitmask in the state, which is what sets the ceiling — and a
// flight plan with more than a few dozen burns in it is not a flight plan.
// Exported because a plan can arrive from outside the program — a saved setup —
// and whoever reads one has to know what will fit.
const MaxNodes = 64

// NodeTarget is what a control point aims at, when it is not simply told what to burn.
//
// A node with a target is a correction rather than a manoeuvre: it says where the
// trajectory should end up and the delta-v is solved for when the moment arrives. That is
// what a real trajectory correction is, and it is the only way a chain of gravity assists
// can be written down at all — a chained flyby amplifies the last bit of the integrator, so
// a plan of fixed numbers is a plan solved for one exact path through the arithmetic and no
// other. A correction re-solves against the path the flight is actually on.
type NodeTarget int

const (
	// TargetNone is a plain burn: DeltaV is what it spends.
	TargetNone NodeTarget = iota
	// TargetFlybyPeriapsis aims the closest approach to TargetBody at TargetValue metres
	// from its centre. The value is *signed*: which side the vehicle passes decides
	// whether the pass adds energy or takes it away, so it is part of the aim and not a
	// detail of it.
	TargetFlybyPeriapsis
	// TargetPeriod aims the orbital period about the body the vehicle is at, in seconds,
	// straight out of this burn.
	TargetPeriod
	// TargetPeriodAfterFlyby aims the period the *next pass of TargetBody* leaves behind.
	// This is how a resonant return is actually written: the propellant to reshape an orbit
	// by months is not aboard, and does not have to be — the flyby does the reshaping and
	// the correction only decides where the flyby happens. A few metres a second a hundred
	// million kilometres out is worth kilometres of aim at the planet.
	TargetPeriodAfterFlyby
	// TargetStation keeps station near TargetBody's second Lagrange point, and it exists
	// because nothing else can express that. The point is a saddle: a shade under the right
	// insertion and the vehicle falls back down the well, a shade over and it drifts off along
	// the anti-Sun line, and neither the distance to anything nor any period says which side of
	// the ridge the trajectory is on. What does is where the vehicle *ends up*, so this aims the
	// signed offset from the point — measured along the line out from the parent, at the end of
	// Horizon — at TargetValue metres, which is normally zero. Falling back reads negative and
	// drifting out reads positive, so the residual crosses zero once and a bisection can find it.
	//
	// The instability is what makes it work: over a hundred days a metre a second of error grows
	// into millions of kilometres, so the measurement is enormously sensitive in exactly the
	// direction the solve needs. It is the same thing a real halo-orbit targeter does — propagate,
	// see which way it runs, correct.
	TargetStation
	// TargetKinds is how many there are, which is what the interface's cycle button counts with.
	TargetKinds
)

// Node is one scheduled burn, or one control point.
type Node struct {
	T      float64   // s since liftoff, when to light the engine
	Frame  BurnFrame //
	Pitch  float64   // deg above the local horizon, for BurnPitch
	DeltaV float64   // m/s of ideal delta-v to spend before shutting down
	// Separate drops the stage this burn used once it is over. A spent booster
	// carried through a coast has to go before the engine above it can fire.
	Separate bool

	// Target, and what it wants. With a target, DeltaV is an output: Limit is the most
	// the correction may spend and Horizon how far ahead the aim is measured.
	Target      NodeTarget
	TargetBody  int
	TargetValue float64
	Limit       float64
	Horizon     float64

	// Two aims at once — a safe pass distance *and* a particular period, which is what a
	// resonant return wants — are not held by one node. They are held by two, an early one
	// for the energy and a late one for the aim, which is how a real design separates them
	// and needs no two-dimensional solver: see solve.go.

	// Solved says the delta-v in DeltaV came from the solver rather than from an author,
	// and Missed that the solver could not get there. Both are read by the interface and
	// neither is an input.
	Solved bool
	Missed bool
}

// Direction is the unit vector the node wants thrust along, given the state
// relative to the body the vehicle is at.
func (n *Node) Direction(pos, vel Vec2) Vec2 {
	switch n.Frame {
	case BurnRetrograde:
		return vel.Unit().Scale(-1)
	case BurnRadialOut:
		return pos.Unit()
	case BurnRadialIn:
		return pos.Unit().Scale(-1)
	case BurnPitch:
		up := pos.Unit()
		return ThrustDirection(up, up.Perp(), n.Pitch)
	default:
		// Prograde. With no velocity to speak of there is no prograde either, so
		// fall back to straight up rather than to a zero vector.
		if vel.Len() < 1e-9 {
			return pos.Unit()
		}
		return vel.Unit()
	}
}

// pendingNode is the earliest node that has not run yet, or -1. Nodes are held
// in whatever order they were typed in and searched by time, so editing a time
// mid-flight cannot make the plan skip one.
func (s *Sim) pendingNode() int {
	best := -1
	for i := range s.Cfg.Nodes {
		if i >= MaxNodes || s.St.NodesDone&(1<<uint(i)) != 0 {
			continue
		}
		if best < 0 || s.Cfg.Nodes[i].T < s.Cfg.Nodes[best].T {
			best = i
		}
	}
	return best
}

// nextNodeTime is when the next burn is due, or +Inf. The step planner clamps to
// it so that an ignition lands on its exact instant instead of being noticed ten
// minutes late by a coast step.
func (s *Sim) nextNodeTime() float64 {
	i := s.pendingNode()
	if i < 0 {
		return math.Inf(1)
	}
	return s.Cfg.Nodes[i].T
}

// checkNodes starts a burn whose time has come. It is called from the phase
// machine, so a node can only ever light between steps.
func (s *Sim) checkNodes() {
	if s.St.Node >= 0 || s.St.Landed {
		return
	}
	i := s.pendingNode()
	if i < 0 || s.St.T < s.Cfg.Nodes[i].T-1e-9 {
		return
	}

	// Only a vehicle with nothing left to wait for can take a node: in the
	// middle of staging, the sequence it is already running comes first.
	if s.St.Phase != PhaseCoast {
		return
	}

	// A burn needs an engine, and an empty stage is not one. This only ever
	// happens to a stage the sequence declined to hand over — one carrying an
	// IgniteOnNode stage above it — and carrying a dead stage into a burn is not
	// something any flight does, so it goes overboard here rather than blocking
	// the plan. Without this the node was quietly marked spent and the mission
	// simply stopped.
	for s.St.Stage < len(s.Cfg.Rocket.Stages)-1 && s.St.Prop[s.St.Stage] <= 1e-9 {
		s.St.Stage++
		s.mark(EvSeparation)
	}

	// A control point is solved at the instant it is reached, which is when the state it
	// has to correct is finally known. Real navigation works the same way round.
	//
	// The solve is spread over the calls that follow rather than done here: it is twenty-five
	// flights of the mission ahead, and no clock advances until it is finished. Whoever
	// finishes it calls igniteNode, so the burn still starts at this exact instant.
	if s.Cfg.Nodes[i].Target != TargetNone && !s.Cfg.Nodes[i].Solved {
		if s.noSolve {
			// A copy never solves, and this is where that rule is enforced rather than
			// merely stated. A control point costs twenty-seven flights of the mission
			// ahead, so a copy that solved one would cost that on top of its own flight —
			// which is what a drawn prediction was doing from the moment a control point
			// came inside its horizon: 2.4 seconds per prediction, several times over on
			// the way to Venus, and reported as the simulation freezing. A pending
			// correction is one nobody knows the size of yet, so the copy flies the path
			// without it and the drawn curve says what happens if nothing is done.
			s.igniteNode(i)
			return
		}
		s.startSolve(i)
		if s.job != nil {
			return
		}
	}

	s.igniteNode(i)
}

// igniteNode is the rest of checkNodes, from the point where the delta-v is known: mark the node
// spent and light the engine, or mark it spent and do not.
func (s *Sim) igniteNode(i int) {
	s.St.NodesDone |= 1 << uint(i)
	if s.Cfg.Nodes[i].DeltaV <= 0 || s.St.Stage >= len(s.Cfg.Rocket.Stages) ||
		s.St.Prop[s.St.Stage] <= 1e-9 {
		// Nothing to burn with, or nothing asked for. The node is spent either
		// way: leaving it pending would have it retried every step for ever.
		return
	}

	s.St.Node = i
	s.St.NodeDV = 0
	s.St.StageBurnT = 0
	s.setPhase(PhaseBurn)
	s.mark(EvIgnition)
}

// nodeBurnLeft is how long the running node still needs, in seconds, from the
// rocket equation: the exact time at which the remaining delta-v lands. Solving
// it rather than watching for the total to be passed is what keeps a three metre
// a second correction from overshooting by a step's worth of thrust.
func (s *Sim) nodeBurnLeft(thrust, mdot, mass float64) float64 {
	if s.St.Node < 0 || thrust <= 0 || mdot <= 0 || mass <= 0 {
		return math.Inf(1)
	}
	want := s.Cfg.Nodes[s.St.Node].DeltaV - s.St.NodeDV
	if want <= 0 {
		return 0
	}
	// dv = F/mdot * ln(m0/(m0 - mdot*t)), inverted.
	return mass / mdot * (1 - math.Exp(-want*mdot/thrust))
}

// RemoveNode deletes node i and repairs everything that pointed at it: the index
// of the burn in progress, and the mask of the ones already spent. A flight plan
// is edited while it is being flown, so deleting from it has to be a change of
// plan rather than a crash — and deleting the burn that is *running* has to shut
// the engine down, not leave it thrusting for a plan that no longer exists.
func (s *Sim) RemoveNode(i int) {
	if i < 0 || i >= len(s.Cfg.Nodes) {
		return
	}
	s.Cfg.Nodes = append(s.Cfg.Nodes[:i], s.Cfg.Nodes[i+1:]...)

	// The bits above i shift down with the slice; the ones below stay put.
	low := s.St.NodesDone & (1<<uint(i) - 1)
	high := s.St.NodesDone >> uint(i+1) << uint(i)
	s.St.NodesDone = low | high

	switch {
	case s.St.Node == i:
		s.endNode()
		s.setPhase(PhaseCoast)
		s.mark(EvCutoff)
	case s.St.Node > i:
		s.St.Node--
	}
}

// endNode releases the vehicle from a node burn.
func (s *Sim) endNode() {
	s.St.Node = -1
	s.St.NodeDV = 0
}

// PredPoint is one sample of a predicted path, in the frame of Center.
type PredPoint struct {
	T      float64
	Pos    Vec2
	Center int
}

// Predict runs a copy of the simulation forward and reports the path it would
// follow, nodes and staging included. That is the point of it: an osculating
// ellipse says where the vehicle is going if nothing happens, and the whole
// reason a node exists is that something is about to.
//
// The copy shares nothing that gets written to, and its history and events are
// thrown away — a prediction must not be able to disturb the flight it predicts.
func (s *Sim) Predict(horizon float64, maxPoints int) []PredPoint {
	if horizon <= 0 || maxPoints < 2 {
		return nil
	}

	c := *s
	c.St.Prop = append([]float64(nil), s.St.Prop...)
	// The plan is a slice, so a copy of the Sim shares it. That was harmless while every
	// node was a fixed number; a control point writes its solved delta-v back into the
	// plan, and a prediction solving one would edit the flight's own.
	c.Cfg.Nodes = append([]Node(nil), s.Cfg.Nodes...)
	c.Hist, c.Events = nil, nil
	c.HistInterval = math.Inf(1)
	c.accum = 0
	c.dropEphemeris()
	c.job, c.noSolve = nil, true
	// Uncapped, whatever the live flight is playing at. Inheriting the warp rate
	// would leave the prediction running fixed 0.02 s steps at ×1 — and taking
	// the step size from here while routing on that cap means a minute-long
	// fixed step, which in low orbit is not a trajectory at all.
	c.WarpRate = math.Inf(1)
	// And a coarser coast than the flight itself: the path is being drawn, not
	// flown, so a step five times longer costs a few metres of a forty-thousand
	// kilometre arc and five times less work. In low orbit that is the difference
	// between seventeen hundred steps to reach a planned burn and three hundred.
	c.coastScale = predCoastScale

	// The prediction is the flight, not a sketch of it: the same integrator on the
	// same plan. So a burn runs at the same fixed 0.02 s as the real one, and the
	// points are *sampled* out of it rather than taken one per step — the first
	// cut recorded every step and spent all four hundred points on the first
	// eight seconds of a translunar burn.
	end := s.St.T + horizon
	interval := horizon / float64(maxPoints)
	out := make([]PredPoint, 0, maxPoints)
	out = append(out, PredPoint{c.St.T, c.St.Pos, c.St.Center})
	last := c.St.T

	for steps := 0; steps < maxPredSteps && len(out) < maxPoints && !c.St.Done && c.St.T < end; steps++ {
		h := FixedStep
		if c.St.Phase == PhaseBurn && c.Altitude() > c.AtmoTop() {
			// A planned burn in vacuum is a smooth arc, and a preview of it does
			// not need the ascent's step: a translunar injection is five hundred
			// seconds long, which at 0.02 s is twenty-five thousand steps of work
			// several times a second. At a second a step the drawn path moves by
			// centimetres and the cost drops by fifty.
			h = predBurnStep
		}
		if c.coasting() {
			// The same step planner as everywhere else, which brings the guards
			// with it: land on a scheduled burn, and do not reach the air. Forcing
			// the step up to the sampling interval — "no point integrating finer
			// than the drawing" — was a mistake that cost four times what it
			// saved, because the error control rejected the oversized step and
			// halved it two or three times at six gravity evaluations a try.
			// Recording is decoupled from stepping below.
			h = c.plannedStepUncapped()
		}
		if c.advanceOne(h) <= 0 {
			break
		}
		if c.St.T-last >= interval {
			out = append(out, PredPoint{c.St.T, c.St.Pos, c.St.Center})
			last = c.St.T
		}
	}
	if n := len(out); n > 0 && out[n-1].T < c.St.T {
		out = append(out, PredPoint{c.St.T, c.St.Pos, c.St.Center})
	}
	return out
}
