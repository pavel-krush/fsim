package main

import (
	"fmt"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"

	"fsim/sim"
)

// series describes one plotted quantity.
type series struct {
	name  string
	unit  string
	pick  func(sim.Sample) float64
	scale float64 // divide the raw value by this before plotting
	dec   int
}

// GraphScreen plots the recorded flight on a shared time axis.
type GraphScreen struct {
	s     *sim.Sim
	hover int // index of the sample under the cursor, -1 if none
}

func NewGraphScreen(s *sim.Sim) *GraphScreen {
	return &GraphScreen{s: s, hover: -1}
}

// plotSeries is rebuilt every frame rather than cached on the screen, so that
// switching language relabels the plots immediately instead of on the next
// visit.
func plotSeries() []series {
	return []series{
		{T("common.altitude"), T("unit.km"), func(x sim.Sample) float64 { return x.Alt }, 1000, 1},
		{T("graph.inertialSpeed"), T("unit.mps"), func(x sim.Sample) float64 { return x.Speed }, 1, 0},
		{T("common.surfaceSpeed"), T("unit.mps"), func(x sim.Sample) float64 { return x.SurfSpeed }, 1, 0},
		{T("common.dynamicPressure"), T("unit.kpa"), func(x sim.Sample) float64 { return x.Q }, 1000, 2},
		{T("common.mass"), T("unit.t"), func(x sim.Sample) float64 { return x.Mass }, 1000, 1},
		{T("common.acceleration"), "g", func(x sim.Sample) float64 { return x.AccelG }, 1, 2},
		{T("common.pitch"), "°", func(x sim.Sample) float64 { return x.Pitch }, 1, 1},
	}
}

// Update draws the whole screen.
func (g *GraphScreen) Update(a *App, dst *ebiten.Image) {
	u := a.ui
	b := a.Bounds()

	const pad = 12
	headH := 76.0
	footH := 46.0

	g.drawHeader(dst, Rect{pad, pad, b.W - 2*pad, headH})

	body := Rect{pad, pad + headH + 8, b.W - 2*pad, b.H - headH - footH - 3*pad - 8}
	g.drawPlots(a, dst, body)
	g.drawFooter(a, dst, Rect{pad, b.H - footH - pad, b.W - 2*pad, footH})

	if u.keyPressed(ebiten.KeyEscape) {
		a.screen = ScreenFlight
	}
}

// drawHeader shows the verdict and the summary numbers.
func (g *GraphScreen) drawHeader(dst *ebiten.Image, r Rect) {
	panel(dst, r, colPanel)
	st := &g.s.St
	tm := g.s.Telemetry()

	vc := colBad
	switch st.Outcome {
	case sim.OutcomeOrbit:
		vc = colGood
	case sim.OutcomeDecaying, sim.OutcomeSuborbital:
		vc = colWarn
	case sim.OutcomeFlying:
		vc = colTextDim
	}
	drawText(dst, outcomeText(st.Outcome), fontBig, r.X+14, r.Y+10, vc, alignLeft)

	q, qAlt := g.s.MaxQ()
	cells := [][2]string{
		{T("graph.duration"), fmtClock(st.T)},
		{T("common.apoapsis"), altText(tm.ApoAlt)},
		{T("common.periapsis"), altText(tm.PeriAlt)},
		{T("common.eccentricity"), formatNum(tm.Ecc, 4)},
		{T("graph.dvExpended"), speed(st.DeltaV)},
		{T("graph.gravityLoss"), speed(st.GravLoss)},
		{T("graph.dragLoss"), speed(st.DragLoss)},
		{T("graph.steeringLoss"), speed(st.SteerLoss)},
		{"max q", fmtEng(q, T("unit.pa"))},
		{T("graph.atAltitude"), fmt.Sprintf("%s %s", formatNum(qAlt/1000, 1), T("unit.km"))},
		{T("common.maxAcceleration"), fmt.Sprintf("%s g", formatNum(g.s.MaxG(), 2))},
	}
	colW := (r.W - 28) / float64(len(cells))
	for i, c := range cells {
		x := r.X + 14 + float64(i)*colW
		drawText(dst, c[0], fontUISm, x, r.Y+44, colTextFaint, alignLeft)
		drawText(dst, c[1], fontMonoSm, x, r.Y+58, colText, alignLeft)
	}
}

