package main

import (
	"fmt"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/pavel-krush/fsim/sim"
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

	// The visible slice of the timeline. A flight to the Moon is four days long
	// and its ascent is the first ten minutes: on one axis for the whole thing
	// the interesting part is two pixels wide, so the axis has to be movable.
	t0, t1  float64
	ranged  bool
	panning bool
	panT    float64 // time under the cursor when the pan began
}

func NewGraphScreen(s *sim.Sim) *GraphScreen {
	return &GraphScreen{s: s, hover: -1}
}

// flightEnd is where the axis ends when it is showing everything: shortly after
// the last thing that happened, because an orbit has no end and the history keeps
// growing while nothing happens.
func (g *GraphScreen) flightEnd() float64 {
	h := g.s.Hist
	if len(h) == 0 {
		return 1
	}
	end := h[len(h)-1].T
	if evs := g.s.Events; len(evs) > 0 {
		if last := evs[len(evs)-1].T * 1.05; last > 0 && last < end {
			end = last
		}
	}
	if end <= 0 {
		end = 1
	}
	return end
}

// showAll puts the whole flight on the axis.
func (g *GraphScreen) showAll() {
	g.t0, g.t1, g.ranged = 0, g.flightEnd(), true
}

// showAscent puts the axis on the launch: everything up to the verdict, which is
// the part the pitch programme is judged by.
func (g *GraphScreen) showAscent() {
	end := g.flightEnd()
	for _, e := range g.s.Events {
		if e.Kind == sim.EvOrbit {
			end = e.T * 1.15
			break
		}
	}
	g.t0, g.t1, g.ranged = 0, end, true
}

// zoomAxis scales the visible range about a fixed instant, and clampAxis keeps it
// inside the flight.
func (g *GraphScreen) zoomAxis(about, factor float64) {
	span := (g.t1 - g.t0) * factor
	f := 0.5
	if g.t1 > g.t0 {
		f = clamp((about-g.t0)/(g.t1-g.t0), 0, 1)
	}
	g.t0, g.t1 = about-span*f, about+span*(1-f)
	g.clampAxis()
}

