# fsim

A launch simulator that grew a solar system. Set up a planet — or any body of a system of eighteen — its
atmosphere, a vehicle of one to four stages, a pitch programme and a plan of burns → "Launch" → live
ascent with telemetry → the camera pulls back from the launch pad to the Moon's orbit → graphs.
Go + Ebiten, and the physics is real.

The Apollo preset flies the whole thing: Saturn V off the pad, a 192 × 186 km parking orbit at T+604 s,
translunar injection at T+15325 s, and the Moon's sphere of influence two and a half days later.

## Build & run

```
go run .                       # start
go run . -preset apollo-lunar  # start on a preset by name (see sim.Presets), not by position
go run . -shot ./shots         # run the capture script and save a PNG of every screen
go run . -camtrace 700         # print the vehicle's screen coordinates per frame (catches camera shake)
go run . -lang ru              # start with the interface in Russian (default is English)
go test ./...                  # physics and interface
go build ./... && go vet ./...
```

`-shot` exists because Ebiten can only create and read images inside a running game loop — there is no
way to render the UI headless. The flag drives the real loop through the script in `shot.go` and dumps
the canvas to PNG. It is the only way to look at the interface without a human at the keyboard.

## Layout

| File | Contents |
|------|----------|
| `sim/body.go` | One body: radius plus one of {mass, density, g} derives the rest, plus its orbit about its parent. `Normalize()` is mandatory before using `Mu` |
| `sim/system.go` | The tree: Keplerian rails, spheres of influence, gravity with the rail correction, add/remove |
| `sim/solar.go` | The Sun, eight planets and nine major moons, with the real numbers |
| `sim/atmosphere.go` | Gas composition → molar mass and γ; layers with lapse rates → barometric T/P/ρ/a profile |
| `sim/rocket.go` | Stages: mass, propellant, thrust, Isp(p), ṁ, cutoff, ignition mode. Tsiolkovsky, TWR |
| `sim/program.go` | Pitch keyframes, interpolation, prograde-hold mode |
| `sim/node.go` | Manoeuvre nodes: a time, a direction, a Δv — and `Predict`, which flies a copy of the plan |
| `sim/orbit.go` | `Vec2` plus osculating elements from (r, v) |
| `sim/sim.go` | State, RK4 step, staging and node state machine, verdicts, Δv loss accounting, telemetry, history |
| `sim/coast.go` | The adaptive step: what a vehicle that is only falling gets instead of 0.02 s, and the time warp's step cap |
| `sim/presets.go` | Earth/Falcon-9, two Apollos, Mars, Moon, Kerbin — all six reach orbit, and the Apollos go further |
| `main.go` | `App` — the three-screen state machine, `ebiten.Game` |
| `theme.go` | Palette and fonts (goregular/gomono, compiled in, no asset files on disk), and what colour each body is |
| `ui.go` | Immediate-mode toolkit: `NumField`, `Button`, `Radio`, `Checkbox`, `Dropdown`, `Scroll` |
| `lang.go` | Locale loading and lookup, RU/EN switching, dispatch for events, verdicts, phases, presets, bodies |
| `assets/locale/*.json` | All interface text, one file per language, flat dotted keys |
| `render.go` | `Rect`, primitives, `Camera` (world metres → pixels, with rotation) and its inverse |
| `screen_setup.go` | Four-column parameter form: the body editor, atmosphere, vehicle, keyframes, derived figures, presets |
| `screen_flight.go` | Trajectory, bodies, rails, prediction, launch pad, camera, flight plan, telemetry, time controls |
| `screen_graphs.go` | Seven plots on a movable time axis, event ruler, scrubber |
| `shot.go` | Scripted run for screenshots |

## The system of bodies

The world is a `sim.System`: a flat slice of bodies forming a tree, root at index 0. The root does not
move. Everything else runs on **Keplerian rails** — `StateAt(i, t)` is analytic, so a jump of three days
costs no more than a jump of a second and nothing drifts. `Config.System` empty means a system of one
body built from `Config.Body`, which is what every single-planet configuration is.

- **A body's parent always sits at a lower index.** That one invariant makes a cycle impossible to
  express, makes every walk up the tree terminate by construction, and leaves the slice in topological
  order. `Normalize` enforces it, clamping bad data to the root rather than trusting the author.
- **`Config.Body` is the launch body's editable face.** `New` copies it *into* the system when it has a
  radius, then mirrors it back, so the setup screen's fields still work on a multi-body preset — the first
  cut copied the other way and editing the planet was a silent no-op. A caller that fills the system and
  leaves `Body` empty, which is every test that builds a system by hand, is left alone.
- **The state is measured from `State.Center`**, the deepest body whose sphere of influence contains the
  vehicle, in a frame that does not rotate with it. Not from the root: heliocentric coordinates are
  ~1.5e11 m, where float64 resolves 3e-5 m, and the ascent tests assert altitudes to 1e-6 m. They would
  have failed silently. Body-centred keeps the numbers where they were.
- **The sphere of influence chooses a frame, not a physics.** Every body pulls at all times.
- **`refocus` runs first in `postStep`**, so a frame change only ever lands on a step boundary and no
  reading is ever half in one frame and half in another. The transformation is exact: the same point,
  written from a different centre.
- **The rail correction runs up the chain of ancestors and no further.** The frame of a body that is
  itself on rails is not inertial, so the acceleration the rails give it has to come back out of the
  gravity sum — otherwise its parent drags the whole picture sideways. A true N-body indirect term sums
  over *every* perturber; that would contradict the rails, under which a body does not respond to its own
  children. In the root's frame there is no correction at all, because the root does not move.
- **The rails use the parent's mu alone,** not the sum of the two masses that exact relative motion would
  use. Consequence: the rails agree exactly with the parent's real pull at the child's centre, so a
  vehicle in lunar orbit feels only the tidal residual — and the sidereal month comes out 0.47% long
  (27.44 days against 27.32). Using the sum reverses the trade: the month is right and a phantom
  3.3e-5 m/s² appears everywhere near the child. Nobody can see the month; everybody sees an orbit drift.
- **Known lie, from the same bargain:** with the parent nailed in place, the Moon's pull on a low Earth
  orbit is uncompensated, so it is some 27× the real tidal effect — 3.3e-5 m/s², periodic rather than
  secular, and invisible next to 9.8. The fix, if it ever matters, is barycentric rails, where a parent
  wobbles about the barycentre of each link.
- **A system of one body is the old single-planet model to the last digit.** `Gravity` returns early for
  it, keeping the same arithmetic shape, and `TestOneBodySystemIsTheOldModel` pins that. All five presets
  were checked bit-for-bit across the change, all 17 significant digits of the final state.
- Only the launch body has an atmosphere. Air on every body waits for a setup screen that can describe
  it; until then `atmoTop` and the drag lookup return vacuum away from the launch body.

## Physics — what matters

- **The integrator step is fixed at `sim.FixedStep` = 0.02 s wherever anything is happening** — engine
  running, air outside, wheels on the ground. A vehicle that is only falling gets the adaptive step
  instead; see the section below, and note that the old promise of a trajectory independent of the warp
  setting survives only for the powered and atmospheric parts of a flight.
