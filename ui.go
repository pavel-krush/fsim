package main

import (
	"fmt"
	"image/color"
	"math"
	"strconv"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
)

// A small immediate-mode toolkit. Ebiten ships no widgets, and the setup
// screen needs a few dozen numeric fields, so the cheapest thing that works is
// a handful of draw-and-test helpers keyed by the address of the value they
// edit.

// UI carries one frame's worth of input state plus the editing focus, which is
// the only thing that has to survive between frames.
type UI struct {
	MX, MY float64
	Down   bool // left button held
	Click  bool // left button went down this frame
	// Wheel is this frame's scrolling, in notches: one detent of a mouse wheel is
	// 1 whatever the platform calls it. See normalizeWheel.
	Wheel float64
	DT    float64

	// wheelUnit is how much raw scrolling this device calls one notch.
	wheelUnit float64

	runes []rune
	keys  []ebiten.Key

	focus   any    // address of the value being edited
	editBuf string // what the user has typed so far
	editBad bool   // the buffer does not parse

	clip     []Rect
	consumed bool // a widget has already taken this frame's click

	// Overlays — dropdown lists, tooltips — float above the rest of the
	// interface. Their drawing is deferred to the end of the frame so it lands
	// on top, and it goes to the whole canvas rather than to whatever clipped
	// sub-image the widget itself was drawn into.
	Overlay *ebiten.Image
	Bounds  Rect
	// ForcePointer overrides the real cursor. Only the screenshot script sets
	// it: hover states have no other way of reaching a scripted run.
	ForcePointer *struct{ X, Y float64 }

	openList   any  // identity of the open dropdown, nil when none
	listRect   Rect // where that list was drawn, carried over from last frame
	insideList bool // set while the owning dropdown handles its own area
	deferred   []func()
}

// NewUI builds the toolkit state.
func NewUI() *UI { return &UI{wheelUnit: 1} }

// wheelUnit is bounded so that one freak event cannot desensitise the session, and
// decays while nothing is scrolling so that swapping a trackpad for a mouse
// recalibrates. wheelZoomMax is how much of a notch one frame may ever be worth.
const (
	wheelUnitMin   = 1.0
	wheelUnitMax   = 400.0
	wheelUnitDecay = 0.995
	wheelZoomMax   = 1.0
)

// normalizeWheel turns whatever the platform calls scrolling into notches, one
// detent of a mouse wheel being 1.
//
// It has to, because the same number means wildly different things. Ebiten's
// desktop backend hands over what GLFW gives it, which is 1 per detent. Its browser
// backend hands over the raw `deltaY` of the DOM event — a Windows mouse in Chrome
// sends 100 of them per detent, a Firefox in line mode sends 3, and a trackpad
// sends a stream of small fractions with a momentum tail. Ebiten does not
// normalise: there is a TODO in its source where deltaMode would be read. So a
// zoom of exp(wheel*0.18) — a comfortable ×1.20 per detent natively — was ×6.6e7
// per detent in a browser on Windows, and on a Mac trackpad every event in a flick
// counted as dozens of detents.
//
// The unit is estimated from the largest event seen rather than from the platform,
// which is what the web does about this (normalize-wheel and its descendants) and
// needs no knowledge of the device: whatever the biggest push is, that is one
// notch. Then no single frame is worth more than a notch, which is what makes one
// detent one step everywhere.
//
// The first gesture of a session is counted generously, because every event on its
// rising edge is a new largest and so a whole notch: a trackpad flick that should be
// worth four comes out at six or seven. From the second gesture on the device is
// calibrated. Being slightly eager once is a better failure than the alternatives —
// guessing the platform, or making the first detent do nothing while it measures.
func (u *UI) normalizeWheel(raw float64) float64 {
	if u.wheelUnit < wheelUnitMin {
		u.wheelUnit = wheelUnitMin
	}
	if raw == 0 {
		u.wheelUnit = math.Max(wheelUnitMin, u.wheelUnit*wheelUnitDecay)
		return 0
	}
	u.wheelUnit = clamp(math.Max(math.Abs(raw), u.wheelUnit), wheelUnitMin, wheelUnitMax)
	return clamp(raw/u.wheelUnit, -wheelZoomMax, wheelZoomMax)
}

