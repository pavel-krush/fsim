package main

import (
	"fmt"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
)

// The service readout in the corner of the flight screen: what a frame costs, and
// what the simulation gets done inside it.
//
// The numbers worth having are the ones that answer a question. Frames and ticks
// say whether the loop is keeping up at all. Splitting the frame into the
// simulation and everything else says *which half* is not keeping up — the same
// question the browser build raised, where the answer turned out to be pixels and
// took a pair of screenshots at different window sizes to establish. Microseconds
// per step and steps per frame say what the physics costs and how much of it a
// frame is buying. And the warp actually achieved, against the one asked for, says
// the simulation is falling behind before WarpLimited trips, which only fires once
// a frame has run out of budget entirely.

// perfWindow is how long a sample is accumulated over before it is reported.
// Per-frame numbers are unreadable — they jitter by a factor of two — and in a
// browser they are mostly quantisation: time.Now() there is clamped to something
// like a tenth of a millisecond, which is the same order as a frame's simulation.
const perfWindow = 500 * time.Millisecond

// perfMinSteps is how many integrator steps a window needs before the cost of one
// of them is worth printing.
const perfMinSteps = 20

// perf accumulates a window's worth of measurements and holds the last complete
// one for drawing.
type perf struct {
	since  time.Time
	frames int
	simNs  int64
	allNs  int64
	steps  int64
	simT   float64

	// The last completed window.
	ready         bool
	windowSteps   int64
	simMs, allMs  float64
	usPerStep     float64
	stepsPerFrame float64
	warpDone      float64
	predMs        float64
}

// frame folds one frame into the window. allNs is the whole of Update, simNs the
// part of it that was integration, so the difference is the interface: laying out
// every widget, tessellating the rings, and the trail.
func (p *perf) frame(allNs, simNs, steps int64, simT float64) {
	if p.since.IsZero() {
		p.since = time.Now()
	}
	p.frames++
	p.allNs += allNs
	p.simNs += simNs
	p.steps += steps
	p.simT += simT

	elapsed := time.Since(p.since)
	if elapsed < perfWindow || p.frames == 0 {
		return
	}
	secs := elapsed.Seconds()
	p.simMs = float64(p.simNs) / float64(p.frames) / 1e6
	p.allMs = float64(p.allNs) / float64(p.frames) / 1e6
	p.stepsPerFrame = float64(p.steps) / float64(p.frames)
	p.usPerStep = 0
	if p.steps > 0 {
		p.usPerStep = float64(p.simNs) / float64(p.steps) / 1e3
	}
	p.warpDone = p.simT / secs
	p.windowSteps = p.steps
	p.ready = true

	p.since, p.frames, p.allNs, p.simNs, p.steps, p.simT = time.Now(), 0, 0, 0, 0, 0
}

// pred records the cost of a prediction, which is recomputed at most twice a second
// and so is reported as itself rather than averaged into a window.
func (p *perf) pred(ns int64) {
	if ns > 0 {
		p.predMs = float64(ns) / 1e6
	}
}

// lines is the readout, one string per row. Two of them are conditional: there is
// nothing to say about a prediction that has not been made, and the warp achieved
// is only interesting when it is not the warp asked for.
func (p *perf) lines(warpAsked float64, hist int) []string {
	if !p.ready {
		return nil
	}
	// One line per concern, each carrying its own units: the physics with what it
	// cost and what it bought, then the interface, which is everything else in the
	// frame.
	sim := fmt.Sprintf("%s %.2f ms", T("perf.sim"), p.simMs)
	if p.stepsPerFrame > 0 {
		sim += fmt.Sprintf(" · %s %.0f", T("perf.steps"), p.stepsPerFrame)
	}
	// The cost of a step only once the window has enough of them to be a measurement
	// rather than the clock's resolution: in a browser time.Now() is quantised to
	// about a tenth of a millisecond, so four steps at a step apiece says nothing.
	if p.windowSteps >= perfMinSteps {
		sim += fmt.Sprintf(" · %.2f µs", p.usPerStep)
	}
	// The frame period as well as the rate, because the two lines below are the work
	// done inside Update and the difference from this one is everything Ebiten spends
	// rasterising and presenting afterwards. Four milliseconds of interface at eight
	// frames a second is not a contradiction: it is where the other hundred and
	// twenty went, and saying so is the whole point of showing both.
	fps := ebiten.ActualFPS()
	head := fmt.Sprintf("%.0f fps · %.0f tps", fps, ebiten.ActualTPS())
	if fps > 0 {
		head += fmt.Sprintf(" · %.1f ms", 1000/fps)
	}
	out := []string{
		head,
		sim,
		fmt.Sprintf("%s %.2f ms", T("perf.draw"), p.allMs-p.simMs),
	}
	// A tenth either way is the accumulator carrying a remainder between frames, not
	// the simulation falling behind.
	if warpAsked > 0 && (p.warpDone < warpAsked*0.9 || p.warpDone > warpAsked*1.1) {
		// Two decimals while it is small, because at ×1 asked and a third delivered
		// the interesting figure is 0.33 and rounding it to zero says nothing.
		digits := 0
		if p.warpDone < 10 {
			digits = 2
		}
		out = append(out, fmt.Sprintf("%s ×%s → ×%s", T("perf.warp"),
			formatNum(warpAsked, 0), formatNum(p.warpDone, digits)))
	}
	if p.predMs > 0 {
		out = append(out, fmt.Sprintf("%s %.1f ms · %s %d", T("perf.pred"), p.predMs,
			T("perf.hist"), hist))
	} else {
		out = append(out, fmt.Sprintf("%s %d", T("perf.hist"), hist))
	}
	return out
}
