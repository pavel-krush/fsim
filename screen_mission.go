package main

import (
	"fmt"
	"math"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/pavel-krush/fsim/sim"
)

// The description beside the mission list: what the real mission was, what this one does,
// and the figures worth knowing before flying it.
//
// The list used to drop straight into the editor, which is a lot of parameters to be handed
// with no idea what they add up to — "Proton-K / Blok DM to geostationary" says nothing about
// three burns and five and a half hours of coasting. So the list moved into a column of its
// own and this took the rest of the screen: the arrow keys are now a way of reading down the
// missions rather than only of choosing one.
//
// The prose lives in the locale files, keyed by the preset's identifier, because the physics
// package holds no text. The figures are read out of the configuration and not from a flight:
// the grand tour takes half a minute to fly and nobody is waiting for that to read a
// paragraph.

// fmtSpan writes a length of time the way a description wants it rather than the way the
// mission clock does: "T+10957d 12:00:00" is a correct answer to how long the grand tour is
// allowed to run and a useless one to read.
func fmtSpan(t float64) (string, string) {
	switch {
	case t <= 0:
		return "—", ""
	case t < 2*3600:
		return formatNum(t/60, 0), T("unit.min")
	case t < 3*86400:
		return formatNum(t/3600, 1), T("unit.h")
	case t < 400*86400:
		return formatNum(t/86400, 1), T("unit.d")
	}
	return formatNum(t/(365.25*86400), 1), T("unit.y")
}

// proseMeasure is how wide a paragraph is allowed to get. Around ninety characters at
// this size, which is the far end of comfortable and well short of what a 1500 px window
// would otherwise hand it.
const proseMeasure = 720.0

// missionPanel is the description, and all it owns is where it has been scrolled to. Which
// mission it describes is the list's business, handed in per frame.
type missionPanel struct {
	body Scroll
	// shown is the row the scroll position belongs to, so that arrowing onto a new
	// mission starts at the top of it rather than half way down the last one.
	shown int
}

// missionKey is the locale prefix for a mission's prose. The saved setup has no
// identifier and no history, so it gets one of its own.
func missionKey(sel int) string {
	presets := sim.Presets()
	if sel >= 0 && sel < len(presets) {
		return "mission." + presets[sel].Name
	}
	return "mission.saved"
}

// missionConfig is the configuration a row would fly. The saved setup is read from the
// store, which is also where the mission list got it, and a failure there simply leaves
// nothing to describe.
func missionConfig(sel int) (sim.Config, bool) {
	presets := sim.Presets()
	if sel >= 0 && sel < len(presets) {
		return presets[sel].Cfg, true
	}
	cfg, ok, err := loadConfig()
	if !ok || err != nil {
		return sim.Config{}, false
	}
	return cfg, true
}

// missionTitle is the name shown at the top, and the identifier beside it.
func missionTitle(sel int) (name, slug string) {
	presets := sim.Presets()
	if sel >= 0 && sel < len(presets) {
		return presetName(presets[sel].Name), presets[sel].Name
	}
	return T("presets.saved"), ""
}

// draw puts the description in r, and reports whether the button in it was pressed.
//
// The title spans both columns rather than sitting in the prose one: "Proton-K / Blok DM to
// geostationary" at this size is wider than half the panel, and it ran straight over the
// figures when it lived in the left-hand column.
func (p *missionPanel) draw(a *App, dst *ebiten.Image, r Rect, sel int) (open bool) {
	if sel != p.shown {
		p.shown, p.body.Offset = sel, 0
	}
	panel(dst, r, colPanel)

	inner := Rect{r.X + 18, r.Y + 14, r.W - 36, r.H - 28}

	name, slug := missionTitle(sel)
	drawText(dst, name, fontBig, inner.X, inner.Y, colText, alignLeft)
	if slug != "" {
		drawText(dst, slug, fontMono, inner.X+textWidth(name, fontBig)+14,
			inner.Y+fontBig.Size-fontMono.Size-2, colTextFaint, alignLeft)
	}
	titleH := fontBig.Size + 20

	// The way on, at the bottom of the description rather than under the list: it belongs
	// to the mission being described.
	const btnH = 38.0
	btn := Rect{inner.Right() - 220, inner.Bottom() - btnH, 220, btnH}
	open = a.ui.Button(dst, btn, T("mission.open"), ButtonPrimary)

	content := Rect{inner.X, inner.Y + titleH, inner.W, inner.H - titleH - btnH - 12}

	// The figures take a column of their own where there is room for one, and follow the
	// prose where there is not.
	factsW := math.Min(340, content.W*0.42)
	stacked := content.W < 620
	prose := content
	facts := Rect{}
	if !stacked {
		// The prose is capped at a readable measure rather than stretched to the panel:
		// a line of a hundred and forty characters is a line nobody follows back to its
		// start. The figures keep the right-hand edge.
		prose.W = math.Min(proseMeasure, content.W-factsW-36)
		facts = Rect{content.Right() - factsW, content.Y, factsW, content.H}
	}

	p.drawProse(a, dst, prose, sel, stacked, factsW)
	if !stacked {
		y := facts.Y
		p.factRows(a, dst, facts, sel, &y)
	}
	return open
}

