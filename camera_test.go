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

// An already-flown path must not change. The ground-track rotation is the one
// thing that makes it change — it is measured from now, so as the clock runs the
// Earth-centred part of the path winds up about the Earth while the current point
// stays pinned, which on the way to the Moon looks like the trail being wound like
// a spring. That reading is only worth having while the picture is about the
// ground, so the same ramp that lets the camera go of the local vertical lets go
// of it too.
func TestFlownPathStopsMovingOnceTheViewPullsBack(t *testing.T) {
	a := &App{ui: NewUI()}
	a.ui.DT = 1.0 / 60
	view := Rect{0, 0, 1160, 830}

	s := sim.New(presetNamed(t, "apollo-saturn").Cfg)
	s.RunToEnd() // to the parking orbit
	f := NewFlightScreen(s)

	// Standing on the planet: the trail is a ground track, and it should be.
	f.pendingZoom, f.manualScale = 0, false
	f.cam.Scale = math.Min(view.W, view.H) / 3000 // a 3 km view
	f.manualScale = true
	f.updateCamera(a, view)
	if f.groundHold < 0.99 {
		t.Errorf("on the pad the ground track is held at %.3f, want all of it", f.groundHold)
	}

	// Pulled back to the Moon's orbit: no ground anywhere in the picture.
	f.cam.Scale = math.Min(view.W, view.H) / 8e8
	f.updateCamera(a, view)
	if f.groundHold != 0 {
		t.Fatalf("at the Moon's orbit the ground track is still held at %.3f", f.groundHold)
	}

	// And there, the same sample lands in the same place however long the flight
	// has been running: two and a half days of it, in the frame it is drawn in.
	sm := s.Hist[len(s.Hist)/2]
	before := f.trackPoint(sm)
	s.FastForward(2.5 * 86400)
	f.updateCamera(a, view)
	if after := f.trackPoint(sm); after.Sub(before).Len() > 1e-6 {
		t.Errorf("a sample from T+%.0f s moved %.3g m over two and a half days of flight",
			sm.T, after.Sub(before).Len())
	}
}

// The trail is the path flown in the frame the picture is drawn in, and only that.
// What was flown in another frame can only be mapped into this one by a spiral — the
// true path relative to a moving body — or by a lie about its direction, so it is
// left as a hole instead.
func TestTheTrailOnlyShowsWhatWasFlownInItsFrame(t *testing.T) {
	s := sim.New(presetNamed(t, "apollo-saturn").Cfg)
	moon := s.Cfg.System.IndexOf("moon")
	for !s.St.Done && s.St.Center != moon {
		s.FastForward(s.St.T + 300)
	}
	if s.St.Center != moon {
		t.Fatal("the flight never reached the Moon's sphere of influence")
	}
	s.FastForward(s.St.T + 3600)
	f := NewFlightScreen(s)
	// The ground track is the one deliberate exception to a sample being drawn where
	// it was recorded, and it is held only while the view is on the ground.
	f.groundHold = 0

	var drawn, held int
	for _, h := range s.Hist {
		if w, ok := f.showTrack(h); !ok || w != 1 {
			held++
			continue
		}
		drawn++
		// What is drawn is exact: a sample measured from the body in the middle is
		// already in the coordinates being drawn.
		if d := f.trackPoint(h).Sub(h.Pos).Len(); d > 1e-9 {
			t.Fatalf("a sample of the drawn frame is %.3g m from where it was recorded", d)
		}
	}
	t.Logf("in the Moon's frame: %d samples drawn, %d flown elsewhere and left out", drawn, held)
	if drawn == 0 || held == 0 {
		t.Fatalf("expected a flight with both: %d drawn, %d left out", drawn, held)
	}

	// Back in the Earth's frame, everything flown around the Earth is drawn — both
	// the parking orbit and the coast out, whichever side of the flyby they are on.
	f.lookAt(s.Cfg.LaunchBody)
	for _, h := range s.Hist {
		if h.Center != s.Cfg.LaunchBody {
			continue
		}
		if w, ok := f.showTrack(h); !ok || w != 1 {
			_ = w
			t.Fatal("a sample flown around the Earth is left out of the Earth's frame")
		}
		if d := f.trackPoint(h).Sub(h.Pos).Len(); d > 1e-9 {
			t.Fatalf("an Earth-frame sample is %.3g m from where it was recorded", d)
		}
	}
}

