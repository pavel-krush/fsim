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
	return []Preset{earthFalcon(), apolloSaturn(), apolloLunar(), apolloReturn(), apolloMars(),
		protonZvezda(), protonGeo(), titanAscent(), ioJupiter(), marsAscent(), moonAscent(),
		kerbinAscent(), kerbinMun(), voyagerTour(), parkerSolar()}
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
				Atmo:           EarthAir(),
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

// apolloReturn is the trajectory Apollo 13 came home on: round the Moon and back
// into the atmosphere, on the injection burn alone. Nothing is fired after the
// translunar injection — the return is a property of the trajectory, which is what
// "free" means, and the whole mission is a single number aimed four days ahead.
//
// Two things had to be true at once and there are only two knobs, the burn time and
// its size, so it was found by search over both. Everything either side of the
// answer is worse in an obvious way: a shade less delta-v and the swingby is too
// close, whips the trajectory round and drops it on the Earth at fifty degrees below
// the horizontal and three hundred g; a shade more and the vehicle comes back so
// shallow that it skips off the atmosphere and spends another nine days about it.
//
// What it settles on: past the Moon at 3226 km, home at T+8.3 days, entry interface
// at 10975 m/s and 7.4 degrees below the horizontal — Apollo's corridor was 6.5 —
// and a peak of 14 g. The real thing pulled half that by flying its capsule as a
// wing; there is no lift in this simulator, so a ballistic dive down the same
// corridor is what there is.
func apolloReturn() Preset {
	p := apolloSaturn()
	p.Name = "apollo-return"
	cfg := &p.Cfg
	cfg.Nodes = []Node{{T: 15295, Frame: BurnPrograde, DeltaV: 3192}}
	// Nine days: the flight takes eight and a bit, and the limit has to be a real
	// bound rather than something a verdict happens to switch off.
	cfg.MaxTime = 9 * 86400
	return p
}

// apolloMars is what the EMPIRE studies were about in 1962: whether a Saturn V
// could be pointed at Mars. This one is, and it arrives — a hundred and eighty-six
// days out, one burn at each end, and a high orbit around Mars at the far side.
//
// The lander stays at home, which is the whole reason it works. Fifteen tonnes of
// lunar module and its adapter come off the payload, so the third stage has 4891 m/s
// of throw where Apollo's had 3668 — and 3668 does not reach Mars's orbit at any
// longitude, whatever the phasing. What is left above the S-IVB is the command and
// service module, 28.8 t of it, and the service module is the fourth stage: the
// engine that brakes at Mars, exactly as apolloLunar uses it to brake at the Moon.
//
// The ascent had to be found again for the lighter stack. The same pitch family with
// a shallower tail, and a third stage that shuts down at 30 s instead of 88.9 —
// otherwise the S-II alone puts the vehicle into 1473 x 205 km and there is nothing
// to circularise. It comes out at 204 x 187 km, rounder than Apollo's own.
func apolloMars() Preset {
	p := apolloSaturn()
	p.Name = "apollo-mars"
	cfg := &p.Cfg

	// The command and service module, with the command module inside the fourth
	// stage's dry mass — so nothing is payload, and the mass above the S-IVB is
	// the 28.8 t the ascent was tuned for.
	cfg.Rocket.Payload = 0
	cfg.Rocket.Stages[2].CutoffTime = 30
	cfg.Rocket.Stages = append(cfg.Rocket.Stages, Stage{
		DryMass: 10390, PropMass: 18410,
		ThrustVac: 91190, IspVac: 314, IspSL: 314,
		Throttle: 1, Ignition: IgniteOnNode,
	})
	cfg.Program = Program{Keys: []Keyframe{
		{Time: 0, Pitch: 90},
		{Time: 12, Pitch: 90},
		{Time: 60, Pitch: 62},
		{Time: 108, Pitch: 42},
		{Time: 155, Pitch: 28},
		{Time: 203, Pitch: 18},
		{Time: 251, Pitch: 13},
		{Time: 299, Pitch: 10},
		{Time: 346, Pitch: 8},
		{Time: 394, Pitch: 8},
		{Time: 442, Pitch: 8},
	}}

	// The injection has to happen at the one point in the parking orbit where the
	// escape asymptote comes out along the Earth's own motion round the Sun.
	// Anywhere else and the same 3690 m/s is thrown sideways: at T+3000 s it buys a
	// heliocentric aphelion of 1.07 AU instead of 1.62, which is to say nothing at
	// all. Then the spent S-IVB goes overboard with 15 t still in it, because a
	// hydrogen stage that has been in the cold for six months is not going to
	// relight anyway.
	//
	// The braking burn is eleven minutes long and lands the vehicle in a 95159 x
	// 91139 km orbit at e = 0.021. High, because a chemical stack arrives at 3 km/s
	// of hyperbolic excess and 2410 m/s is what the service module can spend on it.
	cfg.Nodes = []Node{
		{T: 4500, Frame: BurnPrograde, DeltaV: 3690, Separate: true},
		{T: 16104985, Frame: BurnRetrograde, DeltaV: 2410},
	}

	cfg.TargetOrbit = 190000
	// Two hundred days, against a mission of a hundred and eighty-six.
	cfg.MaxTime = 200 * 86400
	return p
}

