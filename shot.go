package main

import (
	"fmt"
	"image"
	"image/png"
	"log"
	"os"
	"path/filepath"

	"github.com/hajimehoshi/ebiten/v2"
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
	// openLang forces the language picker open. Its list only exists while the
	// pointer is on it, so a scripted run has no other way to photograph it.
	openLang bool
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
			{name: "2-pad", screen: ScreenFlight, advance: 2},
			{name: "3-liftoff", screen: ScreenFlight, advance: 18},
			{name: "4-maxq", screen: ScreenFlight, advance: 45},
			{name: "4b-flight-lang", screen: ScreenFlight, openLang: true},
			{name: "5-staging", screen: ScreenFlight, advance: 100},
			{name: "6-insertion", screen: ScreenFlight, advance: 400},
			{name: "7-orbit", screen: ScreenFlight, advance: 900},
			{name: "8-graphs", screen: ScreenGraphs, graphs: true},
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
		a.flight.s.Advance(st.advance - a.flight.s.St.T)
		// Snap the camera instead of easing, so the capture is not mid-zoom.
		a.flight.cam.Scale = 0
	}

	if st.openLang {
		a.ui.openList = "language"
	} else {
		a.ui.openList = nil
	}

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
