package sim

// Preset is a ready-to-fly configuration. Name is a stable identifier, not
// display text: the setup screen translates it.
type Preset struct {
	Name string
	Cfg  Config
}

// gasIndex maps a gas name to its slot in Gases.
func gasIndex(name string) int {
	for i := range Gases {
		if Gases[i].Name == name {
			return i
		}
	}
	return -1
}

// mix builds a mole-fraction slice from name/fraction pairs.
func mix(pairs ...any) []float64 {
	f := make([]float64, len(Gases))
	for i := 0; i+1 < len(pairs); i += 2 {
		name, _ := pairs[i].(string)
		v, _ := pairs[i+1].(float64)
		if j := gasIndex(name); j >= 0 {
			f[j] = v
		}
	}
	return f
}

// Composition is a named gas mixture the setup screen can drop in whole. Name
// is a stable identifier, not display text.
type Composition struct {
	Name      string
	Fractions []float64
}

// Compositions are the mixtures offered in the setup screen. Mole fractions,
// near enough to the real bodies to be worth the name.
func Compositions() []Composition {
	return []Composition{
		{"earth", mix("N2", 0.7808, "O2", 0.2095, "Ar", 0.0093, "CO2", 0.0004)},
		{"mars", mix("CO2", 0.9532, "N2", 0.0270, "Ar", 0.0160, "O2", 0.0013)},
		{"venus", mix("CO2", 0.9650, "N2", 0.0350)},
		{"titan", mix("N2", 0.9420, "CH4", 0.0565, "H2", 0.0010)},
		{"gasGiant", mix("H2", 0.8980, "He", 0.1020)},
		{"steam", mix("H2O", 1)},
	}
}

// earthISA is the International Standard Atmosphere layer structure, extended
// with an isothermal tail so drag fades out smoothly instead of stopping dead.
func earthISA() []Layer {
	return []Layer{
		{0, -0.0065},
		{11000, 0},
		{20000, 0.001},
		{32000, 0.0028},
		{47000, 0},
		{51000, -0.0028},
		{71000, -0.002},
		{84852, 0},
	}
}

// Presets are the configurations offered on the setup screen.
func Presets() []Preset {
	return []Preset{earthFalcon(), marsAscent(), moonAscent(), kerbin()}
}

// DefaultConfig is what the setup screen starts with.
func DefaultConfig() Config { return earthFalcon().Cfg }

func earthFalcon() Preset {
	return Preset{
		Name: "earth-falcon",
		Cfg: Config{
			Body: Body{
				Name:           "Earth",
				Radius:         6371000,
				MassSource:     FromMass,
				Mass:           5.97237e24,
				RotationPeriod: 86164.1,
			},
			Atmo: Atmosphere{
				Fractions:       mix("N2", 0.7808, "O2", 0.2095, "Ar", 0.0093, "CO2", 0.0004),
				Layers:          earthISA(),
				SurfaceTemp:     288.15,
				SurfacePressure: 101325,
				Top:             140000,
			},
			Rocket: Rocket{
				Payload:  19000,
				Cd:       0.4,
				Diameter: 3.66,
				Stages: []Stage{
					{
						DryMass: 25600, PropMass: 395700,
						ThrustVac: 8227000, IspVac: 311, IspSL: 282,
						Throttle: 1, SepDelay: 3,
					},
					{
						DryMass: 4000, PropMass: 92670,
						ThrustVac: 934000, IspVac: 348, IspSL: 348,
						Throttle: 1, CutoffTime: 325.0,
						Ignition: IgniteAfterDelay, IgnitionDelay: 5,
					},
				},
			},
			Program: Program{Keys: []Keyframe{
				{Time: 0, Pitch: 90},
				{Time: 5, Pitch: 90},
				{Time: 43, Pitch: 69},
				{Time: 81, Pitch: 52},
				{Time: 119, Pitch: 38},
				{Time: 157, Pitch: 27},
				{Time: 195, Pitch: 18},
				{Time: 232, Pitch: 11},
				{Time: 270, Pitch: 7},
				{Time: 308, Pitch: 3},
				{Time: 346, Pitch: 1},
				{Time: 384, Pitch: 0},
			}},
			TargetOrbit: 250000,
			MaxTime:     3600,
		},
	}
}

