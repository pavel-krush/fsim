package main

import (
	"fmt"
	"image"
	"image/png"
	"log"
	"os"
	"path/filepath"

	"github.com/hajimehoshi/ebiten/v2"

	"fsim/sim"
)

// Screenshot mode. Ebiten images can only be created and read inside a running
// game loop, so there is no way to render the interface headlessly. This flag
// drives the real loop through a scripted sequence instead, dumping a PNG at
// each step, which is what makes the UI reviewable without a human at the
// keyboard.

// shotStep is one scripted frame capture.
type shotStep struct {
	name string
	// advance is how many seconds of flight to fast-forward before capturing.
	advance float64
	screen  Screen
	graphs  bool
	// openLang forces the language picker open, and hover parks the pointer at
	// a fixed spot. Neither state can be reached by a scripted run otherwise:
	// there is no mouse.
	openLang bool
	openMix  bool
	hover    *struct{ X, Y float64 }
	// stages rebuilds the vehicle to this many stages, and scrollRocket winds
	// the vehicle column down, which is the only way a capture can show the
	// stages a two-stage preset does not have.
	stages       int
	scrollRocket float64
	// zoom multiplies the camera's automatic scale and focus pins a body, which
	// is how the ladder from the pad out to the Moon gets captured: neither is
	// reachable without a mouse and a Tab key. focus is one-based, because the
	// zero value has to mean "leave it on the vehicle" and body zero is a
	// perfectly good thing to look at.
	zoom  float64
	focus int
	// plan drops a flight plan onto the running simulation, which is the only
	// way a script can show the manoeuvre panel and the predicted path.
	plan []sim.Node
	// graphAscent zooms the graph screen's time axis onto the launch, which on a
	// four-day flight is the first two pixels of it.
	graphAscent bool
}

type shotRunner struct {
	dir   string
	steps []shotStep
	i     int
	warm  int
}

func newShotRunner(dir string) *shotRunner {
	return &shotRunner{
		dir: dir,
		steps: []shotStep{
			{name: "1-setup", screen: ScreenSetup},
			{name: "1b-setup-lang", screen: ScreenSetup, openLang: true},
			{name: "1c-setup-info", screen: ScreenSetup, hover: &struct{ X, Y float64 }{914, 105}},
			{name: "1d-setup-info-atmo", screen: ScreenSetup, hover: &struct{ X, Y float64 }{542, 127}},
			{name: "1e-setup-info-low", screen: ScreenSetup, hover: &struct{ X, Y float64 }{914, 355}},
			{name: "1f-setup-info-gas", screen: ScreenSetup, hover: &struct{ X, Y float64 }{542, 289}},
			{name: "1g-setup-mixture", screen: ScreenSetup, openMix: true},
			{name: "2-pad", screen: ScreenFlight, advance: 2},
			{name: "3-liftoff", screen: ScreenFlight, advance: 18},
			{name: "4-maxq", screen: ScreenFlight, advance: 45},
			{name: "4b-flight-lang", screen: ScreenFlight, openLang: true},
			{name: "5-staging", screen: ScreenFlight, advance: 100},
			{name: "6-insertion", screen: ScreenFlight, advance: 400},
			{name: "7-orbit", screen: ScreenFlight, advance: 900},
			{name: "7b-orbiting", screen: ScreenFlight, advance: 12000},
			{name: "8-graphs", screen: ScreenGraphs, graphs: true},
			// The ladder of scales, all of the same instant in the same flight:
			// the pad, the planet, the Moon's orbit, and the Moon itself.
			{name: "8a-zoom-planet", screen: ScreenFlight, zoom: 0.15},
			{name: "8b-zoom-system", screen: ScreenFlight, zoom: 0.015},
			{name: "8c-focus-moon", screen: ScreenFlight, focus: 2},
			// The translunar injection the preset ships with, before it fires: the
			// panel shows the burn and the prediction shows where it goes.
			{name: "8d-plan", screen: ScreenFlight, zoom: 0.06},
			{name: "8e-plan-wide", screen: ScreenFlight, zoom: 0.012},
			// And after it: two and a half days out, in the Moon's own frame.
			{name: "8f-lunar-approach", screen: ScreenFlight, advance: 227000, focus: 10, zoom: 0.4},
			{name: "8g-lunar-flyby", screen: ScreenFlight, advance: 232000, focus: 10, zoom: 3},
			// Four days of flight on one axis, and then the same axis on the ten
			// minutes of it that the ascent took.
			{name: "8h-late", screen: ScreenFlight, advance: 360000, zoom: 0.02},
			{name: "8i-graphs-lunar", screen: ScreenGraphs, graphs: true},
			{name: "8j-graphs-ascent", screen: ScreenGraphs, graphs: true, graphAscent: true},
			// Last, because they edit the configuration: a four-stage vehicle
			// assembled out of a two-stage preset is not something the flight
			// captures above should be flying.
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
	switch {
	case st.screen == ScreenFlight && a.flight == nil:
		a.Launch()
		a.flight.paused = true
	case st.graphs:
		a.ShowGraphs(a.flight.s)
	default:
		a.screen = st.screen
	}
	if st.advance > 0 && a.flight != nil {
		// FastForward, not Advance: a scripted jump of four hours is not a frame,
		// and Advance would give up after one frame's worth of steps.
		a.flight.s.FastForward(st.advance)
		// Snap the camera instead of easing, so the capture is not mid-zoom.
		a.flight.cam.Scale = 0
	}

	if a.flight != nil {
		a.flight.zoomBias = 1
		if st.zoom > 0 {
			a.flight.zoomBias = st.zoom
		}
		a.flight.focus = st.focus - 1
		// Only when a step brings its own, or every other step would wipe the plan
		// the preset ships with — which is how the translunar burn quietly failed
		// to happen and the capture shots came out in low Earth orbit.
		if st.plan != nil {
			a.flight.s.Cfg.Nodes = st.plan
		}
		a.flight.pred = nil
		a.flight.cam.Scale = 0 // snap, so the capture is not mid-zoom
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

	if st.graphAscent && a.graphs != nil {
		a.graphs.showAscent()
	}

	switch {
	case st.openLang:
		a.ui.openList = "language"
	case st.openMix:
		a.ui.openList = "mixture"
	default:
		a.ui.openList = nil
	}
	a.ui.ForcePointer = st.hover

	sr.i++
	return true
}

// save writes the current canvas to disk.
func (sr *shotRunner) save(a *App) {
	if sr.warm < 2 || sr.i == 0 || sr.i > len(sr.steps) {
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