// ioJupiter launches from Io, which is the smallest thing here to leave and the
// deepest gravity well to be standing in. Io itself is easy — 1809 m/s of circular
// speed at the surface and no air at all — but Jupiter is 4.2e8 m away and 318 times
// the Earth's mass, and Io's own sphere of influence is only 7840 km wide. Four and
// a third radii. So an orbit round it is a thing you have barely got room for.
// Getting out of the sphere takes only 417 m/s, the edge being that close rather than
// at infinity, against the 739 of a full escape; the preset spends 750 anyway, for
// the reason given at the node below.
//
// What it ends up in is an orbit round Jupiter crossing Io's own, and the verdict says
// Jupiter, because that is who has it now.
// The vehicle is invented; two stages and a fifth of Apollo's lunar module.
func ioJupiter() Preset {
	sys := SolarSystem()
	io := sys.IndexOf("io")
	// Io has an atmosphere of sulphur dioxide at a billionth of a bar, which is
	// nothing to fly through: vacuum, like the Moon.

	return Preset{
		Name: "io-jupiter",
		Cfg: Config{
			System: sys, LaunchBody: io, Body: sys.Bodies[io],
			Rocket: Rocket{
				Payload: 300, Cd: 0.3, Diameter: 3.0,
				Stages: []Stage{
					{
						DryMass: 900, PropMass: 1400,
						ThrustVac: 12000, IspVac: 311, IspSL: 311,
						Throttle: 1, SepDelay: 2,
					},
					{
						// Shut down at 132 s with two thirds of the tank still in
						// it: the rest is the trip to Jupiter, and there is 1803 m/s
						// of it for a 750 m/s departure.
						DryMass: 300, PropMass: 700,
						ThrustVac: 5000, IspVac: 311, IspSL: 311,
						Throttle: 1, CutoffTime: 132,
						Ignition: IgniteImmediate,
					},
				},
			},
			Program: Program{Keys: []Keyframe{
				{Time: 0, Pitch: 90},
				{Time: 20, Pitch: 90},
				{Time: 47, Pitch: 76},
				{Time: 75, Pitch: 63},
				{Time: 102, Pitch: 52},
				{Time: 129, Pitch: 41},
				{Time: 156, Pitch: 31},
				{Time: 184, Pitch: 23},
				{Time: 211, Pitch: 15},
				{Time: 238, Pitch: 9},
				{Time: 265, Pitch: 5},
				{Time: 293, Pitch: 1},
				{Time: 320, Pitch: 0},
			}},
			// The departure has to leave *outwards*, which is a question of where in
			// the parking orbit it happens: the same 750 m/s at T+2000 s drops the
			// vehicle inside Io's orbit and at T+4500 s lifts it above. And 750
			// rather than the 417 that gets out of the sphere or the 739 of a proper
			// escape, because it is the value either side of which the resulting
			// orbit wanders back through Io's sphere inside a month.
			Nodes: []Node{{T: 4500, Frame: BurnPrograde, DeltaV: 750}},

			TargetOrbit: 60000,
			MaxTime:     12 * 3600,
		},
	}
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
			// Two days, against a plan that finishes in six and a quarter hours: a
			// belt satellite is one whose day matches the planet's, so the run wants
			// room for a revolution of each to see that it does.
			MaxTime: 2 * 86400,
		},
	}
}

