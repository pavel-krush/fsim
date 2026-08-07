package main

import (
	"math"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/pavel-krush/fsim/sim"
)

// The first screen: pick a mission and nothing else. What used to be first — four
// columns of every number the model has — is a great deal to be handed before you
// have said what you are trying to fly, so it comes second now.
//
// A preset given on the command line skips this screen, which is what -preset was
// always for: it names the thing you already know you want.

// PresetScreen is the mission list.
type PresetScreen struct {
	// sel is the row the keyboard is on. The mouse does not move it: a pointer
	// hovering one row while the keyboard sits on another is two selections, and
	// only one of them can be right.
	sel int

	// hasSaved is whether the user's own stored setup gets a row. It is decided
	// once, here, rather than per frame: the answer costs a read of the disk.
	//
	// The row is *last*, which is not a matter of taste. Every index in this screen
	// and in -preset counts into sim.Presets(), and a row at the front would shift
	// all of them — the kind of quiet renumbering this project has been bitten by
	// more than once. Appended, it changes nothing above it.
	hasSaved bool
}

func NewPresetScreen(sel int) *PresetScreen {
	s := &PresetScreen{sel: sel}
	// Offered only if there is something to offer, and only if it still reads: a
	// row that reports a broken file every time it is clicked is not a mission.
	if _, ok, err := loadConfig(); ok && err == nil {
		s.hasSaved = true
	}
	return s
}

// rows is how many the list has: the presets, and the saved setup if there is one.
func (s *PresetScreen) rows() int {
	n := len(sim.Presets())
	if s.hasSaved {
		n++
	}
	return n
}

// presetRowH is how tall a row would like to be, presetRowMin how short it will go
// to fit, and presetListW how wide the list is: enough for the longest name in
// either language with the identifier beside it.
const (
	presetRowH   = 42.0
	presetRowMin = 22.0
	presetListW  = 640.0
	presetRowGap = 6.0
)

// presetLayout fits n rows into the area it is given. In a window the program owns
// they are 42 px tall and the block is centred; in a browser the window is whatever
// the browser is, and a 760 px one had thirteen rows overlapping the header at the
// top and running off the bottom. So the rows shrink to fit, down to a floor, and
// the width comes in with the area.
func presetLayout(body Rect, n int) (rowH float64, area Rect) {
	// The hint below the list counts towards the centring, or the block sits low.
	const hint = 30.0
	rowH = presetRowH
	if n > 0 {
		if fit := (body.H - hint - float64(n-1)*presetRowGap) / float64(n); fit < rowH {
			rowH = math.Max(presetRowMin, fit)
		}
	}
	listH := float64(n)*(rowH+presetRowGap) - presetRowGap
	w := math.Min(presetListW, body.W-40)
	return rowH, Rect{
		X: body.X + (body.W-w)/2,
		Y: body.Y + math.Max(0, (body.H-listH-hint)/2),
		W: w,
		H: listH,
	}
}

// move walks the keyboard selection, stopping at the ends rather than wrapping:
// a list this short is easier to aim at when it has ends you can feel.
func (s *PresetScreen) move(delta, n int) {
	s.sel += delta
	if s.sel < 0 {
		s.sel = 0
	}
	if s.sel >= n {
		s.sel = n - 1
	}
}

// pick loads a preset and moves on to the setup screen. Everything the editor
// points into belongs to the configuration, so the screen is built fresh rather
// than told to change its mind.
func (s *PresetScreen) pick(a *App, i int) {
	presets := sim.Presets()

	// The saved setup is read afresh rather than kept from construction, so that the
	// editor gets its own slices to mutate: the vehicle, the layers and the
	// keyframes are all edited in place, and handing over a shared copy would mean
	// editing the stored one too.
	if s.hasSaved && i == len(presets) {
		cfg, ok, err := loadConfig()
		if !ok || err != nil {
			return
		}
		a.ui.cancel()
		s.sel = i
		a.cfg = cfg
		// Which preset it came from is not a question a saved setup answers, and
		// the dropdown has no way to say "none", so it says the first one. The same
		// small lie the LOAD button in that screen already tells.
		a.setup = NewSetupScreen(0)
		a.screen = ScreenSetup
		return
	}

	if i < 0 || i >= len(presets) {
		return
	}
	a.ui.cancel()
	s.sel = i
	a.cfg = presets[i].Cfg
	a.setup = NewSetupScreen(i)
	a.screen = ScreenSetup
}

