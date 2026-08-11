# fsim

A launch simulator that grew a solar system. Set up a planet — or any body of a system of eighteen — its
atmosphere, a vehicle of one to four stages, a pitch programme and a plan of burns → "Launch" → live
ascent with telemetry → the camera pulls back from the launch pad to the Moon's orbit → graphs.
Go + Ebiten, and the physics is real.

The Apollo preset flies the whole thing: Saturn V off the pad, a 192 × 186 km parking orbit at T+604 s,
translunar injection at T+15325 s, and the Moon's sphere of influence two and a half days later.

## Build & run

```
go run .                       # start on the mission list
go run . -preset apollo-lunar  # skip the list: straight to the editor on that preset, by name
go run . -preset titan-ascent -fly   # skip the editor too, and launch
go run . -shot ./shots         # run the capture script and save a PNG of every screen
go run . -camtrace 700         # print the vehicle's screen coordinates per frame (catches camera shake)
go run . -lang ru              # start with the interface in Russian (default is English)
go test ./...                  # physics and interface
go build ./... && go vet ./...
web/build.sh                   # the same thing for the browser, into web/
```

The browser build is the same program: `GOOS=js GOARCH=wasm`, Ebiten's own js backend, and
`App.Layout` already takes whatever size it is given, so the canvas is the page. `web/build.sh`
writes `web/fsim.wasm` (17 MB, 4.2 over gzip) and copies Go's `wasm_exec.js` out of GOROOT so the
loader always matches the toolchain; both are generated and both are gitignored. It has to be served
over HTTP — a `file://` page cannot fetch the wasm:

```
web/build.sh && (cd web && python3 -m http.server 8080)
```

- **What costs is pixels, not physics.** Measured in headless Chrome, which renders in software with
  SwiftShader: 1400 x 940 ran at a sixth of real time and 750 x 470 at five sixths — a quarter of the
  pixels for five times the rate. Ebiten's js backend puts every widget, line and glyph through WebGL,
  so a real GPU is the whole difference. The simulation itself is the same arithmetic it is natively.
- **The wheel arrives in different units on every platform, so it is normalised into
  notches** (`UI.normalizeWheel`). Ebiten's desktop backend passes on what GLFW gives it, which is 1 per
  detent; its browser backend passes the DOM event's raw `deltaY`, and there is a TODO in its source where
  `deltaMode` would be read. A Windows mouse in Chrome sends 100 per detent, Firefox in line mode sends 3,
  a trackpad sends a stream of fractions with a momentum tail. So `exp(wheel*0.18)` — a comfortable ×1.20
  per detent natively — was **×6.6e7** per detent in a browser on Windows, and every event of a Mac
  trackpad flick counted as dozens of detents.
  The unit is estimated from the largest event seen rather than from the platform, which is what the web
  does about this (`normalize-wheel` and its descendants): whatever the biggest push is, that is one notch,
  bounded, decaying while idle so that swapping a trackpad for a mouse recalibrates, and clamped so no
  single frame is worth more than a notch. The first gesture of a session is counted generously — every
  event on its rising edge is a new largest — and from the second it is calibrated.
  Normalising in `BeginFrame` rather than at the three places that use it means the zoom, the graph axis
  and the setup screen's column scroll all kept their constants and the desktop feel is unchanged.
- **The query string is the command line** (`args_js.go`, `//go:build js`): `?preset=apollo-mars&lang=ru`
  becomes `-preset apollo-mars -lang ru` in `os.Args` from an `init`, before `flag.Parse` looks. Only those
  two flags — `-shot` writes files and `-camtrace` prints to a console nobody has open. A value that names
  no preset and no language is *dropped* rather than passed on, because `main` treats both as fatal and a
  mistyped link would otherwise leave whoever clicked it looking at a blank page with the reason in a
  console they will never open.
- Verified end to end through the DevTools protocol — load, render, press Enter, fly — rather than by
  assuming a successful compile means a working page.
- **`.github/workflows/pages.yml` builds it and publishes `web/` to GitHub Pages** on every push to
  master. The wasm is built there rather than committed: seventeen megabytes of generated binary would
  cost another seventeen in the history on every rebuild.
- **A stalled queue cannot be waited out, which is why the fallback exists.** `actions/deploy-pages` caps
  its own timeout at ten minutes — asking for thirty gets "timeout value is greater than the allowed maximum
  - timeout set to the maximum of 600000 milliseconds" — so there is no number that helps.
- **`web/deploy.sh` is the fallback, and exists because the queue does stall.** One afternoon a deployment
  sat in `deployment_queued` for ten minutes and timed out — on GitHub's own `pages-build-deployment` bot as
  well as on our workflow, with the same artefact that had gone through in five and a half minutes that
  morning. So there is a hand deploy: it force-pushes an orphan commit of `web/` to `gh-pages`, which keeps
  the seventeen megabytes from accumulating in the history, and it serves once the Pages source is pointed
  at the branch instead of at Actions. Nothing about GitHub Pages avoids Actions entirely, incidentally —
  the branch route runs their own workflow, which is why the failure looked identical from both sides.
- **The workflow gates on `go test ./sim/...`, not on the whole suite.** On Linux the interface package
  wants X11 and GL headers to build and a `DISPLAY` to so much as *import* — Ebiten's package init calls
  `glfw.Init()` — so running tests that never open a window costs six minutes of dev packages plus xvfb
  in a job whose purpose is to publish a page. The physics is pure Go and takes twenty seconds, and the
  wasm build compiles the whole program anyway, so nothing that fails to build gets past. Both failures
  were found the same way: by reading the run, not by assuming a green laptop means a green runner.

`-shot` exists because Ebiten can only create and read images inside a running game loop — there is no
way to render the UI headless. The flag drives the real loop through the script in `shot.go` and dumps
the canvas to PNG. It is the only way to look at the interface without a human at the keyboard.

