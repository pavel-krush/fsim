package main

import (
	"fmt"
	"math"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"

	"fsim/sim"
)

// SetupScreen is the parameter form: planet, atmosphere, rocket, pitch
// programme, plus the derived numbers that tell you whether the thing can fly
// before you press launch.
type SetupScreen struct {
	colPlanet Scroll
	colAtmo   Scroll
	colRocket Scroll
	colProg   Scroll
}

func NewSetupScreen() *SetupScreen { return &SetupScreen{} }

// Update draws the whole screen and handles its input.
func (s *SetupScreen) Update(a *App, dst *ebiten.Image) {
	b := a.Bounds()

	// Keep the derived planet quantities in step with whatever was typed.
	a.cfg.Body.Normalize()

	const pad = 12
	headH := 44.0
	footH := 108.0

	s.drawHeader(a, dst, Rect{pad, pad, b.W - 2*pad, headH})

	body := Rect{pad, pad + headH + 8, b.W - 2*pad, b.H - headH - footH - 3*pad - 8}
	colW := (body.W - 3*pad) / 4

	s.column(a, dst, Rect{body.X, body.Y, colW, body.H}, T("setup.secPlanet"), &s.colPlanet, s.planetRows)
	s.column(a, dst, Rect{body.X + colW + pad, body.Y, colW, body.H}, T("setup.secAtmosphere"), &s.colAtmo, s.atmoRows)
	s.column(a, dst, Rect{body.X + 2*(colW+pad), body.Y, colW, body.H}, T("common.vehicle"), &s.colRocket, s.rocketRows)
	s.column(a, dst, Rect{body.X + 3*(colW+pad), body.Y, colW, body.H}, T("setup.secPitch"), &s.colProg, s.programRows)

	s.drawFooter(a, dst, Rect{pad, b.H - footH - pad, b.W - 2*pad, footH})
}

// rowCursor lays out a column of form rows from the top down.
type rowCursor struct {
	x, y, w float64
}

func (c *rowCursor) next(h float64) Rect {
	r := Rect{c.x, c.y, c.w, h}
	c.y += h
	return r
}

func (c *rowCursor) gap(h float64) { c.y += h }

// column draws one titled, scrollable column of the form.
func (s *SetupScreen) column(a *App, dst *ebiten.Image, r Rect, title string, sc *Scroll, rows func(*App, *ebiten.Image, *rowCursor)) {
	u := a.ui
	panel(dst, r, colPanel)

	head := Rect{r.X + 10, r.Y + 6, r.W - 20, 20}
	u.SectionHeader(dst, head, title)

	inner := Rect{r.X + 10, head.Bottom() + 4, r.W - 20, r.Bottom() - head.Bottom() - 12}
	clip := inner.Sub(dst)
	if clip == nil {
		return
	}

	y := sc.Begin(u, inner)
	c := &rowCursor{x: inner.X, y: y, w: inner.W - 8}
	rows(a, clip, c)
	sc.End(u, dst, inner, c.y)
}

