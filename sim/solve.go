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

// solveSteps is how many flights one control point costs: the bracket plus the bisection.
// Twenty-eight halvings of the limit is a resolution of a millimetre a second on a 300 m/s
// correction, which is far past what the aim can use.
const (
	solveBracket = 9
	solveHalving = 22
)

// solveNode fills in the delta-v for a control point and marks whether it got there.
//
// The sign convention is that a negative solution is the same burn the other way round: the
// node's frame is flipped and the magnitude kept, because the burn machinery downstream
// wants a positive delta-v and a direction.
func (s *Sim) solveNode(i int) {
	n := &s.Cfg.Nodes[i]
	if n.Limit <= 0 {
		n.Solved, n.Missed = true, true
		n.DeltaV = 0
		return
	}

	miss := func(dv float64) (float64, bool) { return s.nodeMiss(i, dv) }

	// Bracket first: the aim is monotone in delta-v over any stretch that does not cross a
	// sphere of influence, and not globally, so a sign change has to be found rather than
	// assumed.
	lo, hi := -n.Limit, n.Limit
	fLo, okLo := miss(lo)
	fHi, okHi := miss(hi)
	if okLo && okHi && fLo*fHi > 0 {
		// Same side at both ends. Walk the interval for a crossing; if there is none, the
		// aim is out of reach with this much delta-v and the best of the two ends is what
		// the flight gets.
		found := false
		prevX, prevF, prevOK := lo, fLo, okLo
		for k := 1; k <= solveBracket; k++ {
			x := lo + (hi-lo)*float64(k)/float64(solveBracket)
			f, ok := miss(x)
			if ok && prevOK && f*prevF <= 0 {
				lo, hi, fLo, fHi = prevX, x, prevF, f
				found = true
				break
			}
			prevX, prevF, prevOK = x, f, ok
		}
		if !found {
			best, bestF := lo, math.Abs(fLo)
			if math.Abs(fHi) < bestF {
				best, bestF = hi, math.Abs(fHi)
			}
			s.applyNodeSolution(n, best, true)
			return
		}
	}
	if !okLo || !okHi {
		// The aim cannot be measured at one end — the target body is never approached at
		// all, say. Nothing to bisect towards.
		s.applyNodeSolution(n, 0, true)
		return
	}

	for range solveHalving {
		mid := (lo + hi) / 2
		f, ok := miss(mid)
		if !ok {
			break
		}
		if f*fLo <= 0 {
			hi, fHi = mid, f
		} else {
			lo, fLo = mid, f
		}
	}
	s.applyNodeSolution(n, (lo+hi)/2, false)
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

	switch s.Cfg.Nodes[i].Target {
	case TargetFlybyPeriapsis:
		d, ok := c.flyPastBody(s.Cfg.Nodes[i], false)
		return d - s.Cfg.Nodes[i].TargetValue, ok
	case TargetPeriodAfterFlyby:
		p, ok := c.flyPastBody(s.Cfg.Nodes[i], true)
		return p - s.Cfg.Nodes[i].TargetValue, ok
	case TargetPeriod:
		return c.periodMiss(s.Cfg.Nodes[i])
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
func (c *Sim) flyPastBody(n Node, wantPeriod bool) (float64, bool) {
	if n.TargetBody < 0 || n.TargetBody >= len(c.Cfg.System.Bodies) {
		return 0, false
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
		return 0, false
	}
	if !wantPeriod {
		return bestSign * best, true
	}
	o := ComputeOrbit(c.RootPos(), c.RootVel(), c.Cfg.System.Bodies[0].Mu)
	if !o.Bound() || o.Period <= 0 {
		// Unbound is infinitely long, which is the far side of every target a resonance
		// asks for and gives the bisection somewhere to walk.
		return 1e12, true
	}
	return o.Period, true
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