- **A step names its moment; it does not count seconds.** T+45 is max q on a Saturn V and a fifth of the
  way to it on Titan; T+232000 is a lunar flyby on one preset and an idle coast on another. `shotTimeline`
  flies a throwaway copy of the same flight first and the steps resolve against its events — the flight is
  deterministic, so every instant it finds is one the captured run hits exactly.
- **A moment or a body the preset does not have skips its step** rather than saving the previous frame
  under a new name. Airless bodies have no max q; a system of one body has no arrival.
- **Bodies are named, not indexed**, for the reason everything else here is: `focus: 10` was the Moon until
  Mars grew two of its own, and there is no index ten at all in a system of two.
- **`TestShotStepsAreInOrderForEveryPreset` guards the one thing that can go wrong.** `FastForward` does
  not rewind, so a step resolving to something already past would quietly capture the previous frame.

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
| `sim/presets.go` | Fifteen of them, and the invented Kerbin system. All reach orbit; nine carry a flight plan and seven leave the body they launched from |
| `main.go` | `App` — the four-screen state machine, `startScreen`, `newApp`, `ebiten.Game` |
| `theme.go` | Palette and fonts (goregular/gomono, compiled in, no asset files on disk), and what colour each body is |
| `ui.go` | Immediate-mode toolkit: `NumField`, `Button`, `Radio`, `Checkbox`, `Dropdown`, `Scroll` |
| `lang.go` | Locale loading and lookup, RU/EN switching, dispatch for events, verdicts, phases, presets, bodies |
| `assets/locale/*.json` | All interface text, one file per language, flat dotted keys |
| `perf.go` | The service readout: what a frame costs, split into physics and everything else |
| `render.go` | `Rect`, primitives, `Camera` (world metres → pixels, with rotation) and its inverse |
| `screen_presets.go` | The first screen: the mission list, and nothing else |
| `screen_mission.go` | The second: what the mission is, in prose from the locale and figures from the config |
| `store.go` | Saving a setup: JSON in, JSON out, and what a loaded one has to be before it is flown |
| `store_native.go`, `store_js.go` | Where it is kept — a file in the user's config directory, or `localStorage` |
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
- **`Config.Body` is an input only when there is no system, and a read-back mirror ever after.** That is
  what makes a single-planet configuration — and every test that writes one by hand — work, and it is the
  only direction that leaves the editor working: the first column writes through the tree, because it has
  to for the other seventeen bodies, and `EnsureSystem` used to copy the stale mirror back over the top on
  the next call. Diameter, mass, rotation period — every edit to the planet the pad is on snapped back a
  frame later with nothing on screen to say why. `TestEditingTheLaunchBodySticks` drives a real frame of
  the editor and pins it; the two directions cannot both be live.
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
- **Every body carries its own air**, in `Body.Atmo`, and a zero value is a vacuum. The vehicle flies
  through whatever it is next to: `forces` reads `Center().Atmo`, `AtmoTop` follows the frame, and the
  profile of each one is derived in `System.Normalize` with *that body's* surface gravity — the same gas at
  the same surface pressure thins out at a different rate under a different pull, which is why it cannot be
  prepared once for the configuration. In the solar system Venus, Earth, Mars and Titan have air and
  nothing else does.

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
- **The g-load is divided by `G0`, not by the local surface gravity.** A g is 9.80665 m/s² wherever the
  vehicle is. Dividing by the body underneath — which is what `maxG` and `Telemetry.AccelG` used to do —
  made the figure mean "local surface gravities": Titan's ascent read 4.9 where the crew would have felt
  0.68, and the number *stepped* as the vehicle crossed into a moon's sphere of influence, because the
  divisor changed with the frame. `kerbin-mun` reported 22 g for a burn pulling 3.7. The interface's own
  thresholds (amber at 4, red at 6) are human tolerances, so they only ever meant real g.
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
- **The trail and the event markers** are drawn through `FlightScreen.trackPoint`: a sample taken at time
  `t` is turned forward by `ω·(T−t)` and then shifted into the frame being drawn *as of its own time*. The
  current point does not move, so the orbit ellipse and the rings, which all refer to instant `T`, stay
  consistent. In orbit the trail lags the ellipse by `ω·T` (2° over 500 s) — that is the ground track, and
  it should.
- **The rotation belongs to the pad, so `ω` is the launch body's and it is applied only to samples measured
  from the launch body, before the shift rather than after.** Using the *frame* body's rotation on
  everything — which is what the first cut did — puts a ground track where there is no ground: ninety days
  into the Mars transfer, drawn in the Sun's frame, `ω_sun·(T−t)` is forty-six radians, and the ascent's
  markers came out round by the orbit of Venus. In the Sun's frame they now sit on the Earth's rail at the
  point the launch happened, which is where they happened.
- **And it is weighted by `groundHold`, the same ramp that lets the camera go of the local vertical.** The
  angle is measured from *now*, so as the clock runs the Earth-centred part of the path turns about the
  Earth while the current point stays pinned: on the way to the Moon that reads as the trail being wound up
  like a spring, and a path already flown has no business changing. The ground track is only worth having
  while the picture is about the ground, so it fades out over the same stretch where standing on a planet
  becomes looking at one — full on the pad, nothing at all by the Moon's orbit or in another body's frame.
  The cost is that the `ω·T` lag in low orbit is scaled down with everything else; the gain is that nothing
  that has already happened moves again.