// And the picture does not move when the frame changes. Everything is written from
// the new centre from that instant, the camera's own centre included, so without
// carrying the view across the whole picture slides by the distance between the two
// bodies — 384,000 km on the way to the Moon.
func TestFrameHandOverKeepsTheViewStill(t *testing.T) {
	a := &App{ui: NewUI()}
	a.ui.DT = 1.0 / 60
	view := Rect{0, 0, 1160, 830}

	s := sim.New(presetNamed(t, "apollo-saturn").Cfg)
	moon := s.Cfg.System.IndexOf("moon")
	for !s.St.Done && s.St.Center != moon {
		s.FastForward(s.St.T + 60)
	}
	cross := s.St.T

	s = sim.New(presetNamed(t, "apollo-saturn").Cfg)
	s.FastForward(cross - 120)
	f := NewFlightScreen(s)
	for range 90 { // let the automatic framing settle
		f.updateCamera(a, view)
	}
	// The zoom is a separate decision and is eased on purpose: entering a sphere of
	// influence changes the automatic span by a factor of fourteen, and gliding
	// through that moves everything on screen because a zoom does. Hold it, so what
	// is left to measure is the change of frame alone.
	f.manualScale = true
	if f.frameBody() == moon {
		t.Fatal("already in the Moon's frame before the crossing")
	}
	x0, y0 := f.cam.Project(f.vehiclePos())

	s.FastForward(cross + 1)
	f.updateCamera(a, view)
	if f.frameBody() != moon {
		t.Fatalf("still in body %d's frame past the crossing", f.frameBody())
	}
	x1, y1 := f.cam.Project(f.vehiclePos())

	if d := math.Hypot(x1-x0, y1-y0); d > 1 {
		t.Errorf("the vehicle moved %.0f px across the change of frame", d)
	}
}

// The frame just left is let go of, not kept and not redrawn: held exactly where it
// was drawn while it fades, and gone after that. Held there it wears the wrong shape
// for the frame it is now in, so it cannot stay — but at the instant of the crossing
// the two frames agree, which is why nothing moves as the picture changes hands.
func TestThePastFadesInPlaceAfterACrossing(t *testing.T) {
	a := &App{ui: NewUI()}
	a.ui.DT = 1.0 / 60
	view := Rect{0, 0, 1160, 830}

	s := sim.New(presetNamed(t, "apollo-saturn").Cfg)
	moon := s.Cfg.System.IndexOf("moon")
	for !s.St.Done && s.St.Center != moon {
		s.FastForward(s.St.T + 60)
	}
	cross := s.St.T

	s = sim.New(presetNamed(t, "apollo-saturn").Cfg)
	s.FastForward(cross - 120)
	f := NewFlightScreen(s)
	for range 90 {
		f.updateCamera(a, view)
	}
	f.groundHold = 0 // pulled back, so no ground track in the way

	// A sample from the parking orbit: whole, and where it was recorded.
	sm := s.Hist[sampleAt(s.Hist, 3000)]
	if w, ok := f.showTrack(sm); !ok || w != 1 {
		t.Fatalf("before the crossing a parking-orbit sample has weight %g, ok %v", w, ok)
	}
	x0, y0 := f.cam.Project(f.trackPoint(sm))

	// Across the crossing: still drawn, still at full strength, and — the whole
	// point — still in the same place on screen. The coordinates it is written in
	// have changed by 384,000 km; the picture has not moved.
	s.FastForward(cross + 1)
	f.updateCamera(a, view)
	w, ok := f.showTrack(sm)
	if !ok {
		t.Fatal("the frame just left is not drawn at all")
	}
	if w != 1 {
		t.Errorf("weight %g on the frame the crossing happened, want the fade to start whole", w)
	}
	x1, y1 := f.cam.Project(f.trackPoint(sm))
	if d := math.Hypot(x1-x0, y1-y0); d > 4 {
		t.Errorf("it moved %.1f px as the picture changed hands", d)
	}

	// Then it fades.
	for range 20 {
		f.updateCamera(a, view)
	}
	if w, ok := f.showTrack(sm); !ok || w >= 1 || w <= 0 {
		t.Errorf("weight %g a third of a second later, want it fading", w)
	}

	// And after the fade it is gone, rather than left up in the wrong shape.
	for range int(ghostFade/a.ui.DT) + 2 {
		f.updateCamera(a, view)
	}
	if _, ok := f.showTrack(sm); ok {
		t.Error("the frame just left is still drawn a second and a half later")
	}
}

