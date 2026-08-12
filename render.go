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

// drawCalls counts anti-aliased vector draws per frame, for measuring. Nothing reads it in a
// release; it is here because "the interface costs twenty milliseconds" is a claim that needs a
// number under it.
var drawCalls int

// line draws a straight segment.
func line(dst *ebiten.Image, x0, y0, x1, y1, w float64, c color.Color) {
	drawCalls++
	vector.StrokeLine(dst, float32(x0), float32(y0), float32(x1), float32(y1), float32(w), c, true)
}

// Polyline is a curve drawn as *one* stroke rather than as a segment per pair of points, and it is
// the difference between a trajectory view that runs and one that does not.
//
// An anti-aliased line in Ebiten is not a cheap call. `vector.StrokeLine` with antialias on builds a
// path, strokes it and renders it through the stencil-buffer atlas — per call — where the same
// function with antialias off is a single batched DrawImage of a white quad. The trajectory view
// draws thousands of anti-aliased segments when the camera is pulled back: eight planet rails at a
// hundred and sixty segments each, a trail across the screen, a predicted path of four hundred
// points. In a browser, where every one of those crosses into WebGL, that was twenty milliseconds a
// frame of interface time, reported as the whole thing lagging once the vehicle left the planet.
//
// One path per curve is one stencil render per curve, with the joins computed once instead of a
// round cap at both ends of every segment. The points are kept in a reusable buffer because this
// runs every frame for every rail.
type Polyline struct {
	pts  []float32
	path vector.Path
}

// Reset empties it for the next curve.
func (l *Polyline) Reset() { l.pts = l.pts[:0] }

// Add appends a point. Points closer than a pixel to the previous one are dropped: the curves here
// are sampled in world units, so at any distance most of them land on the same pixel, and a sub-pixel
// segment costs the same as a long one.
func (l *Polyline) Add(x, y float64) {
	if n := len(l.pts); n >= 2 {
		if math.Abs(x-float64(l.pts[n-2]))+math.Abs(y-float64(l.pts[n-1])) < 1 {
			return
		}
	}
	l.pts = append(l.pts, float32(x), float32(y))
}

// Break lifts the pen: what follows starts a new run rather than continuing this one. It is what
// keeps a line from being drawn across a hole the vehicle did not fly through.
func (l *Polyline) Break() {
	if n := len(l.pts); n > 0 && !(math.IsNaN(float64(l.pts[n-2])) && math.IsNaN(float64(l.pts[n-1]))) {
		nan := float32(math.NaN())
		l.pts = append(l.pts, nan, nan)
	}
}

// Stroke draws whatever has been added, as one path, and empties the buffer.
func (l *Polyline) Stroke(dst *ebiten.Image, w float64, c color.Color) {
	defer l.Reset()
	l.path.Reset()
	pen := false
	runs := 0
	for i := 0; i+1 < len(l.pts); i += 2 {
		x, y := l.pts[i], l.pts[i+1]
		if math.IsNaN(float64(x)) {
			pen = false
			continue
		}
		if !pen {
			l.path.MoveTo(x, y)
			pen = true
			runs++
			continue
		}
		l.path.LineTo(x, y)
	}
	if runs == 0 {
		return
	}
	drawCalls++
	so := &vector.StrokeOptions{Width: float32(w), LineJoin: vector.LineJoinRound}
	do := &vector.DrawPathOptions{AntiAlias: true}
	do.ColorScale.ScaleWithColor(c)
	vector.StrokePath(dst, &l.path, so, do)
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
	// And never a dash shorter than a couple of pixels: Polyline drops points that land on the
	// same pixel, so a sub-pixel dash would come out as nothing at all.
	if r*seg < 2 {
		seg = 2 / r
	}
	// One path with a run per dash rather than a draw per dash: see Polyline. A ring at orbital
	// scale is two hundred dashes, and each was its own anti-aliased render.
	var l Polyline
	for a := 0.0; a < 2*math.Pi; a += seg * 2 {
		l.Add(cx+r*math.Cos(a), cy+r*math.Sin(a))
		l.Add(cx+r*math.Cos(a+seg), cy+r*math.Sin(a+seg))
		l.Break()
	}
	l.Stroke(dst, 1, c)
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