- **The trail reaches back one revolution, not a fixed number of seconds** (`trailSpan`). Fifteen minutes
  covers an ascent and is a tenth of a pixel of an interplanetary cruise, where it left the flown path
  invisible and the vehicle apparently drawn from nowhere. What the window guards against is *revolutions*
  — a trail that wraps the same orbit over and over is one smear — so the bound is one period of the orbit
  the vehicle is on, and a trajectory that is not coming back round gets the whole flight.
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
  without saying by what is not a verdict. Both screens colour the verdict, and `OutcomeCaptured` was
  missing from the good-news case for as long as it existed: "IN ORBIT AROUND MOON" in the same red as a
  crater.
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
- **`bodyName` and `presetName` are lookups, not switches.** Seventeen bodies and fifteen presets is where
  a switch stops being worth writing; a missing entry renders as the identifier, which is the same safety
  net `T` has. The locale keys *are* the identifiers — `preset.earth-falcon`, `body.mun` — so there is no
  slug-to-key mapping to keep in step with anything.

### The air, per body

`Body.Atmo` is one atmosphere per body, and the four in the solar system that have enough of it to fly
through are Venus, Earth, Mars and Titan. `EarthAir`, `MarsAir`, `VenusAir` and `TitanAir` in `solar.go`
are functions rather than values, because an `Atmosphere` carries slices and a shared one would be edited
from every system built out of that file at once.

- **`Top` means the same thing on every body: the altitude at which the profile reaches 1e-9 kg/m³**, which
  is the density Earth's own ceiling cuts off at. It did not mean that. Each was set to whatever suited the
  preset launched from it, so Mars stopped at 90 km with the profile still at 7.8e-7 — 2600 times Earth's —
  Titan at 500 km with 660 times, and Venus at 250 km with a ten-thousandth of a billionth, a hundred
  kilometres of nothing. `TestEveryCeilingIsAtTheSameDensity` pins the rule now, and `DensityAt` exists
  because choosing a ceiling needs the one question `State` refuses to answer: what the profile would still
  say above it.
- **It matters beyond tidiness, because `Top` is also the line an orbit has to clear to count as one.** With
  Mars's ceiling at 90 km, `mars-ascent` reached "orbit" with its periapsis at 92 — two kilometres above a
  cliff edge drawn just under it, on a planet whose real air would have dragged it down in days. Titan's was
  579 above 500. Both had to be flown again: **Mars is 249 x 195 km and Titan 1212 x 1072**, with 45 and
  352 km of margin, and `TestEveryPresetClearsTheAirItFliesIn` wants ten per cent of the ceiling as the
  minimum for every preset.
- **Mars needed a tail *below* the horizon**, which nothing else here does: −18° at the end of the burn is
  what stops the climb while the horizontal speed is still building, and it is the only thing in that pitch
  family that raises a periapsis rather than an apoapsis. The kick-stage-at-apoapsis answer that Kerbin and
  Titan use cannot work there — the upper stage has 2.6 km/s and circularising at apoapsis from a standstill
  wants 3.5.
- **Kerbin keeps its cliff, on purpose.** Its air ends abruptly at 70 km with 3.1e-6 kg/m³ still in it,
  because that is where the game it comes from ends its atmosphere. Making it consistent with the real
  bodies would stop it being Kerbin. `TestKerbinKeepsItsCliff` states that as an exception rather than
  leaving it to look like an oversight.
- **The gas giants are airless on purpose.** An atmosphere here is measured *from a surface* — a base
  pressure and a temperature at a radius — and Jupiter has no surface to measure from. Between a made-up
  cloud deck and nothing, nothing is the honest answer.
- **Venus is described and not launched from.** 92 bar at 737 K is a surface density of 65 kg/m³, fifty
  times Earth's, where Titan at four times Earth's was already the hardest preset here to fly.
- **Above the air, and on a body with none, `State` returns nothing at all** — no temperature and no speed
  of sound. It used to hand back the surface values, which put a Mach number on a vehicle in orbit: Mach 21
  at 300 km over the Earth, Mach 33 at ninety million metres from Mars. A surface temperature without air
  is a radiative question this simulator does not ask, so the two presets that carried a trace atmosphere
  purely to feed that readout — the Moon and Io — are plain vacuum again.
- **`surfaceP` stays the launch body's**, because it is the pressure the engine's sea-level Isp was
  *rated* at, not a property of where the vehicle happens to be. An engine does not get a different rating
  by flying to Mars.
- **All thirteen presets are bit-for-bit unchanged across the move**, verified at all 17 digits of the
  final state: every atmosphere the presets defined was the same data now attached to the body.

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
  1926 × 1776 km at e = 0.021 where the run ends, with the service module still half full. A lunar orbit is
  perturbed enough that those two numbers are a snapshot rather than a constant: the apoapsis swings through
  some 150 km over the days that follow, so one instant is the best anything here can quote.
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

### The grand tour, which is four windows solved one after another

`voyager-tour` is Voyager 2's mission: one injection and four gravity assists, Jupiter to
Saturn to Uranus to Neptune and out of the system. It is the longest flight here — twenty-four
years to the last encounter — and the only one that gets where it is going on borrowed energy.

- **The windows are solved, not searched.** The mean anomalies in `solar.go` are not an
  ephemeris, so each planet can be *put* where the flight crosses its orbit: fly the
  trajectory, take the crossing time and heliocentric longitude, and invert the same Kepler
  equation the rails run on for the mean anomaly that body needs at t = 0. The same trick as
  Mars's window, four times over.
- **But it has to be iterated, and that is the interesting part.** Moving a planet changes
  the very trajectory the phase was solved from: its own gravity perturbs years of cruise,
  and every close pass upstream amplifies the difference. One pass leaves a miss of a whole
  astronomical unit; four bring it inside the sphere of influence. Chained assists are
  chaotic and this is what that looks like in numbers.
- **Which is also why the phases had to be re-solved against the configuration that ships.**
  The pitch keyframes were rounded to one decimal place for readability, and that alone moved
  the Jupiter pass by six radii and lost the rest of the tour — the preset sailed past Uranus
  at thirteen hundred radii, ninety-seven years out. A preset is data; the data has to be
  solved for the data.
