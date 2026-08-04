package sim

import "math"

// IgnitionMode decides when an upper stage lights its engine.
type IgnitionMode int

const (
	// IgniteImmediate lights the engine as soon as separation completes.
	IgniteImmediate IgnitionMode = iota
	// IgniteAfterDelay waits IgnitionDelay seconds after separation.
	IgniteAfterDelay
	// IgniteAtApoapsis coasts to apoapsis and burns there. This is what makes
	// a circularisation burn possible.
	IgniteAtApoapsis
	// IgniteOnNode never lights by itself: the flight plan does it. A stage like
	// this also stops the staging sequence from handing over to it, so whatever it
	// sits on stays attached until a burn says to drop it — which is how a
	// spacecraft keeps its spent booster through a coast and jettisons it with the
	// burn that no longer needs it.
	IgniteOnNode
)

// Stage is one propulsive stage of the vehicle.
type Stage struct {
	DryMass  float64 // kg, structure without propellant
	PropMass float64 // kg, usable propellant at ignition

	ThrustVac float64 // N, at full throttle in vacuum
	IspVac    float64 // s
	IspSL     float64 // s, at the planet's surface pressure
	Throttle  float64 // 0..1, constant for the whole burn

	// CutoffTime ends the burn this many seconds after the stage ignites.
	// Zero means burn until the propellant runs out.
	CutoffTime float64

	// SepDelay is the coast between this stage's shutdown and the next stage
	// separating, in seconds.
	SepDelay float64

	Ignition      IgnitionMode
	IgnitionDelay float64 // s, used by IgniteAfterDelay
}

// WetMass of the stage at ignition, kg.
func (s *Stage) WetMass() float64 { return s.DryMass + s.PropMass }

// MassFlow is the propellant consumption rate while burning, kg/s. It is held
// constant: the engine runs at a fixed throttle setting, so it swallows
// propellant at a fixed rate regardless of altitude. Only the exhaust velocity
// (and therefore thrust) changes with ambient pressure.
func (s *Stage) MassFlow() float64 {
	if s.IspVac <= 0 {
		return 0
	}
	return s.throttle() * s.ThrustVac / (s.IspVac * G0)
}

// Isp at ambient pressure p, linearly interpolated between the sea-level and
// vacuum figures. refP is the planet's surface pressure.
func (s *Stage) Isp(p, refP float64) float64 {
	if refP <= 0 {
		return s.IspVac
	}
	f := p / refP
	if f < 0 {
		f = 0
	} else if f > 1 {
		f = 1
	}
	return s.IspVac - (s.IspVac-s.IspSL)*f
}

// Thrust at ambient pressure p, N.
func (s *Stage) Thrust(p, refP float64) float64 {
	return s.MassFlow() * s.Isp(p, refP) * G0
}

// BurnTime is how long the stage can burn before running dry, s.
func (s *Stage) BurnTime() float64 {
	mdot := s.MassFlow()
	if mdot <= 0 {
		return 0
	}
	full := s.PropMass / mdot
	if s.CutoffTime > 0 && s.CutoffTime < full {
		return s.CutoffTime
	}
	return full
}

func (s *Stage) throttle() float64 {
	t := s.Throttle
	if t <= 0 {
		return 1
	}
	if t > 1 {
		return 1
	}
	return t
}

// Rocket is the whole vehicle: a payload riding on an ordered list of stages,
// stage 0 firing first.
type Rocket struct {
	Payload  float64 // kg
	Cd       float64 // drag coefficient, referenced to the body cross-section
	Diameter float64 // m, body diameter used for the reference area
	Stages   []Stage
}

// Area is the reference cross-section, m^2.
func (r *Rocket) Area() float64 {
	return math.Pi * r.Diameter * r.Diameter / 4
}

// MassAtStage is the vehicle mass with stage i fully fuelled and every stage
// below it already dropped, kg.
func (r *Rocket) MassAtStage(i int) float64 {
	m := r.Payload
	for j := i; j < len(r.Stages); j++ {
		m += r.Stages[j].WetMass()
	}
	return m
}

// LiftoffMass is the mass on the pad, kg.
func (r *Rocket) LiftoffMass() float64 { return r.MassAtStage(0) }

// StageDeltaV is the ideal Tsiolkovsky delta-v of stage i using its vacuum Isp,
// m/s. It ignores any cutoff time — it is the propellant's full potential.
func (r *Rocket) StageDeltaV(i int) float64 {
	if i < 0 || i >= len(r.Stages) {
		return 0
	}
	s := &r.Stages[i]
	m0 := r.MassAtStage(i)
	m1 := m0 - s.PropMass
	if m1 <= 0 || m0 <= 0 {
		return 0
	}
	return s.IspVac * G0 * math.Log(m0/m1)
}

// TotalDeltaV sums the ideal delta-v of every stage, m/s.
func (r *Rocket) TotalDeltaV() float64 {
	var sum float64
	for i := range r.Stages {
		sum += r.StageDeltaV(i)
	}
	return sum
}

// LiftoffTWR is the thrust-to-weight ratio on the pad at the given surface
// pressure and gravity. Below 1 the rocket cannot leave the ground.
func (r *Rocket) LiftoffTWR(surfaceP, surfaceG float64) float64 {
	if len(r.Stages) == 0 || surfaceG <= 0 {
		return 0
	}
	w := r.LiftoffMass() * surfaceG
	if w <= 0 {
		return 0
	}
	return r.Stages[0].Thrust(surfaceP, surfaceP) / w
}