- **`Advance` carries the remainder** in `Sim.accum`. Ragged frame times give exactly the same
  trajectory as an even `Step(FixedStep)` loop. `TestAdvanceMatchesFixedSteps` guards this.
- **The step is only ever shortened to a positive value.** `Step` used to treat a collapsed step as
  burnout and empty the tank — the floating-point crumb at the end of every `Advance` ate a whole stage.
- **Mass inside a step is an exact linear function of time** (`burnContext`), so all four Runge-Kutta
  stages see a consistent mass.
- **Δv over a step is integrated in closed form,** `F/ṁ · ln(m₀/m₁)`, not as a rectangle: mass drops
  during the step and the left-hand rectangle systematically undershoots (0.013% per stage — it showed
  up in the Tsiolkovsky test).
- **`Orbit.Bound()` tests the energy, not the eccentricity.** A purely radial fall has e = 1 exactly,
  and an eccentricity test declared free fall an escape trajectory.
- **`Body.Mu` is only filled in by `Normalize()`.** A local copy of `Config` does not have it — always
  take it from `s.Cfg.Body` of an already constructed simulation.
- **A rocket with TWR < 1 sits on the pad and burns propellant.** That is the `holdOnPad` branch;
  without it the vehicle sinks through the planet. Its altitude is exactly 0 there, which is why a
  crash is detected with `alt < 0` rather than `<= 0`.
- **Gravity losses use the flight path angle in the frame rotating with the planet.** In the inertial
  frame the velocity at liftoff is horizontal (from rotation) and the losses would come out zero.
- **g in the barometric formula is the surface value, held constant.** The standard ISA simplification,
  worth about 3% at 100 km.
- **Reaching orbit is a verdict, not the end of the run.** `settle` records the outcome and lets the
  vehicle keep going round for ever; `finish` is for the terminal cases — crashed, escaped — and `stop`
  ends the run when the clock runs out. `Settled()` is what the interface watches, not `Done`.
- **`MaxTime` only applies while there is no verdict.** It exists to cut short a flight that is going
  nowhere; one that made orbit has somewhere to be. That does mean a settled flight never ends on its
  own, so three things had to be bounded behind it: the history thins by `coastRecordFactor` once
  settled, apoapsis stops being marked (it comes round every revolution for ever), and the graph axis
  stops shortly after the last event instead of stretching to hours of flat line.
- **The max-q marker is placed in hindsight.** The peak is only knowable once it has passed, so
  `checkMaxQPassed` waits until the pressure drops below a quarter of the maximum and then inserts the
  event through `markAt` at the real instant of the peak — keeping the event list in chronological
  order, otherwise the ruler on the graph screen assigns label rows incorrectly.
  **`MarkMaxQ` does nothing while the flight is running.** The graph screen also opens on a pause
  mid-ascent, and that call used to pin the marker to whatever the running maximum was — permanently,
  because the `maxQMarked` flag silences the automatic detection. The forced placement lives in
  `emitMaxQ`, called only by `checkMaxQPassed` and `finish`.

## Coasting, and the time warp

Three days to the Moon is thirteen million fixed steps, which no warp setting can chew through. So
`sim/coast.go` gives a vehicle that is *only falling* — `PhaseCoast`, above the air, off the ground — an
error-controlled Cash-Karp step, and leaves everything else on the 0.02 s the ascent was tuned on. A day
of coasting comes out 1 mm from the fixed-step answer in 1265 steps instead of 4.3 million; the transfer
to the Moon takes 846.

- **The trajectory now depends on the warp setting during a coast**, because `stepCap` limits how far one
  step may reach — a step longer than a frame's worth of simulated time would jump instead of playing. It
  is bounded by `coastTol`, not free, and **at ×1 the cap is exactly the fixed step, so a real-time flight
  is bit-for-bit what the simulator has always produced.** `TestWarpRateOneIsTheOldFixedStep` pins that,
  and all five presets were checked bit-for-bit across the whole change.
- **Frame rate still cannot matter.** The step is a pure function of the state; the accumulator decides
  where a frame stops, never how far a step goes.
- **`coastTarget` deliberately carries nothing between steps.** A textbook controller remembers the step
  it settled on, which makes the trajectory depend on the controller's history as well as on the state.
  Here the target is a fixed fraction of the local timescale — `min(√(r³/μ), r/v)` — so it can only be
  shrunk by a rejection inside one step, never grown across steps. The `r/v` term is what stops a fast
  flyby from proposing a step it will only have to reject.
- **Routing is decided on the requested step, not on the cap** (`h <= FixedStep` goes to the old
  integrator). The first cut tested the cap instead and deadlocked: with a controller that only learns by
  taking a step, a rule of "only long steps go through the propagator" leaves it at 0.02 s for ever. The
  target being state-derived is what makes the simple test work.
- **A long step may not reach the air.** The decision to take one is made at the start of it, so
  `plannedStep` shortens to half the time to the atmosphere boundary whenever the vehicle is descending.
  Without it a ten-minute step ends underground and reports a crash from a place the vehicle never flew
  through. On an airless body the same guard measures to the surface.
- **`PhaseSepWait` and `PhaseIgnitionWait` are not coasting** even with the engine off: both have a timer
  that has to land on its exact instant. `PhaseCoast` is the only phase nothing can turn back into a burn.
- **Gravity losses over a coast step are integrated as a trapezoid.** Over ten minutes the flight path
  angle turns through too much for the left-hand rectangle the fixed path gets away with.
- **`Advance` has a step budget and drops the debt when it runs out.** Some regimes cannot be bought at
  any warp: ×1e6 inside the atmosphere is fifty million fixed steps a second. Past `maxStepsPerAdvance`
  the call gives up, sets `WarpLimited`, and the flight screen says so in the corner — otherwise the
  setting looks broken.
- **A scripted jump wants `FastForward`, not `Advance`.** No cap, no budget, lands exactly on the time
  asked for. `-shot` advances that way; using `Advance` truncated a four-hour jump to 400 s and the
  screenshots quietly came out at the wrong time.
- The warp ladder is geometric to ×1e6 (`warpSteps`), which is eight buttons — as many as the bottom bar
  has room for at 46 px each. `warpLabel` writes ×1000 as "×1k".
- **The mission clock switches format twice**, at an hour and at a day, and rounds *before* choosing the
  format. Picking the format from the raw seconds printed 3599.97 s as "T+60:00.0" — the same carry trap
  the clock already had one level down.

## Frames of reference — easy to get lost in

Integration happens in the **inertial** frame centred on the planet. But the rocket stands on a rotating
pad and carries its 465 m/s away with it, so in the inertial frame it "drifts" 6 km sideways over the
first 15 seconds while climbing 400 m. Physically correct, reads as the rocket being blown off course.

- **Downrange** is measured from the pad in the frame rotating with the planet: the launch site's angle
  is advanced by `ω·t`. Without that the field reported the planet's own rotation as distance flown.
