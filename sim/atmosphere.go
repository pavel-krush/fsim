package sim

import "math"

// Gas is one component of an atmosphere.
type Gas struct {
	Name      string
	MolarMass float64 // kg/mol
	Gamma     float64 // adiabatic index of the pure gas
}

// Gases the UI offers. Mole fractions over this list define an atmosphere.
var Gases = []Gas{
	{"N2", 0.0280134, 1.400},
	{"O2", 0.0319988, 1.400},
	{"CO2", 0.0440095, 1.289},
	{"Ar", 0.0399480, 1.667},
	{"H2O", 0.0180153, 1.330},
	{"CH4", 0.0160425, 1.320},
	{"H2", 0.0020159, 1.410},
	{"He", 0.0040026, 1.667},
}

// Layer is one slab of atmosphere with a constant temperature gradient.
// Layers must be sorted by BaseAlt ascending, and the first one must start at 0.
type Layer struct {
	BaseAlt float64 // m above surface
	Lapse   float64 // K/m; negative means it gets colder with altitude
}

// Atmosphere is a layered model in hydrostatic equilibrium. Composition sets
// the molar mass and adiabatic index; the layers plus surface conditions set
// the pressure profile.
type Atmosphere struct {
	// Fractions are mole fractions parallel to Gases. They are normalised to
	// sum to 1 in Prepare, so they can be entered as percentages.
	Fractions []float64

	Layers          []Layer
	SurfaceTemp     float64 // K
	SurfacePressure float64 // Pa
	Top             float64 // m; above this the density is taken as zero

	// Derived by Prepare.
	molarMass float64
	gamma     float64
	baseT     []float64
	baseP     []float64
	surfaceG  float64
	ready     bool
}

// AtmoState is the local state of the air at some altitude.
type AtmoState struct {
	Temp     float64 // K
	Pressure float64 // Pa
	Density  float64 // kg/m^3
	Sound    float64 // m/s, speed of sound
}

// MolarMass of the mixture, kg/mol.
func (a *Atmosphere) MolarMass() float64 { return a.molarMass }

// Gamma of the mixture, dimensionless.
func (a *Atmosphere) Gamma() float64 { return a.gamma }

// IsVacuum reports whether this atmosphere exerts no drag at all.
func (a *Atmosphere) IsVacuum() bool {
	return a.SurfacePressure <= 0 || len(a.Layers) == 0 || a.Top <= 0
}

// Prepare computes the mixture properties and walks the layers from the
// surface upwards, recording the temperature and pressure at each layer base.
// surfaceG is the planet's surface gravity, which is held constant through the
// barometric integration — the usual ISA simplification, worth about 3% at
// 100 km on Earth.
func (a *Atmosphere) Prepare(surfaceG float64) {
	a.surfaceG = surfaceG
	a.ready = true
	a.mixture()

	n := len(a.Layers)
	a.baseT = make([]float64, n)
	a.baseP = make([]float64, n)
	if n == 0 || a.SurfacePressure <= 0 {
		return
	}

	a.baseT[0] = a.SurfaceTemp
	a.baseP[0] = a.SurfacePressure
	for i := 1; i < n; i++ {
		dh := a.Layers[i].BaseAlt - a.Layers[i-1].BaseAlt
		t, p := a.walk(i-1, dh)
		a.baseT[i] = t
		a.baseP[i] = p
	}
}

// mixture derives molar mass and adiabatic index from the mole fractions.
// Gamma is mixed through the molar heat capacities, not averaged directly:
// Cv_mix = sum(x_i * R/(gamma_i - 1)), then gamma = (Cv_mix + R) / Cv_mix.
func (a *Atmosphere) mixture() {
	var sum, m, cv float64
	for i, x := range a.Fractions {
		if i >= len(Gases) || x <= 0 {
			continue
		}
		sum += x
		m += x * Gases[i].MolarMass
		cv += x * Rgas / (Gases[i].Gamma - 1)
	}
	if sum <= 0 {
		// No composition given: fall back to dry Earth air.
		a.molarMass = 0.0289644
		a.gamma = 1.4
		return
	}
	a.molarMass = m / sum
	cv /= sum
	a.gamma = (cv + Rgas) / cv
}

// walk integrates temperature and pressure from the base of layer i upwards by
// dh metres, staying inside that layer.
func (a *Atmosphere) walk(i int, dh float64) (temp, pressure float64) {
	l := a.Layers[i]
	tb, pb := a.baseT[i], a.baseP[i]

	t := tb + l.Lapse*dh
	if t < 1 {
		// A steep negative lapse rate can drive the temperature through zero.
		// Clamp so the model stays finite instead of producing NaNs.
		t = 1
	}

	exponent := a.surfaceG * a.molarMass / Rgas
	if l.Lapse == 0 {
		return t, pb * math.Exp(-exponent*dh/tb)
	}
	return t, pb * math.Pow(tb/t, exponent/l.Lapse)
}

// State returns the air state at altitude h above the surface.
func (a *Atmosphere) State(h float64) AtmoState {
	if !a.ready {
		a.Prepare(a.surfaceG)
	}
	if a.IsVacuum() || h >= a.Top {
		return AtmoState{Temp: a.SurfaceTemp, Sound: a.soundSpeed(a.SurfaceTemp)}
	}
	if h < 0 {
		h = 0
	}

	i := a.layerAt(h)
	t, p := a.walk(i, h-a.Layers[i].BaseAlt)
	rho := p * a.molarMass / (Rgas * t)

	return AtmoState{Temp: t, Pressure: p, Density: rho, Sound: a.soundSpeed(t)}
}

// layerAt finds the index of the layer containing altitude h.
func (a *Atmosphere) layerAt(h float64) int {
	i := 0
	for i+1 < len(a.Layers) && a.Layers[i+1].BaseAlt <= h {
		i++
	}
	return i
}

func (a *Atmosphere) soundSpeed(t float64) float64 {
	if t <= 0 || a.molarMass <= 0 {
		return 0
	}
	return math.Sqrt(a.gamma * Rgas * t / a.molarMass)
}