// drawEventRuler is a strip of staged-event labels above the plots. They live
// here rather than on the traces because several events land within a few
// seconds of each other and would otherwise print on top of one another.
func (g *GraphScreen) drawEventRuler(dst *ebiten.Image, r Rect, tMax float64) {
	panel(dst, r, colPanel)
	if tMax <= 0 {
		return
	}
	axisW := 56.0
	plot := Rect{r.X + axisW, r.Y, r.W - axisW - 14, r.H}

	// Three rows. A staging sequence puts cutoff, separation and ignition
	// within a few seconds of each other, so each label goes on the first row
	// where it actually fits rather than simply alternating.
	var lastX [rulerRows]float64
	for i := range lastX {
		lastX[i] = -1e9
	}
	for _, e := range g.s.Events {
		if e.T < 0 || e.T > tMax {
			continue
		}
		x := plot.X + e.T/tMax*plot.W
		c := colWarn
		switch e.Kind {
		case sim.EvEnd, sim.EvOrbit:
			c = colGood
		case sim.EvMaxQ:
			c = colMaxQ
		case sim.EvLiftoff, sim.EvApoapsis:
			c = colTextDim
		}

		row := 0
		for i := range lastX {
			if x >= lastX[i]+6 {
				row = i
				break
			}
			// Nothing free: fall back to the row that clears earliest.
			if lastX[i] < lastX[row] {
				row = i
			}
		}
		line(dst, x, r.Y, x, r.Bottom(), 1, c)
		label := eventLabel(e.Kind)
		w := textWidth(label, fontMonoSm)
		ly := r.Y + 3 + float64(row)*(fontMonoSm.Size+4)
		if x+3+w > plot.Right() {
			// Near the right edge, hang the label off the other side of the
			// line rather than letting the panel clip it.
			drawText(dst, label, fontMonoSm, x-3, ly, c, alignRight)
			lastX[row] = x
		} else {
			drawText(dst, label, fontMonoSm, x+3, ly, c, alignLeft)
			lastX[row] = x + w + 3
		}
	}
}

// rulerRows is how many stacked label rows the event ruler offers.
const rulerRows = 3

// drawPlots stacks one panel per series over a common time axis.
func (g *GraphScreen) drawPlots(a *App, dst *ebiten.Image, r Rect) {
	h := g.s.Hist
	if len(h) < 2 {
		drawText(dst, T("graph.noData"), fontUI, r.X+r.W/2, r.Y+r.H/2, colTextDim, alignCenter)
		return
	}
	// An orbit has no end, so the history keeps growing while nothing happens.
	// Stop the axis shortly after the last thing that did, or hours of flat
	// line would squash the whole ascent into the first few pixels.
	tMax := h[len(h)-1].T
	if evs := g.s.Events; len(evs) > 0 {
		if last := evs[len(evs)-1].T * 1.05; last > 0 && last < tMax {
			tMax = last
		}
	}
	if tMax <= 0 {
		tMax = 1
	}

	// The cursor scrubs every plot at once.
	u := a.ui
	g.hover = -1
	if u.hover(r) {
		f := clamp((u.MX-r.X-56)/(r.W-70), 0, 1)
		g.hover = sampleAt(h, f*tMax)
	}

	rulerH := rulerRows*(fontMonoSm.Size+4) + 6
	g.drawEventRuler(dst, Rect{r.X, r.Y, r.W, rulerH}, tMax)

	plots := Rect{r.X, r.Y + rulerH + 6, r.W, r.H - rulerH - 6}
	all := plotSeries()
	n := len(all)
	gap := 6.0
	ph := (plots.H - gap*float64(n-1)) / float64(n)
	for i, s := range all {
		g.drawPlot(dst, Rect{plots.X, plots.Y + float64(i)*(ph+gap), plots.W, ph},
			s, plotColors[i%len(plotColors)], tMax, i == n-1)
	}
}

