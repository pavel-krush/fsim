package main

import (
	"testing"

	"github.com/pavel-krush/fsim/sim"
)

func init() { loadLocales() }

// Where a run begins. Naming a preset is saying the choice is already made, and
// asking to fly is saying the editor is in the way too — which is what a scripted
// capture and a shared link both want.
func TestStartScreenFollowsWhatWasAsked(t *testing.T) {
	cases := []struct {
		named, fly bool
		want       Screen
	}{
		{false, false, ScreenPresets},
		{true, false, ScreenSetup},
		{false, true, ScreenFlight},
		{true, true, ScreenFlight},
	}
	for _, c := range cases {
		if got := startScreen(c.named, c.fly); got != c.want {
			t.Errorf("startScreen(named=%v, fly=%v) = %d, want %d", c.named, c.fly, got, c.want)
		}
	}
}

// Picking a mission loads it and moves on. Everything the editor points into
// belongs to the configuration, so the screen has to be built fresh rather than
// told to change its mind — the same trap loadPreset had.
func TestPickingAMissionLoadsItAndMovesOn(t *testing.T) {
	presets := sim.Presets()
	i := -1
	for n, p := range presets {
		if p.Name == "apollo-mars" {
			i = n
		}
	}
	if i < 0 {
		t.Fatal("no apollo-mars preset")
	}

	a := &App{ui: NewUI(), cfg: presets[0].Cfg}
	a.presets = NewPresetScreen(0)
	a.setup = NewSetupScreen(0)

	a.presets.pick(a, i)

	if a.screen != ScreenSetup {
		t.Errorf("screen %d after picking, want setup", a.screen)
	}
	if a.cfg.LaunchBody != presets[i].Cfg.LaunchBody || len(a.cfg.Nodes) != len(presets[i].Cfg.Nodes) {
		t.Error("the configuration is not the one picked")
	}
	if a.setup.preset != i {
		t.Errorf("the editor is on preset %d, want %d", a.setup.preset, i)
	}
	// A stale body index into a system that has just been replaced is how this
	// screen would crash; loadPreset has the same guard.
	a.cfg.EnsureSystem()
	if a.setup.selBody >= len(a.cfg.System.Bodies) {
		t.Errorf("the body editor points at %d of %d", a.setup.selBody, len(a.cfg.System.Bodies))
	}

	// Out of range does nothing rather than panicking, because the list grows.
	a.screen = ScreenPresets
	a.presets.pick(a, len(presets))
	a.presets.pick(a, -1)
	if a.screen != ScreenPresets {
		t.Error("an out-of-range pick moved on anyway")
	}
}

// The keyboard walks the list and stops at the ends: a short list is easier to aim
// at when it has ends you can feel.
func TestTheListStopsAtItsEnds(t *testing.T) {
	n := len(sim.Presets())
	s := NewPresetScreen(0)

	s.move(-1, n)
	if s.sel != 0 {
		t.Errorf("up from the first row went to %d", s.sel)
	}
	s.move(n+5, n)
	if s.sel != n-1 {
		t.Errorf("down past the last row went to %d of %d", s.sel, n)
	}
	s.move(-3, n)
	if s.sel != n-4 {
		t.Errorf("three up from the end is %d, want %d", s.sel, n-4)
	}
}

// Every row has to be reachable in the window the program asks for: a mission that
// cannot be clicked is a mission nobody can fly.
func TestEveryMissionFitsOnTheScreen(t *testing.T) {
	n := len(sim.Presets())
	b := Rect{0, 0, winW, winH}
	const pad = 12
	headH := 44.0
	body := Rect{pad, pad + headH + 8, b.W - 2*pad, b.H - headH - 3*pad - 8}
	listH := float64(n)*(presetRowH+6) - 6
	area := Rect{body.X, body.Y + (body.H-listH)/2, body.W, listH}

	if listH > body.H {
		t.Fatalf("%d rows need %.0f px of the %.0f the window has", n, listH, body.H)
	}
	first, last := presetRowRect(area, 0), presetRowRect(area, n-1)
	if first.Y < body.Y {
		t.Errorf("the first row starts at %.0f, above the area's %.0f", first.Y, body.Y)
	}
	if last.Bottom() > body.Bottom() {
		t.Errorf("the last row ends at %.0f, below the area's %.0f", last.Bottom(), body.Bottom())
	}
	if first.X < body.X || first.Right() > body.Right() {
		t.Errorf("a row spans %.0f..%.0f, outside %.0f..%.0f",
			first.X, first.Right(), body.X, body.Right())
	}
}

// -fly is for tests and for a link that wants to show a mission rather than hand
// over an editor: it skips both screens and puts a vehicle on the pad.
func TestFlyStartsOnThePad(t *testing.T) {
	presets := sim.Presets()
	i := -1
	for n, p := range presets {
		if p.Name == "titan-ascent" {
			i = n
		}
	}
	if i < 0 {
		t.Fatal("no titan-ascent preset")
	}

	a := newApp(i, true, true)
	if a.screen != ScreenFlight {
		t.Fatalf("screen %d with -fly, want the flight screen", a.screen)
	}
	if a.flight == nil {
		t.Fatal("nothing was launched")
	}
	if got := a.flight.s.Cfg.Body.Name; got != "titan" {
		t.Errorf("flying from %q, want titan", got)
	}
	if a.flight.s.St.T != 0 || !a.flight.s.St.Landed {
		t.Errorf("the vehicle is at T+%.1f and Landed=%v, want it on the pad",
			a.flight.s.St.T, a.flight.s.St.Landed)
	}

	// And without it, the same preset lands in the editor rather than in flight.
	if b := newApp(i, true, false); b.screen != ScreenSetup || b.flight != nil {
		t.Errorf("without -fly: screen %d, flight %v", b.screen, b.flight != nil)
	}
	// And with nothing named at all, the list.
	if c := newApp(0, false, false); c.screen != ScreenPresets {
		t.Errorf("with no preset named: screen %d, want the list", c.screen)
	}
}
