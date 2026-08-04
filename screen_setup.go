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

	// selBody is which body of the system the first column is editing. The
	// launch body is a separate choice: a system is worth looking at whether or
	// not the pad is on the body being looked at.
	selBody int
}

func NewSetupScreen() *SetupScreen { return &SetupScreen{} }

// Update draws the whole screen and handles its input.
func (s *SetupScreen) Update(a *App, dst *ebiten.Image) {
	b := a.Bounds()

	// Keep the system's derived quantities in step with whatever was typed, and
	// make sure there is a system at all: a single-planet configuration is one
	// body, and the editor works on the tree either way.
	a.cfg.EnsureSystem()
	if s.selBody < 0 || s.selBody >= len(a.cfg.System.Bodies) {
		s.selBody = a.cfg.LaunchBody
	}

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

	// Again, so that the footer's derived numbers include this frame's edits
	// rather than lagging them by one.
	a.cfg.EnsureSystem()
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
	u := a.ui
	sys := &a.cfg.System
	b := &sys.Bodies[s.selBody]

	s.bodyPicker(a, dst, c.next(rowH+2), sys)
	c.gap(4)

	// Which body the pad is on. Everything else about a body can be edited from
	// here whether or not it is the one being launched from.
	launch := s.selBody == a.cfg.LaunchBody
	if u.Checkbox(dst, c.next(20), T("setup.launchHere"), &launch) && launch {
		a.cfg.LaunchBody = s.selBody
	} else if !launch && s.selBody == a.cfg.LaunchBody {
		// Unticking the box would leave the vehicle nowhere. The way to move the
		// pad is to tick it on another body.
		launch = true
	}
	c.gap(6)

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
	u.NumField(dst, c.next(rowH), T("setup.rotationPeriod"), &b.RotationPeriod, NumOpt{Unit: T("unit.h"), Scale: 3600, Min: -1e9, Max: 1e9, Info: "setup.rotationPeriod.info"})
	u.ReadOnly(dst, c.next(rowH), T("setup.equatorialSpeed"), formatNum(b.EquatorialSpeed(), 1), T("unit.mps"))

	s.orbitRows(a, dst, c, sys)
	s.bodyButtons(a, dst, c, sys)

	c.gap(10)
	u.SectionHeader(dst, c.next(20), T("setup.secTarget"))
	lb := &sys.Bodies[a.cfg.LaunchBody]
	u.NumField(dst, c.next(rowH), T("setup.orbitAltitude"), &a.cfg.TargetOrbit, NumOpt{Unit: T("unit.km"), Scale: 1000, Min: 0, Max: 1e9, Info: "setup.orbitAltitude.info"})
	u.ReadOnly(dst, c.next(rowH), T("setup.circularSpeed"), formatNum(lb.CircularSpeed(a.cfg.TargetOrbit), 0), T("unit.mps"))
	u.ReadOnly(dst, c.next(rowH), T("setup.escapeSpeed"), formatNum(lb.EscapeSpeed(0), 0), T("unit.mps"))
	u.NumField(dst, c.next(rowH), T("setup.timeLimit"), &a.cfg.MaxTime, NumOpt{Unit: T("unit.min"), Scale: 60, Min: 60, Max: 1e6, Info: "setup.timeLimit.info"})
}

// bodyPicker chooses which body the column edits. The launch body is marked, so
// that a system of seventeen does not need counting through to find the pad.
func (s *SetupScreen) bodyPicker(a *App, dst *ebiten.Image, r Rect, sys *sim.System) {
	items := make([]string, len(sys.Bodies))
	for i := range sys.Bodies {
		items[i] = bodyName(sys.Bodies[i].Name)
		if i == a.cfg.LaunchBody {
			items[i] += " " + T("setup.padMark")
		}
	}

	labelW := textWidth(T("setup.bodyLabel"), fontUISm) + 6
	drawText(dst, T("setup.bodyLabel"), fontUISm, r.X, r.Y+(r.H-fontUISm.Size)/2, colTextFaint, alignLeft)

	box := Rect{r.X + labelW, r.Y, r.W - labelW, r.H}
	if picked := a.ui.Dropdown(dst, box, "body", items, s.selBody); picked != s.selBody {
		// Every field in this column is bound to an address inside the body being
		// left, so a pending edit has to go with it.
		a.ui.cancel()
		s.selBody = picked
	}
}