func marsAscent() Preset {
	return Preset{
		Name: "mars",
		Cfg: Config{
			Body: Body{
				Name:           "Mars",
				Radius:         3389500,
				MassSource:     FromMass,
				Mass:           6.4171e23,
				RotationPeriod: 88642,
			},
			Atmo: Atmosphere{
				Fractions:       mix("CO2", 0.9532, "N2", 0.027, "Ar", 0.016, "O2", 0.0013),
				Layers:          []Layer{{0, -0.0009}, {60000, 0}},
				SurfaceTemp:     210,
				SurfacePressure: 610,
				Top:             90000,
			},
			Rocket: Rocket{
				Payload:  400,
				Cd:       0.3,
				Diameter: 2.0,
				Stages: []Stage{
					{
						DryMass: 1200, PropMass: 3500,
						ThrustVac: 40000, IspVac: 340, IspSL: 335,
						Throttle: 1, SepDelay: 2,
					},
					{
						DryMass: 400, PropMass: 900,
						ThrustVac: 12000, IspVac: 350, IspSL: 350,
						Throttle: 1, CutoffTime: 164.0,
						Ignition: IgniteAfterDelay, IgnitionDelay: 3,
					},
				},
			},
			Program: Program{Keys: []Keyframe{
				{Time: 0, Pitch: 90},
				{Time: 20, Pitch: 90},
				{Time: 37, Pitch: 84},
				{Time: 53, Pitch: 78},
				{Time: 70, Pitch: 71},
				{Time: 87, Pitch: 65},
				{Time: 103, Pitch: 58},
				{Time: 120, Pitch: 52},
				{Time: 137, Pitch: 45},
				{Time: 153, Pitch: 37},
				{Time: 170, Pitch: 30},
				{Time: 187, Pitch: 21},
				{Time: 203, Pitch: 12},
				{Time: 220, Pitch: 0},
			}},
			TargetOrbit: 120000,
			MaxTime:     3600,
		},
	}
}

func moonAscent() Preset {
	return Preset{
		Name: "moon",
		Cfg: Config{
			Body: Body{
				Name:           "Moon",
				Radius:         1737400,
				MassSource:     FromMass,
				Mass:           7.342e22,
				RotationPeriod: 2360591,
			},
			Atmo: Atmosphere{
				Fractions:       mix("He", 1),
				Layers:          nil,
				SurfaceTemp:     250,
				SurfacePressure: 0,
				Top:             0,
			},
			Rocket: Rocket{
				Payload:  300,
				Cd:       0.3,
				Diameter: 3.0,
				Stages: []Stage{
					{
						DryMass: 800, PropMass: 1000,
						ThrustVac: 8000, IspVac: 311, IspSL: 311,
						Throttle: 1, SepDelay: 2,
					},
					{
						DryMass: 300, PropMass: 300,
						ThrustVac: 4000, IspVac: 311, IspSL: 311,
						Throttle: 1, CutoffTime: 116.0,
						Ignition: IgniteImmediate,
					},
				},
			},
			Program: Program{Keys: []Keyframe{
				{Time: 0, Pitch: 90},
				{Time: 20, Pitch: 90},
				{Time: 55, Pitch: 72},
				{Time: 90, Pitch: 57},
				{Time: 125, Pitch: 44},
				{Time: 160, Pitch: 33},
				{Time: 195, Pitch: 23},
				{Time: 230, Pitch: 16},
				{Time: 265, Pitch: 10},
				{Time: 300, Pitch: 6},
				{Time: 335, Pitch: 3},
				{Time: 370, Pitch: 1},
				{Time: 405, Pitch: 0},
			}},
			TargetOrbit: 50000,
			MaxTime:     3600,
		},
	}
}

func kerbin() Preset {
	return Preset{
		Name: "kerbin",
		Cfg: Config{
			Body: Body{
				Name:           "Kerbin",
				Radius:         600000,
				MassSource:     FromMass,
				Mass:           5.2915158e22,
				RotationPeriod: 21549.425,
			},
			Atmo: Atmosphere{
				Fractions:       mix("N2", 0.78, "O2", 0.21, "Ar", 0.01),
				Layers:          []Layer{{0, -0.008}, {9000, -0.004}, {25000, 0.001}, {45000, 0}},
				SurfaceTemp:     288.15,
				SurfacePressure: 101325,
				Top:             70000,
			},
			Rocket: Rocket{
				Payload:  1000,
				Cd:       0.3,
				Diameter: 2.5,
				Stages: []Stage{
					{
						DryMass: 4000, PropMass: 11000,
						ThrustVac: 340000, IspVac: 300, IspSL: 265,
						Throttle: 1, SepDelay: 2,
					},
					{
						DryMass: 1200, PropMass: 2600,
						ThrustVac: 90000, IspVac: 350, IspSL: 350,
						Throttle: 1, CutoffTime: 64.3,
						Ignition: IgniteAtApoapsis,
					},
				},
			},
			Program: Program{Keys: []Keyframe{
				{Time: 0, Pitch: 90},
				{Time: 2, Pitch: 90},
				{Time: 34, Pitch: 78},
				{Time: 65, Pitch: 67},
				{Time: 96, Pitch: 57},
				{Time: 128, Pitch: 47},
				{Time: 160, Pitch: 38},
				{Time: 191, Pitch: 30},
				{Time: 222, Pitch: 22},
				{Time: 254, Pitch: 16},
				{Time: 286, Pitch: 10},
				{Time: 317, Pitch: 5},
				{Time: 348, Pitch: 2},
				{Time: 380, Pitch: 0},
			}},
			TargetOrbit: 90000,
			MaxTime:     3600,
		},
	}
}
