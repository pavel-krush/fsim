package main

import (
	"fmt"
	"math"

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

	s.column(a, dst, Rect{body.X, body.Y, colW, body.H}, "ПЛАНЕТА", &s.colPlanet, s.planetRows)
	s.column(a, dst, Rect{body.X + colW + pad, body.Y, colW, body.H}, "АТМОСФЕРА", &s.colAtmo, s.atmoRows)
	s.column(a, dst, Rect{body.X + 2*(colW+pad), body.Y, colW, body.H}, "РАКЕТА", &s.colRocket, s.rocketRows)
	s.column(a, dst, Rect{body.X + 3*(colW+pad), body.Y, colW, body.H}, "ПРОГРАММА ТАНГАЖА", &s.colProg, s.programRows)

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
	u.NumField(dst, c.next(rowH), "Диаметр", &b.Radius, NumOpt{Unit: "км", Scale: 500, Min: 500, Max: 1e10})
	u.ReadOnly(dst, c.next(rowH), "Радиус", fmt.Sprintf("%s км", formatNum(b.Radius/1000, 1)))

	c.gap(8)
	drawText(dst, "Задавать массу через:", fontUISm, c.x, c.next(16).Y+2, colTextFaint, alignLeft)

	src := (*int)(&b.MassSource)
	u.Radio(dst, c.next(18), "массу", src, int(sim.FromMass))
	u.Radio(dst, c.next(18), "среднюю плотность", src, int(sim.FromDensity))
	u.Radio(dst, c.next(18), "g на поверхности", src, int(sim.FromSurfaceG))
	c.gap(6)

	// Exactly one of the three is editable; the other two follow from it.
	switch b.MassSource {
	case sim.FromMass:
		u.NumField(dst, c.next(rowH), "Масса", &b.Mass, NumOpt{Unit: "×10²¹ кг", Scale: 1e21, Min: 1e10, Max: 1e30})
		u.ReadOnly(dst, c.next(rowH), "Плотность", fmt.Sprintf("%s кг/м³", formatNum(b.Density, 0)))
		u.ReadOnly(dst, c.next(rowH), "g поверхности", fmt.Sprintf("%s м/с²", formatNum(b.SurfaceG, 3)))
	case sim.FromDensity:
		u.ReadOnly(dst, c.next(rowH), "Масса", fmt.Sprintf("%s ×10²¹ кг", formatNum(b.Mass/1e21, 3)))
		u.NumField(dst, c.next(rowH), "Плотность", &b.Density, NumOpt{Unit: "кг/м³", Min: 1, Max: 1e6})
		u.ReadOnly(dst, c.next(rowH), "g поверхности", fmt.Sprintf("%s м/с²", formatNum(b.SurfaceG, 3)))
	case sim.FromSurfaceG:
		u.ReadOnly(dst, c.next(rowH), "Масса", fmt.Sprintf("%s ×10²¹ кг", formatNum(b.Mass/1e21, 3)))
		u.ReadOnly(dst, c.next(rowH), "Плотность", fmt.Sprintf("%s кг/м³", formatNum(b.Density, 0)))
		u.NumField(dst, c.next(rowH), "g поверхности", &b.SurfaceG, NumOpt{Unit: "м/с²", Min: 0.001, Max: 1000})
	}

	c.gap(8)
	u.NumField(dst, c.next(rowH), "Период вращения", &b.RotationPeriod, NumOpt{Unit: "ч", Scale: 3600, Min: 0, Max: 1e9})
	u.ReadOnly(dst, c.next(rowH), "Скорость экватора", fmt.Sprintf("%s м/с", formatNum(b.EquatorialSpeed(), 1)))

	c.gap(10)
	u.SectionHeader(dst, c.next(20), "ЦЕЛЬ")
	u.NumField(dst, c.next(rowH), "Высота орбиты", &a.cfg.TargetOrbit, NumOpt{Unit: "км", Scale: 1000, Min: 0, Max: 1e9})
	u.ReadOnly(dst, c.next(rowH), "Круговая скорость", fmt.Sprintf("%s м/с", formatNum(b.CircularSpeed(a.cfg.TargetOrbit), 0)))
	u.ReadOnly(dst, c.next(rowH), "Вторая космическая", fmt.Sprintf("%s м/с", formatNum(b.EscapeSpeed(0), 0)))
	u.NumField(dst, c.next(rowH), "Лимит времени", &a.cfg.MaxTime, NumOpt{Unit: "мин", Scale: 60, Min: 60, Max: 1e6})
}

