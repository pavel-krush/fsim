package main

import (
	"math"
	"testing"

	"fsim/sim"
)

// The axis labels go through the locale table, so it has to be loaded — the
// interface does that at startup and a test binary has no startup.
func init() { loadLocales() }

// flownGraph is a graph screen over a real flight, which is what the axis
// controls have to work against.
func flownGraph(t *testing.T) *GraphScreen {
	t.Helper()
	s := sim.New(sim.DefaultConfig())
	s.RunToEnd()
	if len(s.Hist) < 10 {
		t.Fatalf("the reference flight recorded %d samples", len(s.Hist))
	}
	g := NewGraphScreen(s)
	g.showAll()
	return g
}

// The same axis carries an ascent read in seconds and a transfer read in days,
// and the label has to follow the span rather than the value.
func TestAxisTimeFollowsTheSpan(t *testing.T) {
	cases := []struct {
		at, span float64
		want     string
	}{
		{0, 300, "0s"},
		{184, 300, "184s"},
		{125, 3000, "2:05"},
		{3725, 3000, "62:05"},
		{5400, 86400, "1h30m"},
		{227318, 400000, "2d15h"},
		{0, 400000, "0d00h"},
	}
	for _, c := range cases {
		if got := axisTime(c.at, c.span); got != c.want {
			t.Errorf("axisTime(%g, %g) = %q, want %q", c.at, c.span, got, c.want)
		}
	}
}

// Panning and zooming must not be able to walk the axis off the end of the
// flight, or leave it inverted.
func TestAxisStaysInsideTheFlight(t *testing.T) {
	g := flownGraph(t)
	end := g.flightEnd()

	for _, move := range []struct {
		name string
		do   func()
	}{
		{"pan far past the end", func() { g.t0 += 1e9; g.t1 += 1e9; g.clampAxis() }},
		{"pan far before the start", func() { g.t0 -= 1e9; g.t1 -= 1e9; g.clampAxis() }},
		{"zoom out beyond the flight", func() { g.zoomAxis(end/2, 1e6) }},
		{"zoom in past a single sample", func() { g.zoomAxis(end/2, 1e-9) }},
	} {
		move.do()
		if g.t1 <= g.t0 {
			t.Errorf("%s: axis inverted (%g..%g)", move.name, g.t0, g.t1)
		}
		if g.t0 < -1e-9 || g.t1 > end+1e-9 {
			t.Errorf("%s: axis at %g..%g, outside 0..%g", move.name, g.t0, g.t1, end)
		}
	}
}

// Zooming holds the instant under the cursor still. That is the whole of what
// makes a wheel over a plot feel like a wheel over a map.
func TestZoomHoldsTheAnchor(t *testing.T) {
	g := flownGraph(t)
	end := g.flightEnd()
	about := end * 0.3

	before := (about - g.t0) / (g.t1 - g.t0)
	g.zoomAxis(about, 0.25)
	after := (about - g.t0) / (g.t1 - g.t0)

	if math.Abs(after-before) > 1e-9 {
		t.Errorf("the anchor moved from %.6f to %.6f of the way across", before, after)
	}
	if span := g.t1 - g.t0; math.Abs(span-end*0.25) > 1e-6 {
		t.Errorf("span = %g after a quarter zoom, want %g", span, end*0.25)
	}
}

// The ascent view ends at the verdict, because that is the part a launch is
// judged by — and on a flight to the Moon it is a fifth of a per cent of the axis.
func TestAscentViewEndsAtTheVerdict(t *testing.T) {
	g := flownGraph(t)

	var orbit float64
	for _, e := range g.s.Events {
		if e.Kind == sim.EvOrbit {
			orbit = e.T
			break
		}
	}
	if orbit == 0 {
		t.Fatal("the reference flight never reached orbit")
	}

	g.showAscent()
	if g.t0 != 0 {
		t.Errorf("ascent view starts at %g, want liftoff", g.t0)
	}
	if g.t1 < orbit || g.t1 > orbit*1.3 {
		t.Errorf("ascent view ends at %g, want a little past the verdict at %g", g.t1, orbit)
	}
}

// The visible slice has to reach one sample past each edge, or a trace that
// crosses the whole plot would start and stop inside it.
func TestVisibleRangeOverhangsTheEdges(t *testing.T) {
	g := flownGraph(t)
	g.t0, g.t1 = g.flightEnd()*0.4, g.flightEnd()*0.6
	g.clampAxis()

	first, last := g.visibleRange()
	h := g.s.Hist
	if first > 0 && h[first].T > g.t0 {
		t.Errorf("first visible sample is at %g, inside the left edge at %g", h[first].T, g.t0)
	}
	if last < len(h)-1 && h[last].T < g.t1 {
		t.Errorf("last visible sample is at %g, inside the right edge at %g", h[last].T, g.t1)
	}
}