// BeginFrame samples the input devices. canvas is the whole drawing area:
// overlays paint onto it directly, and its size decides whether a dropdown
// opens downwards or upwards.
func (u *UI) BeginFrame(canvas *ebiten.Image, dt float64) {
	b := canvas.Bounds()
	u.Overlay = canvas
	u.Bounds = Rect{float64(b.Min.X) / uiScale, float64(b.Min.Y) / uiScale,
		float64(b.Dx()) / uiScale, float64(b.Dy()) / uiScale}
	// The cursor arrives in the frame's own coordinates, which are device pixels now; the widgets
	// think in logical ones. See uiScale.
	mx, my := ebiten.CursorPosition()
	u.MX, u.MY = float64(mx)/uiScale, float64(my)/uiScale
	if u.ForcePointer != nil {
		u.MX, u.MY = u.ForcePointer.X, u.ForcePointer.Y
	}
	u.Down = ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft)
	u.Click = inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft)
	_, wy := ebiten.Wheel()
	u.Wheel = u.normalizeWheel(wy)
	u.DT = dt

	u.runes = ebiten.AppendInputChars(u.runes[:0])
	u.keys = inpututil.AppendJustPressedKeys(u.keys[:0])
	u.clip = u.clip[:0]
	u.consumed = false
	u.deferred = u.deferred[:0]
}

// EndFrame draws whatever was deferred, on top of everything else, and commits
// an in-progress edit if the user clicked somewhere else.
func (u *UI) EndFrame() {
	for _, fn := range u.deferred {
		fn()
	}
	u.deferred = u.deferred[:0]
	if u.Click && !u.consumed && u.focus != nil {
		u.commit()
	}
}

// fenced reports whether a point belongs to an open dropdown list rather than
// to whatever widget is asking about it.
func (u *UI) fenced() bool {
	return u.openList != nil && !u.insideList && u.listRect.Contains(u.MX, u.MY)
}

// PushClip limits hit testing to r until the matching PopClip.
func (u *UI) PushClip(r Rect) { u.clip = append(u.clip, r) }

// PopClip removes the innermost clip rectangle.
func (u *UI) PopClip() {
	if n := len(u.clip); n > 0 {
		u.clip = u.clip[:n-1]
	}
}

// hover reports whether the pointer is inside r and not clipped away.
func (u *UI) hover(r Rect) bool {
	if !r.Contains(u.MX, u.MY) || u.fenced() {
		return false
	}
	for _, c := range u.clip {
		if !c.Contains(u.MX, u.MY) {
			return false
		}
	}
	return true
}

// clicked reports a fresh click inside r and marks it as consumed.
func (u *UI) clicked(r Rect) bool {
	if u.Click && !u.consumed && u.hover(r) {
		u.consumed = true
		return true
	}
	return false
}

// keyPressed reports whether k went down this frame.
func (u *UI) keyPressed(k ebiten.Key) bool {
	for _, x := range u.keys {
		if x == k {
			return true
		}
	}
	return false
}

// Editing reports whether a text field currently has focus. Screens use this
// to keep their keyboard shortcuts from firing while the user types.
func (u *UI) Editing() bool { return u.focus != nil }

// numTarget is what an edit is being applied to when it commits.
type numTarget struct {
	val   *float64
	scale float64
	min   float64
	max   float64
	after func()
}

var activeTarget numTarget

// commit parses the edit buffer into the focused value and drops focus.
func (u *UI) commit() {
	tgt := activeTarget
	if tgt.val != nil {
		if v, err := parseNum(u.editBuf); err == nil {
			v *= tgt.scale
			if tgt.min != tgt.max {
				v = clamp(v, tgt.min, tgt.max)
			}
			if *tgt.val != v {
				*tgt.val = v
				if tgt.after != nil {
					tgt.after()
				}
			}
		}
	}
	u.focus = nil
	u.editBuf = ""
	u.editBad = false
	activeTarget = numTarget{}
}

