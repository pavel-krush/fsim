package sim

import "math"

// Physical constants shared across the simulation.
const (
	// G is the gravitational constant, m^3 kg^-1 s^-2.
	G = 6.67430e-11
	// Rgas is the universal gas constant, J mol^-1 K^-1.
	Rgas = 8.314462618
	// G0 is standard gravity. It only appears in the definition of specific
	// impulse (Isp is seconds of thrust per unit weight flow at Earth's
	// surface), so it stays 9.80665 even on other planets.
	G0 = 9.80665
)

// MassSource selects which independent quantity the user typed in. Radius is
// always an input; the remaining three of {mass, density, surface gravity} are
// derived from radius plus whichever one this names.
type MassSource int

const (
	FromMass MassSource = iota
	FromDensity
	FromSurfaceG
)

// Body is one body of a System: a planet, a moon, or the star at the root. Only
// Radius plus one of Mass/Density/SurfaceG is meaningful input; call Normalize
// to fill in the rest.
type Body struct {
	Name   string
	Radius float64 // m

	MassSource MassSource
	Mass       float64 // kg
	Density    float64 // kg/m^3, mean
	SurfaceG   float64 // m/s^2

	RotationPeriod float64 // s, sidereal; 0 means non-rotating

	// Parent is the index of the body this one orbits, and is always lower than
	// this body's own index. The root has -1, which System.Normalize sets.
	Parent int
	// The orbit about the parent, in the one plane everything shares. A body
	// with no semi-major axis sits on its parent's centre and stays there.
	SemiMajor float64 // m
	Ecc       float64
	ArgPeri   float64 // rad, where periapsis points
	MeanAnom0 float64 // rad, mean anomaly at t = 0

	// Derived by Normalize, and left out of JSON for it: what is worth writing down
	// is the inputs. The root's sphere of influence is +Inf besides, which JSON has
	// no way to spell — so a saved system would not be writable at all with these
	// in it.
	Mu  float64 `json:"-"` // m^3/s^2, standard gravitational parameter
	SOI float64 `json:"-"` // m, radius of the sphere of influence
}

// Normalize derives the dependent quantities from Radius and MassSource.
func (b *Body) Normalize() {
	v := b.Volume()
	switch b.MassSource {
	case FromDensity:
		b.Mass = b.Density * v
	case FromSurfaceG:
		b.Mass = b.SurfaceG * b.Radius * b.Radius / G
	}
	b.Mu = G * b.Mass
	if v > 0 {
		b.Density = b.Mass / v
	}
	if b.Radius > 0 {
		b.SurfaceG = b.Mu / (b.Radius * b.Radius)
	}
}

// Volume of the body treated as a sphere, m^3.
func (b *Body) Volume() float64 {
	return 4.0 / 3.0 * math.Pi * b.Radius * b.Radius * b.Radius
}

// Diameter in metres.
func (b *Body) Diameter() float64 { return 2 * b.Radius }

// GravityAt returns gravitational acceleration magnitude at radius r from the
// centre, m/s^2.
func (b *Body) GravityAt(r float64) float64 {
	if r <= 0 {
		return 0
	}
	return b.Mu / (r * r)
}

// AngularVelocity is the body's rotation rate, rad/s.
func (b *Body) AngularVelocity() float64 {
	if b.RotationPeriod == 0 {
		return 0
	}
	return 2 * math.Pi / b.RotationPeriod
}

// EquatorialSpeed is the ground speed of a point on the equator, m/s. This is
// the free velocity a launch site gets from planetary rotation.
func (b *Body) EquatorialSpeed() float64 {
	return b.AngularVelocity() * b.Radius
}

// CircularSpeed is the speed of a circular orbit at altitude h, m/s.
func (b *Body) CircularSpeed(h float64) float64 {
	return math.Sqrt(b.Mu / (b.Radius + h))
}

// EscapeSpeed at altitude h, m/s.
func (b *Body) EscapeSpeed(h float64) float64 {
	return math.Sqrt(2 * b.Mu / (b.Radius + h))
}