// A drag is a pan and has nothing to say about which way is up. It used to have
// plenty: leaving the vehicle behind switched the rotation target from the vehicle's
// own radius to the world's +Y axis and moved the whole way there in one frame, so a
// click on the pad — which sits on the +X axis — turned the picture ninety degrees.
func TestAClickDoesNotRotateTheWorld(t *testing.T) {
	a := &App{ui: NewUI()}
	a.ui.DT = 1.0 / 60
	view := Rect{0, 0, 1160, 830}

	s := sim.New(presetNamed(t, "apollo-saturn").Cfg)
	f := NewFlightScreen(s)
	f.updateCamera(a, view) // on the pad, following the vehicle, zoomed right in
	before := f.cam.Rot

	// What handleCamera does on the first pixel of a drag.
	f.freePos, f.follow = f.cam.Center, camFree
	f.updateCamera(a, view)

	if d := math.Abs(angleDelta(before, f.cam.Rot)); d > 0.001 {
		t.Errorf("the world turned %.1f degrees on a click", d*180/math.Pi)
	}
	// And it stays put over the frames that follow, rather than creeping.
	for range 120 {
		f.updateCamera(a, view)
	}
	if d := math.Abs(angleDelta(before, f.cam.Rot)); d > 0.001 {
		t.Errorf("the world turned %.1f degrees over two seconds of dragging", d*180/math.Pi)
	}
}

// Pinning a body does put the world's own axes up — there is no local vertical to
// speak of out there — but it takes about half a second over it. Snapping is what
// read as a rendering fault.
func TestPinningABodyTurnsTheWorldGently(t *testing.T) {
	a := &App{ui: NewUI()}
	a.ui.DT = 1.0 / 60
	view := Rect{0, 0, 1160, 830}

	s := sim.New(presetNamed(t, "apollo-saturn").Cfg)
	f := NewFlightScreen(s)
	f.updateCamera(a, view)
	before := f.cam.Rot

	f.lookAt(s.Cfg.System.IndexOf("moon"))
	f.updateCamera(a, view)
	step := math.Abs(angleDelta(before, f.cam.Rot))
	full := math.Abs(angleDelta(before, math.Pi/2))
	if step > full/4 {
		t.Errorf("one frame moved %.0f%% of the way round", step/full*100)
	}
	if step == 0 {
		t.Error("it did not start turning at all")
	}

	for range 120 {
		f.updateCamera(a, view)
	}
	if d := math.Abs(angleDelta(f.cam.Rot, math.Pi/2)); d > 0.01 {
		t.Errorf("after two seconds it is still %.1f degrees off", d*180/math.Pi)
	}
}

// A scripted capture has to be settled: the zoom, the change of frame and the
// rotation all ease, and a screenshot taken part way through any of them is a
// screenshot of nothing in particular. The pinned views came out eighty degrees from
// where they settle before snapCamera took the rotation on as well.
func TestAScriptedCaptureIsSettled(t *testing.T) {
	a := &App{ui: NewUI()}
	a.ui.DT = 1.0 / 60
	view := Rect{0, 0, 1160, 830}

	s := sim.New(presetNamed(t, "apollo-saturn").Cfg)
	f := NewFlightScreen(s)
	f.updateCamera(a, view)

	f.lookAt(s.Cfg.System.IndexOf("moon"))
	f.snapCamera()
	f.updateCamera(a, view)

	if d := math.Abs(angleDelta(f.cam.Rot, math.Pi/2)); d > 0.001 {
		t.Errorf("one frame after snapCamera the rotation is %.1f degrees off", d*180/math.Pi)
	}
	if f.camHoldK != 0 || f.ghostK != 0 {
		t.Errorf("the view is still being held: camHold %g, ghost %g", f.camHoldK, f.ghostK)
	}
}

// Launching on its own does not turn anything: the pad is on the +X axis, the camera
// follows the vehicle, and the vertical it points up is the vehicle's own radius — so
// the rotation starts at zero and stays there through the first seconds of the climb.
// Worth pinning separately from the click, because "I launch and it turns ninety
// degrees" and "I launch, click, and it turns ninety degrees" are different faults and
// only the second one existed.
func TestLaunchingAloneDoesNotTurnTheCamera(t *testing.T) {
	a := &App{ui: NewUI()}
	a.ui.DT = 1.0 / 60
	view := Rect{0, 0, 1160, 830}

	s := sim.New(presetNamed(t, "earth-falcon").Cfg)
	f := NewFlightScreen(s)

	for i := range 600 { // ten seconds off the pad
		f.updateCamera(a, view)
		if d := math.Abs(angleDelta(f.cam.Rot, 0)); d > 0.02 {
			t.Fatalf("frame %d: the camera has turned %.1f degrees since liftoff", i, d*180/math.Pi)
		}
		s.Advance(a.ui.DT)
	}
}