// cancel drops the edit without applying it.
func (u *UI) cancel() {
	u.focus = nil
	u.editBuf = ""
	u.editBad = false
	activeTarget = numTarget{}
}

// parseNum accepts both decimal separators and ignores spaces used as
// thousands separators.
func parseNum(s string) (float64, error) {
	s = strings.TrimSpace(strings.ReplaceAll(s, " ", ""))
	s = strings.ReplaceAll(s, ",", ".")
	if s == "" || s == "-" {
		return 0, strconv.ErrSyntax
	}
	return strconv.ParseFloat(s, 64)
}

// labelColW caps the label column. It has to clear the widest label that goes
// through a field — "Задержка разделения", 134 px — plus the info mark that
// sits at its right edge, or the two collide.
const labelColW = 160

// unitColW is the strip kept clear to the right of every labelled field for
// its unit. It fits the widest unit in the locale files ("×10²¹ kg", 44 px)
// with room for the gap.
const unitColW = 48

// NumOpt configures a numeric field.
type NumOpt struct {
	Unit  string
	Scale float64 // stored value divided by this is what the user sees
	Min   float64 // in stored units; Min == Max disables clamping
	Max   float64
	Dec   int // digits after the point, -1 for automatic
	After func()
	// Info is a locale key for an explanation of the parameter. When set, a
	// mark appears next to the label and reveals the text on hover.
	Info string
}

// NumField draws a label, an editable box and a unit suffix inside r. It
// returns true on the frame the value changes.
func (u *UI) NumField(dst *ebiten.Image, r Rect, label string, val *float64, o NumOpt) bool {
	if o.Scale == 0 {
		o.Scale = 1
	}
	if o.Dec == 0 {
		o.Dec = -1
	}

	// A field with no label has no label column: the box is the whole row. The
	// first cut reserved the strip either way, which left an unlabelled 104 px
	// cell with 39 px of box and the leading digit of its own value cut off.
	labelW := 0.0
	if label != "" {
		labelW = math.Min(r.W*0.52, labelColW)
	}
	box := Rect{r.X + labelW, r.Y, r.W - labelW, r.H}
	switch {
	case label != "":
		// A labelled field is a form row, and form rows stack into a column:
		// reserve the same strip for the unit on every one of them, whatever
		// that unit happens to be, so all the boxes end on the same line.
		box.W -= unitColW
	case o.Unit != "":
		// No label means a cell in a compact grid — the layer and keyframe
		// editors — where a fixed reserve would eat most of the width.
		box.W -= textWidth(o.Unit, fontUISm) + 6
	}

	drawText(dst, label, fontUI, r.X, r.Y+(r.H-fontUI.Size)/2-1, colTextDim, alignLeft)
	if o.Info != "" {
		// A fixed column just before the input box, not trailing the label:
		// label widths differ per field and per language, and marks scattered
		// at ragged positions read as clutter once many rows carry one.
		u.InfoMark(dst, box.X-18, r.Y+r.H/2, T(o.Info))
	}
	if o.Unit != "" {
		drawText(dst, o.Unit, fontUISm, box.Right()+4, r.Y+(r.H-fontUISm.Size)/2, colTextFaint, alignLeft)
	}

	focused := u.focus == val
	bg, border := colPanelHi, colBorder
	switch {
	case focused && u.editBad:
		border = colBad
	case focused:
		border = colAccent
	case u.hover(box):
		border = colAccentDim
	}
	fillRect(dst, box, bg)
	strokeRect(dst, box, 1, border)

	changed := false
	if u.clicked(box) && !focused {
		if u.focus != nil {
			u.commit()
		}
		u.focus = val
		u.editBuf = formatNum(*val/o.Scale, o.Dec)
		u.editBad = false
		activeTarget = numTarget{val: val, scale: o.Scale, min: o.Min, max: o.Max, after: o.After}
		focused = true
	}

	shown := formatNum(*val/o.Scale, o.Dec)
	col := colText
	if focused {
		before := *val
		u.typeInto()
		shown = u.editBuf + "_"
		col = colAccent
		if u.keyPressed(ebiten.KeyEnter) || u.keyPressed(ebiten.KeyNumpadEnter) || u.keyPressed(ebiten.KeyTab) {
			u.commit()
			changed = *val != before
			shown = formatNum(*val/o.Scale, o.Dec)
			col = colText
		} else if u.keyPressed(ebiten.KeyEscape) {
			u.cancel()
			shown = formatNum(*val/o.Scale, o.Dec)
			col = colText
		}
	}

	clip := box.Sub(dst)
	if clip != nil {
		drawText(clip, shown, fontMono, box.Right()-6, box.Y+(box.H-fontMono.Size)/2-1, col, alignRight)
	}
	return changed
}