func (s *SetupScreen) planetRows(a *App, dst *ebiten.Image, c *rowCursor) {
	b := &a.cfg.Body
	u := a.ui

	// Bound to the radius with a halved scale, so the widget keeps a stable
	// identity across frames: the field edits the very value it displays.
	u.NumField(dst, c.next(rowH), T("setup.diameter"), &b.Radius, NumOpt{Unit: T("unit.km"), Scale: 500, Min: 500, Max: 1e10, Info: "setup.diameter.info"})
	u.ReadOnly(dst, c.next(rowH), T("setup.radius"), formatNum(b.Radius/1000, 1), T("unit.km"))

	c.gap(8)
	drawText(dst, T("setup.massSourceLabel"), fontUISm, c.x, c.next(16).Y+2, colTextFaint, alignLeft)

	src := (*int)(&b.MassSource)
	u.Radio(dst, c.next(18), T("setup.massSourceMass"), src, int(sim.FromMass))
	u.Radio(dst, c.next(18), T("setup.massSourceDensity"), src, int(sim.FromDensity))
	u.Radio(dst, c.next(18), T("setup.massSourceGravity"), src, int(sim.FromSurfaceG))
	c.gap(6)

	// Exactly one of the three is editable; the other two follow from it.
	switch b.MassSource {
	case sim.FromMass:
		u.NumField(dst, c.next(rowH), T("setup.mass"), &b.Mass, NumOpt{Unit: T("unit.e21kg"), Scale: 1e21, Min: 1e10, Max: 1e30, Info: "setup.mass.info"})
		u.ReadOnly(dst, c.next(rowH), T("setup.density"), formatNum(b.Density, 0), T("unit.kgm3"))
		u.ReadOnly(dst, c.next(rowH), T("setup.surfaceGravity"), formatNum(b.SurfaceG, 3), T("unit.mps2"))
	case sim.FromDensity:
		u.ReadOnly(dst, c.next(rowH), T("setup.mass"), formatNum(b.Mass/1e21, 3), T("unit.e21kg"))
		u.NumField(dst, c.next(rowH), T("setup.density"), &b.Density, NumOpt{Unit: T("unit.kgm3"), Min: 1, Max: 1e6, Info: "setup.density.info"})
		u.ReadOnly(dst, c.next(rowH), T("setup.surfaceGravity"), formatNum(b.SurfaceG, 3), T("unit.mps2"))
	case sim.FromSurfaceG:
		u.ReadOnly(dst, c.next(rowH), T("setup.mass"), formatNum(b.Mass/1e21, 3), T("unit.e21kg"))
		u.ReadOnly(dst, c.next(rowH), T("setup.density"), formatNum(b.Density, 0), T("unit.kgm3"))
		u.NumField(dst, c.next(rowH), T("setup.surfaceGravity"), &b.SurfaceG, NumOpt{Unit: T("unit.mps2"), Min: 0.001, Max: 1000, Info: "setup.surfaceGravity.info"})
	}

	c.gap(8)
	u.NumField(dst, c.next(rowH), T("setup.rotationPeriod"), &b.RotationPeriod, NumOpt{Unit: T("unit.h"), Scale: 3600, Min: 0, Max: 1e9, Info: "setup.rotationPeriod.info"})
	u.ReadOnly(dst, c.next(rowH), T("setup.equatorialSpeed"), formatNum(b.EquatorialSpeed(), 1), T("unit.mps"))

	c.gap(10)
	u.SectionHeader(dst, c.next(20), T("setup.secTarget"))
	u.NumField(dst, c.next(rowH), T("setup.orbitAltitude"), &a.cfg.TargetOrbit, NumOpt{Unit: T("unit.km"), Scale: 1000, Min: 0, Max: 1e9, Info: "setup.orbitAltitude.info"})
	u.ReadOnly(dst, c.next(rowH), T("setup.circularSpeed"), formatNum(b.CircularSpeed(a.cfg.TargetOrbit), 0), T("unit.mps"))
	u.ReadOnly(dst, c.next(rowH), T("setup.escapeSpeed"), formatNum(b.EscapeSpeed(0), 0), T("unit.mps"))
	u.NumField(dst, c.next(rowH), T("setup.timeLimit"), &a.cfg.MaxTime, NumOpt{Unit: T("unit.min"), Scale: 60, Min: 60, Max: 1e6, Info: "setup.timeLimit.info"})
}

