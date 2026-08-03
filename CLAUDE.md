# fsim

A launch simulator: a two-stage rocket flown to orbit around an arbitrary planet. Set up the planet, the
atmosphere, the vehicle and the pitch programme → "Launch" → live ascent with telemetry → graphs.
Go + Ebiten, and the physics is real.

## Build & run

```
go run .                       # start
go run . -preset 2             # start on preset 2 (0..3)
go run . -shot ./shots         # run the capture script and save a PNG of every screen
go run . -camtrace 700         # print the vehicle's screen coordinates per frame (catches camera shake)
go run . -lang ru              # start with the interface in Russian (default is English)
go test ./sim/                 # physics
go build ./... && go vet ./...
```

`-shot` exists because Ebiten can only create and read images inside a running game loop — there is no
way to render the UI headless. The flag drives the real loop through the script in `shot.go` and dumps
the canvas to PNG. It is the only way to look at the interface without a human at the keyboard.

## Layout

| File | Contents |
|------|----------|
| `sim/body.go` | Planet: radius plus one of {mass, density, g} derives the rest. `Normalize()` is mandatory before using `Mu` |
| `sim/atmosphere.go` | Gas composition → molar mass and γ; layers with lapse rates → barometric T/P/ρ/a profile |
| `sim/rocket.go` | Stages: mass, propellant, thrust, Isp(p), ṁ, cutoff, ignition mode. Tsiolkovsky, TWR |
| `sim/program.go` | Pitch keyframes, interpolation, prograde-hold mode |
| `sim/orbit.go` | `Vec2` plus osculating elements from (r, v) |
| `sim/sim.go` | State, RK4 step, staging state machine, Δv loss accounting, telemetry, history |
| `sim/presets.go` | Earth/Falcon-9, Mars, Moon, Kerbin — all four actually reach orbit |
| `main.go` | `App` — the three-screen state machine, `ebiten.Game` |
| `theme.go` | Palette and fonts (goregular/gomono, compiled in, no asset files on disk) |
| `ui.go` | Immediate-mode toolkit: `NumField`, `Button`, `Radio`, `Checkbox`, `Dropdown`, `Scroll` |
| `lang.go` | Locale loading and lookup, RU/EN switching, dispatch for events, verdicts, phases, presets |
| `assets/locale/*.json` | All interface text, one file per language, flat dotted keys |
| `render.go` | `Rect`, primitives, `Camera` (world metres → pixels, with rotation) |
| `screen_setup.go` | Four-column parameter form, keyframe editor, derived figures, presets |
| `screen_flight.go` | Trajectory, launch pad, automatic camera, telemetry panel, time controls |
| `screen_graphs.go` | Seven plots on a shared time axis, event ruler, scrubber |
| `shot.go` | Scripted run for screenshots |

## The system of bodies

The world is a `sim.System`: a flat slice of bodies forming a tree, root at index 0. The root does not
move. Everything else runs on **Keplerian rails** — `StateAt(i, t)` is analytic, so a jump of three days
costs no more than a jump of a second and nothing drifts. `Config.System` empty means a system of one
body built from `Config.Body`, which is what every single-planet configuration is.

- **A body's parent always sits at a lower index.** That one invariant makes a cycle impossible to
  express, makes every walk up the tree terminate by construction, and leaves the slice in topological
  order. `Normalize` enforces it, clamping bad data to the root rather than trusting the author.
- **`Config.Body` is a mirror, not an input, once `New` has run.** It is a copy of
  `System.Bodies[LaunchBody]`, kept so that everything already reading `Cfg.Body` — the whole interface —
  still gets the planet being launched from. Edit the system, not the mirror.
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

- **The integrator step is fixed, `sim.FixedStep` = 0.02 s.** Time warp runs more steps per frame rather
  than a longer step, so the trajectory depends on neither the frame rate nor the ×500 setting.
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

## Frames of reference — easy to get lost in

Integration happens in the **inertial** frame centred on the planet. But the rocket stands on a rotating
pad and carries its 465 m/s away with it, so in the inertial frame it "drifts" 6 km sideways over the
first 15 seconds while climbing 400 m. Physically correct, reads as the rocket being blown off course.

- **Downrange** is measured from the pad in the frame rotating with the planet: the launch site's angle
  is advanced by `ω·t`. Without that the field reported the planet's own rotation as distance flown.
- **The trail and the event markers** are drawn through `Sim.GroundFrame` — a sample taken at time `t`
  is rotated forward by `ω·(T−t)`. The current point does not move, so the orbit ellipse and the rings,
  which all refer to instant `T`, stay consistent. In orbit the trail lags the ellipse by `ω·T` (2° over
  500 s) — that is the ground track, and it should.
- **`Telemetry.Speed` is inertial; `SurfSpeed`/`VertSpeed`/`HorizSpeed` are relative to the ground.**
  The panel shows both. Mixing them in one column produces 475 m/s next to a vertical 65 and a
  horizontal 6.

## Interface language

All interface text lives in `assets/locale/ru.json` and `assets/locale/en.json`, embedded into the
binary with `go:embed`, and is fetched by key: `T("flight.downrange")`. A picker sits in the setup
header and in the bottom bars of the flight and graph screens.

- **The `sim` package holds no text at all.** Events carry only a `Kind`, verdicts only an `Outcome`,
  presets an identifier (`earth-falcon`). Labels come from `lang.go`. The physics must not know about
  language.
- **A missing key renders as the key itself, not as nothing**, so a typo shows up the moment the screen
  is opened. The same goes for a format string that lost a verb: `Sprintf` does not panic, it writes
  `%!(EXTRA string=11.0)` into the text. Both mistakes announce themselves on screen, which is why
  `lang_test.go` no longer tries to catch them ahead of time — that took a source scanner that could
  not tell code from comments, and a second pass over every format string, to buy very little.
- **`lang_test.go` checks the locale files against each other and nothing else**: the same key set in
  every language, and no blank values. Orphaned keys are left to be noticed in passing.
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

## Rendering — traps

- **The camera is rotated:** `Camera.Rot` is the angle of the local vertical, so "up" on screen points
  away from the planet. The launch site sits on the +X axis, and without the rotation the ascent would
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