// typeInto folds this frame's keystrokes into the edit buffer.
func (u *UI) typeInto() {
	for _, r := range u.runes {
		if (r >= '0' && r <= '9') || r == '.' || r == ',' || r == '-' || r == 'e' || r == 'E' || r == '+' {
			u.editBuf += string(r)
		}
	}
	if u.keyPressed(ebiten.KeyBackspace) && len(u.editBuf) > 0 {
		u.editBuf = u.editBuf[:len(u.editBuf)-1]
	}
	_, err := parseNum(u.editBuf)
	u.editBad = err != nil && u.editBuf != ""
}

// ReadOnly draws a label with a computed value that cannot be edited. The
// value and its unit are passed apart so the row lands on the same two columns
// an editable row uses: the number ends where the input boxes end, and the
// unit sits in the strip beside them.
func (u *UI) ReadOnly(dst *ebiten.Image, r Rect, label, value, unit string) {
	valueRight := r.Right() - unitColW
	drawText(dst, label, fontUI, r.X, r.Y+(r.H-fontUI.Size)/2-1, colTextFaint, alignLeft)
	drawText(dst, value, fontMono, valueRight-6, r.Y+(r.H-fontMono.Size)/2-1, colTextDim, alignRight)
	if unit != "" {
		drawText(dst, unit, fontUISm, valueRight+4, r.Y+(r.H-fontUISm.Size)/2, colTextFaint, alignLeft)
	}
}

// Button draws a clickable button and reports whether it was pressed.
func (u *UI) Button(dst *ebiten.Image, r Rect, label string, style ButtonStyle) bool {
	bg, fg, border := colPanelHi, colText, colBorder
	switch style {
	case ButtonPrimary:
		bg, fg, border = colAccentDim, color.NRGBA{0xea, 0xf3, 0xff, 0xff}, colAccent
	case ButtonActive:
		bg, fg, border = colAccentDim, colText, colAccent
	case ButtonDanger:
		fg = colBad
	}
	if u.hover(r) {
		bg = lighten(bg, 0.18)
		if u.Down {
			bg = lighten(bg, -0.1)
		}
	}
	fillRect(dst, r, bg)
	strokeRect(dst, r, 1, border)
	drawText(dst, label, fontUI, r.X+r.W/2, r.Y+(r.H-fontUI.Size)/2-1, fg, alignCenter)
	return u.clicked(r)
}

// ButtonStyle selects a button's colours.
type ButtonStyle int

const (
	ButtonNormal ButtonStyle = iota
	ButtonPrimary
	ButtonActive
	ButtonDanger
)

// tipWidth is how wide an info tooltip is allowed to get before wrapping.
const tipWidth = 340

// wrapText breaks a paragraph-separated string into lines that fit within
// maxW. A blank entry in the result is a paragraph break.
func wrapText(s string, f *text.GoTextFace, maxW float64) []string {
	var out []string
	for _, para := range strings.Split(s, "\n\n") {
		words := strings.Fields(para)
		if len(words) == 0 {
			continue
		}
		line := words[0]
		for _, w := range words[1:] {
			if textWidth(line+" "+w, f) <= maxW {
				line += " " + w
				continue
			}
			out = append(out, line)
			line = w
		}
		out = append(out, line, "")
	}
	if n := len(out); n > 0 && out[n-1] == "" {
		out = out[:n-1]
	}
	return out
}

