package main

import (
	"math"
	"testing"
)

// The same number means different things on different platforms, so a zoom built on
// the raw value is a different zoom on each. One detent has to be one step whether
// it arrives as 1 from GLFW, as 100 from a Windows mouse in Chrome, or as a stream
// of fractions from a trackpad.
func TestOneDetentIsOneStepWhateverSendsIt(t *testing.T) {
	// What a zoom of exp(w*0.18) does with a notch: the step the flight screen uses.
	step := func(w float64) float64 { return math.Exp(w * 0.18) }

	cases := []struct {
		name  string
		raw   float64 // one detent, as this platform reports it
		wants float64 // the zoom factor it should produce
	}{
		{"glfw on the desktop", 1, 1.197},
		{"a Windows mouse in Chrome", 100, 1.197},
		{"Firefox in line mode", 3, 1.197},
		{"a browser sending 120 per detent", 120, 1.197},
	}
	for _, c := range cases {
		u := NewUI()
		got := step(u.normalizeWheel(c.raw))
		if math.Abs(got-c.wants) > 0.01 {
			t.Errorf("%s: one detent of %g zooms by %.3f, want %.3f", c.name, c.raw, got, c.wants)
		}
		// And the raw value it replaces, for the record: this is the bug.
		if c.raw >= 100 && step(c.raw) < 1e6 {
			t.Errorf("%s: the raw value should be the absurd case, and is %.3g", c.name, step(c.raw))
		}
	}
}

// A trackpad flick is many events with a momentum tail. It should add up to a
// gesture, not to a wall: each event is a fraction of a notch, and no single one can
// be worth more than a whole one.
func TestATrackpadFlickAddsUpToAGesture(t *testing.T) {
	u := NewUI()
	flick := []float64{2, 6, 14, 28, 41, 33, 18, 9, 4, 2, 1}
	total := 0.0
	for _, d := range flick {
		n := u.normalizeWheel(d)
		if n > wheelZoomMax+1e-9 {
			t.Fatalf("one event of %g came out as %.3f notches", d, n)
		}
		total += n
	}
	if total < 1 || total > 8 {
		t.Errorf("the first flick came to %.2f notches, want a handful", total)
	}

	// The first flick of a session is counted generously: every event on its rising
	// edge is a new largest, so it is a whole notch. By the second the device is
	// calibrated and a flick is worth what it should be — around four notches, which
	// is a zoom of two.
	second := 0.0
	for _, d := range flick {
		second += u.normalizeWheel(d)
	}
	if second >= total {
		t.Errorf("the second flick (%.2f) was not cheaper than the first (%.2f)", second, total)
	}
	if z := math.Exp(second * 0.18); z < 1.5 || z > 3 {
		t.Errorf("a calibrated flick zooms by %.2f, want something like two", z)
	}
	// Under the old arithmetic the same flick was 158 notches, which is exp(28) of
	// zoom — the entire range of the camera in one gesture.
	if raw := 158.0; math.Exp(raw*0.18) < 1e10 {
		t.Error("the raw sum should be the absurd case")
	}
}

// Swapping a trackpad for a mouse has to recalibrate, and a freak event must not
// desensitise the session for ever.
func TestTheUnitRecoversAndIsBounded(t *testing.T) {
	u := NewUI()

	// A page-mode event ten times bigger than anything sane.
	u.normalizeWheel(4000)
	if u.wheelUnit > wheelUnitMax {
		t.Errorf("the unit ran to %g, past the ceiling of %g", u.wheelUnit, wheelUnitMax)
	}

	// Idle frames bring it back down, so the next device gets its own calibration.
	for range 2000 {
		u.normalizeWheel(0)
	}
	if u.wheelUnit != wheelUnitMin {
		t.Errorf("after two thousand idle frames the unit is %g, want %g", u.wheelUnit, wheelUnitMin)
	}

	// And a detent right after that is still a detent, not a fraction.
	if n := u.normalizeWheel(100); math.Abs(n-1) > 1e-9 {
		t.Errorf("a detent after the idle came out as %.3f notches", n)
	}
}

// Direction is preserved, because a zoom that goes the wrong way is worse than one
// that goes too far.
func TestScrollingBackZoomsBack(t *testing.T) {
	u := NewUI()
	if n := u.normalizeWheel(-100); n >= 0 {
		t.Errorf("scrolling back gave %.3f", n)
	}
	if n := u.normalizeWheel(100); n <= 0 {
		t.Errorf("scrolling forward gave %.3f", n)
	}
}
