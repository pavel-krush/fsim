package main

import (
	"fmt"
	"math"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/pavel-krush/fsim/sim"
)

// The second screen: what the mission you just picked actually is, before the four
// columns of numbers that let you change it.
//
// Picking a row used to drop straight into the editor, which is a lot of parameters to be
// handed with no idea what they add up to — "Proton-K / Blok DM to geostationary" says
// nothing about three burns and five and a half hours of coasting. So there is a page in
// between: what the real mission was, what this one does, and the figures worth knowing
// before flying it.
//
// The prose lives in the locale files, keyed by the preset's identifier, because the
// physics package holds no text. The figures are read out of the configuration and not
// from a flight: the grand tour takes half a minute to fly and nobody is waiting for that
// to read a description.

// fmtSpan writes a length of time the way a description wants it rather than the way the
// mission clock does: "T+10957d 12:00:00" is a correct answer to how long the grand tour
// is allowed to run and a useless one to read.
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

// MissionScreen shows one mission in detail.
type MissionScreen struct {
	// sel is the row picked in the list, which is an index into the same list — the
	// presets plus, possibly, the saved setup on the end.
	sel  int
	body Scroll
}

func NewMissionScreen(sel int) *MissionScreen { return &MissionScreen{sel: sel} }

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

// proceed opens the editor on this mission, which is what the list used to do directly.
func (s *MissionScreen) proceed(a *App) {
	cfg, ok := missionConfig(s.sel)
	if !ok {
		return
	}
	// Every field in the editor is bound to an address inside the configuration being
	// replaced, so the pending edit goes first — the same care loadPreset takes.
	a.ui.cancel()
	a.cfg = cfg
	a.cfg.EnsureSystem()
	// Which preset the editor's dropdown shows. The saved setup came from nowhere in
	// particular and the dropdown has no way to say so, so it says the first one.
	preset := 0
	if s.sel >= 0 && s.sel < len(sim.Presets()) {
		preset = s.sel
	}
	a.setup = NewSetupScreen(preset)
	a.screen = ScreenSetup
}

func (s *MissionScreen) back(a *App) {
	a.ui.cancel()
	a.presets = NewPresetScreen(s.sel)
	a.screen = ScreenPresets
}

func (s *MissionScreen) Update(a *App, dst *ebiten.Image) {
	u := a.ui
	b := a.Bounds()

	if u.keyPressed(ebiten.KeyEscape) {
		s.back(a)
		return
	}
	if u.keyPressed(ebiten.KeyEnter) || u.keyPressed(ebiten.KeyNumpadEnter) {
		s.proceed(a)
		return
	}

	const pad = 12
	headH, footH := 44.0, 52.0
	panel(dst, Rect{pad, pad, b.W - 2*pad, headH}, colPanel)
	drawText(dst, "FSIM", fontBig, pad+14, pad+(headH-fontBig.Size)/2-2, colAccent, alignLeft)
	drawText(dst, T("setup.tagline"), fontUISm, pad+14+textWidth("FSIM", fontBig)+10,
		pad+(headH-fontUISm.Size)/2, colTextFaint, alignLeft)
	u.LangPicker(dst, Rect{b.W - pad - 10 - langPickerW, pad + 8, langPickerW, headH - 16})

	body := Rect{pad, pad + headH + 8, b.W - 2*pad, b.H - headH - footH - 3*pad - 8}
	panel(dst, body, colPanel)

	// The facts sit in a column of their own on the right, wide enough for the longest
	// label in either language, and the prose takes what is left. In a narrow window
	// the facts go under the prose instead of squeezing both.
	factsW := math.Min(340, body.W*0.42)
	stacked := body.W < 720
	prose := Rect{body.X + 18, body.Y + 14, body.W - 36, body.H - 28}
	facts := Rect{}
	if !stacked {
		// The prose is capped at a readable measure rather than stretched to the
		// window: a line of a hundred and forty characters is a line nobody follows
		// back to its start. The facts keep the right-hand edge.
		prose.W = math.Min(proseMeasure, body.W-factsW-54)
		facts = Rect{body.Right() - 18 - factsW, body.Y + 14, factsW, body.H - 28}
	}

	s.drawProse(a, dst, prose, stacked, factsW)
	if !stacked {
		s.drawFacts(a, dst, facts)
	}

	// The two ways on: back to the list, or into the editor. Enter and Escape do the
	// same, and the hint says so.
	foot := Rect{pad, b.H - footH - pad, b.W - 2*pad, footH}
	panel(dst, foot, colPanel)
	if u.Button(dst, Rect{foot.X + 12, foot.Y + 10, 150, foot.H - 20}, T("mission.back"), ButtonNormal) {
		s.back(a)
		return
	}
	if u.Button(dst, Rect{foot.Right() - 12 - 220, foot.Y + 10, 220, foot.H - 20},
		T("mission.open"), ButtonPrimary) {
		s.proceed(a)
		return
	}
	drawText(dst, T("mission.hint"), fontUISm, foot.X+foot.W/2, foot.Y+(foot.H-fontUISm.Size)/2,
		colTextDim, alignCenter)
}

// drawProse is the title and the text: what the real mission was, and what this one does
// here. In a narrow window the figures follow the text in the same scrolling column.
func (s *MissionScreen) drawProse(a *App, dst *ebiten.Image, r Rect, stacked bool, factsW float64) {
	u := a.ui
	y := s.body.Begin(u, r)
	key := missionKey(s.sel)

	name, slug := missionTitle(s.sel)
	drawText(dst, name, fontBig, r.X, y, colText, alignLeft)
	if slug != "" {
		drawText(dst, slug, fontMono, r.X+textWidth(name, fontBig)+14, y+fontBig.Size-fontMono.Size-2,
			colTextFaint, alignLeft)
	}
	y += fontBig.Size + 16

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
		s.factRows(a, dst, Rect{r.X, y, math.Min(factsW, r.W), 0}, &y)
	}
	s.body.End(u, dst, r, y)
}

// drawFacts is the right-hand column: everything worth knowing that can be read straight
// off the configuration.
func (s *MissionScreen) drawFacts(a *App, dst *ebiten.Image, r Rect) {
	y := r.Y
	s.factRows(a, dst, r, &y)
}

func (s *MissionScreen) factRows(a *App, dst *ebiten.Image, r Rect, y *float64) {
	u := a.ui
	cfg, ok := missionConfig(s.sel)
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
