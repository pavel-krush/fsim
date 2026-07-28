package main

import (
	"errors"
	"flag"
	"log"

	"github.com/hajimehoshi/ebiten/v2"

	"fsim/sim"
)

const (
	winW = 1500
	winH = 940
)

// Screen is which page the app is showing.
type Screen int

const (
	ScreenSetup Screen = iota
	ScreenFlight
	ScreenGraphs
)

// App is the whole program: a configuration being edited, a simulation being
// flown, and the three screens that show them.
type App struct {
	ui     *UI
	screen Screen

	cfg    sim.Config
	setup  *SetupScreen
	flight *FlightScreen
	graphs *GraphScreen

	canvas *ebiten.Image
	w, h   int

	shots     *shotRunner // non-nil only in screenshot mode
	traceLeft int         // frames left to print camera coordinates for
}

// Ebiten calls Update at a fixed rate and Draw only when it needs a new frame.
// The toolkit is immediate mode and reads just-pressed input, so the whole
// interface is built during Update into an offscreen canvas that Draw blits.
// That keeps input sampling and drawing in lockstep at exactly one to one.
func (a *App) Update() error {
	if a.canvas == nil || a.w == 0 {
		return nil
	}
	if a.shots != nil {
		if err := a.runShots(); err != nil {
			return err
		}
	}
	if a.traceLeft > 0 {
		if err := a.camTrace(); err != nil {
			return err
		}
	}

	a.ui.BeginFrame(1.0/float64(ebiten.TPS()), a.Bounds())
	a.canvas.Fill(colBG)

	switch a.screen {
	case ScreenSetup:
		a.setup.Update(a, a.canvas)
	case ScreenFlight:
		a.flight.Update(a, a.canvas)
	case ScreenGraphs:
		a.graphs.Update(a, a.canvas)
	}

	a.ui.EndFrame()
	if a.shots != nil {
		a.shots.save(a)
	}
	return nil
}

func (a *App) Draw(screen *ebiten.Image) {
	if a.canvas == nil {
		return
	}
	screen.DrawImage(a.canvas, nil)
}

func (a *App) Layout(outW, outH int) (int, int) {
	if outW != a.w || outH != a.h {
		a.w, a.h = outW, outH
		if a.canvas != nil {
			a.canvas.Deallocate()
		}
		a.canvas = ebiten.NewImage(outW, outH)
	}
	return outW, outH
}

// Bounds is the full drawing area.
func (a *App) Bounds() Rect { return Rect{0, 0, float64(a.w), float64(a.h)} }

// Launch builds a simulation from the current configuration and switches to
// the flight screen.
func (a *App) Launch() {
	a.flight = NewFlightScreen(sim.New(a.cfg))
	a.screen = ScreenFlight
}

// ShowGraphs hands the finished simulation to the graph screen.
func (a *App) ShowGraphs(s *sim.Sim) {
	s.MarkMaxQ()
	a.graphs = NewGraphScreen(s)
	a.screen = ScreenGraphs
}

func main() {
	shotDir := flag.String("shot", "", "run the capture script and save screenshots of every screen into this directory")
	preset := flag.Int("preset", 0, "index of the preset to start from (0 is the first)")
	camTrace := flag.Int("camtrace", 0, "print the vehicle's screen coordinates for N frames of live flight")
	langCode := flag.String("lang", "en", "interface language to start in: en or ru")
	flag.Parse()

	loadLocales()
	l, ok := localeCode[*langCode]
	if !ok {
		log.Fatalf("unknown language %q; expected one of en, ru", *langCode)
	}
	lang = l

	initFonts()

	presets := sim.Presets()
	cfg := sim.DefaultConfig()
	if *preset > 0 && *preset < len(presets) {
		cfg = presets[*preset].Cfg
	}

	app := &App{
		ui:  NewUI(),
		cfg: cfg,
	}
	app.setup = NewSetupScreen()
	if *shotDir != "" {
		app.shots = newShotRunner(*shotDir)
	}
	app.traceLeft = *camTrace

	ebiten.SetWindowSize(winW, winH)
	ebiten.SetWindowTitle("fsim — orbital launch simulator")
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)

	if err := ebiten.RunGame(app); err != nil && !errors.Is(err, ebiten.Termination) {
		log.Fatal(err)
	}
}
