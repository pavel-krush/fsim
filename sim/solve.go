package sim

import "math"

// Solving a control point: what delta-v, right now, puts the trajectory where the plan says
// it should end up.
//
// The method is a bracket and a bisection over *flown copies* of the flight — the same
// integrator, the same plan, the same bodies — because nothing cheaper is honest. A conic
// approximation would give an answer that the flight then misses, which is exactly the
// failure a correction exists to prevent.
//
// Two rules the whole file is built around:
//
//   - The iteration count is fixed, never a time budget. A solver that stopped when it ran
//     out of milliseconds would make the trajectory depend on how busy the machine was,
//     which is the one thing this simulator refuses to let anything depend on.
//   - A copy never solves. It inherits whatever its control points have already been solved
//     to and flies those, so the recursion is one level deep by construction.

// How many flights one control point costs: the bracket plus the bisection, twenty-five in all.
// Sixteen halvings of a 150 m/s budget is a resolution of two thousandths of a metre a second,
// which is already far past what an aim can use — the chaos of the approach is worth kilometres,
// not millimetres — and each halving is a whole flight of the mission ahead.
const (
	solveBracket = 9
	solveHalving = 16
	// solveBurnStep is the step a solving flight integrates a vacuum burn at, and it is what a
	// control point costs. The correction engine here is small — a hydrazine thruster of a few
	// tens of newtons — so a candidate burn runs for minutes, which at the fixed 0.02 s is tens
	// of thousands of steps, and a control point flies twenty-five candidates inside the one
	// frame it comes due in. That was ten to thirty seconds of frozen interface per correction,
	// measured on Parker's three.
	//
	// A second a step is the figure a drawn prediction already uses, for the same reason: the
	// arc is smooth vacuum thrust, and the cutoff is solved rather than watched for, so what a
	// coarser step costs is the shape of the burn and not where it ends. It made no difference
	// to any solved delta-v here — Parker's three came out identical — and it is four to five
	// times less work.
	solveBurnStep = 1.0
)

// A solve runs one candidate flight at a time, spread over as many calls as it takes, and the
// mission clock does not advance while it does. Which is not an optimisation: a control point
// costs twenty-five flights of the mission ahead, and Parker's third correction is six seconds
// of them. Done inside one frame that is a window that has stopped responding, with no way to
// draw a word of explanation, because nothing can be drawn until the frame ends.
//
// The sequence of candidates is exactly the sequence the whole thing in one go would evaluate,
// so the answer does not depend on how many were run per call — and therefore not on the frame
// rate, the machine or the warp setting. That is the same promise the integrator makes.
type solveJob struct {
	node  int
	stage int
	// The bracket, and where the walk of it has got to.
	lo, hi, fLo, fHi float64
	okLo, okHi       bool
	k                int
	prevX, prevF     float64
	prevOK           bool
	Flights          int // how many candidates have been flown, for the interface to show
}

// The stages of a solve, in the order they run.
const (
	stageLow = iota
	stageHigh
	stageBracket
	stageBisect
)

// SolveFlights is what a control point costs in candidate flights, and what the interface
// counts against: the two ends of the bracket, the walk of it, and the bisection.
const SolveFlights = 2 + solveBracket + solveHalving

// Solving reports whether a correction is being worked out, and how far along it is. The flight
// screen says so, because a clock that has stopped with no explanation reads as a hang.
func (s *Sim) Solving() (node, flights int, active bool) {
	if s.job == nil {
		return 0, 0, false
	}
	return s.job.node, s.job.Flights, true
}

// PumpSolve does that much of a pending solve and reports whether one is still going. The
// interface calls it every frame, pause or no pause: a correction is not mission time, and a
// paused flight that has stopped inside a solve would never come out of it.
func (s *Sim) PumpSolve(budget int) bool {
	s.pumpSolve(budget)
	return s.job != nil
}

// startSolve opens a solve for node i. The state it is solved against is the state now, which
// is why it is started here rather than in advance: real navigation works the same way round.
func (s *Sim) startSolve(i int) {
	n := &s.Cfg.Nodes[i]
	if n.Limit <= 0 {
		n.Solved, n.Missed = true, true
		n.DeltaV = 0
		return
	}
	s.job = &solveJob{node: i, lo: -n.Limit, hi: n.Limit}
}

// finishSolve runs the rest of the solve at once. Anything that is not playing a flight to a
// human wants this: a test, a scripted capture, an instant run.
func (s *Sim) finishSolve() {
	for s.job != nil {
		s.pumpSolve(1)
	}
}

// pumpSolve flies up to that many candidates. One per frame is what the live flight does: a
// candidate is a hundred milliseconds or more and cannot be interrupted, so a smaller unit of
// work than one is not available.
func (s *Sim) pumpSolve(budget int) {
	for k := 0; k < budget && s.job != nil; k++ {
		if s.stepSolve() {
			i := s.job.node
			s.job = nil
			// Straight on to the burn, in this same call. Letting the flight take a step
			// first would light the engine a step late, and out here a step is minutes.
			s.igniteNode(i)
		}
	}
}