// titanAscent launches from Titan, which is the strangest ascent in the system and
// took the most work to make possible at all.
//
// Four times Earth's surface density under a seventh of its gravity. The air is so
// thick that a rocket cannot go fast in it — at 22 kN of thrust the terminal
// velocity at the surface is 173 m/s — and so deep that it is still worth 1e-9 of
// the surface density at 435 km, which is where the atmosphere's nominal top has to
// be. That in turn puts a closed orbit at 600 km, and every one of those facts fights
// the launch.
//
// What comes out of it: seven and a half minutes of climbing straight up at a few
// hundred metres a second, a turn once the air is behind, and a kick stage at
// apoapsis. Drag costs 622 m/s and gravity 1263, against 1682 to be in orbit at all.
// The vehicle is invented — nobody has built this — but the numbers are Titan's.
func titanAscent() Preset {
	sys := SolarSystem()
	titan := sys.IndexOf("titan")
	return Preset{
		Name: "titan-ascent",
		Cfg: Config{
			// Titan's air comes with Titan: see TitanAir. 1.5 bar of nitrogen at
			// 94 K, and the reason this preset was the hardest of the lot.
			System: sys, LaunchBody: titan, Body: sys.Bodies[titan],
			Rocket: Rocket{
				Payload:  400,
				Cd:       0.25,
				Diameter: 1.6,
				Stages: []Stage{
					{
						// Methane and oxygen, both of which Titan has lying about.
						DryMass: 900, PropMass: 5000,
						ThrustVac: 22000, IspVac: 340, IspSL: 300,
						Throttle: 1, CutoffTime: 700, SepDelay: 2,
					},
					{
						// The kick stage, lit at apoapsis, which is the only way to
						// stop a single continuous burn from ending while still
						// climbing — the first cut did exactly that and left the low
						// side of the orbit at 50 km however it was tuned.
						DryMass: 300, PropMass: 1300,
						ThrustVac: 5000, IspVac: 340, IspSL: 340,
						Throttle: 1, CutoffTime: 420,
						Ignition: IgniteAtApoapsis,
					},
				},
			},
			// Nine minutes of vertical. Anything less and the turn happens in air thick
			// enough to eat two kilometres a second of drag; anything more and there is
			// not enough propellant left to build the horizontal speed. It was seven
			// and a half until Titan's ceiling moved from 500 km to 720 — the depth at
			// which its air actually reaches Earth's cutoff density — and the orbit had
			// to go up with it. The kick stage had the propellant for it all along: it
			// was burning 450 kg of the 1300 it carries.
			Program: Program{Keys: []Keyframe{
				{Time: 0, Pitch: 90},
				{Time: 550, Pitch: 90},
				{Time: 585, Pitch: 75.6},
				{Time: 620, Pitch: 62.7},
				{Time: 655, Pitch: 51.4},
				{Time: 690, Pitch: 41.5},
				{Time: 725, Pitch: 33.1},
				{Time: 760, Pitch: 26.0},
				{Time: 795, Pitch: 20.3},
				{Time: 830, Pitch: 15.8},
				{Time: 865, Pitch: 12.6},
				{Time: 900, Pitch: 10.4},
				{Time: 935, Pitch: 9.3},
				{Time: 970, Pitch: 9.0},
			}},
			TargetOrbit: 1000000,
			MaxTime:     6 * 3600,
		},
	}
}

func marsAscent() Preset {
	return Preset{
		Name: "mars-ascent",
		Cfg: Config{
			Body: Body{
				Name:           "mars",
				Radius:         3389500,
				MassSource:     FromMass,
				Mass:           6.4171e23,
				RotationPeriod: 88642,
				Atmo:           MarsAir(),
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
						Throttle: 1, CutoffTime: 205.0,
						Ignition: IgniteAfterDelay, IgnitionDelay: 3,
					},
				},
			},
			// Found by search again, because Mars's air got deeper: the ceiling moved
			// from 90 km to 150, and the old programme parked its periapsis at 92.
			// The tail is *below* the horizon — thrust pointed 18° down at the end of
			// the burn is what stops the climb while the horizontal speed is still
			// building, and it is the only thing in this family that raises a
			// periapsis rather than an apoapsis. A kick stage at apoapsis, which is
			// how Kerbin and Titan solve the same problem, cannot work here: this
			// upper stage has 2.6 km/s and circularising at apoapsis from a standstill
			// wants 3.5.
			Program: Program{Keys: []Keyframe{
				{Time: 0, Pitch: 90},
				{Time: 12, Pitch: 90},
				{Time: 37, Pitch: 85.0},
				{Time: 62, Pitch: 79.7},
				{Time: 87, Pitch: 74.2},
				{Time: 112, Pitch: 68.4},
				{Time: 137, Pitch: 62.3},
				{Time: 162, Pitch: 55.8},
				{Time: 187, Pitch: 48.7},
				{Time: 212, Pitch: 41.0},
				{Time: 237, Pitch: 32.4},
				{Time: 262, Pitch: 22.3},
				{Time: 287, Pitch: 9.5},
				{Time: 312, Pitch: -18.0},
			}},
			TargetOrbit: 200000,
			MaxTime:     3600,
		},
	}
}