- **The trail and the event markers** are drawn through `FlightScreen.trackPoint` — a sample taken at
  time `t` is shifted into the frame being drawn *as of its own time*, then rotated forward by `ω·(T−t)`. The current point does not move, so the orbit ellipse and the rings,
  which all refer to instant `T`, stay consistent. In orbit the trail lags the ellipse by `ω·T` (2° over
  500 s) — that is the ground track, and it should.
- **`Telemetry.Speed` is inertial; `SurfSpeed`/`VertSpeed`/`HorizSpeed` are relative to the ground.**
  The panel shows both. Mixing them in one column produces 475 m/s next to a vertical 65 and a
  horizontal 6.

## The solar system, and the verdicts

`sim/solar.go` is the real thing: the Sun, eight planets and nine major moons, with real radii, masses,
semi-major axes and eccentricities. The Apollo preset flies in it, launched from the Earth in it.

- **The inclinations are dropped** — everything shares one plane, which is the whole geometry of this
  simulator. Triton goes round the same way as everything else for the same reason: in one plane there is
  nowhere to put a retrograde orbit. Retrograde *rotation* survives, as a negative `RotationPeriod`.
- **The mean anomalies are not an ephemeris.** Nothing here is tied to a date, so they are spread out to
  make a picture worth looking at, not to put Mars where Mars was on some particular morning.
- **Adding the Sun changed nothing about the ascent.** In the Earth's frame the Sun's pull is all but
  cancelled by the Earth falling towards it too — the rail correction — leaving a tide of 1e-7 m/s².
  `earth-falcon` stays a single-body Earth on purpose: it is the LEO reference whose figures are quoted.
- **Verdicts are ranked, and only ever improve** (`outcomeRank`). Reaching orbit and then being captured
  by a moon is a lunar mission, not a demotion. They are a high-water mark, not a running commentary: a
  temporary capture stays on the record after the vehicle has left.
- **`OutcomeCaptured` and `OutcomeImpact` name their body** in `State.OutcomeBody`, because "captured"
  without saying by what is not a verdict.
- **Escape is only asked about the root, and only when the root holds the vehicle.** A craft in low lunar
  orbit is moving faster than Earth escape at that distance and is going nowhere; the Moon is on a rail
  and the vehicle is attached to the Moon. Asking the question in the wrong frame reported every lunar
  orbit as an escape.
- **A flight that orbited and then came down is `OutcomeCrashed`, not `OutcomeSuborbital`** — the latter
  claims it never got there. It happens: the Moon's pull walks the perigee of a high ellipse down.
- **Unless it had been away, in which case it is `OutcomeReturned`.** `Sim.leftHome` is set by `refocus`
  the first time the centre is not the launch body, and it is the only thing that tells a free return from
  a crash. There is no entry model behind the verdict: the vehicle is flown down through the air it was
  launched through and the g-load is on the graph to be read — 14 g down Apollo's corridor, 300 g if it
  arrives nose-first. What it does *not* claim is that anything aboard survived.
- **`refocus` marks the crossings** as `EvSOIEnter`/`EvSOIExit`, with the body in `Event.Body`, and
  `eventLabel` takes the whole event so it can name it.
- **`bodyName` is a lookup, not a switch.** Seventeen bodies is where a switch stops being worth writing;
  a missing entry renders as the identifier, which is the same safety net `T` has.

### Apollo goes to the Moon, twice

There are two of them. `apollo-saturn` flies past the Moon; `apollo-lunar` brakes into orbit around it.
The rocket is identical to the kilogram — what differs is the bookkeeping and the plan.

- **The command and service module becomes the fourth stage** in the lunar-orbit preset, instead of dead
  payload, so the flight plan can brake with the engine Apollo actually braked with: 18.4 t of propellant
  behind one 91 kN engine at 314 s. Payload drops to the lunar module and its adapter. Liftoff mass is
  unchanged, because nothing moved except which column the numbers sit in.
- **Two model additions were needed, and both say something real.** `IgniteOnNode` is a stage the staging
  sequence never lights — and, crucially, never *hands over to*, so `endBurn` leaves the spent S-IVB
  attached instead of separating over the Atlantic and firing the service module into the parking orbit.
  `Node.Separate` drops the stage a burn used once it is over, because 23 tonnes of empty tank is in the
  way of the engine above it.
- **The insertion burn is five and a half minutes long**, so it is nothing like the impulse a textbook
  hands you: it has to start 200 s *before* closest approach and be sized against the integrated result,
  not against `v_peri − v_circ`. Found by search, like everything else here: T+286000 s and 725 m/s give
  1782 × 1921 km at e = 0.019, with the service module still half full.
- **This one is forgiving where the translunar injection is sharp.** ±25 m/s on the insertion moves the
  periapsis by a couple of hundred kilometres; ±2 m/s on the injection moves the approach by two thousand.

### Mars, and the verdict that thought the Sun had captured it

`apollo-mars` is the longest mission here and the one that found the last verdict bug.

- **A heliocentric orbit is not a capture.** `checkEnd`'s capture branch fires on "the centre is not the
  launch body", and the moment the vehicle leaves the Earth its centre is the Sun — bound, and clear of the
  Sun's surface. So it settled as `OutcomeCaptured` by the Sun, which outranks `OutcomeOrbit` and is true
  of every rock in the system. There is now a case ahead of it for `Center == 0` that settles nothing.
- **The injection only works at one point in the parking orbit.** "Prograde" is prograde in the *Earth's*
  frame, and the escape asymptote has to come out along the Earth's own motion round the Sun. The same
  3690 m/s buys a heliocentric aphelion of 1.62 AU at T+4500 s and 1.07 AU at T+3000 s — the second is
  not a transfer at all, it is a slightly larger orbit than the Earth's. Sweep the node time over one
  parking-orbit period and take the peak.
- **The Apollo stack cannot reach Mars, and that is a payload problem.** 3668 m/s of S-IVB throw gives an
  aphelion of 1.577 AU; Mars's ellipse is at 1.604 AU where the transfer's aphelion points, and no phasing
  fixes a shortfall in *shape*. Dropping the lunar module takes the throw to 4891 m/s.
- **A payload change means the ascent has to be found again.** The same pitch programme on a stack 15 t
  lighter puts the vehicle in 1473 x 205 km, because the S-II alone now overshoots. Sweeping the tail
  pitch, the profile exponent, the turn length and the third stage's cutoff together: (12, 430, 3.5, 8)
  with a 30 s cutoff gives 204 x 187 km at e = 0.0013, rounder than Apollo's own.
- **The window is Mars's `MeanAnom0`, and it was solved rather than searched.** The transfer crosses Mars's
  *ellipse* at a point that does not depend on Mars's phase, so: fly the transfer, take the crossing time
  and heliocentric longitude, invert Kepler for the mean anomaly Mars needs at t = 0. One equation, one
  unknown, and 5.9975 came out of it. It changes nothing about `mars-ascent`, which is bit-for-bit
  identical with Mars somewhere else entirely — the rail correction cancels the Sun at Mars's centre and
  what is left over a six-hour flight is below the printing precision.
