package main

import (
	"fmt"
	"image"
	"image/png"
	"log"
	"math"
	"os"
	"path/filepath"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/pavel-krush/fsim/sim"
)

// Screenshot mode. Ebiten images can only be created and read inside a running
// game loop, so there is no way to render the interface headlessly. This flag
// drives the real loop through a scripted sequence instead, dumping a PNG at
// each step, which is what makes the UI reviewable without a human at the
// keyboard.

// shotTimeline is when things happened in a throwaway run of the very same
// flight. The script needs it because a moment cannot be named in seconds and
// still mean anything across presets: T+232000 is a lunar flyby on one and an
// idle coast on another, and T+45 is max q on a Saturn V and nowhere near it on
// Titan, where the air is four times as thick and the vehicle climbs for seven
// minutes before it turns.
//
// The flight is deterministic, so every instant this finds is an instant the
// captured run will hit exactly.
type shotTimeline struct {
	events []sim.Event
	nodes  []sim.Node
	// end is the last moment of the flight: the verdict for one that finishes and
	// the time limit for one that settles into an orbit and stays there.
	end float64
	// crossing is the body named by the first sphere-of-influence event, which is
	// what a mission that goes anywhere is going to. -1 when there is none.
	crossing int
}

func newShotTimeline(cfg sim.Config) *shotTimeline {
	s := sim.New(cfg)
	limit := s.Cfg.MaxTime
	if limit <= 0 {
		limit = 6 * 3600
	}
	s.FastForward(limit)

	tl := &shotTimeline{events: s.Events, nodes: s.Cfg.Nodes, end: s.St.T, crossing: -1}
	// An arrival names the body arrived at, which is what the mission is about. The
	// first crossing of an interplanetary flight is the *departure* from home, so
	// taking whichever came first pointed the camera at the Earth for the whole of
	// the Mars mission. A flight with no arrival at all — launched from a moon and
	// leaving it — is going to its parent.
	for _, e := range tl.events {
		if e.Kind == sim.EvSOIEnter {
			tl.crossing = e.Body
			break
		}
	}
	if tl.crossing < 0 {
		for _, e := range tl.events {
			if e.Kind == sim.EvSOIExit {
				tl.crossing = s.Cfg.System.Bodies[e.Body].Parent
				break
			}
		}
	}
	return tl
}

// at finds the first event of a kind. The second return is false when the flight
// never had one, which is the normal case rather than an error: an airless body
// has no max q, a single-planet system has no sphere-of-influence crossings, and
// a step that asks about either is simply not available on that preset.
func (tl *shotTimeline) at(k sim.EventKind) (float64, bool) {
	for _, e := range tl.events {
		if e.Kind == k {
			return e.T, true
		}
	}
	return 0, false
}

// shotAt resolves the instant a step captures. False means the flight has no such
// instant and the step is skipped.
type shotAt func(*shotTimeline) (float64, bool)

// atTime is for the handful of moments that really are a number of seconds: the
// pad, and a few seconds off it.
func atTime(t float64) shotAt {
	return func(*shotTimeline) (float64, bool) { return t, true }
}

// afterEvent is off seconds after the first event of a kind.
func afterEvent(k sim.EventKind, off float64) shotAt {
	return func(tl *shotTimeline) (float64, bool) {
		t, ok := tl.at(k)
		return t + off, ok
	}
}

// atFlyby is half way through the first stay inside another body's sphere of
// influence — near enough the closest approach, and the only way to say "the
// flyby" without knowing which body it is or when the vehicle gets there. A
// vehicle that arrives and stays has no exit, so the stay runs to the end.
func atFlyby() shotAt {
	return func(tl *shotTimeline) (float64, bool) {
		enter, ok := tl.at(sim.EvSOIEnter)
		if !ok {
			return 0, false
		}
		out := tl.end
		if exit, ok := tl.at(sim.EvSOIExit); ok && exit > enter {
			out = exit
		}
		return enter + (out-enter)/2, true
	}
}