- **`OutcomeEscape` had to stop being terminal, and that is a model fix rather than a preset
  one.** Both Voyagers were on solar escapes from the Jupiter encounter onwards and went on
  to meet three more planets. Ending the flight at the first unbound orbit made a grand tour
  impossible to express. Escape is now a settled verdict — a high-water mark, like the rest —
  with its own `EvEscape` marker, and it is the one settled verdict the clock still binds:
  an orbit has somewhere to be, and a vehicle leaving for good does not.
- **The four encounters are reproducible and the final verdict is not, which is worth
  understanding.** `FastForward` lands exactly on the instant asked for, so it takes a partial
  step where `Advance` would carry the remainder — and a pass at ten radii amplifies that last
  bit into thousands of kilometres by the next planet. Thousands of kilometres is nothing
  against a sphere of influence half an astronomical unit across, so *which* planets are met
  and *when* holds to a fraction of a per cent however the flight is advanced. Whether the
  Neptune pass adds quite enough to leave the Sun for good does not: one jump says escape, the
  screenshot script's jumps say a 61 AU ellipse. The test asserts the tour and leaves the
  verdict alone.
- **The passes are farther out than the real ones** — 41, 165, 34 and 10 radii against
  Voyager's 5, 3, 4 and 1 — because these are the four that close *as a chain*. Each one
  still does its job: the aphelion goes 23 → 30 → 58 AU and Neptune's pass takes it out of
  the system.
- **The launcher is a Titan IIIE / Centaur** for the reason Proton-K is a Proton-K: the two
  UA1205 solids burn first and alone, and the core lights after they are gone, so the stack
  is genuinely serial. Five stages, of which the last two are the interesting ones — the
  Centaur is cut off seventy seconds in with eleven and a half tonnes left and relit by the
  plan, the same leftovers trick `proton-zvezda` uses, and the TE-364-4 is an `IgniteOnNode`
  stage that finishes the injection.
- **The injection only works at one point in the parking orbit**, as ever: T+5300 s puts the
  escape asymptote along the Earth's own motion and buys a heliocentric aphelion of 10 AU.
  Half an orbit away the same delta-v buys 1.0 AU. It reaches Jupiter's orbit in 689 days,
  against Voyager 2's 688.
- **The audits stop at the first verdict rather than flying the mission** (`flyToVerdict`).
  What they check is the ascent, and a preset whose flight runs for twenty-four years should
  not cost thirty seconds of wall clock in a test about whether its parking orbit clears the
  air. `TestVoyagerTourFliesPastFourPlanets` is the one that flies the whole thing, in a
  single `FastForward` with the encounters read out of the events afterwards: polling every
  two days holds the adaptive step down to two days, where left alone it grows to months.

### Parker, which spends its energy going down

`parker-solar` is the fastest flight here and the only one where every sign is reversed: the
injection is aimed *against* the Earth's motion, and the Venus flyby takes angular momentum
away rather than adding it. First perihelion is 37.1 solar radii at 93.4 km/s — the real
mission's first was 35.7 radii at 95.

- **The node time is the whole mission.** T+2350 s in the parking orbit puts the escape
  asymptote against the Earth's own motion and drops the heliocentric perihelion to 0.18 AU;
  half an orbit later the same 10.3 km/s buys a perihelion of 0.98 AU and an escape from the
  system. It is the same sweep Voyager's injection needed, read from the other end.
- **One Venus flyby, not seven.** The real mission walks the perihelion down over seven years
  by *resonant returns* — each pass leaves the vehicle in an orbit commensurate with Venus's
  year so the next one lines up. A single choice of Venus's phase arranges one encounter; the
  rest would need a search over the resonances, which is a different piece of work. What ships
  is the mission's first orbit, and it is the same first orbit: Venus at 45.5 days against the
  real 46.
- **The pass has to be on the way *in*.** `crossP` looks for the inbound crossing of Venus's
  ellipse, because a pass on the falling leg is the one that takes energy out. It buys
  2.5 solar radii of perihelion — 39.6 without Venus, 37.1 with — which is the same order as
  the real flybys' 0.02 AU apiece.
- **Delta IV Heavy needed a different lie from Proton-K's.** Three common cores burn together
  off the pad, and a serial list cannot hold that: giving stage 1 the two side boosters alone
  is a thrust-to-weight of 0.79 and the stack sits on the pad. So the split is by *thrust
  phase* — stage 1 has all three engines and the propellant burned before the sides go (the
  two sides entire, plus the four minutes the core spends throttled to 55%), and stage 2 is
  the core's remaining 96 t. Liftoff TWR comes out 1.18 against the real 1.2.
- **It is reproducible where the grand tour is not**, and for the reason the tour is not: one
  flyby amplifies nothing. Flown in one jump, in 30-day jumps or in 12-hour jumps, the
  perihelion agrees to a metre a second.

### Io, where there is no room

`io-jupiter` is the only preset where the sphere of influence is a design constraint rather than a
bookkeeping detail. Io's is 7840 km — four and a third radii — so:

- **The parking orbit has to be low**, and 58 x 43 km is not a stylistic choice. An orbit a few hundred
  kilometres up is a significant fraction of the way to the edge, where Jupiter takes over.
- **Leaving does not need escape, but the preset pays for it anyway.** From the parking orbit 417 m/s
  reaches the edge of the sphere and 739 is a full escape — the edge being four radii up rather than at
  infinity is the whole difference. The preset spends 750, eleven more than escape, because that is the
  value whose Jupiter orbit does not wander back in; 700 and 800 both do.
- **Which way out depends on where in the parking orbit the burn happens.** "Prograde" is prograde relative
  to *Io*, and Io is going round Jupiter at 17 km/s: the same 750 m/s at T+2000 s drops the vehicle to
  342 Mm, inside Io's orbit, and at T+4500 s lifts it above. Sweep the node time over one parking
  period, as with the Mars injection, for the same reason.
