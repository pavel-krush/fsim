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
	return []Preset{earthFalcon(), apolloSaturn(), apolloLunar(), protonZvezda(), protonGeo(), marsAscent(), moonAscent(), kerbin()}
}

// DefaultConfig is what the setup screen starts with.
func DefaultConfig() Config { return earthFalcon().Cfg }

func earthFalcon() Preset {
	return Preset{
		Name: "earth-falcon",
		Cfg: Config{
			Body: Body{
				Name:           "earth",
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

// apolloSaturn is Apollo's ride to the parking orbit: Saturn V, three stages,
// the spacecraft stack as payload.
//
// It stops where the simulation stops being able to tell the truth. There is one
// central body here and no Moon to aim at, so the mission modelled is the first
// eleven and a half minutes of it — S-IC, S-II and the S-IVB's first burn into a
// 185 km parking orbit, which is exactly what the vehicle did before it coasted
// round and relit for translunar injection. The S-IVB is loaded with all of its
// propellant, so what is still in the tank at insertion — four fifths of it — is
// what would have gone to the Moon. The LM's ride home is the Moon preset.
func apolloSaturn() Preset {
	sys := SolarSystem()
	earth := sys.IndexOf("earth")

	return Preset{
		Name: "apollo-saturn",
		Cfg: Config{
			// The whole solar system, launched from the Earth in it. Nothing about
			// the ascent changes — the Sun's pull on a vehicle in the Earth's frame
			// is all but cancelled by the Earth falling towards it too, leaving a
			// tide of 1e-7 m/s^2 — and once in orbit the camera can pull back to
			// the Moon's rail, or to Jupiter's.
			System:     sys,
			LaunchBody: earth,
			Body:       sys.Bodies[earth],
			Atmo: Atmosphere{
				Fractions:       mix("N2", 0.7808, "O2", 0.2095, "Ar", 0.0093, "CO2", 0.0004),
				Layers:          earthISA(),
				SurfaceTemp:     288.15,
				SurfacePressure: 101325,
				Top:             140000,
			},
			Rocket: Rocket{
				// The spacecraft: command and service module 28.8 t, lunar module
				// 15.1 t, spacecraft/LM adapter 1.8 t. The escape tower is not
				// modelled — it went overboard with the interstage anyway.
				Payload:  45700,
				Cd:       0.4,
				Diameter: 10.06,
				Stages: []Stage{
					{
						// S-IC: five F-1s, 33.6 MN off the pad. The vacuum figure
						// is what the model works from; the sea-level Isp brings
						// it down to what the pad actually saw.
						DryMass: 130000, PropMass: 2077000,
						ThrustVac: 38850000, IspVac: 304, IspSL: 263,
						Throttle: 1, SepDelay: 1,
					},
					{
						// S-II: five J-2s, burning to depletion.
						DryMass: 40100, PropMass: 443000,
						ThrustVac: 5165000, IspVac: 421, IspSL: 421,
						Throttle: 1, SepDelay: 1,
						Ignition: IgniteImmediate,
					},
					{
						// S-IVB: one J-2, plus the instrument unit in the dry
						// mass. The cutoff is orbital insertion; what is left in
						// the tank is the translunar burn that this simulation
						// has nowhere to send.
						DryMass: 15200, PropMass: 106400,
						ThrustVac: 901000, IspVac: 421, IspSL: 421,
						Throttle: 1, CutoffTime: 88.9,
						Ignition: IgniteImmediate,
					},
				},
			},
			// The programme levels off at nine degrees and stays there rather
			// than dropping to the horizon. A stack this energetic tolerates
			// neither extreme: hold more and it lofts to 350 km, where the third
			// stage circularises at the wrong altitude; let it fall to zero and
			// the vehicle cannot hold 185 km at four kilometres a second, sinks
			// back into the air and spends nine kilometres a second of the
			// budget on drag.
			Program: Program{Keys: []Keyframe{
				{Time: 0, Pitch: 90},
				{Time: 12, Pitch: 90},
				{Time: 53, Pitch: 66},
				{Time: 94, Pitch: 48},
				{Time: 135, Pitch: 35},
				{Time: 176, Pitch: 25},
				{Time: 217, Pitch: 18},
				{Time: 257, Pitch: 14},
				{Time: 298, Pitch: 11},
				{Time: 339, Pitch: 10},
				{Time: 380, Pitch: 9},
			}},
			// Translunar injection, one prograde burn out of the parking orbit with
			// what the S-IVB kept back for it. The time and the delta-v were found
			// by search, the same way the pitch programmes were: the Moon has to
			// be somewhere specific when the vehicle arrives, and the window is
			// not a thing to guess at.
			Nodes: []Node{{T: 15325, Frame: BurnPrograde, DeltaV: 3162}},

			TargetOrbit: 185000,
			// Six days, because the plan above runs for four. The old hour only
			// worked by accident: the limit stops applying once a flight has a
			// verdict, and this one gets its verdict at T+604 s — so the burn
			// fired at T+15325 despite being well past the stated limit.
			MaxTime: 6 * 86400,
		},
	}
}

// apolloLunar is the same vehicle flown further: into orbit around the Moon
// instead of past it.
//
// Nothing about the rocket changes — the difference is bookkeeping. The command
// and service module stops being dead payload and becomes the fourth stage, so the
// flight plan can use the engine Apollo actually braked with. The mass on the pad
// is the same to the kilogram; the S-IVB is dropped after translunar injection
// because from there it is 23 tonnes of empty tank in the way.
func apolloLunar() Preset {
	p := apolloSaturn()
	p.Name = "apollo-lunar"
	cfg := &p.Cfg

	// What the spacecraft was carrying: the lunar module and its adapter.
	cfg.Rocket.Payload = 16900
	cfg.Rocket.Stages = append(cfg.Rocket.Stages, Stage{
		// The service module: 18.4 t of propellant behind one engine of 91 kN at
		// an Isp of 314 s, and the command module riding on top as dry mass.
		DryMass: 10390, PropMass: 18410,
		ThrustVac: 91190, IspVac: 314, IspSL: 314,
		Throttle: 1,
		// Lit by the plan, which also stops the staging sequence from firing it the
		// moment the S-IVB shuts down over the Atlantic.
		Ignition: IgniteOnNode,
	})

	// Translunar injection, then the braking burn at the far end. Both times and
	// both delta-v figures were found by search — an insertion burn of five and a
	// half minutes is nothing like the impulse a textbook would hand you, so it has
	// to start before the closest approach and be sized against the real thing.
	cfg.Nodes = []Node{
		{T: 15325, Frame: BurnPrograde, DeltaV: 3162, Separate: true},
		{T: 286000, Frame: BurnRetrograde, DeltaV: 725},
	}
	return p
}

// protonK is the vehicle both Proton presets fly: three stages in a line, which is
// what makes it modellable here at all. The R-7 family — Vostok, Soyuz — straps
// four boosters around a core and burns them together, and a serial list of stages
// cannot say that without lying about it.
//
// Real figures: six RD-253 on the first stage, four RD-0210/0211 on the second, one
// RD-0212 on the third, and hypergolics all the way up, which is why it sat on the
// pad for weeks without complaint.
func protonK() []Stage {
	return []Stage{
		{
			DryMass: 31100, PropMass: 419410,
			ThrustVac: 9810000, IspVac: 316, IspSL: 285,
			Throttle: 1, SepDelay: 2,
		},
		{
			DryMass: 11000, PropMass: 156113,
			ThrustVac: 2399000, IspVac: 327, IspSL: 327,
			Throttle: 1, SepDelay: 2,
			Ignition: IgniteImmediate,
		},
		{
			// No cutoff here: where the third stage stops is the mission's
			// business, and each preset that flies this rocket says so itself.
			DryMass: 3500, PropMass: 46562,
			ThrustVac: 613000, IspVac: 325, IspSL: 325,
			Throttle: 1,
			Ignition: IgniteImmediate,
		},
	}
}

// protonInsertion is the launcher with its third stage told when to stop, which is
// the one number that differs between the missions it flies.
func protonInsertion(cutoff float64) []Stage {
	st := protonK()
	st[2].CutoffTime = cutoff
	return st
}

// earthSystem is the solar system with the Earth picked out of it, for the presets
// that launch from there.
func earthSystem() (System, int) {
	sys := SolarSystem()
	return sys, sys.IndexOf("earth")
}

// protonZvezda is the Proton-K that put Zvezda up in July 2000: nineteen tonnes of
// space station module, and the launcher only gets it as far as an ellipse.
//
// That is not a shortfall of the model. Proton-K's advertised nineteen tonnes are to
// a couple of hundred kilometres, not to the station's four hundred, so the real
// launch left the module in 180 x 350 km and it climbed to the station over the
// following days on its own engines.
//
// Here the launcher's third stage cuts off early enough to leave 3.3 t in the tank,
// and 43 m/s of that at the first apoapsis is what makes the orbit round: 512 x 408
// km, with the low side sitting exactly at the station's altitude. Then the stage
// goes overboard and the module is left with all 860 kg of its own propellant, which
// is what the station-keeping this simulator does not model would have wanted.
func protonZvezda() Preset {
	sys, earth := earthSystem()
	return Preset{
		Name: "proton-zvezda",
		Cfg: Config{
			System: sys, LaunchBody: earth, Body: sys.Bodies[earth],
			Atmo: Atmosphere{
				Fractions:       mix("N2", 0.7808, "O2", 0.2095, "Ar", 0.0093, "CO2", 0.0004),
				Layers:          earthISA(),
				SurfaceTemp:     288.15,
				SurfacePressure: 101325,
				Top:             140000,
			},
			Rocket: Rocket{
				// The cargo Zvezda carried inside it; the module itself is the
				// stage below.
				Payload:  1300,
				Cd:       0.4,
				Diameter: 7.4,
				Stages: append(protonInsertion(225), Stage{
					// Zvezda: 16.9 t of module, 860 kg of propellant, two engines
					// of 3.07 kN.
					DryMass: 16890, PropMass: 860,
					ThrustVac: 6140, IspVac: 300, IspSL: 300,
					Throttle: 1,
					Ignition: IgniteOnNode,
				}),
			},
			// Baikonur launches at 51.6 degrees and this simulator has one plane, so
			// the pad here is handed all 465 m/s of the equator's rotation instead of
			// the 325 the real site gets. The ascent is that much cheaper than it
			// should be, which is the same lie every preset here tells.
			Program: Program{Keys: []Keyframe{
				{Time: 0, Pitch: 90},
				{Time: 12, Pitch: 90},
				{Time: 68, Pitch: 68},
				{Time: 125, Pitch: 49},
				{Time: 181, Pitch: 35},
				{Time: 237, Pitch: 23},
				{Time: 294, Pitch: 15},
				{Time: 350, Pitch: 8},
				{Time: 407, Pitch: 4},
				{Time: 463, Pitch: 2},
				{Time: 519, Pitch: 1},
				{Time: 576, Pitch: 0},
			}},
			Nodes: []Node{
				// Prograde at the first apoapsis, which raises the low side, and then
				// the spent stage is dropped. This is the one number here the textbook
				// gets right on its own: 43 m/s, against the 43 the two-body sum says.
				{T: 3333, Frame: BurnPrograde, DeltaV: 43, Separate: true},
			},
			TargetOrbit: 408000,
			MaxTime:     6 * 3600,
		},
	}
}

// protonGeo is the other thing Proton-K did for thirty years: a communications
// satellite to the geostationary belt, with a Blok DM upper stage doing the work
// above the atmosphere.
//
// Three burns, which is what it takes. The launcher's third stage cuts off in a low
// parking orbit with propellant to spare; at the first periapsis it burns that dry
// and goes overboard; Blok DM raises the far side to 35,786 km, coasts five and a
// half hours to get there, and rounds the orbit off. What comes out has a period of
// 23.96 hours against the sidereal day's 23.93 — nine hundredths of a per cent,
// which for a satellite is a slow drift east and a station-keeping budget.
func protonGeo() Preset {
	sys, earth := earthSystem()
	return Preset{
		Name: "proton-geo",
		Cfg: Config{
			System: sys, LaunchBody: earth, Body: sys.Bodies[earth],
			Atmo: Atmosphere{
				Fractions:       mix("N2", 0.7808, "O2", 0.2095, "Ar", 0.0093, "CO2", 0.0004),
				Layers:          earthISA(),
				SurfaceTemp:     288.15,
				SurfacePressure: 101325,
				Top:             140000,
			},
			Rocket: Rocket{
				// Two and a half tonnes of comsat, which is what the belt was worth
				// in the seventies.
				Payload:  2500,
				Cd:       0.4,
				Diameter: 7.4,
				Stages: append(protonInsertion(225), Stage{
					// Blok DM: one 11D58M, restartable, which is the whole reason this
					// mission is possible at all.
					DryMass: 2140, PropMass: 15050,
					ThrustVac: 84900, IspVac: 361, IspSL: 361,
					Throttle: 1,
					Ignition: IgniteOnNode,
				}),
			},
			Program: Program{Keys: []Keyframe{
				{Time: 0, Pitch: 90},
				{Time: 12, Pitch: 90},
				{Time: 68, Pitch: 68},
				{Time: 125, Pitch: 49},
				{Time: 181, Pitch: 35},
				{Time: 237, Pitch: 23},
				{Time: 294, Pitch: 15},
				{Time: 350, Pitch: 8},
				{Time: 407, Pitch: 4},
				{Time: 463, Pitch: 2},
				{Time: 519, Pitch: 1},
				{Time: 576, Pitch: 0},
			}},
			Nodes: []Node{
				// Everything the third stage has left, at the first periapsis, and then
				// it is dropped. 422 m/s is exactly what 3.3 t buys at this mass, so the
				// tank runs dry as the burn ends either way.
				{T: 3630, Frame: BurnPrograde, DeltaV: 422, Separate: true},
				// Blok DM takes the far side up to the belt...
				{T: 3750, Frame: BurnPrograde, DeltaV: 2016},
				// ...and five and a half hours later, rounds the orbit off up there.
				{T: 22711, Frame: BurnPrograde, DeltaV: 1472},
			},
			TargetOrbit: 35786000,
			MaxTime:     6 * 86400,
		},
	}
}

func marsAscent() Preset {
	return Preset{
		Name: "mars",
		Cfg: Config{
			Body: Body{
				Name:           "mars",
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
				Name:           "moon",
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
				Name:           "kerbin",
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