func (g *GraphScreen) clampAxis() {
	end := g.flightEnd()
	// A second is as far in as it is worth going: the history is recorded at
	// tenths, so anything tighter is drawing between samples.
	if g.t1-g.t0 < 1 {
		mid := (g.t0 + g.t1) / 2
		g.t0, g.t1 = mid-0.5, mid+0.5
	}
	if span := g.t1 - g.t0; span >= end {
		g.t0, g.t1 = 0, end
		return
	}
	if g.t0 < 0 {
		g.t1 -= g.t0
		g.t0 = 0
	}
	if g.t1 > end {
		g.t0 -= g.t1 - end
		g.t1 = end
	}
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
	case sim.OutcomeOrbit, sim.OutcomeCaptured, sim.OutcomeReturned, sim.OutcomeEscape:
		vc = colGood
	case sim.OutcomeDecaying, sim.OutcomeSuborbital:
		vc = colWarn
	case sim.OutcomeFlying:
		vc = colTextDim
	}
	drawText(dst, outcomeText(st.Outcome, bodyName(g.s.Cfg.System.Bodies[st.OutcomeBody].Name)), fontBig, r.X+14, r.Y+10, vc, alignLeft)

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
		{T("common.maxQ"), fmtEng(q, T("unit.pa"))},
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
func (g *GraphScreen) drawEventRuler(dst *ebiten.Image, r Rect) {
	panel(dst, r, colPanel)
	if g.t1 <= g.t0 {
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
		if e.T < g.t0 || e.T > g.t1 {
			continue
		}
		x := plot.X + (e.T-g.t0)/(g.t1-g.t0)*plot.W
		c := colWarn
		switch e.Kind {
		case sim.EvEnd, sim.EvOrbit:
			c = colGood
		case sim.EvMaxQ:
			c = colMaxQ
		case sim.EvLiftoff, sim.EvApoapsis:
			c = colTextDim
		}

		row := -1
		for i := range lastX {
			if x >= lastX[i]+6 {
				row = i
				break
			}
		}
		line(dst, x, r.Y, x, r.Bottom(), 1, c)
		if row < 0 {
			// Every row is still occupied at this x. Zoomed out to four days, the
			// whole ascent lands inside two pixels and eight labels used to print
			// on top of each other; the tick is the whole of what a marker can
			// usefully say there, and zooming in gets the words back.
			continue
		}
		label := eventLabel(e, &g.s.Cfg.System)
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
	if !g.ranged {
		g.showAll()
	}

	u := a.ui
	// The plot area in x, which is what the axis maps onto.
	axisX, axisR := r.X+56, r.Right()-14
	atCursor := func() float64 {
		f := clamp((u.MX-axisX)/(axisR-axisX), 0, 1)
		return g.t0 + f*(g.t1-g.t0)
	}

	if u.hover(r) {
		if u.Wheel != 0 {
			g.zoomAxis(atCursor(), math.Exp(-u.Wheel*0.2))
		}
		if u.Click {
			g.panning, g.panT = true, atCursor()
		}
	}
	if !u.Down {
		g.panning = false
	}
	if g.panning {
		// Drag the timeline under the cursor: whatever instant was grabbed stays
		// grabbed, which is the only panning gesture nobody has to be taught.
		if shift := g.panT - atCursor(); shift != 0 {
			g.t0 += shift
			g.t1 += shift
			g.clampAxis()
		}
	}

	// The cursor scrubs every plot at once.
	g.hover = -1
	if u.hover(r) && !g.panning {
		g.hover = sampleAt(h, atCursor())
	}

	rulerH := rulerRows*(fontMonoSm.Size+4) + 6
	g.drawEventRuler(dst, Rect{r.X, r.Y, r.W, rulerH})

	plots := Rect{r.X, r.Y + rulerH + 6, r.W, r.H - rulerH - 6}
	all := plotSeries()
	n := len(all)
	gap := 6.0
	ph := (plots.H - gap*float64(n-1)) / float64(n)
	for i, s := range all {
		g.drawPlot(dst, Rect{plots.X, plots.Y + float64(i)*(ph+gap), plots.W, ph},
			s, plotColors[i%len(plotColors)], i == n-1)
	}
}

// drawPlot renders one series with its own vertical scale.
func (g *GraphScreen) drawPlot(dst *ebiten.Image, r Rect, s series, c color.NRGBA, showTimeAxis bool) {
	panel(dst, r, colPanel)
	h := g.s.Hist

	// Scaled to what is on screen, not to the whole flight. Zoomed into the
	// ascent of a lunar mission, a vertical axis sized by four days of orbit
	// would draw the entire launch along the bottom edge.
	first, last := g.visibleRange()
	lo, hi := math.Inf(1), math.Inf(-1)
	for i := first; i <= last; i++ {
		v := s.pick(h[i]) / s.scale
		lo, hi = math.Min(lo, v), math.Max(hi, v)
	}
	if math.IsInf(lo, 1) {
		lo, hi = 0, 1
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
	toX := func(t float64) float64 { return plot.X + (t-g.t0)/(g.t1-g.t0)*plot.W }

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
		if e.T < g.t0 || e.T > g.t1 {
			continue
		}
		x := toX(e.T)
		line(dst, x, plot.Y, x, plot.Bottom(), 1, colGrid)
	}

	clip := plot.Sub(dst)
	if clip == nil {
		return
	}
	g.drawTrace(clip, h[first:last+1], s, c, toX, toY)

	drawText(dst, s.name+", "+s.unit, fontUISm, plot.X+6, r.Y+4, c, alignLeft)

	// The scrubber: a vertical line plus the value at that instant.
	if g.hover >= 0 && g.hover < len(h) && h[g.hover].T >= g.t0 && h[g.hover].T <= g.t1 {
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
		span := g.t1 - g.t0
		for i := 0; i <= 6; i++ {
			// Not named t: that is the translation helper.
			at := g.t0 + span*float64(i)/6
			a := alignCenter
			switch i {
			case 0:
				a = alignLeft
			case 6:
				a = alignRight
			}
			drawText(dst, axisTime(at, span), fontMonoSm,
				toX(at), r.Bottom()-fontMonoSm.Size-1, colTextFaint, a)
		}
	}
}

// visibleRange is the slice of history inside the axis, with one sample either
// side so the trace enters and leaves the plot rather than starting inside it.
func (g *GraphScreen) visibleRange() (int, int) {
	h := g.s.Hist
	if len(h) == 0 {
		// An empty slice rather than a negative index: nothing calls this without
		// history today, and nothing should crash if something starts to.
		return 0, -1
	}
	first := sampleAt(h, g.t0)
	last := sampleAt(h, g.t1)
	if first > 0 {
		first--
	}
	if last < len(h)-1 {
		last++
	}
	if last < first {
		last = first
	}
	return first, last
}

// drawTrace draws one series, decimated by pixel column.
//
// Four days of orbit at one sample every five seconds is seventy thousand points,
// fifty to a pixel, and drawing every one of them is both slow and no more
// accurate. Each column is drawn as the range its samples covered, so a max-q
// spike between two pixels is still a spike and not an average.
func (g *GraphScreen) drawTrace(dst *ebiten.Image, h []sim.Sample, s series,
	c color.NRGBA, toX, toY func(float64) float64) {
	if len(h) == 0 {
		return
	}

	col := math.Floor(toX(h[0].T))
	lo, hi := toY(s.pick(h[0])/s.scale), toY(s.pick(h[0])/s.scale)
	lastY := lo
	prevX, prevY := col, lo
	joined := false

	flush := func() {
		if !joined {
			prevX, prevY, joined = col, lastY, true
			line(dst, col, lo, col, hi, 1.5, c)
			return
		}
		// Join to where the previous column left off, then show the spread.
		line(dst, prevX, prevY, col, lastY, 1.5, c)
		line(dst, col, lo, col, hi, 1.5, c)
		prevX, prevY = col, lastY
	}

	for i := 1; i < len(h); i++ {
		x, y := math.Floor(toX(h[i].T)), toY(s.pick(h[i])/s.scale)
		if x != col {
			flush()
			col, lo, hi = x, y, y
		}
		lo, hi = math.Min(lo, y), math.Max(hi, y)
		lastY = y
	}
	flush()
}

// axisTime labels the time axis at whatever scale the visible span deserves: an
// ascent is read in seconds and a transfer in days, and the same axis has to do
// both.
func axisTime(at, span float64) string {
	secs := int(math.Round(at))
	switch {
	case span < 600:
		return fmt.Sprintf("%.0f%s", at, T("unit.s"))
	case span < 3*3600:
		return fmt.Sprintf("%d:%02d", secs/60, secs%60)
	case span < 3*86400:
		return fmt.Sprintf("%dh%02dm", secs/3600, secs%3600/60)
	default:
		return fmt.Sprintf("%dd%02dh", secs/86400, secs%86400/3600)
	}
}

// buttonIf marks a button as the state currently in force.
func buttonIf(active bool) ButtonStyle {
	if active {
		return ButtonActive
	}
	return ButtonNormal
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

	// The axis controls. Dragging and the wheel do the fine work; these two are
	// the way back out of it.
	whole := g.t0 <= 0 && g.t1 >= g.flightEnd()-1e-9
	if u.Button(dst, Rect{r.X + 492, by, 110, bh}, T("graph.rangeAll"), buttonIf(whole)) {
		g.showAll()
	}
	if u.Button(dst, Rect{r.X + 610, by, 130, bh}, T("graph.rangeAscent"), ButtonNormal) {
		g.showAscent()
	}

	u.LangPicker(dst, Rect{r.Right() - 10 - langPickerW, by, langPickerW, bh})
	infoRight := r.Right() - 20 - langPickerW
	// Whatever is left between the last button and the language picker. Both of the things
	// drawn here are right-aligned against the picker while the buttons grow from the left, so
	// without this the scrubber's readout printed through the range buttons — which is what the
	// screenshots of this screen had been showing.
	room := infoRight - (r.X + 750)

	if g.hover >= 0 && g.hover < len(g.s.Hist) {
		sm := g.s.Hist[g.hover]
		info := fmt.Sprintf("%s   h %s   v %s   %s %s   %s %s",
			fmtClock(sm.T), fmtEng(sm.Alt, T("unit.m")), speed(sm.Speed),
			T("common.apoapsis"), altText(sm.ApoAlt),
			T("common.periapsis"), altText(sm.PeriAlt))
		if textWidth(info, fontMono) <= room {
			drawText(dst, info, fontMono, infoRight, r.Y+(r.H-fontMono.Size)/2-1, colTextDim, alignRight)
		}
	} else if textWidth(T("graph.hint"), fontUISm) <= room {
		drawText(dst, T("graph.hint"),
			fontUISm, infoRight, r.Y+(r.H-fontUISm.Size)/2, colTextFaint, alignRight)
	}
}