// InfoMark draws a small circled "i" whose left edge sits at x, vertically
// centred on cy, and shows body in a tooltip while the pointer is over it.
//
// The tooltip is deferred like a dropdown list so that it lands on top of
// everything, and it is drawn onto the canvas rather than the caller's clipped
// sub-image, which would otherwise cut it off at the edge of a column.
func (u *UI) InfoMark(dst *ebiten.Image, x, cy float64, body string) {
	const d = 13
	mark := Rect{x, cy - d/2, d, d}

	col := colTextFaint
	if u.hover(mark) {
		col = colAccent
	}
	ring(dst, mark.X+d/2, mark.Y+d/2, d/2-0.5, 1, col)
	drawText(dst, "i", fontUISm, mark.X+d/2, mark.Y+(d-fontUISm.Size)/2-1, col, alignCenter)

	if !u.hover(mark) || body == "" {
		return
	}

	// Body copy, not a label: the normal UI face with a touch more leading
	// than a form row, since these are whole sentences to be read.
	const pad = 10
	lines := wrapText(body, fontUI, tipWidth-2*pad)
	lh := fontUI.Size + 5
	box := Rect{mark.Right() + 8, cy - 12, tipWidth, float64(len(lines))*lh + 2*pad}

	// Keep it on screen: flip to the other side of the mark if it overflows to
	// the right, then slide it up if it overflows at the bottom.
	if box.Right() > u.Bounds.Right()-8 {
		box.X = mark.X - 8 - box.W
	}
	box.Y = clamp(box.Y, u.Bounds.Y+8, math.Max(u.Bounds.Y+8, u.Bounds.Bottom()-8-box.H))

	over := u.Overlay
	u.deferred = append(u.deferred, func() {
		fillRect(over, box, colPanel)
		strokeRect(over, box, 1, colAccentDim)
		y := box.Y + pad
		for _, ln := range lines {
			drawText(over, ln, fontUI, box.X+pad, y, colText, alignLeft)
			y += lh
		}
	})
}

// Dropdown draws a selector showing the current choice and, while open, a list
// of the alternatives. It returns the selected index, which is the one passed
// in unless the user picked something else this frame.
//
// id identifies the dropdown across frames; anything comparable will do.
func (u *UI) Dropdown(dst *ebiten.Image, r Rect, id any, items []string, sel int) int {
	open := u.openList == id

	border := colBorder
	if open {
		border = colAccent
	} else if u.hover(r) {
		border = colAccentDim
	}
	fillRect(dst, r, colPanelHi)
	strokeRect(dst, r, 1, border)

	label := ""
	if sel >= 0 && sel < len(items) {
		label = items[sel]
	}
	drawText(dst, label, fontUI, r.X+8, r.Y+(r.H-fontUI.Size)/2-1, colText, alignLeft)
	drawCaret(dst, r.Right()-13, r.Y+r.H/2, open)

	if u.clicked(r) {
		if open {
			u.openList = nil
		} else {
			u.openList = id
		}
		return sel
	}
	if !open {
		return sel
	}

	// Open downwards, or upwards when the list would fall off the bottom —
	// which is what happens for the copies living in the bottom bars.
	itemH := r.H
	listH := itemH * float64(len(items))
	list := Rect{r.X, r.Bottom() + 2, r.W, listH}
	if list.Bottom() > u.Bounds.Bottom() {
		list = Rect{r.X, r.Y - 2 - listH, r.W, listH}
	}
	u.listRect = list

	// Clicks that land on the list belong to the list, whatever was drawn
	// underneath it earlier in the frame.
	u.insideList = true
	chosen := sel
	for i := range items {
		if u.clicked(Rect{list.X, list.Y + float64(i)*itemH, list.W, itemH}) {
			chosen = i
			u.openList = nil
		}
	}
	hovered := -1
	for i := range items {
		if u.hover(Rect{list.X, list.Y + float64(i)*itemH, list.W, itemH}) {
			hovered = i
		}
	}
	u.insideList = false

	// Anything else closes it. The click still reaches whatever it landed on.
	if u.Click && !u.consumed {
		u.openList = nil
	}

	over := u.Overlay
	u.deferred = append(u.deferred, func() {
		fillRect(over, list, colPanel)
		for i, it := range items {
			ir := Rect{list.X, list.Y + float64(i)*itemH, list.W, itemH}
			// The current choice is marked by the colour of its text. A rule
			// down the edge of the row would sit on the list's own border and
			// merely make it look thicker for one row.
			fg := colTextDim
			if i == hovered {
				fillRect(over, ir, colPanelHi)
				fg = colText
			}
			if i == sel {
				fg = colAccent
			}
			drawText(over, it, fontUI, ir.X+8, ir.Y+(ir.H-fontUI.Size)/2-1, fg, alignLeft)
		}
		strokeRect(over, list, 1, colAccent)
	})
	return chosen
}