- **T+4500 s is also the value that does not come back.** The resulting orbit crosses Io's own, and 700 and
  800 m/s both wander back through the sphere of influence within the month. The test counts frame changes
  and wants exactly one.

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
- **`kerbin-ascent` stays a system of one body.** Its numbers are quoted in the README, and a Mun with a gravity of
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
- **Titan's air reaches Earth's own cutoff density at 712 km**, so `Top` is 720 and `settle` wants a
  periapsis above it. That puts the target orbit at 1000 km, which is a far harder ask than the 185 km the
  Earth presets aim at. Every attempt that looked like an orbit was coming out as `OutcomeDecaying` and the
  tuner was discarding it unseen. It shipped for a while with `Top` at 500 km — a cliff at 1.9e-7 kg/m³,
  660 times Earth's — and the preset's periapsis four kilometres above it; see the ceilings section.
- **The vehicle had to be sized for the planet, not tuned into it.** Twice the first guess: 7.9 t, two
  stages, 22 kN. It now reaches 1212 x 1072 km, and the propellant for that was always there: the kick
  stage was burning 450 kg of the 1300 it carries.

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
  from 0.11% off to under a tenth of one. It reads 0.03% at the end of the run, and drifts the way every
  other perturbed orbit here does. `TestProtonGeoPresetReachesTheBelt` asserts half a per cent.
- **"Burn what is left" is written as the number, not as a huge one.** The first node asks for 422 m/s
  because that is exactly what 3.3 t buys at that mass, so the tank runs dry as the burn ends either way.
  The tuner used 9999 to find it; a preset should not ship with a magic number standing in for a
  measurement.
- **The audit's target check does not apply to a preset with a plan.** Its verdict comes in a parking orbit
  and the mission goes on from there, so all the ascent owes is an orbit that clears the air.

### The flyby preset

The preset carries one node: a prograde translunar injection at T+15325 s of 3162 m/s, out of the parking
orbit, on what the S-IVB kept back. It enters the Moon's sphere of influence at **T+2.63 days**, passes
**1791 km** over the surface and leaves again at T+4.0 days.

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
  at entry the two-body hyperbola said 59 km *below* the surface and the real trajectory passed 1791 km
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

The identifiers say what a preset is: a plain ascent is `<body>-ascent`, and anything that goes somewhere
is `<from>-<to>` or named for its launcher. So `mars-ascent` and `moon-ascent` rather than `mars` and
`moon`, which is also what the doc comments had been calling them.

The pitch programmes and the final cutoffs were found by search: a profile generator,
`pitch = tail + (90 − tail)·(1 − f)^p` over the fraction f of the turn, plus a sweep or a bisection on the
last stage's cutoff for a periapsis at the target.
The tuner lived in `sim/zz_tune_test.go` and has been deleted — if the presets ever drift, writing it
again beats tuning by hand. Note the tail: the Apollo profile only works with a **positive** asymptote of nine
degrees, so the first generator — which could only decay to zero — could not express it at all.

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

## The first screen

`ScreenPresets` is where a run begins: fifteen missions, one per row, and nothing else. What used to be
first — four columns of every number the model has — is a great deal to be handed before you have said
what you are trying to fly, so it comes second.

- **`startScreen` is the whole of the decision** and is a pure function of two booleans, which is why it
  has a test: the list, or the editor with a mission in it, or a vehicle already on the pad. Naming a
  preset means the choice is made, so the list would be in the way; `-fly` means the editor is too.
- **`-fly` exists for tests and for links.** A scripted capture or a shared URL usually wants the flight,
  not two screens of preamble, and `?fly=1` carries it in the browser. `newApp` is where it lands, which
  is also why the app's construction is a function rather than eight lines inside `main`.
- **The row shows the identifier as well as the name**, dim and monospaced on the right. It is what
  `-preset` and `?preset=` take and there is nowhere else in the interface to find it out.
- **The rows shrink to fit whatever window there is** (`presetLayout`), down to a floor of 22 px. In a
  window the program sizes itself that never matters; in a browser the window is whatever the browser is,
  and 1200 x 760 had thirteen rows overlapping the header at the top and running off the bottom. The test
  checks five window sizes, including that one, and that no two rows overlap — two rows sharing a click
  is the other way this goes wrong.
- **The keyboard's row and the mouse's hover are different things.** Arrows move the selection, the
  pointer only lights what is under it: a pointer hovering one row while the keyboard sits on another is
  two selections, and only one of them can be right.
- **Picking builds the editor fresh** (`NewSetupScreen`) rather than telling the old one to change its
  mind, and cancels the pending edit first. Every field in that screen is bound to an address inside the
  configuration being replaced — the same trap `loadPreset` documents.
- **There is no way back to the list**, deliberately: the editor has its own preset dropdown, which is
  the same choice without losing what you have typed.

## The mission page

Picking a row leads here rather than into the editor: what the real mission was, what this
one does, and the figures worth knowing before flying it. Four columns of every number the
model has is the *how*; this is the *what*, and it was missing.

- **The prose lives in the locale files**, two keys per mission — `mission.<id>.history` and
  `mission.<id>.here` — because the physics package holds no text and the identifiers are
  already the locale keys. `TestEveryMissionHasItsOwnWords` fails on a preset that ships
  without a description in either language, which is the only way to notice: a missing key
  renders as the key, on a screen nobody opens until after the preset is shipped.
- **The figures are read out of the configuration, never flown.** Liftoff mass, stages,
  payload, thrust-to-weight, ideal Δv, the plan's burns and the time limit are all `Cfg`
  reads and cost nothing. The grand tour takes half a minute to fly, and nobody is waiting
  for that to read a paragraph.
