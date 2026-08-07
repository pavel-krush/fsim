package sim

// The solar system, as far as a plane can hold it. Radii and masses are the real
// ones; the orbits are the real semi-major axes and eccentricities with the
// inclinations dropped, because everything here shares one plane.
//
// The mean anomalies are not an ephemeris. Nothing in this simulator is tied to a
// date, so they are spread out to make a picture worth looking at rather than to
// put Mars where Mars was on any particular morning. Triton's retrograde orbit
// goes the same way round as everything else for the same reason: in one plane
// there is nowhere to put it.

// SolarSystem is the real thing, seventeen bodies deep.
func SolarSystem() System {
	sys := System{Bodies: []Body{
		{Name: "sun", Radius: 6.957e8, MassSource: FromMass, Mass: 1.989e30, RotationPeriod: 2.164e6},

		{Name: "mercury", Radius: 2.4397e6, MassSource: FromMass, Mass: 3.3011e23,
			RotationPeriod: 5.0670e6, Parent: 0, SemiMajor: 5.7909e10, Ecc: 0.2056, MeanAnom0: 0.3},
		{Name: "venus", Radius: 6.0518e6, MassSource: FromMass, Mass: 4.8675e24,
			RotationPeriod: -2.0997e7, Parent: 0, SemiMajor: 1.0821e11, Ecc: 0.0068, MeanAnom0: 2.1,
			Atmo: VenusAir()},
		{Name: "earth", Radius: 6.371e6, MassSource: FromMass, Mass: 5.97237e24,
			RotationPeriod: 86164.1, Parent: 0, SemiMajor: 1.4960e11, Ecc: 0.0167, MeanAnom0: 0,
			Atmo: EarthAir()},
		{Name: "mars", Radius: 3.3895e6, MassSource: FromMass, Mass: 6.4171e23,
			RotationPeriod: 88642, Parent: 0, SemiMajor: 2.2794e11, Ecc: 0.0934, ArgPeri: 0.9,
			// Not a picture choice like the rest of them: this one is a launch
			// window. It puts Mars where the transfer in the mars-flyby preset
			// crosses its orbit, a hundred and eighty days out.
			MeanAnom0: 5.9975, Atmo: MarsAir()},
		{Name: "jupiter", Radius: 6.9911e7, MassSource: FromMass, Mass: 1.8982e27,
			RotationPeriod: 35730, Parent: 0, SemiMajor: 7.7857e11, Ecc: 0.0489, MeanAnom0: 3.6},
		{Name: "saturn", Radius: 5.8232e7, MassSource: FromMass, Mass: 5.6834e26,
			RotationPeriod: 38052, Parent: 0, SemiMajor: 1.4335e12, Ecc: 0.0565, MeanAnom0: 5.2},
		{Name: "uranus", Radius: 2.5362e7, MassSource: FromMass, Mass: 8.681e25,
			RotationPeriod: -62064, Parent: 0, SemiMajor: 2.8725e12, Ecc: 0.0457, MeanAnom0: 1.4},
		{Name: "neptune", Radius: 2.4622e7, MassSource: FromMass, Mass: 1.0243e26,
			RotationPeriod: 57996, Parent: 0, SemiMajor: 4.4951e12, Ecc: 0.0113, MeanAnom0: 4.5},
	}}

	// The moons come after every planet, which keeps the parent-before-child
	// invariant without anyone having to count indices by hand.
	add := func(parent string, b Body) {
		b.Parent = sys.IndexOf(parent)
		sys.Bodies = append(sys.Bodies, b)
	}
	add("earth", Body{Name: "moon", Radius: 1.7374e6, MassSource: FromMass, Mass: 7.342e22,
		RotationPeriod: 2360591, SemiMajor: 3.844e8, Ecc: 0.0549, ArgPeri: 0.5, MeanAnom0: 0.9})
	add("mars", Body{Name: "phobos", Radius: 1.1267e4, MassSource: FromMass, Mass: 1.0659e16,
		RotationPeriod: 27554, SemiMajor: 9.376e6, Ecc: 0.0151, MeanAnom0: 1.1})
	add("mars", Body{Name: "deimos", Radius: 6.2e3, MassSource: FromMass, Mass: 1.4762e15,
		RotationPeriod: 109123, SemiMajor: 2.3463e7, Ecc: 0.0002, MeanAnom0: 4.0})
	add("jupiter", Body{Name: "io", Radius: 1.8216e6, MassSource: FromMass, Mass: 8.932e22,
		RotationPeriod: 152854, SemiMajor: 4.217e8, Ecc: 0.0041, MeanAnom0: 0.7})
	add("jupiter", Body{Name: "europa", Radius: 1.5608e6, MassSource: FromMass, Mass: 4.8e22,
		RotationPeriod: 306822, SemiMajor: 6.711e8, Ecc: 0.009, MeanAnom0: 2.9})
	add("jupiter", Body{Name: "ganymede", Radius: 2.6341e6, MassSource: FromMass, Mass: 1.4819e23,
		RotationPeriod: 618154, SemiMajor: 1.0704e9, Ecc: 0.0013, MeanAnom0: 5.0})
	add("jupiter", Body{Name: "callisto", Radius: 2.4103e6, MassSource: FromMass, Mass: 1.0759e23,
		RotationPeriod: 1441931, SemiMajor: 1.8827e9, Ecc: 0.0074, MeanAnom0: 1.8})
	add("saturn", Body{Name: "titan", Radius: 2.5747e6, MassSource: FromMass, Mass: 1.3452e23,
		RotationPeriod: 1377648, SemiMajor: 1.2219e9, Ecc: 0.0288, MeanAnom0: 3.3,
		Atmo: TitanAir()})
	add("neptune", Body{Name: "triton", Radius: 1.3534e6, MassSource: FromMass, Mass: 2.139e22,
		RotationPeriod: 507772, SemiMajor: 3.5476e8, Ecc: 0.000016, MeanAnom0: 2.2})

	sys.Normalize()
	return sys
}

