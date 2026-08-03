package sim

import "math"

// System is the set of bodies the vehicle flies among: a tree with the root at
// index 0. The root is the one body that does not move. Everything else runs on
// Keplerian rails about its parent, evaluated analytically at any time, so a
// jump of three days costs no more than a jump of a second and nothing drifts.
//
// The tree is a flat slice with one invariant: a body's parent always sits at a
// lower index. A cycle then cannot be expressed at all, every walk up the tree
// terminates by construction, and the slice is already in topological order.
type System struct {
	Bodies []Body
}

// Normalize derives every body's dependent quantities and its sphere of
// influence, and repairs the parent invariant.
func (s *System) Normalize() {
	for i := range s.Bodies {
		b := &s.Bodies[i]
		b.Normalize()

		if i == 0 {
			b.Parent = -1
			b.SOI = math.Inf(1)
			continue
		}
		// Authored data with a missing parent, or one defined later in the
		// slice, falls back to orbiting the root: wrong, but finite.
		if b.Parent < 0 || b.Parent >= i {
			b.Parent = 0
		}
		if p := &s.Bodies[b.Parent]; b.SemiMajor > 0 && p.Mass > 0 {
			b.SOI = b.SemiMajor * math.Pow(b.Mass/p.Mass, 0.4)
		} else {
			b.SOI = 0
		}
	}
}

// StateAt returns body i's position and velocity relative to the root, m and
// m/s. The root itself is always at rest at the origin.
func (s *System) StateAt(i int, t float64) (pos, vel Vec2) {
	for j := i; j > 0; j = s.Bodies[j].Parent {
		p, v := s.orbitState(j, t)
		pos, vel = pos.Add(p), vel.Add(v)
	}
	return pos, vel
}

// RelState returns body i's state relative to body j.
func (s *System) RelState(i, j int, t float64) (pos, vel Vec2) {
	pi, vi := s.StateAt(i, t)
	pj, vj := s.StateAt(j, t)
	return pi.Sub(pj), vi.Sub(vj)
}

// orbitState is body i's state relative to its parent, from the rails.
//
// The two-body parameter is the parent's mu alone, not the sum of the two
// masses that the exact relative motion would use. That is the deliberate half
// of the bargain: with the parent nailed in place, using the parent's mu makes
// the rails agree exactly with the parent's real pull at the child's centre, so
// a vehicle in orbit around the child feels nothing but the tidal residual.
// Using the sum would put the sidereal month right — the Moon is a full 1/81 of
// the Earth, so it comes out 0.47% long this way — at the price of a phantom
// 3.3e-5 m/s^2 everywhere near the child. Nobody can see the month; everybody
// would see the orbit drift.
func (s *System) orbitState(i int, t float64) (Vec2, Vec2) {
	b := &s.Bodies[i]
	if b.Parent < 0 || b.SemiMajor <= 0 {
		return Vec2{}, Vec2{}
	}
	mu := s.Bodies[b.Parent].Mu
	if mu <= 0 {
		return Vec2{}, Vec2{}
	}

	a, e := b.SemiMajor, b.Ecc
	n := math.Sqrt(mu / (a * a * a))
	sinE, cosE := math.Sincos(solveKepler(b.MeanAnom0+n*t, e))

	f := math.Sqrt(1 - e*e)
	d := 1 - e*cosE
	pos := Vec2{a * (cosE - e), a * f * sinE}
	vel := Vec2{-a * n * sinE / d, a * n * f * cosE / d}
	if b.ArgPeri != 0 {
		pos, vel = pos.Rotate(b.ArgPeri), vel.Rotate(b.ArgPeri)
	}
	return pos, vel
}

// solveKepler inverts Kepler's equation M = E - e*sin(E) for the eccentric
// anomaly by Newton's method. The standard first guess converges in a handful
// of iterations for anything up to about e = 0.95; past that the iteration
// wants a better opening move than this one.
func solveKepler(m, e float64) float64 {
	m = math.Mod(m, 2*math.Pi)
	E := m + e*math.Sin(m)
	for range 32 {
		d := (E - e*math.Sin(E) - m) / (1 - e*math.Cos(E))
		E -= d
		if math.Abs(d) < 1e-13 {
			break
		}
	}
	return E
}

// Frame picks the body a state at rootPos should be measured from: the deepest
// one whose sphere of influence contains the point.
//
// This chooses a frame, not a physics. Every body pulls the vehicle at all
// times whichever frame is current — the sphere of influence only decides which
// centre the numbers are kept relative to, which is what keeps the coordinates
// small near a body and the telemetry meaningful.
func (s *System) Frame(rootPos Vec2, t float64) int {
	center := 0
	for {
		next, nearest := -1, 0.0
		for i := range s.Bodies {
			b := &s.Bodies[i]
			if b.Parent != center || b.SOI <= 0 {
				continue
			}
			p, _ := s.StateAt(i, t)
			if d := rootPos.Sub(p).Len(); d < b.SOI && (next < 0 || d < nearest) {
				next, nearest = i, d
			}
		}
		if next < 0 {
			return center
		}
		center = next
	}
}

// isAncestor reports whether a is somewhere up b's chain of parents.
func (s *System) isAncestor(a, b int) bool {
	for j := b; j > 0; j = s.Bodies[j].Parent {
		if s.Bodies[j].Parent == a {
			return true
		}
	}
	return false
}

// Gravity is the acceleration at a point given as rel, the offset from body
// center, in m/s^2. Every body in the system contributes.
//
// The frame of a body that is itself on rails is not inertial, so the
// acceleration the rails give that body has to come back out — otherwise its
// parent appears to drag the whole picture sideways. That correction runs up the
// chain of ancestors and no further: the bodies are on rails, so a body does not
// respond to its own children or to anything off its own chain. Leaving the
// correction as a sum over every perturber, the way a true N-body integration
// would, would contradict the rails.
func (s *System) Gravity(center int, rel Vec2, t float64) Vec2 {
	c := &s.Bodies[center]

	r := rel.Len()
	if r < 1 {
		r = 1
	}
	up := rel.Scale(1 / r)
	acc := up.Scale(-c.Mu / (r * r))

	// A system of one body is the whole of the original single-planet model,
	// and this early return keeps its arithmetic identical to the last digit.
	if len(s.Bodies) == 1 {
		return acc
	}

	cp, _ := s.StateAt(center, t)
	for i := range s.Bodies {
		if i == center || s.Bodies[i].Mu <= 0 {
			continue
		}
		bp, _ := s.StateAt(i, t)
		d := bp.Sub(cp).Sub(rel) // perturber as seen from the vehicle
		if dl := d.Len(); dl >= 1 {
			acc = acc.Add(d.Scale(s.Bodies[i].Mu / (dl * dl * dl)))
		}
	}
	return acc.Sub(s.railAccel(center, t))
}

// railAccel is the acceleration the rails impose on body i, summed along its
// chain of ancestors. Each link contributes the two-body pull that orbitState
// integrates, which is what makes the correction in Gravity exact rather than
// approximate.
func (s *System) railAccel(i int, t float64) Vec2 {
	var a Vec2
	for j := i; j > 0; j = s.Bodies[j].Parent {
		p := s.Bodies[j].Parent
		d, _ := s.RelState(j, p, t)
		if l := d.Len(); l > 0 {
			a = a.Add(d.Scale(-s.Bodies[p].Mu / (l * l * l)))
		}
	}
	return a
}