// atCoast is a while after insertion, going round and with nothing happening yet:
// half way to the first burn where there is one, three hours in where there is
// not. Anchored to what is either side of it rather than to a number of seconds,
// because the first burn is at T+2000 s on one preset and T+15325 on another, and
// a fixed jump landed the coast capture after the burn it was supposed to precede.
func atCoast() shotAt {
	return func(tl *shotTimeline) (float64, bool) {
		orbit, ok := tl.at(sim.EvOrbit)
		if !ok {
			return 0, false
		}
		if len(tl.nodes) > 0 && tl.nodes[0].T > orbit {
			return orbit + (tl.nodes[0].T-orbit)/2, true
		}
		return orbit + 11000, true
	}
}

// atCruise is the middle of the coast between the departure burn and the arrival,
// which on the interplanetary presets is three months of flight with nothing in it
// — and the only phase the script had no step for, so the one place a drawing bug
// could live unseen.
func atCruise() shotAt {
	return func(tl *shotTimeline) (float64, bool) {
		enter, ok := tl.at(sim.EvSOIEnter)
		if !ok || len(tl.nodes) == 0 {
			return 0, false
		}
		from := tl.nodes[0].T
		if from >= enter {
			return 0, false
		}
		return from + (enter-from)/2, true
	}
}

// beforeArrival is the approach: three hours before the vehicle enters the target's
// sphere of influence, or the last eighth of the coast when the coast is shorter
// than a day. Neither on its own works for both — three hours before the Mun is
// before the transfer's own midpoint, and an eighth of the way to Mars is
// twenty-three days out with the planet nowhere in the picture.
func beforeArrival() shotAt {
	return func(tl *shotTimeline) (float64, bool) {
		enter, ok := tl.at(sim.EvSOIEnter)
		if !ok {
			return 0, false
		}
		lead := 3 * 3600.0
		if len(tl.nodes) > 0 && tl.nodes[0].T < enter {
			if eighth := (enter - tl.nodes[0].T) / 8; eighth < lead {
				lead = eighth
			}
		}
		return enter - lead, true
	}
}

// atNode is off seconds either side of a scheduled burn, negative for before it:
// the manoeuvre panel and the predicted path are only worth a capture while the
// burn is still ahead.
func atNode(i int, off float64) shotAt {
	return func(tl *shotTimeline) (float64, bool) {
		if i < 0 || i >= len(tl.nodes) {
			return 0, false
		}
		return tl.nodes[i].T + off, true
	}
}

// atEnd is where the flight got to, which is where the verdict is.
func atEnd() shotAt {
	return func(tl *shotTimeline) (float64, bool) { return tl.end, true }
}

// resolve turns a step's moment into a time in this flight, clamped to the end of
// it. A step that asks for something past the last moment gets the last moment,
// which is where the verdict is anyway: the three-hour presets have no T+11000 s
// to show.
func (tl *shotTimeline) resolve(st shotStep) (float64, bool) {
	if st.at == nil {
		return 0, false
	}
	t, ok := st.at(tl)
	if !ok {
		return 0, false
	}
	return math.Min(t, tl.end), true
}

// shotStep is one scripted frame capture.
type shotStep struct {
	name string
	// at is the moment of flight to capture, resolved against the timeline. A step
	// that cannot be resolved is skipped, and a step with no at captures wherever
	// the previous one left off. Nothing ever rewinds: the times below are in
	// order for every preset, and FastForward only goes forwards anyway.
	at     shotAt
	screen Screen
	graphs bool
	// openLang forces the language picker open, and hover parks the pointer at
	// a fixed spot. Neither state can be reached by a scripted run otherwise:
	// there is no mouse.
	openLang   bool
	openMix    bool
	openBody   bool
	openPreset bool
	hover      *struct{ X, Y float64 }
	// stages rebuilds the vehicle to this many stages, and scrollRocket winds
	// the vehicle column down, which is the only way a capture can show the
	// stages a two-stage preset does not have.
	stages       int
	scrollRocket float64
	// selBody names the body the setup screen's first column edits. A name the
	// system does not have falls back to the last body in it, which is a moon
	// wherever there is one — the orbital elements only exist off the root.
	selBody string
	// zoom multiplies the camera's automatic scale and focusBody pins the camera,
	// which is how the ladder from the pad out to the Moon gets captured: neither
	// is reachable without a mouse and a Tab key.
	//
	// focusBody is a name rather than an index, because an index means a different
	// body in every system — nine was the Moon until Mars grew two of its own, and
	// there is no index ten at all in a system of two. Three names are special:
	// "root" is whatever the system hangs from, "crossing" is the body this mission
	// is going to, and "soi" is whichever one holds the vehicle at that instant.
	zoom      float64
	focusBody string
	// graphAscent zooms the graph screen's time axis onto the launch, which on a
	// four-day flight is the first two pixels of it.
	graphAscent bool
	// freeHalfway drops the camera into a free view, which no script can reach by
	// dragging: there is no mouse.
	freeHalfway bool
}