// orbitRows edits where the selected body goes. The root does not go anywhere.
func (s *SetupScreen) orbitRows(a *App, dst *ebiten.Image, c *rowCursor, sys *sim.System) {
	u := a.ui
	b := &sys.Bodies[s.selBody]
	if b.Parent < 0 {
		c.gap(10)
		drawText(dst, T("setup.rootNote"), fontUISm, c.x, c.next(16).Y+2, colTextFaint, alignLeft)
		return
	}

	c.gap(10)
	u.SectionHeader(dst, c.next(20), T("setup.secOrbit"))

	// Only bodies defined earlier are offered as a parent, which is exactly the
	// invariant the tree rests on: a child always sits at a higher index than its
	// parent, so a cycle cannot be expressed. Moving a body to a *later* parent
	// would mean reordering the slice, and nothing here needs that.
	items := make([]string, s.selBody)
	for i := range items {
		items[i] = bodyName(sys.Bodies[i].Name)
	}
	row := c.next(rowH + 2)
	labelW := textWidth(T("setup.parentLabel"), fontUISm) + 6
	drawText(dst, T("setup.parentLabel"), fontUISm, row.X, row.Y+(row.H-fontUISm.Size)/2, colTextFaint, alignLeft)
	if picked := u.Dropdown(dst, Rect{row.X + labelW, row.Y, row.W - labelW, row.H},
		"parent", items, b.Parent); picked != b.Parent {
		b.Parent = picked
	}
	c.gap(4)

	u.NumField(dst, c.next(rowH), T("setup.semiMajor"), &b.SemiMajor,
		NumOpt{Unit: T("unit.gm"), Scale: 1e9, Min: 1000, Max: 1e14, Info: "setup.semiMajor.info"})
	u.NumField(dst, c.next(rowH), T("setup.ecc"), &b.Ecc,
		NumOpt{Min: 0, Max: 0.95, Info: "setup.ecc.info"})
	u.NumField(dst, c.next(rowH), T("setup.argPeri"), &b.ArgPeri,
		NumOpt{Unit: "°", Scale: math.Pi / 180, Min: 0, Max: 2 * math.Pi, Info: "setup.argPeri.info"})
	u.NumField(dst, c.next(rowH), T("setup.meanAnom"), &b.MeanAnom0,
		NumOpt{Unit: "°", Scale: math.Pi / 180, Min: 0, Max: 2 * math.Pi, Info: "setup.meanAnom.info"})

	period := sys.Period(s.selBody)
	u.ReadOnly(dst, c.next(rowH), T("setup.period"), formatNum(period/86400, 3), T("unit.d"))
	u.ReadOnly(dst, c.next(rowH), T("setup.soi"), formatNum(b.SOI/1e6, 1), T("unit.thousandKm"))
}