// drawProse is the text: what the real mission was, and what this one does here. In a narrow
// panel the figures follow it in the same scrolling column.
func (p *missionPanel) drawProse(a *App, dst *ebiten.Image, r Rect, sel int, stacked bool, factsW float64) {
	u := a.ui
	y := p.body.Begin(u, r)
	key := missionKey(sel)

	para := func(head, text string) {
		if text == "" {
			return
		}
		if head != "" {
			drawText(dst, head, fontUISm, r.X, y, colAccent, alignLeft)
			y += fontUISm.Size + 8
		}
		for _, l := range wrapText(text, fontUI, r.W) {
			if l != "" {
				drawText(dst, l, fontUI, r.X, y, colTextDim, alignLeft)
			}
			y += fontUI.Size + 6
		}
		y += 10
	}

	// A missing key renders as the key, which is the toolkit's habit everywhere and the
	// fastest way to notice a mission nobody has written up yet.
	para(T("mission.secHistory"), T(key+".history"))
	para(T("mission.secHere"), T(key+".here"))

	if stacked {
		y += 6
		p.factRows(a, dst, Rect{r.X, y, math.Min(factsW, r.W), 0}, sel, &y)
	}
	p.body.End(u, dst, r, y)
}

// factRows is everything worth knowing that can be read straight off the configuration.
func (p *missionPanel) factRows(a *App, dst *ebiten.Image, r Rect, sel int, y *float64) {
	u := a.ui
	cfg, ok := missionConfig(sel)
	if !ok {
		drawText(dst, T("mission.noSaved"), fontUISm, r.X, *y, colTextFaint, alignLeft)
		*y += fontUISm.Size + 6
		return
	}
	cfg.EnsureSystem()

	row := func(label string, value ...string) {
		v, unit := value[0], ""
		if len(value) > 1 {
			unit = value[1]
		}
		u.ReadOnly(dst, Rect{r.X, *y, r.W, rowH}, label, v, unit)
		*y += rowH
	}
	head := func(title string) {
		*y += 8
		u.SectionHeader(dst, Rect{r.X, *y, r.W, 20}, title)
		*y += 22
	}

	lb := &cfg.System.Bodies[cfg.LaunchBody]

	head(T("mission.secWhere"))
	row(T("mission.launchBody"), bodyName(lb.Name))
	if n := len(cfg.System.Bodies); n > 1 {
		row(T("mission.bodies"), fmt.Sprintf("%d", n))
	}
	row(T("mission.target"), formatNum(cfg.TargetOrbit/1000, 0), T("unit.km"))
	span, spanUnit := fmtSpan(cfg.MaxTime)
	row(T("setup.timeLimit"), span, spanUnit)

	head(T("common.vehicle"))
	row(T("setup.liftoffMass"), formatNum(cfg.Rocket.LiftoffMass()/1000, 1), T("unit.t"))
	row(T("setup.stageCount"), fmt.Sprintf("%d", len(cfg.Rocket.Stages)))
	row(T("setup.payload"), formatNum(cfg.Rocket.Payload/1000, 2), T("unit.t"))
	row(T("setup.twr"), formatNum(cfg.Rocket.LiftoffTWR(lb.Atmo.SurfacePressure, lb.SurfaceG), 2))
	row(T("setup.totalDv"), formatNum(cfg.Rocket.TotalDeltaV(), 0), T("unit.mps"))

	// The flight plan, which is the mission after the ascent: one row per burn, in the
	// order they fire rather than the order they are stored.
	if len(cfg.Nodes) > 0 {
		head(T("flight.secPlan"))
		nodes := append([]sim.Node(nil), cfg.Nodes...)
		for i := range nodes {
			for j := i + 1; j < len(nodes); j++ {
				if nodes[j].T < nodes[i].T {
					nodes[i], nodes[j] = nodes[j], nodes[i]
				}
			}
		}
		for i := range nodes {
			row(fmtClock(nodes[i].T), fmt.Sprintf("%s %s", formatNum(nodes[i].DeltaV, 0),
				T("unit.mps")), nodeFrameName(nodes[i].Frame))
		}
	}
}
