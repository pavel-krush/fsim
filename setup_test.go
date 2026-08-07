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