// drawPlot renders one series with its own vertical scale.
func (g *GraphScreen) drawPlot(dst *ebiten.Image, r Rect, s series, c color.NRGBA, tMax float64, showTimeAxis bool) {
	panel(dst, r, colPanel)
	h := g.s.Hist

	lo, hi := math.Inf(1), math.Inf(-1)
	for i := range h {
		v := s.pick(h[i]) / s.scale
		lo, hi = math.Min(lo, v), math.Max(hi, v)
	}
	if hi-lo < 1e-9 {
		hi = lo + 1
	}
	// A little headroom, and pin the axis to zero when the data is one-sided.
	pad := (hi - lo) * 0.08
	hi += pad
	if lo >= 0 {
		lo = 0
	} else {
		lo -= pad
	}

	axisW := 56.0
	plot := Rect{r.X + axisW, r.Y + fontUISm.Size + 8, r.W - axisW - 14, r.H - fontUISm.Size - 12}
	toY := func(v float64) float64 { return plot.Bottom() - (v-lo)/(hi-lo)*plot.H }
	toX := func(t float64) float64 { return plot.X + t/tMax*plot.W }

	// Horizontal grid with labels on the left. The top line sits below the
	// caption row so the two never overlap.
	for i := 0; i <= 2; i++ {
		v := lo + (hi-lo)*float64(i)/2
		y := toY(v)
		line(dst, plot.X, y, plot.Right(), y, 1, colGrid)
		drawText(dst, formatNum(v, s.dec), fontMonoSm, r.X+axisW-6, y-fontMonoSm.Size/2-1, colTextFaint, alignRight)
	}

	// Event guides, drawn under the trace. Their labels are in the ruler.
	for _, e := range g.s.Events {
		if e.T <= 0 || e.T > tMax {
			continue
		}
		x := toX(e.T)
		line(dst, x, plot.Y, x, plot.Bottom(), 1, colGrid)
	}

	clip := plot.Sub(dst)
	if clip == nil {
		return
	}
	px, py := toX(h[0].T), toY(s.pick(h[0])/s.scale)
	for i := 1; i < len(h); i++ {
		x, y := toX(h[i].T), toY(s.pick(h[i])/s.scale)
		line(clip, px, py, x, y, 1.5, c)
		px, py = x, y
	}

	drawText(dst, s.name+", "+s.unit, fontUISm, plot.X+6, r.Y+4, c, alignLeft)

	// The scrubber: a vertical line plus the value at that instant.
	if g.hover >= 0 && g.hover < len(h) {
		sm := h[g.hover]
		x := toX(sm.T)
		line(dst, x, plot.Y, x, plot.Bottom(), 1, colAccentDim)
		y := toY(s.pick(sm) / s.scale)
		circle(dst, x, y, 3, c)

		label := fmt.Sprintf("%s %s", formatNum(s.pick(sm)/s.scale, s.dec), s.unit)
		w := textWidth(label, fontMonoSm) + 8
		bx := x + 8
		if bx+w > plot.Right() {
			bx = x - 8 - w
		}
		box := Rect{bx, plot.Y + 2, w, fontMonoSm.Size + 6}
		fillRect(dst, box, colPanelHi)
		strokeRect(dst, box, 1, colBorder)
		drawText(dst, label, fontMonoSm, box.X+4, box.Y+3, colText, alignLeft)
	}

	if showTimeAxis {
		for i := 0; i <= 6; i++ {
			// Not named t: that is the translation helper.
			at := tMax * float64(i) / 6
			a := alignCenter
			switch i {
			case 0:
				a = alignLeft
			case 6:
				a = alignRight
			}
			drawText(dst, fmt.Sprintf("%.0f%s", at, T("unit.s")), fontMonoSm,
				toX(at), r.Bottom()-fontMonoSm.Size-1, colTextFaint, a)
		}
	}
}

func (g *GraphScreen) drawFooter(a *App, dst *ebiten.Image, r Rect) {
	u := a.ui
	panel(dst, r, colPanel)

	bh := r.H - 16
	by := r.Y + 8
	if u.Button(dst, Rect{r.X + 10, by, 150, bh}, T("graph.toFlight"), ButtonNormal) {
		a.screen = ScreenFlight
	}
	if u.Button(dst, Rect{r.X + 168, by, 150, bh}, T("common.setup"), ButtonNormal) {
		a.screen = ScreenSetup
	}
	if u.Button(dst, Rect{r.X + 326, by, 150, bh}, T("graph.launchAgain"), ButtonPrimary) {
		a.Launch()
	}

	u.LangPicker(dst, Rect{r.Right() - 10 - langPickerW, by, langPickerW, bh})
	infoRight := r.Right() - 20 - langPickerW

	if g.hover >= 0 && g.hover < len(g.s.Hist) {
		sm := g.s.Hist[g.hover]
		info := fmt.Sprintf("%s   h %s   v %s   %s %s   %s %s",
			fmtClock(sm.T), fmtEng(sm.Alt, T("unit.m")), speed(sm.Speed),
			T("common.apoapsis"), altText(sm.ApoAlt),
			T("common.periapsis"), altText(sm.PeriAlt))
		drawText(dst, info, fontMono, infoRight, r.Y+(r.H-fontMono.Size)/2-1, colTextDim, alignRight)
	} else {
		drawText(dst, T("graph.hint"),
			fontUISm, infoRight, r.Y+(r.H-fontUISm.Size)/2, colTextFaint, alignRight)
	}
}