func moonAscent() Preset {
	return Preset{
		Name: "moon-ascent",
		Cfg: Config{
			Body: Body{
				Name:           "moon",
				Radius:         1737400,
				MassSource:     FromMass,
				Mass:           7.342e22,
				RotationPeriod: 2360591,
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

// kerbinSystem is Kerbin with a Mun, which the single-planet kerbin preset does
// not have — that one stays a system of one body so its figures keep meaning what
// they meant. Both bodies are invented and these are the game's own numbers: a
// 600 km planet at one g, a 200 km moon at a sixth of it, twelve thousand
// kilometres out and tidally locked. The sphere of influence works out at 2430 km,
// twelve lunar radii, which is what the game says too.
// titanIIIE is the Titan IIIE / Centaur, five stages in a line.
//
// It is here rather than an R-7 or a Delta for the same reason Proton-K is: the boosters
// burn *first and alone*, and the core lights after they are gone, so the whole stack is
// serial and Rocket.Stages can hold it honestly. The one simplification is three seconds
// of overlap between the solids burning out and the core igniting.
func titanIIIE() Rocket {
	return Rocket{
		// Voyager 2 itself: 825 kg with its hydrazine.
		Payload: 825, Cd: 0.3, Diameter: 3.05,
		Stages: []Stage{
			// Two UA1205 solids, 193 t of propellant each.
			{DryMass: 66912, PropMass: 385554, ThrustVac: 12454000, IspVac: 272, IspSL: 237,
				Throttle: 1, SepDelay: 1},
			// Titan core stage 1: LR87-AJ-11, two chambers on hypergolics.
			{DryMass: 8000, PropMass: 123000, ThrustVac: 2340000, IspVac: 302, IspSL: 258,
				Throttle: 1, SepDelay: 2, Ignition: IgniteAfterDelay},
			// Core stage 2: LR91-AJ-11.
			{DryMass: 2830, PropMass: 33152, ThrustVac: 454000, IspVac: 316, IspSL: 316,
				Throttle: 1, SepDelay: 2, Ignition: IgniteAfterDelay},
			// Centaur D-1T, two RL10s. It is cut off 70 s in, with eleven and a half
			// tonnes still aboard: the parking orbit is all the ascent needs, and the
			// flight plan lights it again for the injection. The same trick Zvezda's
			// third stage uses to circularise.
			{DryMass: 1996, PropMass: 13900, ThrustVac: 146800, IspVac: 444, IspSL: 444,
				Throttle: 1, CutoffTime: 70, Ignition: IgniteAfterDelay, IgnitionDelay: 2},
			// TE-364-4, the kick stage Voyager carried itself, fired by the plan once
			// the Centaur has gone.
			{DryMass: 118, PropMass: 1038, ThrustVac: 66700, IspVac: 286, IspSL: 286,
				Throttle: 1, Ignition: IgniteOnNode},
		},
	}
}

// voyagerTour is Voyager 2's grand tour: Jupiter, Saturn, Uranus, Neptune and out of
// the system, on one injection and four gravity assists.
//
// The four windows were *solved*, not searched — the mean anomalies here are not an
// ephemeris, so each planet is put where the flight crosses its orbit by inverting the
// same Kepler equation the rails run on. But it takes iterating: moving a planet changes
// the very trajectory the phase was solved from, since its own gravity perturbs years of
// cruise and every close pass upstream amplifies the difference. One pass leaves a miss
// of an astronomical unit; four bring it inside the sphere of influence.
//
// The passes are farther out than the real ones — 38, 163, 28 and 15 radii against
// Voyager's 5, 3, 4 and 1 — because these are the four that close *as a chain* in one
// plane. Each one still does the job: the aphelion grows 23 → 30 → 58 AU and Neptune's
// pass leaves the vehicle heading out at 34 AU by the thirtieth year.
//
// The four encounters are solid; the last verdict is not. A pass at ten radii amplifies
// the last bit of the integrator, so whether Neptune adds quite enough to leave the Sun
// for good depends on how the flight was advanced — see the test.
func voyagerTour() Preset {
	sys := SolarSystem()
	earth := sys.IndexOf("earth")

	// The tour, one window per planet. These are the numbers the iteration converged on.
	for _, w := range []struct {
		name string
		m0   float64
	}{
		{"jupiter", voyagerJupiterPhase},
		{"saturn", voyagerSaturnPhase},
		{"uranus", voyagerUranusPhase},
		{"neptune", voyagerNeptunePhase},
	} {
		sys.Bodies[sys.IndexOf(w.name)].MeanAnom0 = w.m0
	}
	sys.Normalize()

	return Preset{
		Name: "voyager-tour",
		Cfg: Config{
			System: sys, LaunchBody: earth, Body: sys.Bodies[earth],
			Rocket: titanIIIE(),
			Program: Program{Keys: []Keyframe{
				{Time: 0, Pitch: 90},
				{Time: 16, Pitch: 90},
				{Time: 39, Pitch: 75.6},
				{Time: 63, Pitch: 62.5},
				{Time: 86, Pitch: 50.6},
				{Time: 109, Pitch: 40.0},
				{Time: 133, Pitch: 30.6},
				{Time: 156, Pitch: 22.5},
				{Time: 179, Pitch: 15.6},
				{Time: 203, Pitch: 10.0},
				{Time: 226, Pitch: 5.6},
				{Time: 249, Pitch: 2.5},
				{Time: 273, Pitch: 0.6},
				{Time: 296, Pitch: 0.0},
			}},
			// The injection, out of the parking orbit and at the one point in it that
			// works: T+5300 s is where the escape asymptote comes out along the Earth's
			// own motion. Half an orbit either side and the same delta-v buys a
			// heliocentric aphelion of 1.0 AU instead of 10.
			//
			// The Centaur is relit for the first 5900 m/s and dropped; the TE-364-4
			// does the last thousand. That reaches Jupiter's orbit in 689 days, which
			// is what Voyager 2 took.
			Nodes: []Node{
				{T: 5300, Frame: BurnPrograde, DeltaV: 5900, Separate: true},
				{T: 5700, Frame: BurnPrograde, DeltaV: 1000},
			},
			TargetOrbit: 185000,
			// Thirty years: Neptune is passed in the twenty-third and there is nothing
			// after it. If the last pass leaves the trajectory unbound the verdict is an
			// escape, which is a settled one — and the clock is what stops the run, since
			// a vehicle on its way out has nowhere to be.
			MaxTime: 30 * 365.25 * 86400,
		},
	}
}

// The four windows, as converged. They are written out rather than solved at startup
// because a preset is data: the search that found them lived in a throwaway test.
const (
	voyagerJupiterPhase = 1.867744
	voyagerSaturnPhase  = 2.714653
	voyagerUranusPhase  = 3.438083
	voyagerNeptunePhase = 3.884496
)

// deltaIVHeavy is the Delta IV Heavy with a Star 48BV on top, which is what threw the
// Parker Solar Probe onto the highest C3 ever flown.
//
// Three common cores burn together off the pad and a serial list cannot hold that. Giving
// stage 1 the two side boosters alone does not work either: that is a thrust-to-weight of
// 0.79 and the stack sits there. So the split here is by *thrust phase* rather than by
// hardware — stage 1 has all three engines and the propellant they burn before the sides
// separate, which is the two sides entire plus the four minutes the core spends throttled
// to 55%, and stage 2 is the core's remaining 96 t on its own engine. Liftoff
// thrust-to-weight comes out 1.18 against the real 1.2.
func deltaIVHeavy() Rocket {
	return Rocket{
		// No payload of its own: the spacecraft is the last stage, because it steers.
		Payload: 0, Cd: 0.35, Diameter: 5.0,
		Stages: []Stage{
			{DryMass: 53520, PropMass: 502580, ThrustVac: 9411000, IspVac: 412, IspSL: 362,
				Throttle: 1, SepDelay: 1},
			{DryMass: 26760, PropMass: 96340, ThrustVac: 3137000, IspVac: 412, IspSL: 362,
				Throttle: 1, SepDelay: 2, Ignition: IgniteAfterDelay},
			// The 5-metre Delta Cryogenic Second Stage, one RL10B-2. Cut off in the
			// parking orbit with 22 t left and relit by the plan.
			{DryMass: 3490, PropMass: 27220, ThrustVac: 110000, IspVac: 462, IspSL: 462,
				Throttle: 1, CutoffTime: 180, Ignition: IgniteAfterDelay, IgnitionDelay: 2},
			// Star 48BV, the solid that finishes the job. A solid fires once, which is
			// why the corrections below are not taken out of its leftovers the way
			// Zvezda's are: the flight plan drops it and steers with the spacecraft.
			{DryMass: 130, PropMass: 2010, ThrustVac: 68600, IspVac: 286, IspSL: 286,
				Throttle: 1, Ignition: IgniteOnNode},
			// Parker itself, which is a stage here rather than payload because it does
			// its own manoeuvring: 52 kg of hydrazine behind four 4.4 N thrusters is
			// 178 m/s, and a correction of twenty metres a second takes fourteen minutes
			// of it.
			{DryMass: 633, PropMass: 52, ThrustVac: 17.6, IspVac: 230, IspSL: 230,
				Throttle: 1, Ignition: IgniteOnNode},
		},
	}
}

// parkerSolar is the Parker Solar Probe: the only preset here that spends its energy going
// *down*, the fastest thing in the collection, and the one whose mission is a chain rather than
// a trajectory.
//
// Everything about it is backwards from the rest. The injection is aimed against the Earth's own
// motion rather than along it — what it buys is not distance but the loss of heliocentric angular
// momentum — and every Venus flyby takes energy *out*. Three of them walk the perihelion from
// 39 solar radii to 23.7, where the vehicle is doing over a hundred kilometres a second.
//
// The flybys are held by **control points** rather than by numbers: each says which side of Venus
// to pass and how close, and the delta-v is solved for when the moment arrives. That is what makes
// this chain reproducible where the grand tour's is not — an aim re-solves against the path the
// flight is actually on, and a chain of fixed numbers is a chain solved for one exact path through
// the arithmetic.
//
// The real mission needed seven flybys over seven years to reach 9.9 radii. Three is what 52 kg
// of hydrazine buys: 139 m/s of the 178 aboard, and the fourth would cost another hundred. The
// rest of the real chain is bought with geometry rather than propellant, over years of design.
func parkerSolar() Preset {
	sys := SolarSystem()
	earth := sys.IndexOf("earth")
	ven := sys.IndexOf("venus")
	vr := sys.Bodies[ven].Radius
	// Venus, put where the vehicle crosses its orbit on the way *down*: a pass on the inbound
	// leg is the one that takes angular momentum away. Solved by iteration against this
	// configuration, rounded keyframes and all.
	sys.Bodies[ven].MeanAnom0 = parkerVenusPhase
	sys.Normalize()

	// One aim per flyby, each a couple of months before its pass. Negative is the side that
	// removes angular momentum — positive would raise the perihelion instead — and closer is
	// more of it: at these speeds a pass at two radii is worth some five solar radii of
	// perihelion. Close to the pass rather than early: a correction far out changes the
	// *period* as well, which breaks the resonance that brings the vehicle back at all.
	aim := func(t, radii, limit float64) Node {
		return Node{T: t, Frame: BurnRadialOut, Target: TargetFlybyPeriapsis,
			TargetBody: ven, TargetValue: radii * vr, Limit: limit, Horizon: 120 * 86400}
	}

	return Preset{
		Name: "parker-solar",
		Cfg: Config{
			System: sys, LaunchBody: earth, Body: sys.Bodies[earth],
			Rocket: deltaIVHeavy(),
			Program: Program{Keys: []Keyframe{
				{Time: 0, Pitch: 90},
				{Time: 28, Pitch: 90},
				{Time: 53, Pitch: 76.7},
				{Time: 78, Pitch: 64.3},
				{Time: 103, Pitch: 52.8},
				{Time: 128, Pitch: 42.2},
				{Time: 153, Pitch: 32.6},
				{Time: 178, Pitch: 23.9},
				{Time: 203, Pitch: 16.3},
				{Time: 228, Pitch: 9.7},
				{Time: 253, Pitch: 4.3},
				{Time: 278, Pitch: 0.1},
				{Time: 303, Pitch: -2.8},
				{Time: 328, Pitch: -4.0},
			}},
			// The injection, and the point in the parking orbit is everything: T+2350 s puts
			// the escape asymptote against the Earth's motion and drops the perihelion to
			// 0.18 AU. Half an orbit later the same 10.3 km/s buys a perihelion of 0.98 AU
			// and an escape from the system instead — the same sweep as Voyager's, read from
			// the other end. The Star 48BV goes overboard afterwards: the spacecraft steers
			// from here on.
			Nodes: []Node{
				{T: 2350, Frame: BurnPrograde, DeltaV: 6900, Separate: true},
				{T: 2750, Frame: BurnPrograde, DeltaV: 3400, Separate: true},
				// The three passes: T+46 days, then T+495 and T+1169, each brought round
				// by the resonance the one before it left — three of its orbits to two
				// Venus years, then five to three.
				aim(20*86400, -3.0, 150),
				aim(430*86400, -2.1, 60),
				aim(1100*86400, -1.8, 150),
			},
			TargetOrbit: 190000,
			// Three and a half years: the third flyby is in the fourth year and there is
			// nothing after it but the same ellipse going round. Every full-mission run
			// pays for this one, the screenshot script included.
			MaxTime: 3.5 * 365.25 * 86400,
		},
	}
}

// parkerVenusPhase is where Venus has to be, as converged. See voyagerTour for why this is
// a number rather than a search.
const parkerVenusPhase = 5.557125

// kerbinAir is Earth's air on a planet a ninth the size: 1 bar at the surface and all
// of it gone by 70 km, which is what makes a direct ascent there impossible and the
// kick stage at apoapsis necessary.
func kerbinAir() Atmosphere {
	return Atmosphere{
		Fractions:       mix("N2", 0.78, "O2", 0.21, "Ar", 0.01),
		Layers:          []Layer{{0, -0.008}, {9000, -0.004}, {25000, 0.001}, {45000, 0}},
		SurfaceTemp:     288.15,
		SurfacePressure: 101325,
		Top:             70000,
	}
}

func kerbinSystem() System {
	sys := System{Bodies: []Body{
		{Name: "kerbin", Radius: 600000, MassSource: FromMass, Mass: 5.2915158e22,
			RotationPeriod: 21549.425, Atmo: kerbinAir()},
		// The mean anomaly is the launch window, solved the same way Mars's was:
		// fly the transfer, take where and when it crosses the Mun's orbit, and
		// put the Mun there — then step off it far enough to miss.
		{Name: "mun", Radius: 200000, MassSource: FromMass, Mass: 9.7600236e20,
			RotationPeriod: 138984, Parent: 0, SemiMajor: 1.2e7, MeanAnom0: 1.64313},
	}}
	sys.Normalize()
	return sys
}

// kerbinMun is the same launcher with a stretched second stage, flown to an orbit
// round the Mun: up in nine minutes, away at T+2000 s, and there five hours later.
//
// The whole trip after insertion costs 1218 m/s, and the stock upper stage has
// 1194 — so it carries 800 kg more propellant, and the cutoff moves with it.
// Everything else is the launcher that was already here.
func kerbinMun() Preset {
	p := kerbinAscent()
	p.Name = "kerbin-mun"
	cfg := &p.Cfg
	sys := kerbinSystem()
	cfg.System, cfg.LaunchBody, cfg.Body = sys, 0, sys.Bodies[0]

	cfg.Rocket.Stages[1].PropMass = 3400
	cfg.Rocket.Stages[1].CutoffTime = 79

	// 868 m/s away, and 350 to stay. The transfer is aimed *past* the Mun on
	// purpose: a dead-centre intercept is a collision, and with 364 m/s of
	// hyperbolic excess against a body this small the grazing impact parameter is
	// 486 km — so 851 to 861 m/s all end in a crater, and the miss has to be
	// bought with another 7.
	cfg.Nodes = []Node{
		{T: 2000, Frame: BurnPrograde, DeltaV: 868},
		{T: 18312, Frame: BurnRetrograde, DeltaV: 350},
	}
	cfg.MaxTime = 12 * 3600
	return p
}

func kerbinAscent() Preset {
	return Preset{
		Name: "kerbin-ascent",
		Cfg: Config{
			Body: Body{
				Name:           "kerbin",
				Radius:         600000,
				MassSource:     FromMass,
				Mass:           5.2915158e22,
				RotationPeriod: 21549.425,
				Atmo:           kerbinAir(),
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
