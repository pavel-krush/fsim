package main

import (
	"strings"
	"testing"
	"time"
)

func init() { loadLocales() }

// A window's worth of frames, as the app would fold them in.
func fill(p *perf, frames int, allNs, simNs, steps int64, simT float64) {
	p.since = time.Now().Add(-perfWindow - time.Millisecond)
	for range frames - 1 {
		p.frames++
		p.allNs += allNs
		p.simNs += simNs
		p.steps += steps
		p.simT += simT
	}
	// The last one closes the window.
	p.frame(allNs, simNs, steps, simT)
}

// Nothing is reported until a window has completed: half a frame of measurement is
// worse than none, and this is the readout people will believe.
func TestNothingIsReportedBeforeAWindow(t *testing.T) {
	var p perf
	p.frame(4e6, 1e6, 30, 0.5)
	if got := p.lines(1, 100); got != nil {
		t.Errorf("reported %v before a window closed", got)
	}
}

// The split is the point of it: the physics, then everything else in the frame.
func TestTheReadoutSplitsPhysicsFromTheRest(t *testing.T) {
	var p perf
	// Thirty frames of 5 ms each, of which 1 ms was integration, 40 steps a frame.
	fill(&p, 30, 5e6, 1e6, 40, 1.0/60)

	l := p.lines(1, 6185)
	if len(l) < 4 {
		t.Fatalf("only %d lines: %v", len(l), l)
	}
	if !strings.Contains(l[1], "1.00 ms") {
		t.Errorf("the physics line reads %q, want a millisecond in it", l[1])
	}
	if !strings.Contains(l[1], "25.00 µs") {
		t.Errorf("the physics line reads %q, want 1 ms over 40 steps = 25 µs", l[1])
	}
	if !strings.Contains(l[2], "4.00 ms") {
		t.Errorf("the interface line reads %q, want 5 ms less the 1 of physics", l[2])
	}
	if !strings.Contains(strings.Join(l, "|"), "6185") {
		t.Errorf("the history size is missing from %v", l)
	}
}

// The cost of one step is a measurement or it is the clock's resolution. Under a
// score of them in a window it is the latter, and saying nothing is better.
func TestTheCostOfAStepIsWithheldWhenItWouldBeNoise(t *testing.T) {
	var p perf
	fill(&p, 8, 120e6, 50e3, 1, 1.0/60) // eight frames, one step each
	l := p.lines(1, 40)
	if strings.Contains(l[1], "µs") {
		t.Errorf("printed a per-step cost from %d steps: %q", p.windowSteps, l[1])
	}
	if !strings.Contains(l[1], "1") {
		t.Errorf("the step count itself should still be there: %q", l[1])
	}
}

// The warp achieved is only worth a line when it is not the warp asked for, and it
// keeps its decimals while it is small: at ×1 asked and a third delivered, the
// interesting figure is 0.33 and rounding it to zero says nothing.
func TestTheWarpLineOnlyAppearsWhenItDisagrees(t *testing.T) {
	// The window is half a second, so thirty frames of a sixtieth is exactly the
	// real time it asked for.
	var kept perf
	fill(&kept, 30, 4e6, 1e6, 30, 1.0/60)
	if s := strings.Join(kept.lines(1, 10), "|"); strings.Contains(s, T("perf.warp")) {
		t.Errorf("reported a warp line while delivering what was asked: %q", s)
	}

	var slow perf
	fill(&slow, 30, 120e6, 1e6, 1, 1.0/600) // a tenth of real time
	s := strings.Join(slow.lines(1, 10), "|")
	if !strings.Contains(s, T("perf.warp")) {
		t.Fatalf("no warp line while running at a tenth of the asked rate: %q", s)
	}
	if !strings.Contains(s, "0.") {
		t.Errorf("the achieved warp lost its decimals: %q", s)
	}
}