- **Five metres a second either side of the injection is a crater.** 3700 m/s hits Mars; 3690 passes at
  95209 km. The gradient through the encounter is about 7000 km per m/s, so the preset ships the value that
  fails *gracefully* — a bad periapsis is a worse orbit, an impact is the end of the mission.

### The Mun, and aiming at something small

`kerbin-mun` is the cheapest mission here to fly and the fiddliest to aim.

- **A dead-centre intercept is a collision.** The window was solved the way Mars's was — fly the transfer,
  take where and when it crosses the Mun's orbit, put the Mun there — and the result was a crater every
  time. With 364 m/s of hyperbolic excess against a body of μ = 6.5e10 the grazing impact parameter is
  486 km, so the *whole* of 851–861 m/s hits: focusing swallows anything closer. Another 7 m/s buys the
  miss, and past that the periapsis climbs about 20 km per m/s.
- **The verdict on the way through was `OutcomeReturned`**, before the braking burn was added — swing past
  the Mun, come back to Kerbin, re-enter. The free-return verdict turned up in a preset that was not trying
  to be one, which is the sort of thing that suggests it was the right shape.
- **`kerbin` stays a system of one body.** Its numbers are quoted in the README, and a Mun with a gravity of
  its own would move them. Same reasoning as `earth-falcon` against the solar system.

### The free return, which is one number aimed four days ahead

`apollo-return` is the same Saturn V with one node on the plan and nothing after it. Two things have to be
true at once — pass the Moon, come back into the atmosphere — and there are exactly two knobs, so it was
searched over both. The landscape is not gentle:

| | |
|---|---|
| the answer | T+15295 s, 3192 m/s → 3226 km past the Moon, entry at 7.4°, **14 g**, home at T+8.27 d |
| −1 m/s | 2757 km, entry at 17.0°, **100 g** |
| −5 m/s | 865 km, entry at 70.6°, **348 g** |
| −7 m/s | hits the Moon |
| +1 m/s | 3689 km, but it grazes the air and comes round again: **19 days** |

- **The osculating perigee at the sphere-of-influence exit is not the entry.** The first cut was tuned on
  it — aim the post-flyby perigee at 60 km and be done — and it produced a 34 km perigee that arrived at
  74° below the horizontal. Three and a half days of lunar perturbation happen after that reading. Tune on
  the flight path angle at the atmosphere's top, measured by flying the whole thing.
- **Entry angle and periselene are coupled**, and the shallow corner of the family passes the Moon high:
  7.4° comes with 3226 km, and the candidates that pass at a few hundred kilometres all arrive at seventy
  degrees. A closer flyby is available at another injection time — 721 km and 4.6° — but it grazes the air
  and comes round for another nine days.
- **The flight is eight and a quarter days**, so `MaxTime` is nine. Every other preset gets its verdict in
  the first ten minutes and the limit never applies; this one ends on it if the tuning ever drifts.

### Proton-K, and why not Soyuz

`proton-zvezda` is the July 2000 launch of Zvezda. Proton-K is in here rather than an R-7 because it is
**serial** — three stages in a line — and `Rocket.Stages` is a serial list. Vostok and Soyuz strap four
boosters around a core and burn them together; a serial list can only lie about that, and the lie would be
in the first thirty seconds of every flight.

- **The launcher is not supposed to reach the station's altitude.** Nineteen tonnes to a couple of hundred
  kilometres is what Proton-K sells; 400 km with the same payload is beyond it, and the tuner said so —
  the best it found was 461 x 308 km. That is also what really happened: the module went up into an ellipse
  and climbed to the station over the following days on its own engines.
- **The circularisation is done by the third stage's leftovers**, not by the module. The cutoff at 225 s
  leaves 3.3 t in the tank, and 43 m/s of it at the first apoapsis gives 513 x 408 km with the low side
  exactly at the station's altitude. `Node.Separate` then drops the stage, and the module is left flying
  alone with all 860 kg of its own propellant — which is what the station-keeping this simulator does not
  model would have wanted.
- **Baikonur is at 51.6 degrees and this simulator has one plane.** The pad is handed all 465 m/s of the
  equator's rotation instead of the 325 the real site gets, so the ascent is that much cheaper than it
  should be. Every preset here tells the same lie; this is the one where it is largest.
- **`protonK` carries no third-stage cutoff.** Where the third stage stops is the mission's business, and
  the first cut left 250 s in the shared vehicle — a number the stage never reaches, so it burned dry and
  the insertion came out 2235 x 325 km instead of 461 x 308. `protonInsertion(cutoff)` is the fix.

### Titan, which was not the cheap one

I called this preset cheap and it was the hardest of the lot. The numbers are why:

| | |
|---|---|
| surface density | **5.14 kg/m³**, four times Earth's |
| speed of sound | 199 m/s, so Mach 1 arrives at walking-pace-times-400 |
| terminal velocity at the surface, 22 kN | **173 m/s** |
| drag losses, first attempts | **1900–3800 m/s** |

- **A rocket cannot go fast in air that thick, so it cannot turn low.** The first cuts turned at 150–300 s
  and spent the entire propellant load on drag: 3849 m/s of it, then a crater.
- **A single continuous burn cannot close the orbit however it is tuned.** It always ends while still
  climbing, and the low side stays at 50 km — 4943 x 48, 14571 x 26, 1221 x 13. The answer is the one
  Kerbin already uses: a kick stage on `IgniteAtApoapsis`.
- **Titan's atmosphere is 435 km deep at Earth's own vacuum threshold** (1e-9 of surface density), so `Top`
  is 500 km, and `settle` wants a periapsis above `Top`. That puts the target orbit at 600 km, which is a
  much harder ask than the 185 km the Earth presets aim at. Every attempt that looked like an orbit was
  coming out as `OutcomeDecaying` and the tuner was discarding it unseen.
- **The vehicle had to be sized for the planet, not tuned into it.** Twice the first guess: 7.9 t, two
  stages, 22 kN. 648 x 578 km came out of the first sweep with it.

### Proton to the belt

`proton-geo` is the same launcher with a Blok DM on top, and it is the preset that needed the most from the
node machinery: three burns, a stage jettisoned mid-plan and five and a half hours of coasting between the
second and the third.

- **A node cannot light an empty stage, and used to be silently marked spent for trying.** The third stage
  empties itself at the first periapsis and the plan then needs the engine above it, so `checkNodes` drops
  any spent stage it is sitting on before looking for propellant. Carrying a dead stage into a burn is not
  something a flight does; blocking the whole mission over it is not something this should do.
- **The measure of geostationary is the period, not the altitude.** The tuner scored candidates on
  |period − sidereal day| rather than on eccentricity, which moved the last burn by 2 m/s and the answer
  from 0.11% off to 0.09%. `TestProtonGeoPresetReachesTheBelt` asserts half a per cent.
- **"Burn what is left" is written as the number, not as a huge one.** The first node asks for 422 m/s
  because that is exactly what 3.3 t buys at that mass, so the tank runs dry as the burn ends either way.
  The tuner used 9999 to find it; a preset should not ship with a magic number standing in for a
  measurement.