// IndexOf finds a body by its identifier, or -1. Names are identifiers rather
// than display text, so this is a lookup and not a search.
func (s *System) IndexOf(name string) int {
	for i := range s.Bodies {
		if s.Bodies[i].Name == name {
			return i
		}
	}
	return -1
}

// The air, for the four bodies here that have enough of it to fly through. Each is a
// function rather than a value because an Atmosphere carries slices, and a shared one
// would be edited from every system built out of this file at once.
//
// The gas giants are left airless on purpose. An atmosphere here is measured from a
// surface — a base pressure and a temperature at a radius — and Jupiter has no surface
// to measure from, so the honest choice between a made-up cloud deck and nothing is
// nothing. Bodies with a trace of gas and nothing to fly through, the Moon and Io among
// them, are vacuum for the same reason they always were.

// EarthAir is the ISA: nitrogen, oxygen, argon and the carbon dioxide, with the
// standard lapse rates and 140 km of it, which is where the vacuum threshold falls.
func EarthAir() Atmosphere {
	return Atmosphere{
		Fractions:       mix("N2", 0.7808, "O2", 0.2095, "Ar", 0.0093, "CO2", 0.0004),
		Layers:          earthISA(),
		SurfaceTemp:     288.15,
		SurfacePressure: 101325,
		Top:             140000,
	}
}

// MarsAir is six millibars of carbon dioxide. Thin enough that an ascent barely
// notices it and thick enough that an arrival very much does.
func MarsAir() Atmosphere {
	return Atmosphere{
		Fractions:       mix("CO2", 0.9532, "N2", 0.027, "Ar", 0.016, "O2", 0.0013),
		Layers:          []Layer{{0, -0.0009}, {60000, 0}},
		SurfaceTemp:     210,
		SurfacePressure: 610,
		Top:             90000,
	}
}

// VenusAir is ninety-two bars at 737 K, which is the reason nothing here launches from
// it: the surface density is 65 kg/m³, fifty times Earth's, and Titan at four times
// Earth's was already the hardest preset to fly.
func VenusAir() Atmosphere {
	return Atmosphere{
		Fractions:       mix("CO2", 0.965, "N2", 0.035),
		Layers:          []Layer{{0, -0.00814}, {60000, -0.0012}, {100000, 0}},
		SurfaceTemp:     737,
		SurfacePressure: 9.2e6,
		Top:             250000,
	}
}

// TitanAir is nitrogen with methane in it: 1.5 bar at 94 K, cooling to the tropopause
// at 44 km and warming again above it, which is the shape Huygens measured going down.
func TitanAir() Atmosphere {
	return Atmosphere{
		Fractions:       mix("N2", 0.9420, "CH4", 0.0565, "H2", 0.0010),
		Layers:          []Layer{{0, -0.00053}, {44000, 0.00053}, {250000, 0}},
		SurfaceTemp:     93.7,
		SurfacePressure: 146700,
		Top:             500000,
	}
}