// stepSolve flies one candidate and reports whether the solve is finished.
//
// The bracket comes first: the aim is monotone in delta-v over any stretch that does not cross
// a sphere of influence, and not globally, so a sign change has to be found rather than assumed.
func (s *Sim) stepSolve() bool {
	j := s.job
	n := &s.Cfg.Nodes[j.node]
	miss := func(dv float64) (float64, bool) {
		j.Flights++
		return s.nodeMiss(j.node, dv)
	}

	switch j.stage {
	case stageLow:
		j.fLo, j.okLo = miss(j.lo)
		j.stage = stageHigh

	case stageHigh:
		j.fHi, j.okHi = miss(j.hi)
		switch {
		case j.okLo && j.okHi && j.fLo*j.fHi > 0:
			// Same side at both ends: walk the interval for a crossing.
			j.stage, j.k = stageBracket, 1
			j.prevX, j.prevF, j.prevOK = j.lo, j.fLo, j.okLo
		case !j.okLo || !j.okHi:
			// The aim cannot be measured at one end — the target body is never approached
			// at all, say. Nothing to bisect towards.
			s.applyNodeSolution(n, 0, true)
			return true
		default:
			j.stage, j.k = stageBisect, 0
		}

	case stageBracket:
		x := j.lo + (j.hi-j.lo)*float64(j.k)/float64(solveBracket)
		f, ok := miss(x)
		if ok && j.prevOK && f*j.prevF <= 0 {
			j.lo, j.hi, j.fLo, j.fHi = j.prevX, x, j.prevF, f
			j.stage, j.k = stageBisect, 0
			return false
		}
		j.prevX, j.prevF, j.prevOK = x, f, ok
		j.k++
		if j.k > solveBracket {
			// No crossing: the aim is out of reach with this much delta-v, and the better
			// of the two ends is what the flight gets.
			best := j.lo
			if math.Abs(j.fHi) < math.Abs(j.fLo) {
				best = j.hi
			}
			s.applyNodeSolution(n, best, true)
			return true
		}

	case stageBisect:
		mid := (j.lo + j.hi) / 2
		f, ok := miss(mid)
		if !ok {
			s.applyNodeSolution(n, (j.lo+j.hi)/2, false)
			return true
		}
		if f*j.fLo <= 0 {
			j.hi, j.fHi = mid, f
		} else {
			j.lo, j.fLo = mid, f
		}
		j.k++
		if j.k >= solveHalving {
			s.applyNodeSolution(n, (j.lo+j.hi)/2, false)
			return true
		}
	}
	return false
}

// applyNodeSolution writes a signed delta-v into a node as a magnitude and a direction.
func (s *Sim) applyNodeSolution(n *Node, dv float64, missed bool) {
	n.Solved, n.Missed = true, missed
	if dv < 0 {
		n.Frame = flipFrame(n.Frame)
		dv = -dv
	}
	n.DeltaV = dv
}

// flipFrame turns a burn direction around. A correction has to be able to go either way, and
// a signed delta-v is not something the burn machinery downstream accepts.
func flipFrame(f BurnFrame) BurnFrame {
	switch f {
	case BurnPrograde:
		return BurnRetrograde
	case BurnRetrograde:
		return BurnPrograde
	case BurnRadialOut:
		return BurnRadialIn
	case BurnRadialIn:
		return BurnRadialOut
	}
	return f
}

// nodeMiss flies a copy with node i set to dv and reports how far the result is from the aim,
// signed so that a bisection has something to cross. The second return is false when the aim
// could not be measured at all.
func (s *Sim) nodeMiss(i int, dv float64) (float64, bool) {
	c := s.solveCopy()
	n := &c.Cfg.Nodes[i]
	c.applyNodeSolution(n, dv, false)
	// The copy runs the node as an ordinary burn: it is already solved as far as it is
	// concerned, which is what keeps a solve out of a solve.
	n.Target = TargetNone

	n2 := s.Cfg.Nodes[i]
	switch n2.Target {
	case TargetFlybyPeriapsis:
		d, _, ok := c.flyPastBody(n2, false)
		return d - n2.TargetValue, ok
	case TargetPeriodAfterFlyby:
		_, p, ok := c.flyPastBody(n2, true)
		return p - n2.TargetValue, ok
	case TargetPeriod:
		return c.periodMiss(n2)
	}
	return 0, false
}