- **The timings in the prose are the tested ones.** T+604 s, 1926 × 1776 km, 37 solar radii:
  every number quoted in a description is one the mission tests already pin, so the two
  drift together or not at all.
- **The prose is capped at a readable measure** (`proseMeasure`, 720 px) instead of being
  stretched to the window. A 1500 px window would otherwise hand it lines of a hundred and
  forty characters, which nobody follows back to the start. Under 720 px of body the figures
  stack under the text instead of squeezing beside it.
- **The configuration is loaded here, not by the list.** `pick` only decides *which* mission;
  `proceed` is what replaces `App.cfg` and builds the editor, with the `u.cancel()` every
  such replacement needs. That keeps the trap in one place instead of two.
- **`-preset` still skips it**, along with the list: naming a mission means the choice is
  made, and the description is part of choosing. `-fly` skips the editor as well, as before.
- **The saved setup has a page too**, with words of its own — it has no history to give — and
  it survives the store being empty by saying so rather than by crashing.

## Saving a setup

Everything the editor produces is one `sim.Config` — the system of bodies, the air, the vehicle, the
pitch programme and the flight plan — so saving it is one file. `SAVE` and `LOAD` sit in the setup
header, and the stored setup gets a row of its own at the bottom of the mission list.

- **One slot, not a library.** The fault being fixed is losing an evening of editing, not the absence
  of a collection. There is nothing to name and nothing to choose.
- **What is written is the inputs.** `Body.Mu` and `Body.SOI` carry `json:"-"` because they are derived —
  and because the root's sphere of influence is `+Inf`, which JSON cannot spell, so a normalized system
  was not writable at all until they came out. `EnsureSystem` derives them again on the way in, which is
  also what clamps a stale launch body and mirrors `Body` back. `TestASystemOnRailsCanBeWrittenAtAll`
  pins the trap, since the symptom is a whole feature failing over one struct tag in another package.
- **The format is versioned, and version 2 was the first time that earned its keep.** Version 1 kept one
  atmosphere in the configuration, for the launch body; version 2 keeps one per body. A version 1 file is
  read twice — once into the live `Config`, once into a shim that still has the dead field — and its air is
  put on the body it described. A compatibility field left in the live struct would outlive the
  compatibility.
- **A stored file is the one input this program gets from a previous version of itself**, so it is the
  one that has to be doubted: `validConfig` refuses a config with no bodies, no radius, no stages or
  more burns than the bitmask holds, and a `version` from the future says so rather than being misread.
  A decoded config cannot contain a NaN — JSON has no literal for one — so the only guard needed on the
  way out is `Marshal`'s own refusal to write one.
- **The round trip is tested by flying it, not by comparing JSON.** Every preset is flown 400 s before
  and after, and the states have to match to the last bit. A config that reads back almost right is the
  worst outcome available: nothing complains and the numbers quietly differ.
- **Loading is `loadPreset` with a different button on it**, and needs the same two things: `u.cancel()`,
  because every field in that screen is bound to an address inside the configuration being replaced, and
  a reset of the body selection, because the stale index is how this screen crashes.
- **The saved row is last in the mission list.** Every index in that screen and in `-preset` counts into
  `sim.Presets()`; a row at the front would shift all of them. It is also read afresh when picked rather
  than kept from construction, so the editor gets its own slices — the stages, layers and keyframes are
  edited in place, and a shared copy would mean editing the stored one too.
- **`-shot` ignores it.** Whether the machine running a capture happens to have saved a setup is not part
  of the program, and a screenshot that depends on it is not reproducible. The tests take the same care:
  `noSavedSetup(t)` points the store at an empty directory, or the row count depends on whose laptop it is.
- **Written through a temporary file and renamed** natively, so a failure half way through leaves the
  previous save intact rather than a truncated file where a setup used to be. In a browser it is
  `localStorage`, per-origin — a local build and the published page keep separate setups, which is the
  right way round — and every call is recovered from a JS exception, because storage is denied outright
  in some privacy modes and a page that dies on a save is worse than one that says it could not save.

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
  and around the Moon, and both are true. What it does *not* mean is that one frame's history can be
  redrawn in another; see below.
- **The shift is taken at the sample's own time**, which is the only mapping that gives a *trajectory* in
  the drawn frame: one curve, no kinks, and nothing that depends on the clock, so a sample once drawn never
  moves again.
- **What was flown in the drawn frame is drawn whole; what was flown in the frame just left is let go of**
  (`showTrack`, `ghostFade`, 1.2 s). A sample of the current frame needs no mapping — it is already in the
  coordinates being drawn, so it cannot be smeared, kinked or displaced. A sample of the frame just left is
  held by an offset frozen at the crossing, which puts it exactly where it was drawn, and fades out from
  there. Anything older is not drawn; the graph screen keeps the whole flight.
- **Both of the other ways of drawing the past are worse, and both were tried.** Map it at each sample's
  own time and you get the true path relative to the drawn body — and the true path relative to a *moving*
  body is a spiral: the revolutions flown around the Earth while waiting for the Moon spread over the
  229,000 km the Moon travelled meanwhile. Leave it held by the frozen offset instead and it keeps its own
  shape but wears the wrong one for the frame it is now in: positions match at the seam and directions do
  not, so the trail reaches the sphere of influence and turns forty-five degrees while the launch markers
  sit a hundred and fifty thousand kilometres off the Earth. Hence neither: held, then let go.
- **A frame change is legitimately a change of picture, and this is the honest way to say so.** The events
  are invariant — where the vehicle was relative to each body, when, how fast, what it spent — but the
  *shape of the curve* is not, and no recomputation makes it so. That is the same fact as a ball in a
  train falling straight down and tracing a parabola from the platform, and the same fact the ascent
  already shows: six kilometres of sideways drift in the inertial frame, a vertical climb in the rotating
  one. So the drawn past is not silently rewritten into a shape it never had; it is shown where it was and
  then released.