// drawCaret draws the little triangle on a dropdown, pointing down when the
// list is closed and up when it is open.
func drawCaret(dst *ebiten.Image, x, y float64, up bool) {
	const w, h = 4.0, 2.5
	dy := h
	if up {
		dy = -h
	}
	line(dst, x-w, y-dy, x, y+dy, 1.5, colTextDim)
	line(dst, x, y+dy, x+w, y-dy, 1.5, colTextDim)
}

// Checkbox draws a labelled tick box and reports whether it was toggled.
func (u *UI) Checkbox(dst *ebiten.Image, r Rect, label string, val *bool) bool {
	boxSize := math.Min(r.H-4, 14)
	box := Rect{r.X, r.Y + (r.H-boxSize)/2, boxSize, boxSize}

	border := colBorder
	if u.hover(r) {
		border = colAccentDim
	}
	fillRect(dst, box, colPanelHi)
	strokeRect(dst, box, 1, border)
	if *val {
		fillRect(dst, box.Inset(3), colAccent)
	}
	if label != "" {
		drawText(dst, label, fontUISm, box.Right()+6, r.Y+(r.H-fontUISm.Size)/2, colTextDim, alignLeft)
	}
	if u.clicked(r) {
		*val = !*val
		return true
	}
	return false
}

// Radio draws one option of a mutually exclusive group and reports selection.
func (u *UI) Radio(dst *ebiten.Image, r Rect, label string, sel *int, value int) bool {
	cx := r.X + 7
	cy := r.Y + r.H/2
	col := colBorder
	if u.hover(r) {
		col = colAccentDim
	}
	ring(dst, cx, cy, 6, 1, col)
	if *sel == value {
		circle(dst, cx, cy, 3.5, colAccent)
	}
	drawText(dst, label, fontUISm, cx+12, r.Y+(r.H-fontUISm.Size)/2, colTextDim, alignLeft)
	if u.clicked(r) {
		*sel = value
		return true
	}
	return false
}

// SectionHeader draws a titled divider.
func (u *UI) SectionHeader(dst *ebiten.Image, r Rect, title string) {
	drawText(dst, title, fontHead, r.X, r.Y+(r.H-fontHead.Size)/2-1, colAccent, alignLeft)
	w := textWidth(title, fontHead) + 8
	y := r.Y + r.H/2
	if r.W > w {
		line(dst, r.X+w, y, r.Right(), y, 1, colBorder)
	}
}

// Scroll is the persistent state of a scrollable column.
type Scroll struct {
	Offset  float64
	content float64
}

// Begin clips drawing to r, applies the wheel and returns the y coordinate the
// first row should be drawn at.
func (s *Scroll) Begin(u *UI, r Rect) float64 {
	if u.hover(r) && u.Wheel != 0 {
		s.Offset -= u.Wheel * 40
	}
	maxOff := math.Max(0, s.content-r.H)
	s.Offset = clamp(s.Offset, 0, maxOff)
	u.PushClip(r)
	return r.Y - s.Offset
}

// End records how tall the content turned out and draws a scrollbar.
func (s *Scroll) End(u *UI, dst *ebiten.Image, r Rect, endY float64) {
	s.content = endY + s.Offset - r.Y
	u.PopClip()

	if s.content <= r.H {
		return
	}
	frac := r.H / s.content
	barH := math.Max(24, r.H*frac)
	maxOff := s.content - r.H
	t := 0.0
	if maxOff > 0 {
		t = s.Offset / maxOff
	}
	bar := Rect{r.Right() - 5, r.Y + t*(r.H-barH), 3, barH}
	fillRect(dst, bar, colBorder)
}