func (s *SetupScreen) atmoRows(a *App, dst *ebiten.Image, c *rowCursor) {
	at := &a.cfg.Atmo
	u := a.ui

	u.NumField(dst, c.next(rowH), "Давление у земли", &at.SurfacePressure, NumOpt{Unit: "кПа", Scale: 1000, Min: 0, Max: 1e8})
	u.NumField(dst, c.next(rowH), "Температура", &at.SurfaceTemp, NumOpt{Unit: "K", Min: 1, Max: 5000})
	u.NumField(dst, c.next(rowH), "Верхняя граница", &at.Top, NumOpt{Unit: "км", Scale: 1000, Min: 0, Max: 1e7})

	c.gap(10)
	u.SectionHeader(dst, c.next(20), "СОСТАВ")
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
		u.NumField(dst, c.next(rowH), sim.Gases[i].Name, &at.Fractions[i],
			NumOpt{Unit: "%", Scale: 0.01, Min: 0, Max: 1})
	}
	u.ReadOnly(dst, c.next(rowH), "Сумма", fmt.Sprintf("%s %%", formatNum(sum*100, 1)))

	// The mixture properties are what the composition actually buys you.
	at.Prepare(a.cfg.Body.SurfaceG)
	u.ReadOnly(dst, c.next(rowH), "Молярная масса", fmt.Sprintf("%s г/моль", formatNum(at.MolarMass()*1000, 2)))
	u.ReadOnly(dst, c.next(rowH), "Показатель γ", formatNum(at.Gamma(), 3))
	st := at.State(0)
	u.ReadOnly(dst, c.next(rowH), "Плотность у земли", fmt.Sprintf("%s кг/м³", formatNum(st.Density, 4)))
	u.ReadOnly(dst, c.next(rowH), "Скорость звука", fmt.Sprintf("%s м/с", formatNum(st.Sound, 1)))

	c.gap(10)
	u.SectionHeader(dst, c.next(20), "СЛОИ")
	drawText(dst, "высота / градиент", fontUISm, c.x, c.next(15).Y+1, colTextFaint, alignLeft)

	remove := -1
	for i := range at.Layers {
		r := c.next(rowH)
		half := (r.W - 26) / 2
		u.NumField(dst, Rect{r.X, r.Y, half, r.H}, "", &at.Layers[i].BaseAlt,
			NumOpt{Unit: "км", Scale: 1000, Min: 0, Max: 1e7})
		u.NumField(dst, Rect{r.X + half + 4, r.Y, half, r.H}, "", &at.Layers[i].Lapse,
			NumOpt{Unit: "K/км", Scale: 0.001, Min: -0.2, Max: 0.2})
		if u.Button(dst, Rect{r.Right() - 18, r.Y + 2, 18, r.H - 4}, "×", ButtonDanger) {
			remove = i
		}
	}
	if remove >= 0 && len(at.Layers) > 1 {
		at.Layers = append(at.Layers[:remove], at.Layers[remove+1:]...)
	}
	if u.Button(dst, c.next(rowH+2), "+ слой", ButtonNormal) {
		top := 0.0
		if n := len(at.Layers); n > 0 {
			top = at.Layers[n-1].BaseAlt + 10000
		}
		at.Layers = append(at.Layers, sim.Layer{BaseAlt: top})
	}
	c.gap(10)
}