// solveCopy is a flight that shares nothing writable with this one.
func (s *Sim) solveCopy() *Sim {
	c := *s
	c.St.Prop = append([]float64(nil), s.St.Prop...)
	c.Cfg.Nodes = append([]Node(nil), s.Cfg.Nodes...)
	c.Hist, c.Events = nil, nil
	c.HistInterval = math.Inf(1)
	c.accum = 0
	c.WarpRate = math.Inf(1)
	c.burnStep = solveBurnStep
	// The coast is *not* coarsened, unlike a drawn prediction, and that is measured rather
	// than cautious: a coarser adaptive target buys nothing here, because the step is already
	// held down by how finely the approach has to be sampled to measure it. So there is no
	// accuracy to trade away in exchange for anything.
	// A copy never solves: it inherits whatever the control points have been solved to and
	// flies those, which is what keeps the recursion one level deep by construction. A
	// candidate that solved a later control point would cost twenty-seven flights inside one
	// of twenty-seven flights.
	c.job, c.noSolve = nil, true
	c.dropEphemeris()
	// The coast is *not* coarsened, unlike a drawn prediction. A prediction that is a few
	// metres off is a line a few metres off; a solve that is a few metres off is a flyby
	// eighteen kilometres from where it was aimed, which was measured. It costs four times
	// as much and one control point still solves in under half a second.
	return &c
}

// flyPastBody flies the next pass of the target body and reports either the signed distance
// of the closest approach or the orbital period the pass left behind.
//
// The sign of a distance is which side the vehicle went past: the cross product of the
// body-relative velocity and position at closest approach. Unsigned, the distance is V-shaped
// in delta-v and there is nothing for a bisection to cross; signed, it is monotone through
// zero — and the side is worth naming anyway, since it decides whether the pass adds energy
// or removes it.
//
// The step is bounded by the time left before arrival rather than by the horizon. A fixed step
// measures the closest approach to whatever it happens to sample — 1300 km of it at cruising
// speed, coarser than the aim being solved for, and the solver spent its precision chasing
// that instead of the trajectory.
func (c *Sim) flyPastBody(n Node, wantPeriod bool) (dist, period float64, ok bool) {
	if n.TargetBody < 0 || n.TargetBody >= len(c.Cfg.System.Bodies) {
		return 0, 0, false
	}
	horizon := n.Horizon
	if horizon <= 0 {
		horizon = 400 * 86400
	}
	end := c.St.T + horizon

	best, bestSign := math.Inf(1), 1.0
	rising := 0
	prev := math.Inf(1)
	for c.St.T < end && !c.St.Done {
		bp, bv := c.Cfg.System.StateAt(n.TargetBody, c.St.T)
		rel := c.RootPos().Sub(bp)
		relV := c.RootVel().Sub(bv)
		d := rel.Len()

		step := (end - c.St.T) / 400
		if step > 5*86400 {
			step = 5 * 86400
		}
		if closing := -rel.Dot(relV) / math.Max(d, 1); closing > 0 {
			if reach := d / (closing * 8); reach < step {
				step = reach
			}
		}
		if step < 0.5 {
			step = 0.5
		}

		if d < best {
			best = d
			bestSign = 1
			if relV.X*rel.Y-relV.Y*rel.X < 0 {
				bestSign = -1
			}
		}
		// Two steps of getting further away is a closest approach that has happened. For a
		// distance that is the answer; for a period the pass has to be *left*, because the
		// orbit it bought is only measurable from outside the sphere of influence.
		if d > prev {
			rising++
			done := rising >= 2 && best < math.Inf(1)
			if wantPeriod {
				done = done && c.St.Center == 0
			}
			if done {
				break
			}
		} else {
			rising = 0
		}
		prev = d

		before := c.St.T
		c.FastForward(c.St.T + step)
		if c.St.T <= before {
			break
		}
	}
	if math.IsInf(best, 1) {
		return 0, 0, false
	}
	dist = bestSign * best
	o := ComputeOrbit(c.RootPos(), c.RootVel(), c.Cfg.System.Bodies[0].Mu)
	period = 1e12
	if o.Bound() && o.Period > 0 {
		period = o.Period
	}
	// Unbound counts as infinitely long, which is the far side of every target a resonance
	// asks for and gives the search somewhere to walk.
	return dist, period, true
}

// periodMiss flies just past the burn and reports the orbital period it left behind, less the
// one asked for. An unbound orbit has no period, so it counts as infinitely long — which is
// the right side of every target and gives the bisection a direction to walk in.
func (c *Sim) periodMiss(n Node) (float64, bool) {
	horizon := n.Horizon
	if horizon <= 0 {
		horizon = 2 * 3600
	}
	end := c.St.T + horizon
	for c.St.T < end && !c.St.Done {
		before := c.St.T
		c.FastForward(math.Min(end, c.St.T+600))
		if c.St.T <= before {
			break
		}
		if c.St.Node < 0 && c.St.Phase == PhaseCoast && c.St.T > before {
			// The burn is over and the vehicle is coasting: whatever it is on now is
			// what the correction bought.
			break
		}
	}
	o := ComputeOrbit(c.St.Pos, c.St.Vel, c.Center().Mu)
	if !o.Bound() || o.Period <= 0 {
		return 1e12, true
	}
	return o.Period - n.TargetValue, true
}