// lighten shifts a colour towards white for positive f, towards black for
// negative f.
func lighten(c color.NRGBA, f float64) color.NRGBA {
	adj := func(v uint8) uint8 {
		x := float64(v)
		if f >= 0 {
			x += (255 - x) * f
		} else {
			x += x * f
		}
		return uint8(clamp(x, 0, 255))
	}
	return color.NRGBA{adj(c.R), adj(c.G), adj(c.B), c.A}
}

// formatNum renders a value for display. With dec < 0 the number of decimals
// follows the magnitude, so both 6371 and 0.0065 stay readable.
func formatNum(v float64, dec int) string {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return "—"
	}
	if dec >= 0 {
		return strconv.FormatFloat(v, 'f', dec, 64)
	}
	a := math.Abs(v)
	switch {
	case a == 0:
		return "0"
	case a >= 1e7 || a < 1e-4:
		return strconv.FormatFloat(v, 'g', 4, 64)
	case a >= 1000:
		return strconv.FormatFloat(v, 'f', 0, 64)
	case a >= 100:
		return trimZeros(strconv.FormatFloat(v, 'f', 1, 64))
	case a >= 1:
		return trimZeros(strconv.FormatFloat(v, 'f', 3, 64))
	default:
		return trimZeros(strconv.FormatFloat(v, 'f', 6, 64))
	}
}

func trimZeros(s string) string {
	if !strings.Contains(s, ".") {
		return s
	}
	s = strings.TrimRight(s, "0")
	return strings.TrimSuffix(s, ".")
}

// fmtEng formats a physical quantity with a sensible unit prefix, for the
// read-only telemetry rows.
func fmtEng(v float64, unit string) string {
	a := math.Abs(v)
	switch {
	case math.IsInf(v, 0):
		return "∞"
	case a >= 1e9:
		return fmt.Sprintf("%.2f %s%s", v/1e9, T("unit.prefixGiga"), unit)
	case a >= 1e6:
		return fmt.Sprintf("%.2f %s%s", v/1e6, T("unit.prefixMega"), unit)
	case a >= 1e3:
		return fmt.Sprintf("%.2f %s%s", v/1e3, T("unit.prefixKilo"), unit)
	default:
		return fmt.Sprintf("%.2f %s", v, unit)
	}
}

// fmtClock renders the mission clock. Under an hour it keeps tenths of a second,
// which is the resolution an ascent is read at; past that it switches to whole
// seconds, then to days, because a flight to the Moon on a T+MM:SS clock reads
// "T+4752:00.0" and means nothing to anybody.
//
// Rounding happens before the split, not after: 11999.98 s split first gives 199
// minutes and 59.98 seconds, which prints as "T+199:60.0", and 86399.6 s split
// first gives day zero, hour 24.
func fmtClock(t float64) string {
	if t < 0 {
		t = 0
	}
	// Rounded first, then the format is chosen from the rounded value — the same
	// trap one level up: 3599.97 s rounds to a flat hour, and picking the format
	// from the raw seconds would print that hour as "T+60:00.0".
	tenths := int64(math.Round(t * 10))
	if tenths < 36000 {
		return fmt.Sprintf("T+%02d:%04.1f", tenths/600, float64(tenths%600)/10)
	}
	secs := (tenths + 5) / 10
	days, rem := secs/86400, secs%86400
	h, m, sec := rem/3600, rem%3600/60, rem%60
	if days == 0 {
		return fmt.Sprintf("T+%d:%02d:%02d", h, m, sec)
	}
	return fmt.Sprintf("T+%dd %02d:%02d:%02d", days, h, m, sec)
}

// measureRows is the standard height of one form row.
const rowH = 22

// textFits reports whether s renders within w pixels in face f.
func textFits(s string, f *text.GoTextFace, w float64) bool { return text.Advance(s, f) <= w }
