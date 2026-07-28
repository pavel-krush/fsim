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
	Wheel  float64
	DT     float64

	runes []rune
	keys  []ebiten.Key

	focus   any    // address of the value being edited
	editBuf string // what the user has typed so far
	editBad bool   // the buffer does not parse

	clip     []Rect
	consumed bool // a widget has already taken this frame's click
}

// NewUI builds the toolkit state.
func NewUI() *UI { return &UI{} }

// BeginFrame samples the input devices. dt is the frame time in seconds.
func (u *UI) BeginFrame(dt float64) {
	mx, my := ebiten.CursorPosition()
	u.MX, u.MY = float64(mx), float64(my)
	u.Down = ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft)
	u.Click = inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft)
	_, wy := ebiten.Wheel()
	u.Wheel = wy
	u.DT = dt

	u.runes = ebiten.AppendInputChars(u.runes[:0])
	u.keys = inpututil.AppendJustPressedKeys(u.keys[:0])
	u.clip = u.clip[:0]
	u.consumed = false
}

// EndFrame commits an in-progress edit if the user clicked somewhere else.
func (u *UI) EndFrame() {
	if u.Click && !u.consumed && u.focus != nil {
		u.commit()
	}
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
	if !r.Contains(u.MX, u.MY) {
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
	t := activeTarget
	if t.val != nil {
		if v, err := parseNum(u.editBuf); err == nil {
			v *= t.scale
			if t.min != t.max {
				v = clamp(v, t.min, t.max)
			}
			if *t.val != v {
				*t.val = v
				if t.after != nil {
					t.after()
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

// NumOpt configures a numeric field.
type NumOpt struct {
	Unit  string
	Scale float64 // stored value divided by this is what the user sees
	Min   float64 // in stored units; Min == Max disables clamping
	Max   float64
	Dec   int // digits after the point, -1 for automatic
	After func()
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

	labelW := math.Min(r.W*0.52, 150)
	box := Rect{r.X + labelW, r.Y, r.W - labelW, r.H}
	unitW := 0.0
	if o.Unit != "" {
		unitW = textWidth(o.Unit, fontUISm) + 6
		box.W -= unitW
	}

	drawText(dst, label, fontUI, r.X, r.Y+(r.H-fontUI.Size)/2-1, colTextDim, alignLeft)
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

// ReadOnly draws a label with a computed value that cannot be edited.
func (u *UI) ReadOnly(dst *ebiten.Image, r Rect, label, value string) {
	labelW := math.Min(r.W*0.52, 150)
	drawText(dst, label, fontUI, r.X, r.Y+(r.H-fontUI.Size)/2-1, colTextFaint, alignLeft)
	drawText(dst, value, fontMono, r.Right()-4, r.Y+(r.H-fontMono.Size)/2-1, colTextDim, alignRight)
	_ = labelW
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
		return fmt.Sprintf("%.2f Г%s", v/1e9, unit)
	case a >= 1e6:
		return fmt.Sprintf("%.2f М%s", v/1e6, unit)
	case a >= 1e3:
		return fmt.Sprintf("%.2f к%s", v/1e3, unit)
	default:
		return fmt.Sprintf("%.2f %s", v, unit)
	}
}

// fmtClock renders seconds as T+MM:SS.
func fmtClock(t float64) string {
	if t < 0 {
		t = 0
	}
	m := int(t) / 60
	s := t - float64(m*60)
	return fmt.Sprintf("T+%02d:%04.1f", m, s)
}

// measureRows is the standard height of one form row.
const rowH = 22

// textFits reports whether s renders within w pixels in face f.
func textFits(s string, f *text.GoTextFace, w float64) bool { return text.Advance(s, f) <= w }