func (s *SetupScreen) atmoRows(a *App, dst *ebiten.Image, c *rowCursor) {
	at := &a.cfg.Atmo
	u := a.ui

	u.NumField(dst, c.next(rowH), T("setup.surfacePressure"), &at.SurfacePressure, NumOpt{Unit: T("unit.kpa"), Scale: 1000, Min: 0, Max: 1e8, Info: "setup.surfacePressure.info"})
	u.NumField(dst, c.next(rowH), T("setup.temperature"), &at.SurfaceTemp, NumOpt{Unit: "K", Min: 1, Max: 5000, Info: "setup.temperature.info"})
	u.NumField(dst, c.next(rowH), T("setup.upperBoundary"), &at.Top, NumOpt{Unit: T("unit.km"), Scale: 1000, Min: 0, Max: 1e7, Info: "setup.upperBoundary.info"})

	c.gap(10)
	u.SectionHeader(dst, c.next(20), T("setup.secComposition"))
	if len(at.Fractions) < len(sim.Gases) {
		f := make([]float64, len(sim.Gases))
		copy(f, at.Fractions)
		at.Fractions = f
	}
	var sum float64
	for i := range sim.Gases {
		sum += at.Fractions[i]
	}
	for i := range sim.Gases {
		u.NumField(dst, c.next(rowH), gasLabel(sim.Gases[i].Name), &at.Fractions[i],
			NumOpt{Unit: "%", Scale: 0.01, Min: 0, Max: 1, Info: "setup.composition.info"})
	}
	u.ReadOnly(dst, c.next(rowH), T("setup.total"), formatNum(sum*100, 1), "%")

	// The mixture properties are what the composition actually buys you.
	at.Prepare(a.cfg.Body.SurfaceG)
	u.ReadOnly(dst, c.next(rowH), T("setup.molarMass"), formatNum(at.MolarMass()*1000, 2), T("unit.gmol"))
	u.ReadOnly(dst, c.next(rowH), T("setup.gamma"), formatNum(at.Gamma(), 3), "")
	st := at.State(0)
	u.ReadOnly(dst, c.next(rowH), T("setup.surfaceDensity"), formatNum(st.Density, 4), T("unit.kgm3"))
	u.ReadOnly(dst, c.next(rowH), T("setup.speedOfSound"), formatNum(st.Sound, 1), T("unit.mps"))

	c.gap(10)
	u.SectionHeader(dst, c.next(20), T("setup.secLayers"))
	drawText(dst, T("setup.layerColumns"), fontUISm, c.x, c.next(15).Y+1, colTextFaint, alignLeft)

	remove := -1
	for i := range at.Layers {
		r := c.next(rowH)
		half := (r.W - 26) / 2
		u.NumField(dst, Rect{r.X, r.Y, half, r.H}, "", &at.Layers[i].BaseAlt,
			NumOpt{Unit: T("unit.km"), Scale: 1000, Min: 0, Max: 1e7})
		u.NumField(dst, Rect{r.X + half + 4, r.Y, half, r.H}, "", &at.Layers[i].Lapse,
			NumOpt{Unit: T("unit.kPerKm"), Scale: 0.001, Min: -0.2, Max: 0.2})
		if u.Button(dst, Rect{r.Right() - 18, r.Y + 2, 18, r.H - 4}, "×", ButtonDanger) {
			remove = i
		}
	}
	if remove >= 0 && len(at.Layers) > 1 {
		at.Layers = append(at.Layers[:remove], at.Layers[remove+1:]...)
	}
	if u.Button(dst, c.next(rowH+2), T("setup.addLayer"), ButtonNormal) {
		top := 0.0
		if n := len(at.Layers); n > 0 {
			top = at.Layers[n-1].BaseAlt + 10000
		}
		at.Layers = append(at.Layers, sim.Layer{BaseAlt: top})
	}
	c.gap(10)
}

