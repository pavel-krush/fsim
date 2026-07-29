# fsim

A launch simulator. Set up a planet, an atmosphere, a two-stage rocket and a pitch programme, press
launch, and watch it fly — with live telemetry, and graphs when it is over.

The physics is real: gravity is integrated, propellant is spent, mass changes as it burns, and the air
pushes back. Nothing is scripted.

![Setup screen](docs/setup.png)

## Running it

```
go run .                       # start
go run . -lang ru              # start in Russian (default is English)
go run . -preset 2             # start on preset 2 (0..3)
go test ./...                  # physics and interface checks
```

## What is modelled

**The planet.** Radius plus one of {mass, mean density, surface gravity} — the other two are derived.
Rotation gives the launch site its own eastward velocity, which the rocket carries away with it, and
the atmosphere turns along with the ground.

**The atmosphere.** Layers with their own temperature gradients, integrated barometrically from the
surface conditions up. The gas mixture sets the molar mass and the adiabatic index, and through them
the scale height and the speed of sound. With Earth's composition and the standard layers, the model
reproduces the published ISA table to within half a per cent at every altitude.

**The rocket.** Two stages, each with its own dry mass, propellant, thrust, throttle and cutoff.
Specific impulse is interpolated between sea level and vacuum against ambient pressure, so thrust
climbs as the air thins. Drag uses a coefficient and a reference area — no CFD, no shape.

**The flight.** Runge-Kutta 4 at a fixed 0.02 s step, shortened to land exactly on staging events. The
delta-v budget is accounted throughout and split into gravity, drag and steering losses, which is
usually the most interesting number on the screen.

![Flight screen](docs/flight.png)

## Presets

Four launchers, all of which actually reach orbit:

| | orbit | Δv spent | max q | peak |
|---|---|---|---|---|
| Earth / Falcon-9 | 304 × 239 km, e = 0.005 | 8995 m/s | 43.2 kPa at 11 km | 5.9 g |
| Mars / light launcher | 137 × 92 km | 4044 m/s | 0.2 kPa at 14 km | 3.7 g |
| Moon / no atmosphere | 53 × 48 km | 1976 m/s | — | 3.3 g |
| Kerbin / KSP-like | 122 × 92 km | 3772 m/s | 38.4 kPa at 9 km | 3.9 g |

The Earth figure comes in below the 9.3–9.5 km/s a real launcher spends because this one lifts off from
the equator and is handed all 465 m/s of the planet's rotation.

Kerbin needs its second stage set to ignite at apoapsis: a 600 km planet wearing an Earth-thick 70 km
atmosphere does not yield to a direct ascent on a single burn.

## Flying it

The rocket is steered by a pitch programme — a table of times and angles, interpolated between, with
90° straight up and 0° along the horizon. A keyframe can instead be set to hold prograde. Stages cut
off on a timer or when the tanks run dry, and the upper stage can be told to light immediately, after
a delay, or at apoapsis.

Getting to orbit is a matter of pitching over neither too early, which flies you into the thick air,
nor too late, which spends everything climbing. The Δv losses panel tells you which mistake you made.

On the flight screen: `SPACE` pauses, `,` and `.` change the time warp, the wheel zooms, `C` puts the
camera back on automatic.

![Graphs](docs/graphs.png)

## Notes

Written in Go with [Ebiten](https://ebitengine.org/). The interface toolkit is hand-rolled — Ebiten
ships no widgets — and the fonts are compiled in, so there are no assets to install and the binary
runs on its own.

`sim/` is the physics and depends on nothing but the standard library, which means it can be run and
tested without a window. `CLAUDE.md` records the decisions and the traps found along the way.
