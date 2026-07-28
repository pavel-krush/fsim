package sim

import "math"

// Vec2 is a planar vector in the inertial frame centred on the planet.
type Vec2 struct{ X, Y float64 }

func (v Vec2) Add(o Vec2) Vec2      { return Vec2{v.X + o.X, v.Y + o.Y} }
func (v Vec2) Sub(o Vec2) Vec2      { return Vec2{v.X - o.X, v.Y - o.Y} }
func (v Vec2) Scale(s float64) Vec2 { return Vec2{v.X * s, v.Y * s} }
func (v Vec2) Dot(o Vec2) float64   { return v.X*o.X + v.Y*o.Y }
func (v Vec2) Cross(o Vec2) float64 { return v.X*o.Y - v.Y*o.X }
func (v Vec2) Len() float64         { return math.Hypot(v.X, v.Y) }
func (v Vec2) Angle() float64       { return math.Atan2(v.Y, v.X) }
func (v Vec2) Perp() Vec2           { return Vec2{-v.Y, v.X} }

// Unit returns the unit vector along v, or the zero vector if v has no length.
func (v Vec2) Unit() Vec2 {
	l := v.Len()
	if l == 0 {
		return Vec2{}
	}
	return Vec2{v.X / l, v.Y / l}
}

// Rotate returns v turned counter-clockwise by a radians.
func (v Vec2) Rotate(a float64) Vec2 {
	s, c := math.Sincos(a)
	return Vec2{v.X*c - v.Y*s, v.X*s + v.Y*c}
}

// Orbit is the osculating two-body ellipse (or hyperbola) that the vehicle
// would coast along if all thrust and drag stopped right now.
type Orbit struct {
	SemiMajor    float64 // m; negative for a hyperbolic trajectory
	Eccentricity float64
	Apoapsis     float64 // m from the planet's centre; +Inf if unbound
	Periapsis    float64 // m from the planet's centre
	Energy       float64 // J/kg, specific orbital energy
	AngMomentum  float64 // m^2/s, specific angular momentum (signed)
	Period       float64 // s; 0 if unbound
	TrueAnomaly  float64 // rad, current position along the orbit
}

// Bound reports whether the trajectory is closed. The test is on the energy
// (equivalently, a positive semi-major axis) rather than on the eccentricity:
// a purely radial fall has e == 1 exactly while still being bound.
func (o Orbit) Bound() bool { return o.SemiMajor > 0 }

// ComputeOrbit derives the osculating elements from a position and velocity.
func ComputeOrbit(r, v Vec2, mu float64) Orbit {
	var o Orbit
	rl, vl := r.Len(), v.Len()
	if rl == 0 || mu == 0 {
		return o
	}

	o.Energy = vl*vl/2 - mu/rl
	o.AngMomentum = r.Cross(v)
	h := o.AngMomentum

	// e = |(v x h)/mu - r_hat|, written out for the planar case.
	ex := (v.Y*h)/mu - r.X/rl
	ey := (-v.X*h)/mu - r.Y/rl
	o.Eccentricity = math.Hypot(ex, ey)

	if o.Energy != 0 {
		o.SemiMajor = -mu / (2 * o.Energy)
	}

	if o.SemiMajor > 0 {
		o.Apoapsis = o.SemiMajor * (1 + o.Eccentricity)
		o.Periapsis = o.SemiMajor * (1 - o.Eccentricity)
		o.Period = 2 * math.Pi * math.Sqrt(o.SemiMajor*o.SemiMajor*o.SemiMajor/mu)
	} else {
		o.Apoapsis = math.Inf(1)
		// For a hyperbola a is negative, so a(1-e) is still the periapsis.
		o.Periapsis = o.SemiMajor * (1 - o.Eccentricity)
	}

	if o.Eccentricity > 1e-12 {
		ev := Vec2{ex, ey}
		cosNu := ev.Dot(r) / (o.Eccentricity * rl)
		if cosNu > 1 {
			cosNu = 1
		} else if cosNu < -1 {
			cosNu = -1
		}
		o.TrueAnomaly = math.Acos(cosNu)
		if r.Dot(v) < 0 {
			o.TrueAnomaly = 2*math.Pi - o.TrueAnomaly
		}
	}
	return o
}

// ApoapsisAlt returns the apoapsis as an altitude above the surface.
func (o Orbit) ApoapsisAlt(radius float64) float64 {
	if math.IsInf(o.Apoapsis, 1) {
		return math.Inf(1)
	}
	return o.Apoapsis - radius
}

// PeriapsisAlt returns the periapsis as an altitude above the surface. It can
// legitimately be negative, which means the coast trajectory hits the ground.
func (o Orbit) PeriapsisAlt(radius float64) float64 { return o.Periapsis - radius }
