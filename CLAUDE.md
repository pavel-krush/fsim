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
go run . -en                   # start with the interface in English
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
| `ui.go` | Immediate-mode toolkit: `NumField`, `Button`, `Radio`, `Checkbox`, `Scroll` |
| `lang.go` | Locale loading and lookup, RU/EN switching, dispatch for events, verdicts, phases, presets |
| `assets/locale/*.json` | All interface text, one file per language, flat dotted keys |
| `render.go` | `Rect`, primitives, `Camera` (world metres → pixels, with rotation) |
| `screen_setup.go` | Four-column parameter form, keyframe editor, derived figures, presets |
| `screen_flight.go` | Trajectory, launch pad, automatic camera, telemetry panel, time controls |
| `screen_graphs.go` | Seven plots on a shared time axis, event ruler, scrubber |
| `shot.go` | Scripted run for screenshots |

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
binary with `go:embed`, and is fetched by key: `T("flight.downrange")`. A toggle sits in the setup
header and in the bottom bars of the flight and graph screens.

- **The `sim` package holds no text at all.** Events carry only a `Kind`, verdicts only an `Outcome`,
  presets an identifier (`earth-falcon`). Labels come from `lang.go`. The physics must not know about
  language.
- **A string key gives up the compiler's help, so `lang_test.go` takes the job over.** It scans the
  source for every `T("...")` and checks the key exists, that both languages carry the same key set,
  that no value is blank, that no key is orphaned, and that a format string takes the same verbs in
  every language. That last one matters: a translation that loses a `%s` panics at print time, on
  whichever screen happens to use it.
- **A missing key renders as `«key»`, not as nothing.** An empty gap reads as a layout bug; the key in
  guillemets reads as what it is.
- **Text assembled from fragments must be a whole format string.** Word order differs between
  languages, so the max-q readout is `"%s на %s км"` / `"%s at %s km"` rather than a label glued to a
  unit. Same for anything numbered: `"СТУПЕНЬ %d"` / `"STAGE %d"`.
- **Cache nothing at screen construction.** The plot captions used to be computed in `NewGraphScreen`,
  and switching language did not relabel them. `plotSeries()` is now called every frame.
- The switch button keeps each language written in its own script (`РУС` / `ENG`), which is the one
  piece of display text deliberately kept out of the locale files.
- CLI flags and log messages stay in English: that is a machine-facing surface, not the interface.
- A third language is one more file plus an entry in `localeFile`; the tests will list every key it is
  missing.

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
- **The number of atmosphere bands follows the scale.** From orbit the whole atmosphere is a few pixels
  deep, and sixteen sub-pixel rings simply vanish.
- **The toolkit identifies a widget by the address of the value it edits.** Do not bind `NumField` to a
  local variable — the address changes every frame and focus is lost. That is why the diameter field is
  bound straight to `Body.Radius` with `Scale: 500`.
- **The whole interface is built in `Update`, not in `Draw`.** The toolkit is immediate mode and reads
  just-pressed input; Ebiten calls `Draw` less often than `Update`, and clicks would be dropped. `Draw`
  only blits the canvas.

## Presets

The pitch programmes and the second-stage cutoffs were found by search (a profile generator,
`pitch = 90·(1-f)^p`, plus a cutoff sweep minimising the deviation from the circular target). The tuner
lived in `sim/zz_tune_test.go` and has been deleted — if the presets ever drift, writing it again beats
tuning by hand.

Earth/Falcon-9: a 304/239 km orbit, Δv 8995 m/s, max q 43 kPa at 11 km, peak 5.9 g. The Δv is below the
real-world 9.3–9.5 km/s because launching from the equator hands over all 465 m/s of rotation.

Kerbin needed its second stage set to ignite **at apoapsis**: a 600 km planet wearing an Earth-thick
70 km atmosphere does not yield to direct ascent on a single burn.
