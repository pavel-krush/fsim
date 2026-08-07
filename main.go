package main

import (
	"errors"
	"flag"
	"log"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/pavel-krush/fsim/sim"
)

const (
	winW = 1500
	winH = 940
)

// Screen is which page the app is showing.
type Screen int

const (
	// ScreenPresets is where a run starts unless it was told what to fly: a list of
	// missions and nothing else. Setup comes second, because four columns of every
	// number the model has is a lot to be handed before you have said what you are
	// trying to fly.
	ScreenPresets Screen = iota
	ScreenSetup
	ScreenFlight
	ScreenGraphs
)

// startScreen is where a run begins. Naming a preset means the choice has already
// been made, so the list would only be in the way; asking to fly means even the
// editor is, which is what tests and scripted captures want.
func startScreen(presetNamed, fly bool) Screen {
	switch {
	case fly:
		return ScreenFlight
	case presetNamed:
		return ScreenSetup
	}
	return ScreenPresets
}

// App is the whole program: a configuration being edited, a simulation being
// flown, and the three screens that show them.
type App struct {
	ui     *UI
	screen Screen

	cfg     sim.Config
	presets *PresetScreen
	setup   *SetupScreen
	flight  *FlightScreen
	graphs  *GraphScreen

	canvas *ebiten.Image
	w, h   int

	perf perf // the service readout in the corner of the flight screen

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

	a.ui.BeginFrame(a.canvas, 1.0/float64(ebiten.TPS()))
	a.canvas.Fill(colBG)

	t0 := time.Now()
	switch a.screen {
	case ScreenPresets:
		a.presets.Update(a, a.canvas)
	case ScreenSetup:
		a.setup.Update(a, a.canvas)
	case ScreenFlight:
		a.flight.Update(a, a.canvas)
	case ScreenGraphs:
		a.graphs.Update(a, a.canvas)
	}
	// The whole frame, and the part of it that was integration: the difference is
	// what the interface costs. Only the flight screen has a simulation to separate.
	var simNs, steps int64
	var simT float64
	if a.screen == ScreenFlight && a.flight != nil {
		simNs, steps, simT = a.flight.simNs, a.flight.simSteps, a.flight.simT
		a.perf.pred(a.flight.predNs)
	}
	a.perf.frame(time.Since(t0).Nanoseconds(), simNs, steps, simT)

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

// newApp builds the program at the screen it should start on, which is the whole of
// what the flags decide between: the mission list, the editor with a mission in it,
// or a vehicle already on its way.
func newApp(chosen int, presetNamed, fly bool) *App {
	presets := sim.Presets()
	a := &App{
		ui:  NewUI(),
		cfg: presets[chosen].Cfg,
	}
	a.presets = NewPresetScreen(chosen)
	a.setup = NewSetupScreen(chosen)
	a.screen = startScreen(presetNamed, fly)
	if a.screen == ScreenFlight {
		a.Launch()
	}
	return a
}

func main() {
	shotDir := flag.String("shot", "", "run the capture script and save screenshots of every screen into this directory")
	presetSlug := flag.String("preset", "", "identifier of the preset to start from, e.g. apollo-lunar; empty for the first")
	camTrace := flag.Int("camtrace", 0, "print the vehicle's screen coordinates for N frames of live flight")
	fly := flag.Bool("fly", false, "skip the mission list and the editor and launch straight away")
	langCode := flag.String("lang", "en", "interface language to start in: en or ru")
	flag.Parse()

	loadLocales()
	l, ok := localeCode[*langCode]
	if !ok {
		log.Fatalf("unknown language %q; expected one of en, ru", *langCode)
	}
	lang = l

	initFonts()

	// By name rather than by position: an index into a list that grows is a thing
	// nobody can remember and every new preset silently redefines.
	presets := sim.Presets()
	chosen := 0
	if *presetSlug != "" {
		chosen = -1
		names := make([]string, len(presets))
		for i, p := range presets {
			names[i] = p.Name
			if p.Name == *presetSlug {
				chosen = i
			}
		}
		if chosen < 0 {
			log.Fatalf("unknown preset %q; expected one of %s", *presetSlug, strings.Join(names, ", "))
		}
	}

	app := newApp(chosen, *presetSlug != "", *fly)
	if *shotDir != "" {
		app.shots = newShotRunner(*shotDir, app.cfg)
	}
	app.traceLeft = *camTrace

	ebiten.SetWindowSize(winW, winH)
	ebiten.SetWindowTitle("fsim — orbital launch simulator")
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)

	if err := ebiten.RunGame(app); err != nil && !errors.Is(err, ebiten.Termination) {
		log.Fatal(err)
	}
}
