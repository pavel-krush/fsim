package main

import (
	"math"
	"testing"

	"fsim/sim"
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
	s := sim.New(sim.Presets()[1].Cfg) // Apollo, which has a system worth aiming at
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