type shotRunner struct {
	dir   string
	tl    *shotTimeline
	steps []shotStep
	i     int
	warm  int
	// saved is whether the step just performed produced a capture. A step whose
	// moment or body the preset does not have is passed over, and passing over a
	// step must not write the previous frame out under its name.
	saved bool
}

func newShotRunner(dir string, cfg sim.Config) *shotRunner {
	return &shotRunner{
		dir: dir,
		tl:  newShotTimeline(cfg),
		steps: []shotStep{
			// The mission list, which is where a run without -preset begins.
			{name: "0-presets", screen: ScreenPresets},
			{name: "1-setup", screen: ScreenSetup},
			{name: "1b-setup-lang", screen: ScreenSetup, openLang: true},
			{name: "1b2-setup-presets", screen: ScreenSetup, openPreset: true},
			{name: "1c-setup-info", screen: ScreenSetup, hover: &struct{ X, Y float64 }{914, 105}},
			{name: "1d-setup-info-atmo", screen: ScreenSetup, hover: &struct{ X, Y float64 }{542, 127}},
			{name: "1e-setup-info-low", screen: ScreenSetup, hover: &struct{ X, Y float64 }{914, 355}},
			{name: "1f-setup-info-gas", screen: ScreenSetup, hover: &struct{ X, Y float64 }{542, 289}},
			{name: "1g-setup-mixture", screen: ScreenSetup, openMix: true},
			{name: "2-pad", screen: ScreenFlight, at: atTime(2)},
			{name: "3-liftoff", screen: ScreenFlight, at: atTime(18)},
			// The peak is only knowable in hindsight, so the event is inserted at
			// the instant it happened and this lands exactly on it. Skipped where
			// there is no air to have a peak in.
			{name: "4-maxq", screen: ScreenFlight, at: afterEvent(sim.EvMaxQ, 0)},
			{name: "4b-flight-lang", screen: ScreenFlight, openLang: true},
			{name: "5-staging", screen: ScreenFlight, at: afterEvent(sim.EvSeparation, 2)},
			// Insertion is the verdict, whenever the preset happens to reach it:
			// T+546 s on the Mars stack and T+2082 on Titan.
			{name: "6-insertion", screen: ScreenFlight, at: afterEvent(sim.EvOrbit, 0)},
			{name: "7-orbit", screen: ScreenFlight, at: afterEvent(sim.EvOrbit, 300)},
			{name: "7b-orbiting", screen: ScreenFlight, at: atCoast()},
			{name: "8-graphs", screen: ScreenGraphs, graphs: true},
			// The ladder of scales, all of the same instant in the same flight:
			// the pad, the planet, the target's orbit, and the target itself.
			{name: "8a-zoom-planet", screen: ScreenFlight, zoom: 0.15},
			{name: "8b-zoom-system", screen: ScreenFlight, zoom: 0.015},
			{name: "8c-focus-target", screen: ScreenFlight, focusBody: "crossing"},
			// A dragged view: following nothing, centred half way to the target.
			{name: "8c2-free-view", screen: ScreenFlight, zoom: 0.02, freeHalfway: true},
			// The burn the preset ships with, before it fires: the panel shows it
			// and the prediction shows where it goes.
			{name: "8d-plan", screen: ScreenFlight, at: atNode(0, -60), zoom: 0.06},
			{name: "8e-plan-wide", screen: ScreenFlight, at: atNode(0, -60), zoom: 0.012},
			// The middle of the cruise, at both scales that make sense there: the
			// system, and the frame the vehicle is actually in.
			{name: "8e2-cruise", screen: ScreenFlight, at: atCruise(), focusBody: "root", zoom: 0.004},
			{name: "8e3-cruise-close", screen: ScreenFlight, at: atCruise(), focusBody: "soi", zoom: 0.5},
			// And after it: three hours out from the target, in its own frame.
			{name: "8f-approach", screen: ScreenFlight,
				at: beforeArrival(), focusBody: "crossing", zoom: 0.4},
			{name: "8g-flyby", screen: ScreenFlight, at: atFlyby(), focusBody: "crossing", zoom: 3},
			// The camera on the Sun, at two scales: the inner system and the lot.
			{name: "8k-inner-system", screen: ScreenFlight, focusBody: "sun", zoom: 0.008},
			{name: "8l-outer-system", screen: ScreenFlight, focusBody: "sun", zoom: 0.0004},
			// Saturn, close enough for the rings.
			{name: "8m-saturn", screen: ScreenFlight, focusBody: "saturn", zoom: 40},
			// Where the flight got to, which is where the verdict is: lunar orbit on
			// one preset, an entry corridor on another, and a parking orbit going
			// round for ever on the ones that end at insertion.
			{name: "8n-arrival", screen: ScreenFlight, at: atEnd(), focusBody: "soi", zoom: 6},
			{name: "8h-late", screen: ScreenFlight, at: atEnd(), zoom: 0.02},
			// The whole flight on one time axis, and then the same axis on the ten
			// minutes of it that the ascent took.
			{name: "8i-graphs-full", screen: ScreenGraphs, graphs: true},
			{name: "8j-graphs-ascent", screen: ScreenGraphs, graphs: true, graphAscent: true},
			// Last, because they edit the configuration: a four-stage vehicle
			// assembled out of a two-stage preset is not something the flight
			// captures above should be flying.
			// The body editor, on a moon rather than on the launch body: that is
			// where the orbital elements are.
			{name: "9c-setup-body", screen: ScreenSetup, selBody: "moon"},
			{name: "9d-setup-bodylist", screen: ScreenSetup, selBody: "moon", openBody: true},
			{name: "9-setup-4stage", screen: ScreenSetup, stages: 4},
			{name: "9b-setup-4stage-bottom", screen: ScreenSetup, stages: 4, scrollRocket: 1e5},
		},
	}
}