- **The audit's target check does not apply to a preset with a plan.** Its verdict comes in a parking orbit
  and the mission goes on from there, so all the ascent owes is an orbit that clears the air.

### The flyby preset

The preset carries one node: a prograde translunar injection at T+15325 s of 3162 m/s, out of the parking
orbit, on what the S-IVB kept back. It enters the Moon's sphere of influence at **T+2.63 days**, passes
**1789 km** over the surface and leaves again at T+4.0 days.

- **The time and the delta-v were found by search**, the same way the pitch programmes were. The Moon has
  to be somewhere specific when the vehicle arrives, and a window is not a thing to guess at.
- **A translunar injection is sharp: two metres a second moves the closest approach by two thousand
  kilometres.** That is why the preset aims 1800 km clear of the surface rather than at the 200 km that
  scored best — a preset that turns into a crater when the integrator changes in the tenth digit is not a
  preset. `TestApolloPresetReachesTheMoon` asserts a wide band for the same reason.
- **It is a flyby because the S-IVB cannot do better.** Capturing from that approach needs some 670 m/s and
  it has 540 left — which is the historical reason Apollo carried a service module with its own engine, and
  exactly what `apollo-lunar` models by making that module a stage. Same rocket, same approach, different
  bookkeeping.
- **The osculating lunar orbit at the crossing can read as bound** while the integrated path is a flyby:
  at entry the two-body hyperbola said 59 km *below* the surface and the real trajectory passed 1789 km
  above it. Over a 60,000 km approach with the Earth still pulling, that is the difference between a
  conic and a trajectory.

## Manoeuvre nodes and the prediction

The pitch programme is a schedule of angles against the clock, and it is the right tool for an ascent —
but it cannot say "two days from now, add three metres a second along the velocity", which is the whole
of flying anywhere beyond the launch body. A `sim.Node` is that sentence: a time, a direction to hold and
a delta-v to spend. `sim/node.go`, edited on the flight screen, drawn as the path it produces.

- **The plan lives in the running simulation** (`Sim.Cfg.Nodes`), not in the app's config, because it is
  edited during flight. `Reset` keeps it; launching afresh from the setup screen starts from
  `Config.Nodes`, which is where a preset can put one.
- **Executed nodes are a bitmask in the state** (`NodesDone`), not a cursor. A cursor goes stale the
  moment a time is edited mid-flight; a bitmask does not care what order the plan is in. Deleting a node
  shifts the mask with it — a subtlety that has to be got right or the wrong burn shows as spent.
- **`State.Node` is -1 when nothing is burning.** Its zero value would mean "node zero is running",
  which is a lively way to start a flight.
- **The cutoff is solved, not watched for.** `nodeBurnLeft` inverts the rocket equation for the instant
  the requested delta-v lands, and `Step` shortens to it. Watching the total go past would overshoot by a
  whole step's worth of thrust, which on a three metre a second correction is a 6% error.
- **A node burn ignores the stage's own `CutoffTime`** and does not stage. The stage timer belongs to the
  ascent and has already had its turn; the node's delta-v is its cutoff.
- **A node with an empty tank is marked spent, not left pending.** Otherwise it is retried on every step
  for the rest of the flight.
- **`plannedStep` lands exactly on the next node.** Otherwise a ten-minute coast step notices the
  ignition ten minutes late, which for a correction burn is the difference between a transfer and a miss.
  For the same reason `coastStep` checks whether the phase machine lit something and hands the step back
  to the fixed integrator: the adaptive propagator knows nothing about thrust.
- **Everything that advances time uses `plannedStepUncapped`**: `RunToEnd`, `FastForward` and `Predict`.
  Each of them used to roll its own — "the fixed step, or the coast target if coasting" — and each of them
  therefore left out both guards that live in the planner: the clamp onto the next scheduled burn and the
  one that stops a descending step from reaching the air. A ten-minute coast step sailed up to ten minutes
  past a lunar insertion, which is the difference between an orbit and a crater. The screenshots caught it;
  no test did, which is why `TestFastForwardLandsOnScheduledBurns` exists now.
- **`Predict` is the flight, not a sketch of it** — the same integrator over the same plan, on a copy that
  shares nothing writable. Burns run at the same fixed 0.02 s, and points are **sampled** out of the run:
  the first cut recorded one per step and spent all four hundred points on the first eight seconds of a
  translunar burn. It also has to set its own `WarpRate`; inheriting the live one made it take
  minute-long *fixed* steps in low orbit, which is not a trajectory at all.
- **The predicted path is drawn in the non-rotating frame**, unlike the trail. A future path turned
  backwards by the ground's rotation is nonsense, which is also why it only appears once the vehicle is
  out of the air.
- **The prediction is cached for half a second.** A long plan is tens of thousands of steps, and
  `maxPredSteps` caps what one recompute may cost.

## Presets