func (s *SetupScreen) rocketRows(a *App, dst *ebiten.Image, c *rowCursor) {
	rk := &a.cfg.Rocket
	u := a.ui

	u.NumField(dst, c.next(rowH), "Полезная нагрузка", &rk.Payload, NumOpt{Unit: "т", Scale: 1000, Min: 0, Max: 1e9})
	u.NumField(dst, c.next(rowH), "Диаметр корпуса", &rk.Diameter, NumOpt{Unit: "м", Min: 0.1, Max: 100})
	u.NumField(dst, c.next(rowH), "Коэффициент Cd", &rk.Cd, NumOpt{Min: 0, Max: 5})
	u.ReadOnly(dst, c.next(rowH), "Площадь миделя", fmt.Sprintf("%s м²", formatNum(rk.Area(), 2)))

	for i := range rk.Stages {
		st := &rk.Stages[i]
		c.gap(10)
		u.SectionHeader(dst, c.next(20), fmt.Sprintf("СТУПЕНЬ %d", i+1))

		u.NumField(dst, c.next(rowH), "Сухая масса", &st.DryMass, NumOpt{Unit: "т", Scale: 1000, Min: 1, Max: 1e9})
		u.NumField(dst, c.next(rowH), "Топливо", &st.PropMass, NumOpt{Unit: "т", Scale: 1000, Min: 0, Max: 1e9})
		u.NumField(dst, c.next(rowH), "Тяга (вакуум)", &st.ThrustVac, NumOpt{Unit: "кН", Scale: 1000, Min: 0, Max: 1e9})
		u.NumField(dst, c.next(rowH), "Isp вакуум", &st.IspVac, NumOpt{Unit: "с", Min: 1, Max: 10000})
		u.NumField(dst, c.next(rowH), "Isp у земли", &st.IspSL, NumOpt{Unit: "с", Min: 1, Max: 10000})
		u.NumField(dst, c.next(rowH), "Дроссель", &st.Throttle, NumOpt{Min: 0, Max: 1})
		u.NumField(dst, c.next(rowH), "Отсечка (0=до конца)", &st.CutoffTime, NumOpt{Unit: "с", Min: 0, Max: 1e6})

		if i < len(rk.Stages)-1 {
			u.NumField(dst, c.next(rowH), "Задержка разделения", &st.SepDelay, NumOpt{Unit: "с", Min: 0, Max: 600})
		}
		if i > 0 {
			drawText(dst, "Зажигание:", fontUISm, c.x, c.next(16).Y+2, colTextFaint, alignLeft)
			ig := (*int)(&st.Ignition)
			u.Radio(dst, c.next(18), "сразу после разделения", ig, int(sim.IgniteImmediate))
			u.Radio(dst, c.next(18), "через задержку", ig, int(sim.IgniteAfterDelay))
			u.Radio(dst, c.next(18), "в апогее", ig, int(sim.IgniteAtApoapsis))
			if st.Ignition == sim.IgniteAfterDelay {
				u.NumField(dst, c.next(rowH), "Задержка зажигания", &st.IgnitionDelay, NumOpt{Unit: "с", Min: 0, Max: 1e5})
			}
		}

		u.ReadOnly(dst, c.next(rowH), "Расход", fmt.Sprintf("%s кг/с", formatNum(st.MassFlow(), 1)))
		u.ReadOnly(dst, c.next(rowH), "Время работы", fmt.Sprintf("%s с", formatNum(st.BurnTime(), 1)))
		u.ReadOnly(dst, c.next(rowH), "Δv ступени", fmt.Sprintf("%s м/с", formatNum(rk.StageDeltaV(i), 0)))
	}
	c.gap(10)
}

