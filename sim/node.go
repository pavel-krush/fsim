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

// maxNodes is how many manoeuvres a flight plan may hold. The executed ones are
// tracked as a bitmask in the state, which is what sets the ceiling — and a
// flight plan with more than a few dozen burns in it is not a flight plan.
const maxNodes = 64

// Node is one scheduled burn.
type Node struct {
	T      float64   // s since liftoff, when to light the engine
	Frame  BurnFrame //
	Pitch  float64   // deg above the local horizon, for BurnPitch
	DeltaV float64   // m/s of ideal delta-v to spend before shutting down
	// Separate drops the stage this burn used once it is over. A spent booster
	// carried through a coast has to go before the engine above it can fire.
	Separate bool
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
		if i >= maxNodes || s.St.NodesDone&(1<<uint(i)) != 0 {
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
	c.Hist, c.Events = nil, nil
	c.HistInterval = math.Inf(1)
	c.accum = 0
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
		if c.St.Phase == PhaseBurn && c.Altitude() > c.atmoTop() {
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