- **The change of frame carries the *view* across** (`handOver`, `camHold`, 0.6 s). Everything is written
  from the new centre from that instant, the camera's own centre included, and a dragged view is stored as a
  point in the frame's coordinates — so without carrying it over, the whole picture slides by the 384,000 km
  between the Earth and the Moon over a bookkeeping change that nothing in the flight marks. The view is
  re-expressed from the new centre and then eased back to whatever the automatic framing wants. The zoom is
  a separate decision that glides on its own; entering a sphere of influence changes the automatic span by a
  factor of fourteen, and that motion is a zoom doing its job. `snapFrame` lands the hand-over at once for a
  scripted capture, because a screenshot taken mid-glide is a screenshot of neither framing.
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
- **A dragged view near the ground turns with the ground.** The launch site is carried east at 465 m/s, so
  a camera held still in the *inertial* frame is a camera the pad slides out of — the whole width of a
  1.5 km view in three seconds. That is what "everything moves sideways when I click" was, and it was
  hiding behind the ninety-degree flip until that was fixed. While the picture is about the ground a free
  camera advances its centre and its rotation by `ω·groundHold·dt`, so a point on the surface holds its
  place on screen exactly; pulled back, the ramp takes it to zero and the view is inertial again, which is
  what an orbit wants.
- **`groundHold` asks about the frame and the scale, not about what the camera follows.** It used to
  require `follow == -1`, which meant a dragged view a kilometre over the pad was not "standing on it" as
  far as the trail was concerned — while obviously being exactly that.
- **A drag never rotates the picture, and pinning a body takes half a second over
  it.** `Rot` is the world angle pointed at the top of the screen: following the vehicle it is the
  vehicle's own radius, so a launch reads as a climb, and anywhere else it is the world's +Y. Those are
  different by a quarter turn on the pad — the launch site sits on the +X axis — and the switch used to
  happen in one frame with `hold = 1`, so a click on the pad turned the whole picture ninety degrees. It
  looked like a rendering fault and was reported as one. A pan has nothing to say about which way is up, so
  it now says nothing; a pinned body eases. `snapCamera` lands all of it at once for a scripted capture,
  which is what keeps the pinned captures from coming out eighty degrees from where they settle.
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
- **Every body draws its own air**, which is the same rule the physics follows. A planet arrived at from
  somewhere else used to be drawn as bare rock, because there was one atmosphere in the picture and it hung
  around the body the flight started from.
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
- **A dragged view is stored in the ground's turning frame, and the turn is derived from the clock.**
  Two faults, one cause. Pinned in inertial space, a dragged view is one the launch site leaves at
  465 m/s — the whole width of a 1.5 km picture in three seconds, which is exactly what "holding the
  mouse button makes the camera drift right" was. So `freePos`/`freeRot` are kept pre-rotated by
  `groundTurn()` (`freeCenter`/`setFree`/`takeFree`), and the drag anchor, the drag shift and the
  wheel's zoom-about-the-cursor correction all go through the same conversion. And `groundTurn` is
  `ω·T·groundHold`, read off the mission clock every frame: the first cut *integrated* `ω·dt` into
  `cam.Rot` and walked straight into the paragraph above, because the simulation advances in whole
  0.02 s steps while a camera turning by `dt` does not — 4.3 px of jitter and 499 reversals in ten
  seconds. Derived, the pad holds its pixel to 1e-12 of one, which is why `padTrack` ignores anything
  under `padQuiet`: the sign of the last bit flips at random and counting that is measuring float64,
  not the screen.
- **Framing is derived from the eased zoom, not the raw `span`.** Otherwise a step in the span — the
  orbit closing, say — jolts the composition while the zoom is still gliding.
- **The automatic framing has to keep the vehicle in the picture, and that is solved rather than assumed.**
  The span was capped at twenty-four body radii, which is a sensible width for a picture *about a planet*
  and hides the vehicle everywhere else: in the Sun's frame it is 1.7e10 m against a vehicle at 1.5e11, so
  three days out from the Earth the screen was one yellow dot with the flight six view-widths off the edge,
  and the same cap bit on the way out of any body past about twelve radii. The cap now yields to the
  vehicle's own distance — and because *where the centre sits* is itself a function of the span through
  `camBlend`, the span needed to keep the vehicle is a function of the span too, so `autoScale` iterates
  three passes of a monotone fixed point instead of guessing at a threshold. The first attempt did guess
  one, and left the vehicle just off the top edge at 1.9 radii.
- **The slide to the body's middle stops once the body has stopped being the subject** (`camFarFade`).
  It is what makes standing on a planet become looking at one, and it assumed there was a planet to stand
  on: in the Sun's frame the vehicle is two hundred solar radii out and the slide never let go, so the
  picker said "the vehicle" while the picture was centred on the Sun with the flight off to one side. The
  fade is zero within four radii — a parking orbit, an approach, anything the body's own shape frames — and
  one past sixteen. **The rotation ramp keeps the old `camBlend` alone**, because tying *that* to the fade
  would point the vehicle's heliocentric radius at the top of the screen and turn the whole picture over
  the months of a cruise.
- **`camBlend` is the one place that knows how the centre slides from the vehicle to the body.**
  `autoScale` and `updateCamera` both need it, and the two disagreeing is precisely the fault above: one
  chose a span believing the centre was still near the vehicle while the other had already moved it to the
  middle of the planet. `TestTheVehicleStaysInThePicture` walks fourteen moments of the Mars flight,
  including both sides of the hand-over into the Sun's frame, and asks only that the vehicle is on screen.
- **The camera focus cannot be lerped towards the planet's centre linearly.** The target is thousands of
  kilometres away, so even a factor of 1e-4 shifts the picture by hundreds of metres and throws the
  launch pad out of a 1.5 km wide view. The blend stays at zero until the span reaches half a planet
  radius.
