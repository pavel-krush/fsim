package main

import (
	"math"
	"testing"

	"github.com/pavel-krush/fsim/sim"
)

// Dragging and zoom-to-cursor are both statements about a world point that has to
// stay under the pointer, so they are only as good as this pair being inverses.
func TestProjectAndUnprojectAreInverses(t *testing.T) {
	cams := []Camera{
		{Center: sim.Vec2{X: 6371000}, Scale: 0.01, Rot: 0, View: Rect{12, 12, 1100, 800}},
		{Center: sim.Vec2{X: 3e8, Y: -1e8}, Scale: 2.4e-6, Rot: 1.7, View: Rect{0, 0, 900, 900}},
		{Center: sim.Vec2{}, Scale: 1e-9, Rot: math.Pi / 2, View: Rect{40, 8, 640, 480}},
	}
	pts := []sim.Vec2{{}, {X: 1000}, {X: -4e7, Y: 9e6}, {X: 1.5e11, Y: -2e11}}

	for _, c := range cams {
		for _, p := range pts {
			x, y := c.Project(p)
			back := c.Unproject(x, y)
			// Relative to the view's own width in world units: a metre out of an
			// astronomical unit is not an error, a pixel is.
			tol := math.Min(c.View.W, c.View.H) / c.Scale * 1e-9
			if d := back.Sub(p).Len(); d > tol {
				t.Errorf("scale %g rot %g: %v came back as %v, %g m out (tolerance %g)",
					c.Scale, c.Rot, p, back, d, tol)
			}
		}
	}
}

// The point under the cursor is what a zoom is about, so it has to be exactly
// where it was afterwards — otherwise the world slides away as you scroll.
func TestZoomHoldsThePointUnderTheCursor(t *testing.T) {
	cam := Camera{Center: sim.Vec2{X: 6371000, Y: 200000}, Scale: 1e-4, Rot: 0.4,
		View: Rect{12, 12, 1100, 800}}
	const mx, my = 400, 300

	under := cam.Unproject(mx, my)
	for _, step := range []float64{0.18, -0.18, 2.5, -4} {
		before := cam.Unproject(mx, my)
		cam.Scale *= math.Exp(step)
		after := cam.Unproject(mx, my)
		// This is the correction the free camera applies.
		cam.Center = cam.Center.Add(before.Sub(after))
	}

	if d := cam.Unproject(mx, my).Sub(under).Len(); d > 1 {
		t.Errorf("the point under the cursor moved %g m over four zoom steps", d)
	}
}

// A camera aimed at a body centres on it, and one that has been dragged centres on
// nothing — the two are different states and the picker says which.
func TestLookAtSetsBothFrameAndFollow(t *testing.T) {
	s := sim.New(presetNamed(t, "apollo-saturn").Cfg) // a system worth aiming at
	f := NewFlightScreen(s)
	moon := s.Cfg.System.IndexOf("moon")

	f.lookAt(moon)
	if f.follow != moon || f.frameBody() != moon {
		t.Errorf("aimed at the Moon: follow %d, frame %d", f.follow, f.frameBody())
	}

	// Out of range is the vehicle, not a panic.
	f.lookAt(len(s.Cfg.System.Bodies) + 5)
	if f.follow != -1 || f.frameBody() != s.St.Center {
		t.Errorf("out-of-range target left follow %d, frame %d", f.follow, f.frameBody())
	}

	// A drag takes over from wherever the camera was, without moving the picture.
	f.cam = Camera{Center: sim.Vec2{X: 7e6}, Scale: 1e-4, View: Rect{0, 0, 800, 600}}
	f.follow, f.freePos = camFree, f.cam.Center
	if f.frameBody() != s.St.Center {
		t.Error("a free view lost the frame it was dragged in")
	}
}

// presetNamed finds a preset by its identifier. By position is how a test starts
// depending on the order of a list that grows.
func presetNamed(t *testing.T, name string) sim.Preset {
	t.Helper()
	for _, p := range sim.Presets() {
		if p.Name == name {
			return p
		}
	}
	t.Fatalf("no preset named %q", name)
	return sim.Preset{}
}

// The ground-track rotation belongs to the launch pad. Applied with the frame
// body's rotation instead of the pad's, and to samples measured from somewhere
// else entirely, it turned the ascent's markers around whatever the camera
// happened to be centred on: ninety days into the Mars transfer, drawn in the
// Sun's frame, max q came out forty-six radians round the Sun — over by Venus.
func TestTrackPointLeavesOldSamplesWhereTheyHappened(t *testing.T) {
	s := sim.New(presetNamed(t, "apollo-mars").Cfg)
	s.FastForward(90 * 86400)
	f := NewFlightScreen(s)

	if f.frameBody() != 0 {
		t.Fatalf("mid-cruise the frame body is %d, expected the root", f.frameBody())
	}
	earth := s.Cfg.System.IndexOf("earth")

	// An early sample: measured from the Earth, a couple of hundred kilometres up.
	var sm sim.Sample
	for _, h := range s.Hist {
		if h.T > 100 {
			sm = h
			break
		}
	}
	if sm.Center != earth {
		t.Fatalf("the sample at T+%.0f s is measured from body %d", sm.T, sm.Center)
	}

	// Drawn in the Sun's frame it has to land where the Earth was when it
	// happened, give or take the couple of hundred kilometres it was up.
	want, _ := s.Cfg.System.StateAt(earth, sm.T)
	got := f.trackPoint(sm)
	if d := got.Sub(want).Len(); d > 3*s.Cfg.System.Bodies[earth].Radius {
		t.Errorf("an ascent sample lands %.3g m from where the Earth was, %.4f of an AU",
			d, d/1.496e11)
	}
}

// A trail measured in seconds cannot serve both an ascent and an interplanetary
// cruise: fifteen minutes of a transfer is a tenth of a pixel, which is why the
// flown path was invisible out there. The bound is one revolution, and a
// trajectory that is not coming back round has none to repeat.
func TestTrailSpanFollowsTheOrbitNotTheClock(t *testing.T) {
	s := sim.New(presetNamed(t, "apollo-mars").Cfg)
	f := NewFlightScreen(s)

	// In the parking orbit: one revolution, so the trail cannot smear.
	s.RunToEnd()
	period := s.Telemetry().Orbit.Period
	if period < 60 {
		t.Fatalf("the parking orbit has a period of %.0f s", period)
	}
	if span := f.trailSpan(); math.Abs(span-period) > 1 && span != trailWindow {
		t.Errorf("in orbit the trail spans %.0f s against a period of %.0f", span, period)
	}

	// Mid-cruise: longer than the flight so far, so the whole path stays drawn.
	s.FastForward(90 * 86400)
	if span := f.trailSpan(); span < s.St.T {
		t.Errorf("mid-cruise the trail spans %.3g s of a %.3g s flight", span, s.St.T)
	}
}
