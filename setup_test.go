package main

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/pavel-krush/fsim/sim"
)

// editorFrame runs one real frame of the setup screen, widgets and all. Ebiten will
// make and draw into an image outside a running game loop, which is what makes this
// possible at all — the screenshot script exists because it cannot render *headless*,
// not because the toolkit refuses to run.
func editorFrame(t *testing.T, a *App, img *ebiten.Image) {
	t.Helper()
	a.ui.BeginFrame(img, 1.0/60)
	a.setup.Update(a, img)
	a.ui.EndFrame()
}

// Editing the planet the pad is on has to stick. It did not: the first column writes
// through the system — it has to, since it edits any of eighteen bodies — and
// EnsureSystem copied Config.Body back over the top on the next call. So a new
// diameter, mass or rotation period snapped back a frame later with nothing on screen
// to say why. This is that bug, at the level it was visible.
func TestEditingTheLaunchBodySticks(t *testing.T) {
	initFonts()
	img := ebiten.NewImage(1600, 1000)

	a := &App{ui: NewUI(), cfg: presetNamed(t, "earth-falcon").Cfg}
	a.canvas, a.w, a.h = img, 1600, 1000
	a.setup, a.screen = NewSetupScreen(0), ScreenSetup

	editorFrame(t, a, img)

	// Exactly what the diameter and rotation fields write.
	lb := &a.cfg.System.Bodies[a.cfg.LaunchBody]
	lb.Radius = 3000000
	lb.RotationPeriod = 40000

	editorFrame(t, a, img)

	lb = &a.cfg.System.Bodies[a.cfg.LaunchBody]
	if lb.Radius != 3000000 {
		t.Errorf("radius came back as %g, want the edited 3e6", lb.Radius)
	}
	if lb.RotationPeriod != 40000 {
		t.Errorf("rotation period came back as %g, want the edited 40000", lb.RotationPeriod)
	}
	// And the derived figures followed the edit rather than the mirror.
	if got := a.cfg.Body.SurfaceG; got != lb.SurfaceG {
		t.Errorf("the mirror reads g = %g while the body has %g", got, lb.SurfaceG)
	}
	// A flight built from it flies the edited planet.
	s := sim.New(a.cfg)
	if got := s.Cfg.Body.Radius; got != 3000000 {
		t.Errorf("the simulation launched from a planet of radius %g", got)
	}
}

// The atmosphere column edits the air of whatever body the first column is on, which is
// the whole point of the air belonging to the bodies. It used to be the launch body's
// and only ever the launch body's, so moving the pad took Earth's atmosphere with it.
func TestTheAtmosphereColumnFollowsTheSelectedBody(t *testing.T) {
	initFonts()
	img := ebiten.NewImage(1600, 1000)

	a := &App{ui: NewUI(), cfg: presetNamed(t, "apollo-lunar").Cfg}
	a.canvas, a.w, a.h = img, 1600, 1000
	a.setup, a.screen = NewSetupScreen(0), ScreenSetup
	a.cfg.EnsureSystem()

	titan := a.cfg.System.IndexOf("titan")
	if titan < 0 {
		t.Fatal("no Titan in the solar system")
	}
	a.setup.selBody = titan
	editorFrame(t, a, img)

	// Typing a new surface pressure where that column writes it.
	a.cfg.System.Bodies[titan].Atmo.SurfacePressure = 200000
	editorFrame(t, a, img)

	if got := a.cfg.System.Bodies[titan].Atmo.SurfacePressure; got != 200000 {
		t.Errorf("Titan's surface pressure came back as %g", got)
	}
	// The launch body's air is untouched: it is a different body's weather.
	if got := a.cfg.System.Bodies[a.cfg.LaunchBody].Atmo.SurfacePressure; got != 101325 {
		t.Errorf("editing Titan changed the Earth's surface pressure to %g", got)
	}
	// And the profile was re-derived with Titan's gravity, not left stale.
	if got := a.cfg.System.Bodies[titan].Atmo.State(0).Pressure; got != 200000 {
		t.Errorf("the profile still starts at %g Pa", got)
	}

	// A body with no air is offered some, and can have it taken away again.
	moon := a.cfg.System.IndexOf("moon")
	a.setup.selBody = moon
	if !a.cfg.System.Bodies[moon].Atmo.IsVacuum() {
		t.Fatal("the Moon has air already")
	}
	editorFrame(t, a, img)
	a.cfg.System.Bodies[moon].Atmo = sim.EarthAir() // what the + atmosphere button does
	editorFrame(t, a, img)
	if a.cfg.System.Bodies[moon].Atmo.IsVacuum() {
		t.Error("the Moon's new air did not survive a frame")
	}
	// Derived with the Moon's own gravity: a sixth of the pull holds the same column
	// up over six times the height, so at 50 km it is far thicker than it is here.
	// The surface density says nothing about it — that is p·M/RT, with no g in it.
	overMoon := a.cfg.System.Bodies[moon].Atmo.State(50000).Density
	overEarth := a.cfg.System.Bodies[a.cfg.LaunchBody].Atmo.State(50000).Density
	if overMoon <= 3*overEarth {
		t.Errorf("at 50 km the air is %g kg/m³ over the Moon and %g over the Earth: "+
			"the body's gravity did not reach the profile", overMoon, overEarth)
	}
}
