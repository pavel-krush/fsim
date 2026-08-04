package main

import (
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/pavel-krush/fsim/sim"
)

// Rect is an axis-aligned rectangle in screen pixels.
type Rect struct{ X, Y, W, H float64 }

func (r Rect) Contains(x, y float64) bool {
	return x >= r.X && x < r.X+r.W && y >= r.Y && y < r.Y+r.H
}
func (r Rect) Right() float64  { return r.X + r.W }
func (r Rect) Bottom() float64 { return r.Y + r.H }

// Inset shrinks the rectangle by d on every side.
func (r Rect) Inset(d float64) Rect {
	return Rect{r.X + d, r.Y + d, r.W - 2*d, r.H - 2*d}
}

// Sub returns the sub-image for clipping drawing to this rectangle.
func (r Rect) Sub(dst *ebiten.Image) *ebiten.Image {
	ir := image.Rect(int(r.X), int(r.Y), int(r.Right()), int(r.Bottom()))
	ir = ir.Intersect(dst.Bounds())
	if ir.Empty() {
		return nil
	}
	return dst.SubImage(ir).(*ebiten.Image)
}

// fillRect paints a solid rectangle.
func fillRect(dst *ebiten.Image, r Rect, c color.Color) {
	vector.FillRect(dst, float32(r.X), float32(r.Y), float32(r.W), float32(r.H), c, false)
}

// strokeRect outlines a rectangle.
func strokeRect(dst *ebiten.Image, r Rect, w float64, c color.Color) {
	vector.StrokeRect(dst, float32(r.X), float32(r.Y), float32(r.W), float32(r.H), float32(w), c, false)
}

// line draws a straight segment.
func line(dst *ebiten.Image, x0, y0, x1, y1, w float64, c color.Color) {
	vector.StrokeLine(dst, float32(x0), float32(y0), float32(x1), float32(y1), float32(w), c, true)
}

// circle draws a filled disc.
func circle(dst *ebiten.Image, x, y, r float64, c color.Color) {
	vector.FillCircle(dst, float32(x), float32(y), float32(r), c, true)
}

// ring draws a circle outline.
func ring(dst *ebiten.Image, x, y, r, w float64, c color.Color) {
	vector.StrokeCircle(dst, float32(x), float32(y), float32(r), float32(w), c, true)
}

// panel draws the standard bordered background used by every UI block.
func panel(dst *ebiten.Image, r Rect, bg color.Color) {
	fillRect(dst, r, bg)
	strokeRect(dst, r, 1, colBorder)
}

// Text alignment for label drawing.
type align int

const (
	alignLeft align = iota
	alignRight
	alignCenter
)

// drawText renders a single line with the baseline box's top-left at (x, y).
func drawText(dst *ebiten.Image, s string, f *text.GoTextFace, x, y float64, c color.Color, a align) {
	if s == "" {
		return
	}
	switch a {
	case alignRight:
		x -= text.Advance(s, f)
	case alignCenter:
		x -= text.Advance(s, f) / 2
	}
	op := &text.DrawOptions{}
	op.GeoM.Translate(x, y)
	op.ColorScale.ScaleWithColor(c)
	text.Draw(dst, s, f, op)
}

// textWidth measures a string in the given face.
func textWidth(s string, f *text.GoTextFace) float64 { return text.Advance(s, f) }

// dashedRing draws a circle outline as dashes, for reference orbits that
// should not compete with the live trajectory.
func dashedRing(dst *ebiten.Image, cx, cy, r float64, c color.Color) {
	if r < 2 {
		return
	}
	// Keep the dash length roughly constant in pixels regardless of radius.
	seg := 10.0 / r
	if seg > 0.15 {
		seg = 0.15
	}
	for a := 0.0; a < 2*math.Pi; a += seg * 2 {
		x0 := cx + r*math.Cos(a)
		y0 := cy + r*math.Sin(a)
		x1 := cx + r*math.Cos(a+seg)
		y1 := cy + r*math.Sin(a+seg)
		line(dst, x0, y0, x1, y1, 1, c)
	}
}

// Camera maps the planet-centred world frame, in metres, onto screen pixels.
// Y is flipped so that up in the rotated frame is up on screen.
//
// Rot is the world angle that gets pointed at the top of the screen. The
// flight view keeps it aligned with the vehicle's local vertical, so "up" on
// screen is always away from the planet. Without that the launch looks like
// it is flying sideways, because the launch site starts on the +X axis.
type Camera struct {
	Center sim.Vec2 // world point shown at the centre of the viewport
	Scale  float64  // pixels per metre
	Rot    float64  // radians
	View   Rect
}

// Project converts a world point into screen coordinates.
func (c *Camera) Project(p sim.Vec2) (float64, float64) {
	d := p.Sub(c.Center).Rotate(math.Pi/2 - c.Rot)
	cx := c.View.X + c.View.W/2
	cy := c.View.Y + c.View.H/2
	return cx + d.X*c.Scale, cy - d.Y*c.Scale
}

// Unproject is Project run backwards: the world point under a screen position.
// This is what makes dragging and zoom-to-cursor possible — both are statements
// about a world point that has to stay under the pointer.
func (c *Camera) Unproject(x, y float64) sim.Vec2 {
	cx := c.View.X + c.View.W/2
	cy := c.View.Y + c.View.H/2
	if c.Scale == 0 {
		return c.Center
	}
	d := sim.Vec2{X: (x - cx) / c.Scale, Y: -(y - cy) / c.Scale}
	return d.Rotate(-(math.Pi/2 - c.Rot)).Add(c.Center)
}

// Dir maps a world direction onto screen axes, without translation or scale.
func (c *Camera) Dir(v sim.Vec2) (float64, float64) {
	d := v.Rotate(math.Pi/2 - c.Rot)
	return d.X, -d.Y
}

// Len converts a world distance into pixels.
func (c *Camera) Len(m float64) float64 { return m * c.Scale }

// expLerp moves cur towards want with a time constant that is independent of
// the frame rate. rate is the fraction of the gap closed per second.
func expLerp(cur, want, rate, dt float64) float64 {
	return want + (cur-want)*math.Exp(-rate*dt)
}

// clamp constrains v to [lo, hi].
func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