// presetRowRect is where row i is drawn.
func presetRowRect(area Rect, rowH float64, i int) Rect {
	return Rect{area.X, area.Y + float64(i)*(rowH+presetRowGap), area.W, rowH}
}

func (s *PresetScreen) Update(a *App, dst *ebiten.Image) {
	u := a.ui
	b := a.Bounds()
	presets := sim.Presets()
	n := s.rows()

	if s.sel < 0 || s.sel >= n {
		s.sel = 0
	}

	const pad = 12
	headH := 44.0
	panel(dst, Rect{pad, pad, b.W - 2*pad, headH}, colPanel)
	drawText(dst, "FSIM", fontBig, pad+14, pad+(headH-fontBig.Size)/2-2, colAccent, alignLeft)
	drawText(dst, T("setup.tagline"), fontUISm, pad+14+textWidth("FSIM", fontBig)+10,
		pad+(headH-fontUISm.Size)/2, colTextFaint, alignLeft)
	u.LangPicker(dst, Rect{b.W - pad - 10 - langPickerW, pad + 8, langPickerW, headH - 16})

	// The list, centred in what is left, so that adding a preset moves the block
	// rather than pushing the last one off the bottom.
	body := Rect{pad, pad + headH + 8, b.W - 2*pad, b.H - headH - 3*pad - 8}
	rowH, area := presetLayout(body, n)

	if u.keyPressed(ebiten.KeyArrowDown) {
		s.move(1, n)
	}
	if u.keyPressed(ebiten.KeyArrowUp) {
		s.move(-1, n)
	}
	if u.keyPressed(ebiten.KeyEnter) || u.keyPressed(ebiten.KeyNumpadEnter) {
		s.pick(a, s.sel)
		return
	}

	// One label pair per row, so that the saved setup is drawn, hovered, selected
	// and clicked by exactly the same code as a preset rather than by a copy of it.
	type row struct{ name, slug string }
	list := make([]row, 0, n)
	for _, p := range presets {
		list = append(list, row{presetName(p.Name), p.Name})
	}
	if s.hasSaved {
		// No identifier: there is no -preset for it. What it is instead is the one
		// row here that is yours.
		list = append(list, row{T("presets.saved"), ""})
	}

	for i, p := range list {
		r := presetRowRect(area, rowH, i)
		hover := u.hover(r)

		// The identifier is the dimmest thing on the row, except on the selected one,
		// where dim against the accent colour is invisible rather than quiet.
		bg, fg, slug := colPanel, colText, colTextFaint
		switch {
		case i == s.sel:
			bg, slug = colAccentDim, colTextDim
		case hover:
			bg = lighten(colPanel, 0.18)
		}
		fillRect(dst, r, bg)
		if i == s.sel {
			strokeRect(dst, r, 1, colAccent)
		}

		drawText(dst, p.name, fontUI, r.X+16, r.Y+(r.H-fontUI.Size)/2-1, fg, alignLeft)
		// The identifier as well as the name: it is what -preset and ?preset= take,
		// and there is nowhere else to find it out.
		drawText(dst, p.slug, fontMono, r.Right()-16, r.Y+(r.H-fontMono.Size)/2-1,
			slug, alignRight)

		if u.clicked(r) {
			s.pick(a, i)
			return
		}
	}

	drawText(dst, T("presets.hint"), fontUISm, b.W/2, area.Bottom()+22, colTextDim, alignCenter)
}