// bodyButtons adds and removes bodies.
func (s *SetupScreen) bodyButtons(a *App, dst *ebiten.Image, c *rowCursor, sys *sim.System) {
	u := a.ui
	c.gap(8)
	r := c.next(rowH + 2)

	if u.Button(dst, Rect{r.X, r.Y, r.W/2 - 3, r.H}, T("setup.addMoon"), ButtonNormal) {
		// A new body always joins as a child of the one on screen, which is the
		// useful case: giving the planet you are looking at a moon. Ten radii out
		// puts it clear of the surface at any size.
		u.cancel()
		parent := &sys.Bodies[s.selBody]
		s.selBody = sys.AddChild(s.selBody, sim.Body{
			Name:       fmt.Sprintf("custom-%d", len(sys.Bodies)),
			Radius:     math.Max(parent.Radius/4, 100000),
			MassSource: sim.FromDensity,
			Density:    3000,
			SemiMajor:  parent.Radius * 10,
		})
	}
	// The root cannot go, and neither can the body the pad is on: deleting it
	// leaves the vehicle standing on whatever the renumbering happens to put
	// there, which in the solar system is the Sun.
	if len(sys.Bodies) > 1 && s.selBody > 0 && s.selBody != a.cfg.LaunchBody {
		if u.Button(dst, Rect{r.X + r.W/2 + 3, r.Y, r.W/2 - 3, r.H}, T("setup.removeBody"), ButtonDanger) {
			u.cancel()
			remap := sys.Remove(s.selBody)
			// Everything that pointed into the old numbering has to be repaired.
			// The launch body cannot have been the one removed — the button is not
			// offered for it — so the remap can only have moved it.
			if n := remap[a.cfg.LaunchBody]; n >= 0 {
				a.cfg.LaunchBody = n
			} else {
				a.cfg.LaunchBody = 0
			}
			s.selBody = a.cfg.LaunchBody
		}
	}
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
	s.mixturePicker(a, dst, c.next(rowH+2), at)
	c.gap(4)

	var sum float64
	for i := range sim.Gases {
		sum += at.Fractions[i]
	}
	for i := range sim.Gases {
		u.NumField(dst, c.next(rowH), gasLabel(sim.Gases[i].Name), &at.Fractions[i],
			NumOpt{Unit: "%", Scale: 0.01, Min: 0, Max: 1, Info: "setup.composition.info",
				After: func() { balanceGases(at, i) }})
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

// mixturePicker drops a whole named composition in, and reports which one is
// currently loaded. Anything the user has since edited reads as custom.
func (s *SetupScreen) mixturePicker(a *App, dst *ebiten.Image, r Rect, at *sim.Atmosphere) {
	comps := sim.Compositions()

	items := make([]string, 0, len(comps)+1)
	items = append(items, T("setup.mixCustom"))
	sel := 0
	for i, comp := range comps {
		items = append(items, T("setup.mix."+comp.Name))
		if sameMixture(at.Fractions, comp.Fractions) {
			sel = i + 1
		}
	}

	labelW := textWidth(T("setup.mixLabel"), fontUISm) + 6
	drawText(dst, T("setup.mixLabel"), fontUISm, r.X, r.Y+(r.H-fontUISm.Size)/2, colTextFaint, alignLeft)

	box := Rect{r.X + labelW, r.Y, r.W - labelW, r.H}
	if picked := a.ui.Dropdown(dst, box, "mixture", items, sel); picked != sel && picked > 0 {
		// Dropping in a new slice orphans any pointer a focused field is
		// holding into the old one, so the edit has to go with it.
		a.ui.cancel()
		at.Fractions = append([]float64(nil), comps[picked-1].Fractions...)
	}
}

// sameMixture compares two mixtures by proportion, so a composition still
// counts as loaded whether it was written as fractions or as percentages.
func sameMixture(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	var sa, sb float64
	for i := range a {
		sa += a[i]
		sb += b[i]
	}
	if sa <= 0 || sb <= 0 {
		return false
	}
	for i := range a {
		if math.Abs(a[i]/sa-b[i]/sb) > 1e-6 {
			return false
		}
	}
	return true
}

// balanceGases rescales every fraction except the one just edited so that the
// mixture adds up to one again, keeping the proportions among the rest.
//
// The physics normalises the mixture anyway, so the total never had to be a
// hundred — but having to make it add up by hand was still the most tedious
// thing on this screen.
func balanceGases(at *sim.Atmosphere, edited int) {
	v := clamp(at.Fractions[edited], 0, 1)
	at.Fractions[edited] = v
	rest := 1 - v

	var others float64
	for j := range at.Fractions {
		if j != edited {
			others += at.Fractions[j]
		}
	}

	if others > 0 {
		k := rest / others
		for j := range at.Fractions {
			if j != edited {
				at.Fractions[j] *= k
			}
		}
		return
	}
	// Nothing left to scale against: hand the whole remainder to the first
	// other gas, so a mixture can never get stuck as a single component with
	// no way back down.
	for j := range at.Fractions {
		if j != edited && rest > 0 {
			at.Fractions[j] = rest
			return
		}
	}
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

// minStages and maxStages bound what the vehicle editor will build. One stage
// is a sounding rocket and a perfectly good thing to fly; four is where the real
// launchers stop. Every stage after that buys less than the one before it and
// pays for it with another full set of engines, tanks and an interstage.
const (
	minStages = 1
	maxStages = 4
)

func (s *SetupScreen) rocketRows(a *App, dst *ebiten.Image, c *rowCursor) {
	rk := &a.cfg.Rocket
	u := a.ui

	u.NumField(dst, c.next(rowH), T("setup.payload"), &rk.Payload,
		NumOpt{Unit: T("unit.t"), Scale: 1000, Min: 0, Max: 1e9, Info: "setup.payload.info"})
	u.NumField(dst, c.next(rowH), T("setup.bodyDiameter"), &rk.Diameter, NumOpt{Unit: T("unit.m"), Min: 0.1, Max: 100, Info: "setup.bodyDiameter.info"})
	u.NumField(dst, c.next(rowH), T("setup.cd"), &rk.Cd, NumOpt{Min: 0, Max: 5, Info: "setup.cd.info"})
	u.ReadOnly(dst, c.next(rowH), T("setup.referenceArea"), formatNum(rk.Area(), 2), T("unit.m2"))
	u.ReadOnly(dst, c.next(rowH), T("setup.stageCount"), formatNum(float64(len(rk.Stages)), 0), "")

	remove := -1
	for i := range rk.Stages {
		st := &rk.Stages[i]
		c.gap(10)
		hdr := c.next(20)
		// The header keeps its own row so the delete button can sit on it: a
		// stage is a dozen fields tall, and a × further down would be read as
		// belonging to whichever field it happened to line up with.
		u.SectionHeader(dst, Rect{hdr.X, hdr.Y, hdr.W - 24, hdr.H}, fmt.Sprintf(T("setup.stageN"), i+1))
		if len(rk.Stages) > minStages {
			if u.Button(dst, Rect{hdr.Right() - 18, hdr.Y + 1, 18, hdr.H - 2}, "×", ButtonDanger) {
				remove = i
			}
		}

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

	// Both edits reshuffle the slice the fields above are bound to, so any
	// pending edit has to be dropped: the focused field holds the address of a
	// stage that is about to be a different stage.
	if remove >= 0 {
		u.cancel()
		removeStage(rk, remove)
	}

	c.gap(10)
	if len(rk.Stages) < maxStages {
		if u.Button(dst, c.next(rowH+2), T("setup.addStage"), ButtonNormal) {
			u.cancel()
			addStage(rk)
		}
	} else {
		drawText(dst, T("setup.stageLimit"), fontUISm, c.x, c.next(rowH).Y+5, colTextFaint, alignLeft)
	}
	c.gap(10)
}

// addStage puts another stage on top, sized off the one currently on top: a
// quarter of its mass and thrust, its vacuum Isp throughout, lighting straight
// after separation. Those are roughly a real launcher's proportions, and the
// result flies — which a row of zeroes would not, and the point of the button is
// to get something to edit rather than something to fill in.
func addStage(rk *sim.Rocket) {
	if len(rk.Stages) >= maxStages {
		return
	}

	prev := sim.Stage{DryMass: 4000, PropMass: 40000, ThrustVac: 400000, IspVac: 340, IspSL: 320}
	if n := len(rk.Stages); n > 0 {
		prev = rk.Stages[n-1]
		// The stage below now has something to separate from, and a separation
		// that takes no time at all reads as a glitch rather than as staging.
		if rk.Stages[n-1].SepDelay <= 0 {
			rk.Stages[n-1].SepDelay = 3
		}
	}

	rk.Stages = append(rk.Stages, sim.Stage{
		DryMass:   math.Max(1, prev.DryMass/4),
		PropMass:  prev.PropMass / 4,
		ThrustVac: prev.ThrustVac / 4,
		IspVac:    prev.IspVac,
		// An upper stage never sees sea level, so the two figures are the same
		// one. Inheriting the booster's sea-level penalty would be a lie.
		IspSL:    prev.IspVac,
		Throttle: 1,
		SepDelay: 3,
		Ignition: sim.IgniteImmediate,
	})
}

// removeStage drops stage i and repairs what the editor stops showing for it.
// The ignition mode only exists for a stage that has something below it to
// separate from, so a stage promoted to the bottom must forget it: the physics
// ignores the setting there, and a hidden value that comes back the moment
// another stage is added is worse than no value.
func removeStage(rk *sim.Rocket, i int) {
	if i < 0 || i >= len(rk.Stages) || len(rk.Stages) <= minStages {
		return
	}
	rk.Stages = append(rk.Stages[:i], rk.Stages[i+1:]...)
	rk.Stages[0].Ignition = sim.IgniteImmediate
	rk.Stages[0].IgnitionDelay = 0
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
			s.loadPreset(a, p.Cfg)
		}
		x += bw + 6
	}
}

// loadPreset drops a whole configuration in.
//
// Everything the screen was pointing into belonged to the old one. The focused
// field is the obvious half — the config's slices are replaced under it — and the
// selected body is the half that crashed: coming from a system of eighteen to a
// system of one leaves the editor holding index nine, and the columns are drawn
// after this runs, not before.
func (s *SetupScreen) loadPreset(a *App, cfg sim.Config) {
	a.ui.cancel()
	a.cfg = cfg
	a.cfg.EnsureSystem()
	s.selBody = a.cfg.LaunchBody
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

	// The stage columns grow with the vehicle, so the width is shared out rather
	// than fixed: nine columns of 190 px would run under the launch button.
	colW := 190.0
	if avail := r.W - 28 - 190; avail > 0 {
		colW = math.Min(colW, avail/float64(len(stats)))
	}
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