The pitch programmes and the final cutoffs were found by search (a profile generator,
`pitch = (90+t)·(1-f)^p − t`, plus a bisection on the last stage's cutoff for a periapsis at the target).
The tuner lived in `sim/zz_tune_test.go` and has been deleted — if the presets ever drift, writing it
again beats tuning by hand. Note the tail term: the Apollo profile only works with a **positive**
asymptote, so a generator that can only decay to zero (as the first one did) cannot express it.

Earth/Falcon-9: a 304/239 km orbit, Δv 8995 m/s, max q 43 kPa at 11 km, peak 5.9 g. The Δv is below the
real-world 9.3–9.5 km/s because launching from the equator hands over all 465 m/s of rotation.

Apollo/Saturn V is three stages to a 192/186 km parking orbit, Δv 8965 m/s, insertion at T+604 s. It is
the only preset that can be checked against a flight that happened, so the numbers are worth keeping
close: real Apollo 11 staged the S-IC at T+161 s (here T+159) and inserted at T+699 s into 186 × 183 km.
Drag losses come out at 45 m/s against the real 40-ish; the constant-throttle model gives the S-II a
354 s burn where the real one ran 384 s on a shifting mixture ratio, which is why insertion is a minute
and a half early. `TestApolloPresetMatchesTheRealAscent` pins all of it.

- **It ends at the parking orbit on purpose.** One central body, no Moon to aim at, so translunar
  injection is not something the simulation can represent — the S-IVB simply keeps four fifths of its
  propellant, which is the TLI burn sitting in the tank. Do not "fix" this by burning it: the result
  would be a 380,000 km ellipse round the Earth, which is not the mission.
- **The pitch programme levels off at 9° and holds it.** Both extremes fail, and they fail differently:
  hold more and the ascent lofts to 350 km, where the third stage circularises at the wrong altitude
  (392 × 185 km); let it fall to zero and the vehicle cannot hold 185 km at 4 km/s, sinks back into the
  air and spends **9 km/s** of the budget on drag. The tuner found the tail angle, not intuition.

Kerbin needed its second stage set to ignite **at apoapsis**: a 600 km planet wearing an Earth-thick
70 km atmosphere does not yield to direct ascent on a single burn.

## Stage count

The vehicle takes **one to four stages** — `minStages`/`maxStages` in `screen_setup.go`. The staging
machine in `sim` never cared how many there were; the editor did, and the bounds are the editor's.

- **Four is where real launchers stop.** Past that, each stage buys less than the one before and pays
  for it with another full set of engines, tanks and an interstage. One stage is a sounding rocket and a
  perfectly reasonable thing to fly.
- **`addStage` derives the new stage from the one below it** — a quarter of its mass and thrust, its
  vacuum Isp for both Isp figures (an upper stage never sees sea level) — so the button hands you
  something that flies to edit, not a column of zeroes. It also gives the stage below a separation delay
  if it had none. The shipped pitch programmes are tuned for two stages, though: a taller stack wants
  its own programme, and will fly suborbital until it gets one.
- **`removeStage` resets the ignition mode of whatever ends up at the bottom.** The first stage lights on
  the pad, the editor only shows the mode for a stage that has something below it, and a hidden value
  that reappears the moment another stage is added is worse than no value at all.
- **The footer shares its width out instead of fixing it.** One Δv column per stage on top of the five
  fixed ones is nine columns; at the old 190 px each they ran under the launch button.
- The `9-*` steps of `-shot` capture a four-stage vehicle. They come last in the script because they
  edit the configuration the flight captures fly.

## Interface language

All interface text lives in `assets/locale/ru.json` and `assets/locale/en.json`, embedded into the
binary with `go:embed`, and is fetched by key: `T("flight.downrange")`. A picker sits in the setup
header and in the bottom bars of the flight and graph screens.

- **The `sim` package holds no text at all.** Events carry only a `Kind`, verdicts only an `Outcome`,
  presets an identifier (`earth-falcon`), bodies likewise (`earth`, `moon` — `bodyName` translates them). Labels come from `lang.go`. The physics must not know about
  language.
- **A missing key renders as the key itself, not as nothing**, so a typo shows up the moment the screen
  is opened. The same goes for a format string that lost a verb: `Sprintf` does not panic, it writes
  `%!(EXTRA string=11.0)` into the text. Both mistakes announce themselves on screen, which is why
  `lang_test.go` no longer tries to catch them ahead of time — that took a source scanner that could
  not tell code from comments, and a second pass over every format string, to buy very little.
- **`lang_test.go` checks the locale files against each other and nothing else**: the same key set in
  every language, no blank values, and **the same substitutions per key** — a format string that loses a
  verb in translation prints `%!(EXTRA string=11.0)` into the interface, and only in the language nobody
  was testing in. Orphaned keys are left to be noticed in passing.
- **Anything that reaches the screen goes through `T`, including the ones that look like symbols.**
  "MAX Q" sat in `eventLabel` as a literal and "max q" was a hardcoded row label in two panels; they read
  as notation rather than as prose, which is exactly how they stayed untranslated. Keyboard names
  (`SPACE`, `TAB`, `ESC`) and `Isp` are the real exceptions.
- **Text assembled from fragments must be a whole format string.** Word order differs between
  languages, so the max-q readout is `"%s на %s км"` / `"%s at %s km"` rather than a label glued to a
  unit. Same for anything numbered: `"СТУПЕНЬ %d"` / `"STAGE %d"`.
- **Cache nothing at screen construction.** The plot captions used to be computed in `NewGraphScreen`,
  and switching language did not relabel them. `plotSeries()` is now called every frame.
- The picker lists each language written in its own script (`English`, `Русский`), which is the one
  piece of display text deliberately kept out of the locale files: a picker that translated its own
  options would be useless to whoever cannot read the language currently selected. `langOrder` decides
  the order and starts with the default.
- CLI flags and log messages stay in English: that is a machine-facing surface, not the interface.
- The program starts in English (`defaultLang`), and that is also where a key missing from the selected
  language falls back to. `-lang ru` starts in Russian.
- A third language is one more file plus entries in `localeFile` and `localeCode`; the tests will list
  every key it is missing.

## Looking at it — frames, focus and scale

The picture is built around one body: `FlightScreen.frameBody()`, which follows the vehicle's own sphere
of influence by default and can be pinned to any body with Tab. Everything drawn goes through
`framePoint(p, from, t)`, which shifts a position measured from one body into the frame of another **at
the time it was measured**.

- **A moon has to be drawn in its own frame or it cannot be looked at.** In the launch body's frame the
  Moon crosses the screen at a kilometre a second; centring on it makes the approach readable, and it is
  the same reason the trail's shape depends on the frame — a transfer looks different around the Earth
  and around the Moon, and both are true.
- **The shift is taken at the sample's own time**, not the current one. A track relative to a moving body
  is a sequence of "where was it relative to the body *then*", which is what "in the Moon's frame" means.
- **The camera is three separate decisions — scale, centre, rotation — and each of them becomes the
  user's the moment the user touches it.** `frame` is whose coordinates the world is drawn in; `follow` is
  what sits in the middle of the screen: the vehicle, a body, or `camFree` for a point. Splitting the two
  is what lets the view be pushed around while a moon still holds still in it.
- **Dragging pans, the wheel zooms, `C` gives it all back to the automatic framing.** A drag takes over
  from wherever the camera happened to be (`freePos = cam.Center`), so the picture does not jump on the
  first pixel. Any gesture sets `manualScale`, which stops the easing towards the automatic span —
  otherwise the automatic zoom keeps pulling the rug while the user is looking at something.
- **`Camera.Unproject` is what makes both gestures possible.** Panning and zoom-to-cursor are the same
  statement — a world point has to stay under the pointer — so they are only as good as `Project` and
  `Unproject` being inverses, which `TestProjectAndUnprojectAreInverses` pins.
- **The wheel zooms about the cursor when free and about the centre when following.** Zooming about the
  cursor while locked to a body would walk the body out of the middle, which is the one thing a lock is
  for.
- **Camera gestures are read at the *end* of the frame**, in `handleCamera`, after every widget has had
  its chance at the click. The flight plan panel sits inside the trajectory view, and a press on it must
  not also grab the world; `u.consumed` is the only thing that knows.
- **The picker shows the free state as its own entry.** Claiming to follow the vehicle while the camera
  sits half way to the Moon is a lie about the one thing that control exists to report.
- **The camera lets go of the local vertical as it pulls back**, on the same ramp that slides the centre
  from the vehicle to the planet's middle — standing on a planet becomes looking at one over the same
  stretch. Held all the way out the picture would spin with the orbit, a full turn every five seconds at
  ×1000, and in a body's frame or a dragged view there is no "up" to speak of. It is written as
  `Rot += hold·angleDelta(Rot, want)`, so with `hold = 1-u` and `u = 0` it is still exactly "point the
  vertical at the top of the screen". Multiplying an accumulated angle by `(1-u)` does not work: with a
  few hundred radians on the clock, a 0.01 change in `u` is a full revolution of jolt.
- **The zoom range is ten orders of magnitude**, from a 1.5 km view of the pad to the whole system. The
  scale is absolute once touched; `clamp(1e-12, 1e4)` is only there to keep a runaway wheel from
  overflowing the rasteriser.
- **Every body draws at the detail its pixel radius earns**: under 1.5 px a labelled dot, up to
  `maxRingPx` a disc, beyond that the flat-band mode. A moon you cannot see is a moon you cannot aim at,
  so the dot has a floor of 2 px and a name under it — under, not beside, because beside is where the
  launch pad puts its own label.
- **Only the launch body has air to draw**, which is the same limitation the physics has.
- **`bodyColors` in `theme.go` is the one place that knows Mars is red.** The physics carries identifiers
  and no colours, the same way it carries no text. An identifier with no entry — anything added in the
  editor — comes out grey.
- **All of them are dim on purpose.** The trajectory, the prediction and the markers are what is being read
  on that screen; a planet is scenery, and scenery that competes with the instruments is drawn too
  brightly. Earth keeps the green it has always had, because in the close-up view it is the ground under
  the launch pad.
- **The rim and the dot are derived with `lighten`, not listed.** Eighteen bodies is where three
  hand-picked shades each stops being worth maintaining, and the fixed ratios are what make them read as
  the same body at different sizes.
- **Saturn has rings, and they are decoration**: `bodyRings` in `theme.go`, no mass, no shadow, no
  shepherding moons, and nothing in `sim` knows they exist. The radii are the real ones — C, B and A, with
  the Cassini division as the gap between the last two — and they are drawn face-on as concentric bands,
  which is what a plane seen from above gives. The same convention that makes every orbit here a circle
  rather than an ellipse foreshortened by a viewing angle there is no room for.
- **The name goes past the rings, not past the planet.** At the planet's own edge it lands in the middle of
  them; `ringExtent` is what the label offset asks.
- **A dot is sized by the logarithm of the real radius.** Phobos to the Sun is five orders of magnitude; on
  a linear scale everything but the Sun is one pixel.
- **Names are drawn in a pass of their own, skipping anything already taken.** At system scale a moon and
  its planet are the same pixel, and "Phobos" printed over "Deimos" is worse than one of them going
  unnamed. Index order sets the priority: the Sun and the planets are declared before the moons.
- **Rails are drawn before bodies**, so a body sits on top of its own orbit, and only when the orbit is
  between 24 px and `maxRingPx` across — smaller is unreadable, larger is a straight line.
- **Event labels stop stacking after `maxStackedLabels`.** Zoomed out to the Moon's orbit every event of
  the flight lands on the same pixel, and the old stepping turned into a wall of text down the screen.
- **A body focus centres on the body, not on the vehicle**, and frames it by its sphere of influence
  rather than its radius: the sphere is what an approach is aiming at.

## Rendering — traps

- **The camera is rotated:** `Camera.Rot` is the angle of the local vertical, so "up" on screen points
  away from the planet — but only while zoomed in; see the frames section below for how it lets go. The launch site sits on the +X axis, and without the rotation the ascent would
  look like sideways flight. Push directions (thrust vector, surface tangent) through `Camera.Dir`, not
  through `(v.X, -v.Y)`.
- **Two world modes.** While the planet's radius in pixels is under `maxRingPx`, the ground and the
  atmosphere are concentric rings. Beyond that they become horizontal bands: the rasteriser will not
  survive a circle a million pixels across.
- **The camera centre is not smoothed at all, only the zoom.** Smoothing a position has to happen in
  some frame of reference, and the inertial one is flatly wrong here: the vehicle sweeps through it at
  465 m/s, the camera ends up permanently a hundred metres behind, and the uneven integrator step
  (0.02 s does not divide the 1/60 frame) turns that lag into ±5 px of shake — 150 direction reversals
  over 700 frames. The centre is now derived from the vehicle's current position every frame: zero
  reversals. Checked with `-camtrace N`, which prints the vehicle and pad screen coordinates per frame.
- **Framing is derived from the eased zoom, not the raw `span`.** Otherwise a step in the span — the
  orbit closing, say — jolts the composition while the zoom is still gliding.
- **The camera focus cannot be lerped towards the planet's centre linearly.** The target is thousands of
  kilometres away, so even a factor of 1e-4 shifts the picture by hundreds of metres and throws the
  launch pad out of a 1.5 km wide view. The blend stays at zero until the span reaches half a planet
  radius.
- **The launch pad is drawn in metres** and shrinks on its own as the vehicle climbs; below 7 pixels it
  collapses into a label. That label's anchor is fed into the event-marker spacing, otherwise at
  orbital scale it lands on top of the cutoff marker.
- **The mission clock rounds before it splits.** Taking the minutes off first turned 11999.98 s into
  "T+199:60.0". Only visible once flights could run for hours.
- **The trail is trimmed to `trailWindow` seconds.** With the flight no longer ending at orbit, an
  unbounded trail wraps the planet over and over until the picture is one smear. The window is longer
  than any ascent in the presets, so nothing is lost on the way up.
- **The number of atmosphere bands follows the scale.** From orbit the whole atmosphere is a few pixels
  deep, and sixteen sub-pixel rings simply vanish.
- **The toolkit identifies a widget by the address of the value it edits.** Do not bind `NumField` to a
  local variable — the address changes every frame and focus is lost. That is why the diameter field is
  bound straight to `Body.Radius` with `Scale: 500`.
  **The flip side: anything that reshuffles a slice the fields are bound into must call `UI.cancel()`
  first.** Removing a stage shifts the ones above it down inside the same backing array, so a focused
  field would quietly commit its edit to a different stage. Same reason the preset buttons and the
  mixture picker cancel before replacing their slices.
- **Overlays paint onto `UI.Overlay`, not onto the `dst` they were handed.** The setup columns draw into
  a clipped sub-image, so a tooltip drawn into `dst` would be sliced off at the column edge.
- **An unlabelled `NumField` has no label column.** It used to reserve the strip either way, which left
  a 104 px cell with 39 px of box and the leading digit of its own value cut off. The layer and keyframe
  editors got wider boxes out of the fix.
- **A parameter gets an explanation by setting `NumOpt.Info` to a locale key.** The mark sits in a fixed
  column just before the input box rather than trailing the label: label widths differ per field and
  per language, and marks at ragged positions read as clutter once many rows carry one.
- **An open dropdown needs two things that immediate mode does not give for free.** Its list is drawn
  through `UI.deferred`, flushed in `EndFrame`, so it lands on top of widgets that were drawn later.
  And `UI.fenced` blocks hover and clicks over the list rect for every other widget, so whatever sits
  underneath cannot steal the click — the flight and graph pickers open upwards over panels that were
  already drawn by then.
- **The whole interface is built in `Update`, not in `Draw`.** The toolkit is immediate mode and reads
  just-pressed input; Ebiten calls `Draw` less often than `Update`, and clicks would be dropped. `Draw`
  only blits the canvas.

## The graph screen's time axis

A flight to the Moon is four days long and its ascent is the first ten minutes. On one axis for the whole
thing the interesting part is two pixels wide, so the axis moves: `GraphScreen.t0`/`t1` are the visible
slice, dragged with the mouse, zoomed with the wheel about the instant under the cursor, and reset by two
buttons — the whole flight, or the ascent up to the verdict.

- **The vertical scales follow the visible range, not the whole flight.** Zoomed into the ascent of a
  lunar mission, an altitude axis sized by four days of orbit draws the entire launch along the bottom
  edge. `visibleRange` reaches one sample past each edge so a trace crosses the plot instead of starting
  inside it.
- **Traces are decimated by pixel column.** Four days at one sample every five seconds is seventy
  thousand points, fifty to a pixel; each column is drawn as the range its samples covered, so a max-q
  spike between two pixels is still a spike and not an average of one.
- **Zooming holds the anchor still** — the instant under the cursor stays under the cursor, which is what
  makes a wheel over a plot feel like a wheel over a map. `clampAxis` keeps the range inside the flight
  and refuses to invert it or to go below a second, which is finer than the history is recorded.
- **`axisTime` labels at the scale the span deserves**: seconds, then mm:ss, then hours, then days. Seven
  labels of "227318s" tell you nothing.
- **An event whose label has nowhere to go gets a tick and no label.** Zoomed out to four days, the whole
  ascent lands inside two pixels and eight labels used to print on top of each other. The first cut fell
  back to "the row that clears earliest", which is how the pile-up happened.

## Editing the system

The setup screen's first column edits **one body of the tree**, chosen by a picker at the top with the
launch body marked `(pad)`. Radius, mass, rotation and — for anything that is not the root — the orbital
elements, with the period and the sphere of influence read back as derived figures. `+ moon` gives the
body on screen a satellite; `× body` deletes one.

- **`Config.EnsureSystem` is called at the top of every frame**, so a single-planet configuration is a
  system of one and the editor never has to care which kind it is looking at. It is idempotent, and it is
  called a second time before the footer so the derived numbers include the frame's own edits instead of
  lagging them.
- **The parent dropdown offers only bodies defined earlier**, which is the tree's invariant stated as a
  widget. Moving a body under a *later* parent would mean reordering the slice; nothing needs that, and
  the restriction makes a cycle impossible to express rather than merely unlikely.
- **`System.Remove` takes the whole subtree** and returns an old-to-new remap, because everything pointing
  into the numbering — the launch body, the selection, a state's centre — has to be repaired. Deleting
  Mars deletes Phobos and Deimos; leaving orphans pointing at a slot that now holds Jupiter would be worse
  than any error message. The root cannot be removed: it is the frame everything else is measured in.
- **`System.AddChild` appends.** A body at the end of the slice is after every possible parent, so the
  invariant holds without anyone having to think about indices.
- **Switching the selected body cancels the pending edit.** Every field in the column is bound to an
  address inside the body being left.
- **Unticking "launch from this body" does nothing.** The pad has to be somewhere; the way to move it is
  to tick the box on another body.
- **The atmosphere column is still the launch body's air, whatever body that is.** Move the pad to the
  Moon and Earth's atmosphere goes with it. Per-body air is the next thing this editor wants.

## Stale indices, which is how this thing crashes

Everything here is indexed: bodies into a system, nodes into a plan, a selection
into either. Every crash found so far has been the same shape — an index outliving
the slice it pointed into — so `TestPresetsAreValid`, `TestRemoveRunningNode`,
`TestRemoveEarlierNode`, `TestLoadPresetClearsTheSelection` and
`TestEditingKeepsTheTreeWalkable` exist to keep them found.

- **Loading a preset has to reset the body selection** (`SetupScreen.loadPreset`).
  The clamp at the top of `Update` does not help: the preset buttons are handled in
  `drawHeader`, which runs *before* the columns, so a jump from the solar system to
  a single planet left the editor holding index nine of a slice of one on the same
  frame. That was a hard crash.
- **Deleting a node is `Sim.RemoveNode`, not a slice operation.** The running
  index, the spent-bitmask and an engine that is on for a burn that no longer
  exists all have to be repaired; deleting the burn in progress used to index a
  plan that had just lost that entry. The panel's job is `u.cancel()` and nothing
  else.
- **The launch body cannot be deleted.** The remap would put the pad on whatever
  the renumbering happens to leave at index zero, which in the solar system is the
  Sun.
- **The preset picker is a dropdown and `-preset` takes a name.** Six entries already needed 900 pixels of
  header as buttons, and an index into a list that grows is a thing nobody can remember and every new
  preset silently redefines.
- **A preset's nodes have to fire inside its own time limit.** Apollo's translunar
  burn is at T+15325 s and the preset used to say sixty minutes; it only worked
  because the limit stops applying once there is a verdict. Relying on that is
  relying on an accident, so the limit is now six days — the length of the mission
  it ships with.

## What it costs to run

Measured, because all four of these were guesses that turned out wrong in one
direction or another. A hitch appeared once the vehicle left the atmosphere: the
numbers said the prediction was taking **658 ms** and running twice a second.

| | before | after |
|---|---|---|
| one step, solar system | 17.2 µs | **3.1 µs** |
| one step, single body | 1.08 µs | 0.88 µs |
| one prediction from the parking orbit | 658 ms | **9.5 ms** |

- **A prediction only runs while coasting.** During an ascent the pitch programme
  is flying and a preview of it says nothing — and it is the expensive case,
  because a burn is integrated at the fixed step. The old altitude test let
  predictions through the moment the vehicle passed the top of the atmosphere,
  which is exactly where the stutter appeared.
- **A predicted burn steps at one second, not 0.02.** A translunar injection is
  five hundred seconds of smooth vacuum thrust; at the fixed step that is
  twenty-five thousand steps of work several times a second, and the drawn path
  moves by centimetres for it.
- **`Predict` must not force the step up to the sampling interval.** "No point
  integrating finer than the drawing" cost four times what it saved: the error
  control rejected the oversized step and halved it two or three times, at six
  gravity evaluations a try. Recording is decoupled from stepping already.
- **The ephemeris is cached on four instants** (`Sim.ephemeris`). A Runge-Kutta
  step asks for gravity at three distinct times and evaluates four stages, and
  every answer costs a Kepler solve per body. The cache is keyed on time alone, so
  it has to be dropped when the frame changes: which bodies are in it depends on
  the centre.
- **`System.Contributes` drops the bodies off the frame's own chain, and that is a
  correction as much as a saving.** The rails give a body the two-body motion of
  its chain and nothing else, so a distant planet pulls the vehicle without
  pulling the centre it is measured from. Jupiter's true differential effect on a
  vehicle near the Earth is about 1e-11 m/s²; its uncompensated pull in this model
  was 3e-7, which is twenty kilometres of error over four days bought by including
  it. In the root's own frame nothing is dropped — the root does not move, so every
  pull is honest. `TestDistantPlanetsAreLeftOut` pins both halves.
- The pruning moved the Apollo ascent by **three centimetres** and its periselene
  from 1789 to 1791 km. The other four presets are bit-for-bit unchanged, as they
  have been through every phase of this.