- **The launch pad is drawn in metres** and shrinks on its own as the vehicle climbs; below 7 pixels it
  collapses into a label. That label's anchor is fed into the event-marker spacing, otherwise at
  orbital scale it lands on top of the cutoff marker.
- **The mission clock rounds before it splits.** Taking the minutes off first turned 11999.98 s into
  "T+199:60.0". Only visible once flights could run for hours.
- **The trail is trimmed to one revolution of the current orbit**, with `trailWindow` as the floor so that
  no ascent is cut short. With the flight no longer ending at orbit, an unbounded trail wraps the planet
  over and over until the picture is one smear; a fixed window instead loses the whole of a transfer. See
  the frames section.
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
- **The atmosphere column edits the air of the selected body**, and says whose it is under the header. A
  body with none gets a sentence saying so and a `+ atmosphere` button rather than a column of zeroes to
  puzzle over; one with air gets `× atmosphere`. The offered default is Earth's, as a thing to edit.

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

## The service readout

Under the mission clock, in the corner of the trajectory view: frames and ticks, the frame period, what
the physics cost and what it bought, what the rest of the frame cost, the warp actually delivered, the
last prediction and the size of the history. `perf.go`.

- **The split is the point.** `sim` is the time inside `Advance`; the other line is the whole of `Update`
  minus that, which is the interface — every widget laid out, every ring tessellated, the trail. Which
  half is not keeping up is a question this project has had to answer twice by indirect means: once by
  timing a prediction that turned out to cost 658 ms, and once by taking screenshots at two window sizes
  to establish that the browser build was bound by pixels rather than arithmetic. Now it is on the screen.
- **`Update` is not the whole frame, so the frame period is shown too.** What Ebiten spends rasterising
  and presenting happens after `Update` returns, and the gap between the period and `sim + ui` is exactly
  that. Four milliseconds of interface at eight frames a second is not a contradiction — it is where the
  other hundred and twenty went, and in a browser under software rendering that is the usual reading.
- **Sampled over half a second, not per frame.** Per-frame figures jitter by a factor of two, and in a
  browser `time.Now()` is quantised to about a tenth of a millisecond — the same order as a frame's
  integration — so a single frame's measurement is mostly the clock. For the same reason the cost of one
  step is withheld until a window has twenty of them: a number that is noise is worse than no number.
- **The warp line only appears when the warp is not being delivered**, and keeps two decimals while it is
  small: at ×1 asked and a third achieved the interesting figure is 0.33, and rounding it to zero says
  nothing. It says the simulation is falling behind before `WarpLimited` trips, which only fires once a
  frame has run out of its step budget entirely.
- **A wash goes behind the whole readout, and it is not decoration.** The block sits over whatever the
  trajectory view happens to show: bright blue sky at the start of a flight, green ground under it, black
  space later. Faint grey on the first two is invisible, so the rows are collected before they are drawn —
  the wash cannot be sized until the block is known — and it is dense enough to mute a prediction line
  crossing the corner without reading as a panel. No border, for the same reason.
- **`Sim.Steps` is the one thing the physics gained**, a plain counter in `advanceOne` that nothing in the
  model reads. A prediction runs on a copy, so its steps land in the copy's counter.

## What it costs to run

Measured, because all four of these were guesses that turned out wrong in one
direction or another. A hitch appeared once the vehicle left the atmosphere: the
numbers said the prediction was taking **658 ms** and running twice a second.

| | before | after |
|---|---|---|
| one step, solar system | 17.2 µs | **3.1 µs** |
| one step, single body | 1.08 µs | 0.88 µs |
| one prediction from the parking orbit | 658 ms | **9.5 ms** |
| predictions during a real-time coast | 2 per second | **none until the path is flown** |
| history at T+700 d, Mars | 100,000 samples, 43 MB | **12,500 samples, bounded** |

- **The history is bounded, and it was not.** A settled flight orbits indefinitely, so the record grew
  linearly for as long as the program was left running: 93,000 samples and 43 MB of heap at T+600 days on
  the Mars preset, each one carrying a `PropFrac` slice of its own for the collector to walk. Past
  `maxHist` the history *halves* — every second sample dropped, the recording rate halved with it — so a
  four-hundred-day flight keeps the same twenty thousand samples an hour-long one does. Note that thinning
  the rate alone would have done nothing: during a coast a sample is written per integrator step and the
  steps are minutes long, so the interval was never what bound it.
- **Thinning keeps a coarser record of the whole flight, not a complete record of the recent part.** An
  ascent five days back is still on the graph at half the resolution it had; dropping the oldest instead
  would throw the launch away, which is the one part of a flight everybody wants to look at. It also
  bounds what the trail costs to draw, which is the other thing that had no ceiling — `trailSpan` is
  `+Inf` for a trajectory that is not coming back round, so an interplanetary cruise draws the whole
  flight by design.
- **The prediction is recomputed when the flight has flown into it, not when the clock has ticked.** One
  is 25 to 90 ms — a ten-day horizon through eighteen bodies — and a timer of half a second meant that
  hitch twice a second for the whole of a coast, in return for a curve that had not moved by a pixel. The
  rule is now a floor of half a second of wall clock *and* two per cent of the predicted span actually
  flown: at ×1e6 the span is eaten in milliseconds and the floor binds, so high warp is unchanged; at ×1
  it is minutes between recomputes, and paused it is never.
- **Which is why `planKey` exists, and it is not an optimisation.** A paused flight advances no mission
  time at all, so without a fingerprint of the plan an edited burn would keep the path it produced before
  the edit — for as long as the pause lasted.
- **A stale prediction is free because the path is drawn from the vehicle**, skipping the points already
  flown. The curve is the same curve either way; the only thing staleness could show is a gap between the
  vehicle and the start of its own path, and there is now no way for one to open.
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
