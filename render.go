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

// uiScale is how many device pixels one interface pixel is worth, and it exists because the text
// was being drawn at half the resolution of the screen it was shown on.
//
// Ebiten renders into whatever `Layout` asks for and then stretches that framebuffer to the window.
// Asking for the window's *logical* size — which is what this did — means a display with a device
// scale factor of 2 gets a picture rendered at half density and blown up, and a blown-up glyph does
// not look like a big glyph: it looks like a small one that has been through a photocopier. That is
// the whole of the "it looks like it was made for early-2000s monitors" complaint.
//
// So the frame is now rendered at the *device* resolution and every coordinate that reaches a
// drawing primitive is multiplied by this on the way. Everything above these primitives — every
// layout constant, every widget rectangle, the camera's projection — keeps working in logical
// pixels and does not know this exists.
var uiScale = 1.0

// sc scales one logical coordinate to device pixels.
func sc(v float64) float32 { return float32(v * uiScale) }

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
	ir := image.Rect(int(r.X*uiScale), int(r.Y*uiScale), int(r.Right()*uiScale), int(r.Bottom()*uiScale))
	ir = ir.Intersect(dst.Bounds())
	if ir.Empty() {
		return nil
	}
	return dst.SubImage(ir).(*ebiten.Image)
}

// fillRect paints a solid rectangle.
func fillRect(dst *ebiten.Image, r Rect, c color.Color) {
	vector.FillRect(dst, sc(r.X), sc(r.Y), sc(r.W), sc(r.H), c, false)
}

// strokeRect outlines a rectangle.
func strokeRect(dst *ebiten.Image, r Rect, w float64, c color.Color) {
	vector.StrokeRect(dst, sc(r.X), sc(r.Y), sc(r.W), sc(r.H), sc(w), c, false)
}

// drawCalls counts anti-aliased vector draws per frame, for measuring. Nothing reads it in a
// release; it is here because "the interface costs twenty milliseconds" is a claim that needs a
// number under it.
var drawCalls int

// line draws a straight segment.
func line(dst *ebiten.Image, x0, y0, x1, y1, w float64, c color.Color) {
	drawCalls++
	vector.StrokeLine(dst, sc(x0), sc(y0), sc(x1), sc(y1), sc(w), c, true)
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
	// Logical coordinates in, device pixels out at Stroke: the sub-pixel test below is therefore
	// about *logical* pixels, which is what the picture is composed in.
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
			l.path.MoveTo(sc(float64(x)), sc(float64(y)))
			pen = true
			runs++
			continue
		}
		l.path.LineTo(sc(float64(x)), sc(float64(y)))
	}
	if runs == 0 {
		return
	}
	drawCalls++
	so := &vector.StrokeOptions{Width: sc(w), LineJoin: vector.LineJoinRound}
	do := &vector.DrawPathOptions{AntiAlias: true}
	do.ColorScale.ScaleWithColor(c)
	vector.StrokePath(dst, &l.path, so, do)
}

// circle draws a filled disc.
func circle(dst *ebiten.Image, x, y, r float64, c color.Color) {
	drawCalls++
	vector.FillCircle(dst, sc(x), sc(y), sc(r), c, true)
}

// ring draws a circle outline.
func ring(dst *ebiten.Image, x, y, r, w float64, c color.Color) {
	drawCalls++
	vector.StrokeCircle(dst, sc(x), sc(y), sc(r), sc(w), c, true)
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
	// Drawn from a face built at the *device* size rather than by scaling a small one up. A glyph
	// rasterised at 26 px is a glyph; a 13 px glyph stretched to 26 is a smear, and telling those
	// two apart is the entire point of this.
	df := deviceFace(f)
	switch a {
	case alignRight:
		x -= textWidth(s, f)
	case alignCenter:
		x -= textWidth(s, f) / 2
	}
	op := &text.DrawOptions{}
	op.GeoM.Translate(x*uiScale, y*uiScale)
	op.ColorScale.ScaleWithColor(c)
	text.Draw(dst, s, df, op)
}

// textWidth measures a string in the given face, in logical pixels. Measured on the device face and
// divided back down, so that a layout is laid out against the glyphs that will actually be drawn
// rather than against a nominal size they are only near.
func textWidth(s string, f *text.GoTextFace) float64 {
	return text.Advance(s, deviceFace(f)) / uiScale
}

// deviceFace is f at the device size, cached: building a face allocates, and this runs for every
// string on the screen every frame.
var deviceFaces = map[faceKey]*text.GoTextFace{}

type faceKey struct {
	src  *text.GoTextFaceSource
	size float64
}

func deviceFace(f *text.GoTextFace) *text.GoTextFace {
	if uiScale == 1 {
		return f
	}
	k := faceKey{f.Source, f.Size * uiScale}
	if df, ok := deviceFaces[k]; ok {
		return df
	}
	df := &text.GoTextFace{Source: f.Source, Size: k.size}
	deviceFaces[k] = df
	return df
}

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