func (s *SetupScreen) programRows(a *App, dst *ebiten.Image, c *rowCursor) {
	p := &a.cfg.Program
	u := a.ui

	drawText(dst, "90° — вертикаль, 0° — горизонт.", fontUISm, c.x, c.next(15).Y, colTextFaint, alignLeft)
	drawText(dst, "«по вектору» держит тягу вдоль скорости.", fontUISm, c.x, c.next(17).Y, colTextFaint, alignLeft)

	c.gap(4)
	hdr := c.next(16)
	drawText(dst, "время, с", fontUISm, hdr.X, hdr.Y+1, colTextFaint, alignLeft)
	drawText(dst, "угол, °", fontUISm, hdr.X+hdr.W*0.34, hdr.Y+1, colTextFaint, alignLeft)
	drawText(dst, "по вектору", fontUISm, hdr.X+hdr.W*0.62, hdr.Y+1, colTextFaint, alignLeft)

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
			drawText(dst, "прогр.", fontUISm, ghost.X+ghost.W/2, ghost.Y+(ghost.H-fontUISm.Size)/2, colTextFaint, alignCenter)
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
	if u.Button(dst, Rect{add.X, add.Y, add.W/2 - 3, add.H}, "+ точка", ButtonNormal) {
		t, pitch := 0.0, 90.0
		if n := len(p.Keys); n > 0 {
			t = p.Keys[n-1].Time + 30
			pitch = math.Max(0, p.Keys[n-1].Pitch-10)
		}
		p.Keys = append(p.Keys, sim.Keyframe{Time: t, Pitch: pitch})
	}
	if u.Button(dst, Rect{add.X + add.W/2 + 3, add.Y, add.W/2 - 3, add.H}, "сортировать", ButtonNormal) {
		p.Sort()
	}

	c.gap(12)
	u.SectionHeader(dst, c.next(20), "ПРОФИЛЬ")
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
	drawText(dst, fmt.Sprintf("%.0f с", tMax), fontUISm, in.Right()-2, in.Bottom()-fontUISm.Size-1, colTextFaint, alignRight)
}

// drawHeader is the title bar with the preset buttons.
func (s *SetupScreen) drawHeader(a *App, dst *ebiten.Image, r Rect) {
	u := a.ui
	panel(dst, r, colPanel)

	drawText(dst, "FSIM", fontBig, r.X+14, r.Y+(r.H-fontBig.Size)/2-2, colAccent, alignLeft)
	drawText(dst, "симулятор выведения на орбиту", fontUISm, r.X+14+textWidth("FSIM", fontBig)+10,
		r.Y+(r.H-fontUISm.Size)/2, colTextFaint, alignLeft)

	presets := sim.Presets()
	bw := 168.0
	x := r.Right() - 10 - float64(len(presets))*(bw+6)
	drawText(dst, "пресет:", fontUISm, x-52, r.Y+(r.H-fontUISm.Size)/2, colTextDim, alignLeft)
	for _, p := range presets {
		if u.Button(dst, Rect{x, r.Y + 8, bw, r.H - 16}, p.Name, ButtonNormal) {
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
		{"Стартовая масса", fmt.Sprintf("%s т", formatNum(rk.LiftoffMass()/1000, 1)), colText},
		{"Тяговооружённость", formatNum(twr, 2), twrColor(twr)},
		{"Δv суммарный", fmt.Sprintf("%s м/с", formatNum(rk.TotalDeltaV(), 0)), colText},
		{"Нужно на орбиту", fmt.Sprintf("~%s м/с", formatNum(need, 0)), colTextDim},
		{"Запас Δv", fmt.Sprintf("%s м/с", formatNum(rk.TotalDeltaV()-need*1.28, 0)), marginColor(rk.TotalDeltaV() - need*1.28)},
	}
	for i := range rk.Stages {
		stats = append(stats, stat{
			fmt.Sprintf("Δv ступени %d", i+1),
			fmt.Sprintf("%s м/с", formatNum(rk.StageDeltaV(i), 0)),
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
		msg = "Тяговооружённость ниже 1 — ракета не оторвётся от стола."
	case len(rk.Stages) == 0:
		msg = "Нет ни одной ступени."
	case rk.TotalDeltaV() < need:
		msg = "Суммарного Δv не хватит даже без потерь."
	}
	if msg != "" {
		drawText(dst, msg, fontUI, r.X+14, r.Y+62, colWarn, alignLeft)
	} else {
		drawText(dst, "Готов к запуску. Клавиша Enter или кнопка справа.", fontUI, r.X+14, r.Y+62, colTextFaint, alignLeft)
	}

	btn := Rect{r.Right() - 190, r.Y + 18, 176, r.H - 36}
	launch := u.Button(dst, btn, "СТАРТ", ButtonPrimary)
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
