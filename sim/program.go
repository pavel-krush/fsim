package sim

import "math"

// Keyframe is one point in the pitch program. Pitch is measured from the local
// horizon: 90 degrees is straight up along the local vertical, 0 degrees is
// horizontal in the downrange direction.
type Keyframe struct {
	Time  float64 // s since liftoff
	Pitch float64 // degrees
	// Prograde overrides Pitch and holds the thrust along the current velocity
	// vector. Without this the upper stage cannot follow a clean insertion.
	Prograde bool
}

// Program is the pitch schedule, sorted by time. Before the first keyframe the
// first value is held; after the last one the last value is held.
type Program struct {
	Keys []Keyframe
}

// Pitch returns the commanded pitch in degrees at time t. fpa is the current
// flight path angle in degrees, used to resolve prograde keyframes so that they
// can be interpolated against absolute ones.
func (p *Program) Pitch(t, fpa float64) float64 {
	if len(p.Keys) == 0 {
		return 90
	}
	resolve := func(k Keyframe) float64 {
		if k.Prograde {
			return fpa
		}
		return k.Pitch
	}

	if t <= p.Keys[0].Time {
		return resolve(p.Keys[0])
	}
	last := len(p.Keys) - 1
	if t >= p.Keys[last].Time {
		return resolve(p.Keys[last])
	}

	for i := 0; i < last; i++ {
		a, b := p.Keys[i], p.Keys[i+1]
		if t < a.Time || t > b.Time {
			continue
		}
		span := b.Time - a.Time
		if span <= 0 {
			return resolve(b)
		}
		f := (t - a.Time) / span
		pa, pb := resolve(a), resolve(b)
		return pa + (pb-pa)*f
	}
	return resolve(p.Keys[last])
}

// Sorted reports whether the keyframes are in non-decreasing time order.
func (p *Program) Sorted() bool {
	for i := 1; i < len(p.Keys); i++ {
		if p.Keys[i].Time < p.Keys[i-1].Time {
			return false
		}
	}
	return true
}

// Sort orders the keyframes by time using an insertion sort — the list is a
// handful of entries typed in by hand, so this is plenty.
func (p *Program) Sort() {
	for i := 1; i < len(p.Keys); i++ {
		k := p.Keys[i]
		j := i - 1
		for j >= 0 && p.Keys[j].Time > k.Time {
			p.Keys[j+1] = p.Keys[j]
			j--
		}
		p.Keys[j+1] = k
	}
}

// FlightPathAngle is the angle of the velocity vector above the local horizon,
// in degrees, for a vehicle at position r moving at v.
func FlightPathAngle(r, v Vec2) float64 {
	if v.Len() == 0 {
		return 90
	}
	up := r.Unit()
	vertical := v.Dot(up)
	horizontal := v.Sub(up.Scale(vertical)).Len()
	return math.Atan2(vertical, horizontal) * 180 / math.Pi
}

// ThrustDirection converts a commanded pitch into a unit vector in the
// inertial frame. up is the local vertical and east is the downrange
// direction, which is the direction the planet rotates towards.
func ThrustDirection(up, east Vec2, pitchDeg float64) Vec2 {
	s, c := math.Sincos(pitchDeg * math.Pi / 180)
	return up.Scale(s).Add(east.Scale(c)).Unit()
}