// step performs the next scripted action and saves the canvas. It returns
// false once the script is exhausted.
func (sr *shotRunner) step(a *App) bool {
	if sr.i >= len(sr.steps) {
		return false
	}
	// Let the first frames settle so fonts and the camera are warmed up.
	if sr.warm < 2 {
		sr.warm++
		return true
	}

	st := sr.steps[sr.i]
	sr.i++
	sr.saved = false

	// Resolve everything the step asks of the flight before touching the app: a
	// step that wants a moment, a body or a burn this preset does not have is
	// passed over rather than captured at whatever happens to be on screen.
	target, wants := sr.tl.resolve(st)
	if st.at != nil && !wants {
		return true
	}
	// Whether a body exists does not depend on when, so the skip is decided here
	// and the index taken again after the jump — "soi" means the frame the vehicle
	// is in at the instant being captured, and this early it is still in the
	// previous one.
	switch st.focusBody {
	case "", "soi", "root":
	default:
		if _, ok := sr.body(a, st.focusBody); !ok {
			return true
		}
	}
	if st.freeHalfway && sr.tl.crossing < 0 {
		return true
	}

	switch {
	case st.screen == ScreenFlight && a.flight == nil:
		a.Launch()
		a.flight.paused = true
	case st.graphs:
		a.ShowGraphs(a.flight.s)
	default:
		a.screen = st.screen
	}
	if wants && a.flight != nil {
		// FastForward, not Advance: a scripted jump of four hours is not a frame,
		// and Advance would give up after one frame's worth of steps. It takes an
		// instant rather than a duration and never runs backwards, so a step that
		// resolves to something already past simply captures where the flight is.
		a.flight.s.FastForward(target)
		// Settle the camera instead of easing it, or the capture lands half way
		// through a zoom, a change of frame, or a rotation.
		a.flight.snapCamera()
	}

	if a.flight != nil {
		a.flight.manualScale = false
		if st.zoom > 0 {
			a.flight.pendingZoom = st.zoom
		}
		focus := -1 // the vehicle, unless the step names something
		if st.focusBody != "" {
			if i, ok := sr.body(a, st.focusBody); ok {
				focus = i
			}
		}
		a.flight.lookAt(focus)
		if st.freeHalfway {
			a.flight.follow = camFree
			a.flight.freePos = a.flight.framePoint(sim.Vec2{}, sr.tl.crossing, a.flight.s.St.T).Scale(0.5)
		}
		// Nothing here writes Cfg.Nodes. A step used to be able to bring its own
		// plan, and the assignment ran on every step, which wiped the one the
		// preset ships with: the translunar burn quietly failed to happen and the
		// deep-space captures all came out in low Earth orbit.
		a.flight.pred = nil
		a.flight.snapCamera()
	}

	for st.stages > 0 && len(a.cfg.Rocket.Stages) > st.stages {
		removeStage(&a.cfg.Rocket, len(a.cfg.Rocket.Stages)-1)
	}
	for st.stages > 0 && len(a.cfg.Rocket.Stages) < st.stages {
		addStage(&a.cfg.Rocket)
	}
	if st.scrollRocket > 0 {
		a.setup.colRocket.Offset = st.scrollRocket
	}

	if st.selBody != "" {
		// A named body if the system has one, otherwise the last of them — which is
		// a moon in any system that has one, and nothing to edit in a system of one.
		sys := &a.cfg.System
		i := sys.IndexOf(st.selBody)
		if i < 0 {
			i = len(sys.Bodies) - 1
		}
		if i <= 0 {
			return true
		}
		a.setup.selBody = i
	}
	if st.graphAscent && a.graphs != nil {
		a.graphs.showAscent()
	}

	switch {
	case st.openLang:
		a.ui.openList = "language"
	case st.openMix:
		a.ui.openList = "mixture"
	case st.openBody:
		a.ui.openList = "body"
	case st.openPreset:
		a.ui.openList = "preset"
	default:
		a.ui.openList = nil
	}
	a.ui.ForcePointer = st.hover

	sr.saved = true
	return true
}