// gasLabel renders a chemical formula the way it is written: the digits become
// subscripts, so N2 shows as N₂. The names in sim stay plain ASCII, because
// there they are lookup keys for the presets, not display text.
func gasLabel(name string) string {
	var b strings.Builder
	for _, r := range name {
		if r >= '0' && r <= '9' {
			b.WriteRune('₀' + (r - '0'))
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func (s *SetupScreen) rocketRows(a *App, dst *ebiten.Image, c *rowCursor) {
	rk := &a.cfg.Rocket
	u := a.ui

	u.NumField(dst, c.next(rowH), T("setup.payload"), &rk.Payload,
		NumOpt{Unit: T("unit.t"), Scale: 1000, Min: 0, Max: 1e9, Info: "setup.payload.info"})
	u.NumField(dst, c.next(rowH), T("setup.bodyDiameter"), &rk.Diameter, NumOpt{Unit: T("unit.m"), Min: 0.1, Max: 100, Info: "setup.bodyDiameter.info"})
	u.NumField(dst, c.next(rowH), T("setup.cd"), &rk.Cd, NumOpt{Min: 0, Max: 5, Info: "setup.cd.info"})
	u.ReadOnly(dst, c.next(rowH), T("setup.referenceArea"), formatNum(rk.Area(), 2), T("unit.m2"))

	for i := range rk.Stages {
		st := &rk.Stages[i]
		c.gap(10)
		u.SectionHeader(dst, c.next(20), fmt.Sprintf(T("setup.stageN"), i+1))

		u.NumField(dst, c.next(rowH), T("setup.dryMass"), &st.DryMass, NumOpt{Unit: T("unit.t"), Scale: 1000, Min: 1, Max: 1e9, Info: "setup.dryMass.info"})
		u.NumField(dst, c.next(rowH), T("setup.propellant"), &st.PropMass, NumOpt{Unit: T("unit.t"), Scale: 1000, Min: 0, Max: 1e9, Info: "setup.propellant.info"})
		u.NumField(dst, c.next(rowH), T("setup.thrustVac"), &st.ThrustVac, NumOpt{Unit: T("unit.kn"), Scale: 1000, Min: 0, Max: 1e9, Info: "setup.thrustVac.info"})
		u.NumField(dst, c.next(rowH), T("setup.ispVac"), &st.IspVac, NumOpt{Unit: T("unit.s"), Min: 1, Max: 10000, Info: "setup.ispVac.info"})
		u.NumField(dst, c.next(rowH), T("setup.ispSL"), &st.IspSL, NumOpt{Unit: T("unit.s"), Min: 1, Max: 10000, Info: "setup.ispSL.info"})
		u.NumField(dst, c.next(rowH), T("setup.throttle"), &st.Throttle, NumOpt{Min: 0, Max: 1, Info: "setup.throttle.info"})
		u.NumField(dst, c.next(rowH), T("setup.cutoff"), &st.CutoffTime, NumOpt{Unit: T("unit.s"), Min: 0, Max: 1e6, Info: "setup.cutoff.info"})

		if i < len(rk.Stages)-1 {
			u.NumField(dst, c.next(rowH), T("setup.sepDelay"), &st.SepDelay, NumOpt{Unit: T("unit.s"), Min: 0, Max: 600, Info: "setup.sepDelay.info"})
		}
		if i > 0 {
			drawText(dst, T("setup.ignitionLabel"), fontUISm, c.x, c.next(16).Y+2, colTextFaint, alignLeft)
			ig := (*int)(&st.Ignition)
			u.Radio(dst, c.next(18), T("setup.ignitionImmediate"), ig, int(sim.IgniteImmediate))
			u.Radio(dst, c.next(18), T("setup.ignitionDelayed"), ig, int(sim.IgniteAfterDelay))
			u.Radio(dst, c.next(18), T("setup.ignitionApoapsis"), ig, int(sim.IgniteAtApoapsis))
			if st.Ignition == sim.IgniteAfterDelay {
				u.NumField(dst, c.next(rowH), T("setup.ignitionDelay"), &st.IgnitionDelay, NumOpt{Unit: T("unit.s"), Min: 0, Max: 1e5, Info: "setup.ignitionDelay.info"})
			}
		}

		u.ReadOnly(dst, c.next(rowH), T("setup.massFlow"), formatNum(st.MassFlow(), 1), T("unit.kgs"))
		u.ReadOnly(dst, c.next(rowH), T("setup.burnTime"), formatNum(st.BurnTime(), 1), T("unit.s"))
		u.ReadOnly(dst, c.next(rowH), T("setup.stageDv"), formatNum(rk.StageDeltaV(i), 0), T("unit.mps"))
	}
	c.gap(10)
}

func (s *SetupScreen) programRows(a *App, dst *ebiten.Image, c *rowCursor) {
	p := &a.cfg.Program
	u := a.ui

	drawText(dst, T("setup.pitchHintAngles"), fontUISm, c.x, c.next(15).Y, colTextFaint, alignLeft)
	drawText(dst, T("setup.pitchHintPrograde"), fontUISm, c.x, c.next(17).Y, colTextFaint, alignLeft)

	c.gap(4)
	hdr := c.next(16)
	drawText(dst, T("setup.keyTime"), fontUISm, hdr.X, hdr.Y+1, colTextFaint, alignLeft)
	drawText(dst, T("setup.keyPitch"), fontUISm, hdr.X+hdr.W*0.34, hdr.Y+1, colTextFaint, alignLeft)
	drawText(dst, T("setup.keyPrograde"), fontUISm, hdr.X+hdr.W*0.62, hdr.Y+1, colTextFaint, alignLeft)

	remove := -1
	for i := range p.Keys {
		r := c.next(rowH)
		w := r.W - 22
		u.NumField(dst, Rect{r.X, r.Y, w * 0.31, r.H}, "", &p.Keys[i].Time, NumOpt{Min: 0, Max: 1e6, Dec: 1})
		if !p.Keys[i].Prograde {
			u.NumField(dst, Rect{r.X + w*0.34, r.Y, w * 0.26, r.H}, "", &p.Keys[i].Pitch, NumOpt{Min: -90, Max: 90, Dec: 1})
		} else {
			ghost := Rect{r.X + w*0.34, r.Y, w * 0.26, r.H}
			fillRect(dst, ghost, colPanel)
			strokeRect(dst, ghost, 1, colBorder)
			drawText(dst, T("setup.keyProgradeShort"), fontUISm, ghost.X+ghost.W/2, ghost.Y+(ghost.H-fontUISm.Size)/2, colTextFaint, alignCenter)
		}
		u.Checkbox(dst, Rect{r.X + w*0.64, r.Y, 20, r.H}, "", &p.Keys[i].Prograde)
		if u.Button(dst, Rect{r.Right() - 18, r.Y + 2, 18, r.H - 4}, "×", ButtonDanger) {
			remove = i
		}
	}
	if remove >= 0 && len(p.Keys) > 1 {
		p.Keys = append(p.Keys[:remove], p.Keys[remove+1:]...)
	}

	c.gap(4)
	add := c.next(rowH + 2)
	if u.Button(dst, Rect{add.X, add.Y, add.W/2 - 3, add.H}, T("setup.addPoint"), ButtonNormal) {
		t, pitch := 0.0, 90.0
		if n := len(p.Keys); n > 0 {
			t = p.Keys[n-1].Time + 30
			pitch = math.Max(0, p.Keys[n-1].Pitch-10)
		}
		p.Keys = append(p.Keys, sim.Keyframe{Time: t, Pitch: pitch})
	}
	if u.Button(dst, Rect{add.X + add.W/2 + 3, add.Y, add.W/2 - 3, add.H}, T("setup.sort"), ButtonNormal) {
		p.Sort()
	}

	c.gap(12)
	u.SectionHeader(dst, c.next(20), T("setup.secProfile"))
	s.drawPitchPreview(a, dst, c.next(120))
	c.gap(10)
}

// drawPitchPreview plots the interpolated pitch schedule so the keyframe table
// can be read at a glance.
func (s *SetupScreen) drawPitchPreview(a *App, dst *ebiten.Image, r Rect) {
	p := &a.cfg.Program
	panel(dst, r, colPanelHi)
	if len(p.Keys) == 0 {
		return
	}

	tMax := p.Keys[len(p.Keys)-1].Time
	if tMax <= 0 {
		tMax = 1
	}
	tMax *= 1.1

	in := r.Inset(6)
	for _, g := range []float64{0, 30, 60, 90} {
		y := in.Bottom() - g/90*in.H
		line(dst, in.X, y, in.Right(), y, 1, colGrid)
		drawText(dst, fmt.Sprintf("%.0f", g), fontUISm, in.X+2, y-fontUISm.Size, colTextFaint, alignLeft)
	}

	const steps = 120
	px, py := 0.0, 0.0
	for i := 0; i <= steps; i++ {
		t := tMax * float64(i) / steps
		// The preview cannot know the live flight path angle, so prograde
		// keyframes are shown at zero — the value they tend to late in flight.
		pitch := clamp(p.Pitch(t, 0), -90, 90)
		x := in.X + in.W*t/tMax
		y := in.Bottom() - pitch/90*in.H
		if i > 0 {
			line(dst, px, py, x, y, 1.5, colAccent)
		}
		px, py = x, y
	}
	for _, k := range p.Keys {
		if k.Time > tMax {
			continue
		}
		x := in.X + in.W*k.Time/tMax
		pitch := k.Pitch
		if k.Prograde {
			pitch = 0
		}
		circle(dst, x, in.Bottom()-clamp(pitch, -90, 90)/90*in.H, 2.5, colWarn)
	}
	drawText(dst, fmt.Sprintf("%.0f %s", tMax, T("unit.s")), fontUISm, in.Right()-2, in.Bottom()-fontUISm.Size-1, colTextFaint, alignRight)
}

// drawHeader is the title bar with the preset buttons.
func (s *SetupScreen) drawHeader(a *App, dst *ebiten.Image, r Rect) {
	u := a.ui
	panel(dst, r, colPanel)

	drawText(dst, "FSIM", fontBig, r.X+14, r.Y+(r.H-fontBig.Size)/2-2, colAccent, alignLeft)
	drawText(dst, T("setup.tagline"), fontUISm, r.X+14+textWidth("FSIM", fontBig)+10,
		r.Y+(r.H-fontUISm.Size)/2, colTextFaint, alignLeft)

	u.LangPicker(dst, Rect{r.Right() - 10 - langPickerW, r.Y + 8, langPickerW, r.H - 16})

	presets := sim.Presets()
	bw := 168.0
	x := r.Right() - 20 - langPickerW - float64(len(presets))*(bw+6)
	drawText(dst, T("setup.presetLabel"), fontUISm, x-52, r.Y+(r.H-fontUISm.Size)/2, colTextDim, alignLeft)
	for _, p := range presets {
		if u.Button(dst, Rect{x, r.Y + 8, bw, r.H - 16}, presetName(p.Name), ButtonNormal) {
			a.cfg = p.Cfg
		}
		x += bw + 6
	}
}

// drawFooter shows the derived launch numbers and the start button.
func (s *SetupScreen) drawFooter(a *App, dst *ebiten.Image, r Rect) {
	u := a.ui
	panel(dst, r, colPanel)

	rk := &a.cfg.Rocket
	b := &a.cfg.Body
	surfaceP := a.cfg.Atmo.SurfacePressure
	twr := rk.LiftoffTWR(surfaceP, b.SurfaceG)
	need := b.CircularSpeed(a.cfg.TargetOrbit) - b.EquatorialSpeed()

	type stat struct {
		label, value string
		col          interface {
			RGBA() (uint32, uint32, uint32, uint32)
		}
	}
	stats := []stat{
		{T("setup.liftoffMass"), fmt.Sprintf("%s %s", formatNum(rk.LiftoffMass()/1000, 1), T("unit.t")), colText},
		{T("setup.twr"), formatNum(twr, 2), twrColor(twr)},
		{T("setup.totalDv"), speed(rk.TotalDeltaV()), colText},
		{T("setup.neededDv"), "~" + speed(need), colTextDim},
		{T("setup.marginDv"), speed(rk.TotalDeltaV() - need*1.28), marginColor(rk.TotalDeltaV() - need*1.28)},
	}
	for i := range rk.Stages {
		stats = append(stats, stat{
			fmt.Sprintf(T("setup.stageDvN"), i+1),
			speed(rk.StageDeltaV(i)),
			colTextDim,
		})
	}

	colW := 190.0
	x := r.X + 14
	for _, st := range stats {
		drawText(dst, st.label, fontUISm, x, r.Y+14, colTextFaint, alignLeft)
		drawText(dst, st.value, fontMono, x, r.Y+32, st.col, alignLeft)
		x += colW
	}

	// A rocket that cannot lift itself is the one mistake worth shouting about.
	msg := ""
	switch {
	case twr <= 1 && surfaceP >= 0:
		msg = T("setup.warnTwr")
	case len(rk.Stages) == 0:
		msg = T("setup.warnNoStages")
	case rk.TotalDeltaV() < need:
		msg = T("setup.warnDv")
	}
	if msg != "" {
		drawText(dst, msg, fontUI, r.X+14, r.Y+62, colWarn, alignLeft)
	} else {
		drawText(dst, T("setup.readyHint"), fontUI, r.X+14, r.Y+62, colTextFaint, alignLeft)
	}

	btn := Rect{r.Right() - 190, r.Y + 18, 176, r.H - 36}
	launch := u.Button(dst, btn, T("setup.launch"), ButtonPrimary)
	if !u.Editing() && (u.keyPressed(ebiten.KeyEnter) || u.keyPressed(ebiten.KeyNumpadEnter)) {
		launch = true
	}
	if launch && len(rk.Stages) > 0 {
		a.cfg.Program.Sort()
		a.Launch()
	}
}

func twrColor(v float64) interface {
	RGBA() (uint32, uint32, uint32, uint32)
} {
	switch {
	case v < 1.05:
		return colBad
	case v < 1.25 || v > 3:
		return colWarn
	default:
		return colGood
	}
}

func marginColor(v float64) interface {
	RGBA() (uint32, uint32, uint32, uint32)
} {
	switch {
	case v < 0:
		return colBad
	case v < 300:
		return colWarn
	default:
		return colGood
	}
}