// body resolves a step's focus name to an index. "root" is whatever the system
// hangs from, "crossing" is the body the mission is going to, "soi" is the one
// holding the vehicle right now, and anything else is a name to look up.
func (sr *shotRunner) body(a *App, name string) (int, bool) {
	switch name {
	case "root":
		return 0, true
	case "crossing":
		return sr.tl.crossing, sr.tl.crossing >= 0
	case "soi":
		if a.flight == nil {
			return -1, false
		}
		return a.flight.s.St.Center, true
	}
	i := a.cfg.System.IndexOf(name)
	return i, i >= 0
}

// save writes the current canvas to disk.
func (sr *shotRunner) save(a *App) {
	if sr.warm < 2 || sr.i == 0 || sr.i > len(sr.steps) || !sr.saved {
		return
	}
	name := sr.steps[sr.i-1].name
	b := a.canvas.Bounds()
	img := image.NewRGBA(b)
	a.canvas.ReadPixels(img.Pix)

	path := filepath.Join(sr.dir, name+".png")
	f, err := os.Create(path)
	if err != nil {
		log.Printf("screenshot %s: %v", path, err)
		return
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		log.Printf("screenshot %s: %v", path, err)
		return
	}
	fmt.Println("saved", path)
}

// runShots drives the app through the capture script and quits.
func (a *App) runShots() error {
	if !a.shots.step(a) {
		return ebiten.Termination
	}
	return nil
}

// camTrace prints the projected screen position of the vehicle and the launch
// pad on every frame of a live run. Camera shake is invisible in a still, so
// this is how it gets measured: a steady camera keeps the vehicle's pixel
// coordinates moving monotonically instead of oscillating.
func (a *App) camTrace() error {
	if a.flight == nil {
		a.Launch()
	}
	f := a.flight
	x, y := f.cam.Project(f.s.St.Pos)
	px, py := f.cam.Project(f.s.PadPos())
	fmt.Printf("%7.3f  rocket %8.2f %8.2f   pad %8.2f %8.2f\n", f.s.St.T, x, y, px, py)

	a.traceLeft--
	if a.traceLeft <= 0 {
		return ebiten.Termination
	}
	return nil
}
