package main

import (
	"fmt"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"

	"fsim/sim"
)

// warpSteps are the time-warp settings offered on the flight screen. The ladder
// runs to a million because that is what an interplanetary coast costs: three
// days to the Moon is a quarter of a million seconds, and nobody is watching it
// at ×500. It is geometric rather than fine-grained because eight buttons is
// what the bar has room for.
var warpSteps = []float64{1, 5, 20, 100, 1000, 10000, 100000, 1000000}

// FlightScreen flies the simulation and draws the trajectory, the telemetry
// panel and the time controls.
type FlightScreen struct {
	s      *sim.Sim
	paused bool
	warp   int // index into warpSteps

	cam       Camera
	manualCam bool
	zoomBias  float64 // user zoom multiplier on top of the automatic scale

	// focus is the body the picture is built around: -1 follows the vehicle and
	// whatever body currently holds it, which is the default and what the
	// simulator has always done. Anything else pins a body from the system and
	// draws the world from there, which is the only way to watch a moon at all —
	// in the launch body's frame it crosses the screen at a kilometre a second.
	focus int
}

// frameBody is the body at the origin of the drawn world.
func (f *FlightScreen) frameBody() int {
	if f.focus < 0 || f.focus >= len(f.s.Cfg.System.Bodies) {
		return f.s.St.Center
	}
	return f.focus
}

// framePoint maps a position measured from body `from` at time t into the frame
// the picture is drawn in. The shift is taken at the sample's own time, because
// that is where the bodies were then: a track relative to a moving body is what
// "in the Moon's frame" means, and it is a different shape from the same track
// drawn around the Earth.
func (f *FlightScreen) framePoint(p sim.Vec2, from int, t float64) sim.Vec2 {
	center := f.frameBody()
	if from == center {
		return p
	}
	d, _ := f.s.Cfg.System.RelState(from, center, t)
	return p.Add(d)
}

// trackPoint maps a recorded sample into the drawn frame and turns it forward
// with the frame body's rotation, so that a launch reads as a climb off the pad
// instead of the 6 km sideways drift the inertial frame shows: the vehicle
// carries the launch site's eastward velocity, which is real but unhelpful to
// look at. In orbit the track lags the ellipse by omega*T — that is the ground
// track, and it should.
func (f *FlightScreen) trackPoint(sm sim.Sample) sim.Vec2 {
	p := f.framePoint(sm.Pos, sm.Center, sm.T)
	w := f.s.Cfg.System.Bodies[f.frameBody()].AngularVelocity()
	if w == 0 {
		return p
	}
	return p.Rotate(w * (f.s.St.T - sm.T))
}

// vehiclePos is where the vehicle is in the drawn frame, now.
func (f *FlightScreen) vehiclePos() sim.Vec2 {
	return f.framePoint(f.s.St.Pos, f.s.St.Center, f.s.St.T)
}

func NewFlightScreen(s *sim.Sim) *FlightScreen {
	f := &FlightScreen{s: s, zoomBias: 1, focus: -1}
	f.cam.Scale = 0
	return f
}

// Update advances the flight and redraws the screen.
func (f *FlightScreen) Update(a *App, dst *ebiten.Image) {
	u := a.ui
	b := a.Bounds()

	f.handleKeys(u)

	if !f.paused && !f.s.St.Done {
		// The rate goes in as well as the amount: it caps how far one coast step
		// may reach, which is what keeps the picture moving at ×1 instead of
		// jumping ten minutes at a time.
		f.s.WarpRate = warpSteps[f.warp]
		f.s.Advance(u.DT * warpSteps[f.warp])
	}

	const pad = 12
	sideW := 320.0
	footH := 46.0

	view := Rect{pad, pad, b.W - sideW - 3*pad, b.H - footH - 3*pad}
	side := Rect{view.Right() + pad, pad, sideW, b.H - footH - 3*pad}
	foot := Rect{pad, b.H - footH - pad, b.W - 2*pad, footH}

	f.drawTrajectory(a, dst, view)
	f.drawTelemetry(a, dst, side)
	f.drawControls(a, dst, foot)
}

func (f *FlightScreen) handleKeys(u *UI) {
	if u.keyPressed(ebiten.KeySpace) {
		f.paused = !f.paused
	}
	if u.keyPressed(ebiten.KeyPeriod) && f.warp < len(warpSteps)-1 {
		f.warp++
	}
	if u.keyPressed(ebiten.KeyComma) && f.warp > 0 {
		f.warp--
	}
	if u.keyPressed(ebiten.KeyC) {
		f.manualCam = false
		f.zoomBias = 1
		f.focus = -1
	}
	if u.keyPressed(ebiten.KeyTab) {
		// Round the bodies and back to the vehicle. With one body in the system
		// there is nothing to cycle to and the key does nothing.
		f.focus++
		if f.focus >= len(f.s.Cfg.System.Bodies) || len(f.s.Cfg.System.Bodies) < 2 {
			f.focus = -1
		}
	}
}

// updateCamera picks a scale that keeps both the vehicle and the ground below
// it in frame, then eases towards it so the zoom never jumps.
func (f *FlightScreen) updateCamera(a *App, view Rect) {
	f.cam.View = view

	if f.focus >= 0 {
		f.focusCamera(a, view)
		return
	}

	b := f.s.Center()
	pos := f.s.St.Pos
	r := pos.Len()
	alt := f.s.Altitude()

	// Show a window that always contains the launch altitude band plus the
	// vehicle. The floor is tight enough that the pad is a structure rather
	// than a speck for the first few seconds.
	span := math.Max(alt*2.6, 1500)
	o := sim.ComputeOrbit(pos, f.s.St.Vel, b.Mu)
	switch {
	case o.Bound() && o.PeriapsisAlt(b.Radius) > 0:
		// A closed orbit that clears the ground: pull back far enough to see
		// the whole ellipse, which is the interesting picture at that point.
		span = math.Max(span, o.Apoapsis*2.3)
	case o.Bound() && o.Apoapsis > r:
		span = math.Max(span, (o.Apoapsis-b.Radius)*2.4)
	}
	span = math.Min(span, b.Radius*24)

	wantScale := math.Min(view.W, view.H) / span * f.zoomBias

	// Only the zoom is eased. The centre is derived from the vehicle's current
	// position every frame instead of being smoothed towards it, because the
	// smoothing has to happen in some frame of reference and the inertial one
	// is the wrong choice: the vehicle sweeps around the planet at 465 m/s, so
	// an eased camera sits permanently ~100 m behind it and every hitch in the
	// fixed-step integrator turns that lag into a few pixels of shake.
	if f.cam.Scale == 0 {
		f.cam.Scale = wantScale
	} else if !f.manualCam {
		f.cam.Scale = math.Exp(expLerp(math.Log(f.cam.Scale), math.Log(wantScale), 2.5, a.ui.DT))
	}

	// Frame everything off the eased zoom rather than the raw span, so that a
	// step in the span — the orbit closing, say — does not jolt the framing
	// while the zoom is still gliding towards it.
	effSpan := math.Min(view.W, view.H) / f.cam.Scale

	// The focus slides from the vehicle towards the planet's centre as the view
	// widens. The blend has to stay pinned at zero while zoomed in: the target
	// is the planet's centre thousands of kilometres away, so even a one part
	// in ten thousand blend would shove the pad off a 1.5 km wide view.
	// Nothing moves until the span is worth half a planet radius.
	u := clamp((effSpan/b.Radius-0.5)/2.1, 0, 1)
	u = u * u * (3 - 2*u)
	// Close in, sit the vehicle a little below centre so it has sky to climb
	// into rather than staring at the ground.
	drawn := f.vehiclePos()
	f.cam.Center = drawn.Unit().Scale((drawn.Len() + 0.16*effSpan) * (1 - u))

	// And the same ramp lets go of the local vertical. Held all the way out, the
	// picture would spin with the orbit — a full turn every five seconds at
	// ×1000 — and standing on a planet turns into looking at one somewhere
	// around here anyway. At u = 0 this is exactly "point the vertical at the
	// top of the screen", the way it has always been; the shortest-path delta is
	// what keeps the wrap through pi from flipping the world over.
	f.cam.Rot += (1 - u) * angleDelta(f.cam.Rot, drawn.Angle())
}

// focusCamera looks at a body instead of at the vehicle: the body sits at the
// centre, the picture holds still, and only the manual zoom decides how much of
// it is in frame.
func (f *FlightScreen) focusCamera(a *App, view Rect) {
	b := &f.s.Cfg.System.Bodies[f.focus]

	span := b.Radius * 6
	if b.SOI > 0 && !math.IsInf(b.SOI, 1) {
		// A moon is worth seeing with its sphere of influence around it, which
		// is the thing an approach is actually aiming at.
		span = math.Min(b.SOI*2.6, b.Radius*400)
	}
	wantScale := math.Min(view.W, view.H) / span * f.zoomBias

	if f.cam.Scale == 0 {
		f.cam.Scale = wantScale
	} else if !f.manualCam {
		f.cam.Scale = math.Exp(expLerp(math.Log(f.cam.Scale), math.Log(wantScale), 2.5, a.ui.DT))
	}
	f.cam.Center = sim.Vec2{}
	f.cam.Rot += angleDelta(f.cam.Rot, math.Pi/2)
}

// angleDelta is the shortest way round from a to b, in radians.
func angleDelta(a, b float64) float64 {
	d := math.Mod(b-a, 2*math.Pi)
	if d > math.Pi {
		d -= 2 * math.Pi
	} else if d < -math.Pi {
		d += 2 * math.Pi
	}
	return d
}

func (f *FlightScreen) drawTrajectory(a *App, dst *ebiten.Image, view Rect) {
	u := a.ui
	panel(dst, view, colBG)

	if u.hover(view) && u.Wheel != 0 {
		f.zoomBias = clamp(f.zoomBias*math.Exp(u.Wheel*0.18), 1e-7, 200)
	}
	f.updateCamera(a, view)

	clip := view.Sub(dst)
	if clip == nil {
		return
	}
	cam := &f.cam

	f.drawBodies(clip, view, cam)

	// The target orbit, drawn around the body it refers to — the one launched
	// from, which is not necessarily the one currently in the middle.
	tm := f.s.Telemetry()
	if a.cfg.TargetOrbit > 0 {
		lb := &f.s.Cfg.System.Bodies[f.s.Cfg.LaunchBody]
		if rr := cam.Len(lb.Radius + a.cfg.TargetOrbit); rr < maxRingPx && rr > 4 {
			cx, cy := cam.Project(f.framePoint(sim.Vec2{}, f.s.Cfg.LaunchBody, f.s.St.T))
			dashedRing(clip, cx, cy, rr, colTarget)
		}
	}
	f.drawOsculating(clip, cam, tm.Orbit)

	padX, padY, padLabelled := f.drawPad(clip, cam)
	f.drawTrail(clip, cam)
	f.drawEventMarkers(clip, cam, padX, padY, padLabelled)
	f.drawVehicle(clip, cam, tm)
	f.drawScaleBar(clip, view, cam)
	f.drawViewHUD(clip, view, tm)
}

// trailWindow is how much of the flight, in seconds, stays drawn behind the
// vehicle. Longer than any ascent in the presets, so nothing is trimmed on the
// way up; in orbit it becomes an arc that follows the craft round.
const trailWindow = 900

// maxStackedLabels is how many event labels may pile up on one spot before the
// rest go unlabelled.
const maxStackedLabels = 3

// maxRingPx is the largest circle worth tessellating. Beyond this the arc is
// indistinguishable from a straight line anyway, and the vector rasteriser
// would be asked to emit an absurd number of segments.
const maxRingPx = 20000

// drawBodies paints every body in the system, each at whatever detail its size
// on screen deserves. The ladder runs from a labelled dot through a disc to
// concentric rings, and finally — once the curvature is sub-pixel — to
// horizontal bands under the vehicle, which is both faster and what the view
// actually looks like from there.
func (f *FlightScreen) drawBodies(dst *ebiten.Image, view Rect, cam *Camera) {
	sys := &f.s.Cfg.System

	// Rails first, so that a body always sits on top of its own orbit.
	for i := range sys.Bodies {
		f.drawRail(dst, cam, i)
	}
	// Then the bodies, with the one in the middle last: it is the one that can
	// fill the screen, and it should cover everything drawn behind it.
	center := f.frameBody()
	for i := range sys.Bodies {
		if i != center {
			f.drawBody(dst, view, cam, i)
		}
	}
	f.drawBody(dst, view, cam, center)
}

// drawRail traces the orbit a body runs on, around wherever its parent is drawn.
func (f *FlightScreen) drawRail(dst *ebiten.Image, cam *Camera, i int) {
	b := &f.s.Cfg.System.Bodies[i]
	if b.Parent < 0 || b.SemiMajor <= 0 {
		return
	}
	// Too small to read, or too big to tessellate.
	if rr := cam.Len(b.SemiMajor); rr < 24 || rr > maxRingPx {
		return
	}

	parent := f.framePoint(sim.Vec2{}, b.Parent, f.s.St.T)
	a, e := b.SemiMajor, b.Ecc
	minor := a * math.Sqrt(1-e*e)

	const steps = 160
	px, py := 0.0, 0.0
	for k := 0; k <= steps; k++ {
		th := 2 * math.Pi * float64(k) / steps
		p := sim.Vec2{X: a * (math.Cos(th) - e), Y: minor * math.Sin(th)}.Rotate(b.ArgPeri)
		x, y := cam.Project(parent.Add(p))
		if k > 0 {
			line(dst, px, py, x, y, 1, colRail)
		}
		px, py = x, y
	}
}

// drawBody paints one body at the detail its pixel radius allows.
func (f *FlightScreen) drawBody(dst *ebiten.Image, view Rect, cam *Camera, i int) {
	b := &f.s.Cfg.System.Bodies[i]
	pos := f.framePoint(sim.Vec2{}, i, f.s.St.T)
	x, y := cam.Project(pos)
	rpx := cam.Len(b.Radius)

	surface, highlight := colBody, colBodyHi
	if i == f.s.Cfg.LaunchBody {
		surface, highlight = colPlanet, colPlanetHi
	}

	switch {
	case rpx > maxRingPx:
		// Beyond this the rasteriser would be asked for an absurd number of
		// segments for an arc indistinguishable from a straight line. Only the
		// body the vehicle is at can get this big, and only from close up.
		f.drawFlatWorld(dst, view, cam, i)
		return

	case rpx >= 1.5:
		f.drawAir(dst, cam, i, x, y)
		circle(dst, x, y, rpx, surface)
		ring(dst, x, y, rpx, 1.5, highlight)

	default:
		// A dot. Anything smaller than a couple of pixels would otherwise
		// vanish, and a moon you cannot see is a moon you cannot aim at.
		if !view.Inset(-8).Contains(x, y) {
			return
		}
		circle(dst, x, y, 2, highlight)
	}

	// Label anything that is not filling the screen. On the body underfoot the
	// name would just sit in the middle of the ground.
	if rpx >= math.Min(view.W, view.H)/3 || !view.Inset(-8).Contains(x, y) {
		return
	}
	if rpx < 4 {
		// A dot shares its patch of screen with the launch pad's own label,
		// which is drawn above it. Go underneath.
		drawText(dst, bodyName(b.Name), fontUISm, x, y+7, colBodyText, alignCenter)
		return
	}
	drawText(dst, bodyName(b.Name), fontUISm, x+rpx+6, y-7, colBodyText, alignLeft)
}

// drawAir paints the atmosphere as concentric rings above a body's surface.
// Only the launch body has air to draw: describing it for every body needs a
// setup screen that can, which is not this one yet.
func (f *FlightScreen) drawAir(dst *ebiten.Image, cam *Camera, i int, x, y float64) {
	if i != f.s.Cfg.LaunchBody {
		return
	}
	at := &f.s.Cfg.Atmo
	if at.IsVacuum() {
		return
	}
	b := &f.s.Cfg.System.Bodies[i]
	rho0 := at.State(0).Density
	if rho0 <= 0 {
		return
	}

	// Use as many bands as the atmosphere is thick on screen. Zoomed out to the
	// whole planet the air is only a few pixels deep, and sixteen sub-pixel
	// rings would just vanish.
	bands := int(clamp(cam.Len(at.Top)/4, 1, 16))
	for k := bands - 1; k >= 0; k-- {
		lo := at.Top * float64(k) / float64(bands)
		hi := at.Top * float64(k+1) / float64(bands)
		mid := cam.Len(b.Radius + (lo+hi)/2)
		w := cam.Len(hi - lo)
		if mid < 1 || w < 0.4 {
			continue
		}
		ring(dst, x, y, mid, w, color.NRGBA{0x4d, 0x9a, 0xff, airAlpha(at, lo, rho0)})
	}
}

// airAlpha is how solid the air band starting at altitude h should look.
func airAlpha(at *sim.Atmosphere, h, rho0 float64) uint8 {
	if rho0 <= 0 {
		return 0
	}
	return uint8(clamp(math.Pow(at.State(h).Density/rho0, 0.4)*90, 0, 90))
}

// drawFlatWorld is the close-up mode: the camera keeps the local vertical
// pointing at the top of the screen, so the ground and every air layer are
// horizontal lines rather than arcs of a circle a million pixels across.
func (f *FlightScreen) drawFlatWorld(dst *ebiten.Image, view Rect, cam *Camera, i int) {
	b := &f.s.Cfg.System.Bodies[i]
	at := &f.s.Cfg.Atmo
	hasAir := i == f.s.Cfg.LaunchBody && !at.IsVacuum()

	surface, highlight := colBody, colBodyHi
	if i == f.s.Cfg.LaunchBody {
		surface, highlight = colPlanet, colPlanetHi
	}

	// Up is the vertical under the vehicle: this mode is only ever reached from
	// close up, where the vehicle and the body in the middle are the same one.
	up := f.s.St.Pos.Unit()
	tx, ty := cam.Dir(up.Perp())
	long := (view.W + view.H) * 3

	band := func(h, thickness float64, c color.NRGBA) {
		if thickness < 0.4 {
			return
		}
		px, py := cam.Project(up.Scale(b.Radius + h))
		line(dst, px-tx*long, py-ty*long, px+tx*long, py+ty*long, thickness, c)
	}

	if hasAir {
		rho0 := at.State(0).Density
		bands := int(clamp(cam.Len(at.Top)/4, 1, 16))
		for k := bands - 1; k >= 0; k-- {
			lo := at.Top * float64(k) / float64(bands)
			hi := at.Top * float64(k+1) / float64(bands)
			band((lo+hi)/2, cam.Len(hi-lo), color.NRGBA{0x4d, 0x9a, 0xff, airAlpha(at, lo, rho0)})
		}
	}
	// The ground is one very deep stripe hanging below the surface.
	band(-long/cam.Scale/2, long, surface)
	band(0, 1.5, highlight)
}

// drawPad marks the launch site. Close in it is drawn as an actual structure
// scaled in metres, so it shrinks away naturally as the vehicle climbs; once
// it is too small to make out, it collapses into a labelled tick that still
// says where the flight started from.
// It returns the anchor of its own label, if it drew one, so the staging
// markers can space themselves away from it.
func (f *FlightScreen) drawPad(dst *ebiten.Image, cam *Camera) (float64, float64, bool) {
	local := f.s.PadPos()
	pad := f.framePoint(local, f.s.St.Center, f.s.St.T)
	// The pad is a structure on its own planet, so its vertical and its eastward
	// direction are measured there, not in whatever frame it is being drawn in.
	up := local.Unit()
	east := up.Perp()

	x0, y0 := cam.Project(pad)
	// Nothing to draw once it is off the edge, and from another body's frame it
	// is a third of a million kilometres off the edge.
	if !cam.View.Inset(-40).Contains(x0, y0) {
		return 0, 0, false
	}
	ux, uy := cam.Dir(up)

	// A pad a couple of dozen rocket diameters across, with a tower twice as tall.
	width := math.Max(60, f.s.Cfg.Rocket.Diameter*20)
	height := width * 1.8

	if cam.Len(width) < 7 {
		mx, my := x0+ux*11, y0+uy*11
		line(dst, x0, y0, x0+ux*9, y0+uy*9, 1.5, colPad)
		circle(dst, mx, my, 2.5, colPad)
		drawText(dst, T("flight.pad"), fontUISm, mx+6, my-6, colPad, alignLeft)
		return mx, my, true
	}

	// at returns the screen point offset from the pad by a metres sideways and
	// b metres up.
	at := func(a, b float64) (float64, float64) {
		return cam.Project(pad.Add(east.Scale(a)).Add(up.Scale(b)))
	}
	beam := math.Max(1.5, cam.Len(width*0.055))

	// Concrete plinth, sunk into the ground so only its top shows.
	plinth := math.Max(3, cam.Len(width*0.16))
	x1, y1 := at(-width/2, 0)
	x2, y2 := at(width/2, 0)
	line(dst, x1-ux*plinth/2, y1-uy*plinth/2, x2-ux*plinth/2, y2-uy*plinth/2, plinth, colPadDeck)
	line(dst, x1, y1, x2, y2, math.Max(1.5, plinth*0.35), colPad)

	// Service tower off to one side, with a gantry arm reaching over the pad.
	tx, ty := -width/2, height
	bx, by := at(tx, 0)
	ttx, tty := at(tx, ty)
	line(dst, bx, by, ttx, tty, beam, colPad)
	for _, level := range []float64{0.3, 0.55, 0.8} {
		ax, ay := at(tx, height*level)
		cx2, cy2 := at(tx+width*0.34, height*level)
		line(dst, ax, ay, cx2, cy2, beam*0.7, colPadDeck)
	}
	// A short strongback on the far side to frame the vehicle.
	sx, sy := at(width/2, 0)
	stx, sty := at(width/2, height*0.42)
	line(dst, sx, sy, stx, sty, beam*0.8, colPad)
	return 0, 0, false
}

// drawOsculating draws the ellipse the vehicle would coast along right now.
func (f *FlightScreen) drawOsculating(dst *ebiten.Image, cam *Camera, o sim.Orbit) {
	if !o.Bound() || o.SemiMajor <= 0 || cam.Len(o.Apoapsis) > maxRingPx {
		return
	}
	mu := f.s.Center().Mu
	pos, vel := f.s.St.Pos, f.s.St.Vel
	off := f.framePoint(sim.Vec2{}, f.s.St.Center, f.s.St.T)

	// The apsis line direction is the eccentricity vector.
	h := pos.Cross(vel)
	ev := sim.Vec2{
		X: vel.Y*h/mu - pos.X/pos.Len(),
		Y: -vel.X*h/mu - pos.Y/pos.Len(),
	}
	rot := 0.0
	if o.Eccentricity > 1e-9 {
		rot = ev.Angle()
	}
	// Direction of travel decides which way the ellipse is traced, but for a
	// static outline only the shape matters.
	aa, bb := o.SemiMajor, o.SemiMajor*math.Sqrt(1-o.Eccentricity*o.Eccentricity)
	focus := -o.SemiMajor * o.Eccentricity

	const steps = 180
	px, py := 0.0, 0.0
	for i := 0; i <= steps; i++ {
		th := 2 * math.Pi * float64(i) / steps
		p := sim.Vec2{X: focus + aa*math.Cos(th), Y: bb * math.Sin(th)}.Rotate(rot)
		x, y := cam.Project(off.Add(p))
		if i > 0 {
			line(dst, px, py, x, y, 1, colOrbit)
		}
		px, py = x, y
	}
}

func (f *FlightScreen) drawTrail(dst *ebiten.Image, cam *Camera) {
	h := f.s.Hist
	if len(h) < 2 {
		return
	}
	// Only emit a segment once the path has moved a visible distance. The
	// history is sampled in simulated time, so at orbital zoom thousands of
	// points collapse into the same few pixels — and every one of them would
	// still be a separate antialiased draw.
	//
	// That mattered more than it looks. Ebiten queues these cheaply and only
	// resolves the batch when something else draws to the same target, so the
	// bill landed on the next text draw: by orbit it was nineteen milliseconds
	// a frame, and it grew for as long as the flight lasted.
	const minSeg = 1.5

	// Once in orbit the flight has no end, so neither would the trail: it
	// would wrap the planet again and again until the whole picture is one
	// smear. Keep a window that comfortably covers a full ascent, and let
	// anything older drop off the back.
	first := 0
	if cutoff := f.s.St.T - trailWindow; cutoff > 0 {
		first = sampleAt(h, cutoff)
	}
	n := len(h)
	if n-first < 2 {
		return
	}

	px, py := cam.Project(f.trackPoint(h[first]))
	for i := first + 1; i < n; i++ {
		x, y := cam.Project(f.trackPoint(h[i]))
		if i < n-1 && math.Abs(x-px)+math.Abs(y-py) < minSeg {
			continue
		}
		// Older samples fade out, so the recent path stays legible even after
		// the trajectory has wrapped a long way around the planet.
		t := float64(i-first) / float64(n-first)
		c := color.NRGBA{
			uint8(float64(colTrailOld.R) + (float64(colTrail.R)-float64(colTrailOld.R))*t),
			uint8(float64(colTrailOld.G) + (float64(colTrail.G)-float64(colTrailOld.G))*t),
			uint8(float64(colTrailOld.B) + (float64(colTrail.B)-float64(colTrailOld.B))*t),
			0xff,
		}
		line(dst, px, py, x, y, 1.6, c)
		px, py = x, y
	}
}

// drawEventMarkers pins staging events onto the flown path.
func (f *FlightScreen) drawEventMarkers(dst *ebiten.Image, cam *Camera, seedX, seedY float64, seeded bool) {
	hist := f.s.Hist
	if len(hist) == 0 {
		return
	}
	// Staging events land within seconds of each other, which is only a few
	// pixels apart on the trajectory. Ring every one of them, but step the
	// labels down a line at a time while they are still crowding.
	prevX, prevY, step := -1e9, -1e9, 0
	if seeded {
		// The launch pad already claimed a label here; start stepping below it.
		prevX, prevY = seedX, seedY
	}
	for _, e := range f.s.Events {
		if e.Kind == sim.EvLiftoff {
			continue
		}
		i := sampleAt(hist, e.T)
		if i < 0 {
			continue
		}
		x, y := cam.Project(f.trackPoint(hist[i]))
		if !cam.View.Contains(x, y) {
			continue
		}
		c := colWarn
		switch e.Kind {
		case sim.EvEnd, sim.EvOrbit:
			c = colGood
		case sim.EvMaxQ:
			c = colMaxQ
		}
		ring(dst, x, y, 4, 1.5, c)

		label := eventLabel(e.Kind)
		if math.Hypot(x-prevX, y-prevY) < textWidth(label, fontUISm) {
			step++
		} else {
			step = 0
		}
		// Zoomed out far enough, every event of the flight lands on the same
		// pixel and the stepping turns into a wall of text down the screen. Past
		// a few, the ring is the whole of what a marker can usefully say.
		if step <= maxStackedLabels {
			drawText(dst, label, fontUISm, x+7, y-6+float64(step)*(fontUISm.Size+3), c, alignLeft)
		}
		prevX, prevY = x, y
	}
}

// drawVehicle marks the current position with its thrust direction.
func (f *FlightScreen) drawVehicle(dst *ebiten.Image, cam *Camera, tm sim.Telemetry) {
	x, y := cam.Project(f.vehiclePos())

	up := f.s.St.Pos.Unit()
	east := up.Perp()
	dx, dy := cam.Dir(sim.ThrustDirection(up, east, tm.Pitch))

	if tm.Burning {
		// The flame points backwards along the thrust axis.
		line(dst, x, y, x-dx*22, y-dy*22, 3, colFlame)
		line(dst, x, y, x-dx*13, y-dy*13, 5, color.NRGBA{0xff, 0xd9, 0x8a, 0xff})
	}
	line(dst, x, y, x+dx*14, y+dy*14, 2, colText)
	circle(dst, x, y, 4, colText)
	circle(dst, x, y, 2, colBG)
}

// drawScaleBar puts a distance reference in the corner.
func (f *FlightScreen) drawScaleBar(dst *ebiten.Image, view Rect, cam *Camera) {
	if cam.Scale <= 0 {
		return
	}
	// Pick a round distance that lands near 140 px.
	target := 140 / cam.Scale
	mag := math.Pow(10, math.Floor(math.Log10(target)))
	var pick float64
	for _, m := range []float64{1, 2, 5, 10} {
		pick = m * mag
		if pick >= target {
			break
		}
	}
	w := cam.Len(pick)
	x := view.X + 14
	y := view.Bottom() - 18
	line(dst, x, y, x+w, y, 1.5, colTextDim)
	line(dst, x, y-4, x, y+4, 1.5, colTextDim)
	line(dst, x+w, y-4, x+w, y+4, 1.5, colTextDim)

	label := fmt.Sprintf("%s %s", formatNum(pick, 0), T("unit.m"))
	if pick >= 1000 {
		label = fmt.Sprintf("%s %s", formatNum(pick/1000, 0), T("unit.km"))
	}
	drawText(dst, label, fontUISm, x+w/2, y-16, colTextDim, alignCenter)
}

// drawViewHUD is the small overlay in the corner of the trajectory view.
func (f *FlightScreen) drawViewHUD(dst *ebiten.Image, view Rect, tm sim.Telemetry) {
	x, y := view.X+14, view.Y+12
	drawText(dst, fmtClock(tm.T), fontBig, x, y, colText, alignLeft)
	y += 30

	c := colTextDim
	if tm.Burning {
		c = colFlame
	}
	drawText(dst, fmt.Sprintf(T("flight.stagePhase"), tm.Stage+1, phaseText(tm.Phase)),
		fontUISm, x, y, c, alignLeft)

	if f.focus >= 0 && f.focus < len(f.s.Cfg.System.Bodies) {
		y += 20
		name := bodyName(f.s.Cfg.System.Bodies[f.focus].Name)
		drawText(dst, fmt.Sprintf(T("flight.focusOn"), name), fontUISm, x, y, colBodyText, alignLeft)
	}
	if f.s.Settled() {
		y += 22
		verdict := outcomeText(f.s.St.Outcome)
		vc := colBad
		switch f.s.St.Outcome {
		case sim.OutcomeOrbit:
			vc = colGood
		case sim.OutcomeDecaying:
			vc = colWarn
		}
		drawText(dst, verdict, fontHead, x, y, vc, alignLeft)
	}
	if f.manualCam || f.zoomBias != 1 {
		drawText(dst, T("flight.manualCamera"), fontUISm, view.Right()-14, view.Y+12, colTextFaint, alignRight)
	}

	// A warp the current regime cannot deliver is worth saying out loud, or the
	// setting looks broken: inside the atmosphere the step cannot grow, so a
	// million times real time is not on offer at any price.
	if f.s.WarpLimited && !f.paused {
		drawText(dst, T("flight.warpLimited"), fontUISm, view.Right()-14, view.Y+30, colWarn, alignRight)
	}
}

// drawTelemetry is the numeric readout column.
func (f *FlightScreen) drawTelemetry(a *App, dst *ebiten.Image, r Rect) {
	panel(dst, r, colPanel)
	tm := f.s.Telemetry()
	b := &f.s.Cfg.Body

	c := &rowCursor{x: r.X + 12, y: r.Y + 8, w: r.W - 24}
	u := a.ui

	row := func(label, value string, col color.NRGBA) {
		rr := c.next(19)
		drawText(dst, label, fontUISm, rr.X, rr.Y+2, colTextFaint, alignLeft)
		drawText(dst, value, fontMono, rr.Right(), rr.Y+1, col, alignRight)
	}

	u.SectionHeader(dst, c.next(20), T("flight.secPosition"))
	row(T("common.altitude"), fmtEng(tm.Alt, T("unit.m")), colText)
	row(T("flight.downrange"), fmtEng(tm.Downrange, T("unit.m")), colTextDim)
	row(T("common.surfaceSpeed"), speed(tm.SurfSpeed), colText)
	row(T("flight.vertical"), speed(tm.VertSpeed), colTextDim)
	row(T("flight.horizontal"), speed(tm.HorizSpeed), colTextDim)
	row(T("flight.inertial"), speed(tm.Speed), colText)
	row(T("flight.mach"), formatNum(tm.Mach, 2), machColor(tm.Mach))
	row(T("common.pitch"), fmt.Sprintf("%s°", formatNum(tm.Pitch, 1)), colText)

	c.gap(8)
	u.SectionHeader(dst, c.next(20), T("flight.secOrbit"))
	row(T("common.apoapsis"), altText(tm.ApoAlt), apsisColor(tm.ApoAlt, f.s.Cfg.Atmo.Top))
	row(T("common.periapsis"), altText(tm.PeriAlt), apsisColor(tm.PeriAlt, f.s.Cfg.Atmo.Top))
	row(T("common.eccentricity"), formatNum(tm.Ecc, 4), colTextDim)
	if tm.Orbit.Bound() {
		row(T("common.period"), fmt.Sprintf("%s %s", formatNum(tm.Orbit.Period/60, 1), T("unit.min")), colTextDim)
	} else {
		row(T("common.period"), "—", colTextDim)
	}
	row(T("flight.target"), fmt.Sprintf("%s %s", formatNum(a.cfg.TargetOrbit/1000, 0), T("unit.km")), colTextFaint)

	c.gap(8)
	u.SectionHeader(dst, c.next(20), T("common.vehicle"))
	row(T("common.mass"), fmt.Sprintf("%s %s", formatNum(tm.Mass/1000, 2), T("unit.t")), colText)
	row(T("flight.thrust"), fmtEng(tm.Thrust, T("unit.n")), colText)
	row(T("flight.twr"), formatNum(tm.TWR, 2), colTextDim)
	row(T("common.acceleration"), fmt.Sprintf("%s g", formatNum(tm.AccelG, 2)), gColor(tm.AccelG))
	for i, p := range tm.PropFrac {
		bar := c.next(19)
		drawText(dst, fmt.Sprintf(T("flight.propellantN"), i+1), fontUISm, bar.X, bar.Y+2, colTextFaint, alignLeft)
		bw := 110.0
		box := Rect{bar.Right() - bw, bar.Y + 3, bw, 11}
		fillRect(dst, box, colPanelHi)
		fillRect(dst, Rect{box.X, box.Y, box.W * clamp(p, 0, 1), box.H}, propColor(i, tm.Stage))
		strokeRect(dst, box, 1, colBorder)
		drawText(dst, fmt.Sprintf("%.0f%%", p*100), fontMonoSm, box.X-6, bar.Y+2, colTextDim, alignRight)
	}

	c.gap(8)
	u.SectionHeader(dst, c.next(20), T("flight.secEnvironment"))
	row(T("flight.density"), fmt.Sprintf("%s %s", formatNum(tm.Density, 5), T("unit.kgm3")), colTextDim)
	row(T("flight.pressure"), fmtEng(tm.Pressure, T("unit.pa")), colTextDim)
	row(T("flight.temperature"), fmt.Sprintf("%s K", formatNum(tm.Temp, 1)), colTextDim)
	row(T("common.dynamicPressure"), fmtEng(tm.Q, T("unit.pa")), qColor(tm.Q, f.s))
	row(T("flight.drag"), fmtEng(tm.Drag, T("unit.n")), colTextDim)

	c.gap(8)
	u.SectionHeader(dst, c.next(20), T("flight.secBudget"))
	row(T("flight.expended"), speed(tm.DeltaV), colAccent)
	row(T("flight.gravityLosses"), speed(tm.GravLoss), colTextDim)
	row(T("flight.dragLosses"), speed(tm.DragLoss), colTextDim)
	row(T("flight.steeringLosses"), speed(tm.SteerLoss), colTextDim)

	q, qAlt := f.s.MaxQ()
	c.gap(8)
	u.SectionHeader(dst, c.next(20), T("flight.secPeaks"))
	row("max q", fmt.Sprintf(T("flight.maxQAt"), fmtEng(q, T("unit.pa")), formatNum(qAlt/1000, 1)), colTextDim)
	row(T("common.maxAcceleration"), fmt.Sprintf("%s g", formatNum(f.s.MaxG(), 2)), colTextDim)
	row(T("flight.maxAltitude"), fmtEng(f.s.MaxAlt(), T("unit.m")), colTextDim)
	row(T("flight.circularAtTarget"), speed(b.CircularSpeed(a.cfg.TargetOrbit)), colTextFaint)
}

// drawControls is the time bar along the bottom.
func (f *FlightScreen) drawControls(a *App, dst *ebiten.Image, r Rect) {
	u := a.ui
	panel(dst, r, colPanel)

	x := r.X + 10
	bh := r.H - 16
	by := r.Y + 8

	label := T("flight.pause")
	if f.paused {
		label = T("flight.resume")
	}
	if u.Button(dst, Rect{x, by, 120, bh}, label, ButtonNormal) {
		f.paused = !f.paused
	}
	x += 128

	drawText(dst, T("flight.speedLabel"), fontUISm, x, r.Y+(r.H-fontUISm.Size)/2, colTextFaint, alignLeft)
	x += 62
	for i, w := range warpSteps {
		style := ButtonNormal
		if i == f.warp {
			style = ButtonActive
		}
		if u.Button(dst, Rect{x, by, 46, bh}, warpLabel(w), style) {
			f.warp = i
		}
		x += 50
	}

	x += 16
	if u.Button(dst, Rect{x, by, 120, bh}, T("flight.restart"), ButtonNormal) {
		f.s.Reset()
		f.cam.Scale = 0
		f.zoomBias = 1
		f.paused = false
	}
	x += 128
	if u.Button(dst, Rect{x, by, 140, bh}, T("common.setup"), ButtonNormal) {
		a.screen = ScreenSetup
	}
	x += 148

	style := ButtonNormal
	if f.s.Settled() {
		style = ButtonPrimary
	}
	if u.Button(dst, Rect{x, by, 150, bh}, T("flight.graphs"), style) {
		if !f.s.St.Done {
			f.paused = true
		}
		a.ShowGraphs(f.s)
	}

	u.LangPicker(dst, Rect{r.Right() - 10 - langPickerW, by, langPickerW, bh})

	hint := T("flight.hint")
	if len(f.s.Cfg.System.Bodies) > 1 {
		hint = T("flight.hintBodies")
	}
	drawText(dst, hint, fontUISm, r.Right()-20-langPickerW, r.Y+(r.H-fontUISm.Size)/2, colTextFaint, alignRight)
}

// warpLabel writes a warp factor the short way: ×1000 has no business taking
// five characters in a button 46 pixels wide.
func warpLabel(w float64) string {
	switch {
	case w >= 1e6:
		return fmt.Sprintf("×%.0fM", w/1e6)
	case w >= 1000:
		return fmt.Sprintf("×%.0fk", w/1000)
	default:
		return fmt.Sprintf("×%.0f", w)
	}
}

// sampleAt finds the recorded sample closest to time t.
func sampleAt(h []sim.Sample, t float64) int {
	if len(h) == 0 {
		return -1
	}
	lo, hi := 0, len(h)-1
	for lo < hi {
		mid := (lo + hi) / 2
		if h[mid].T < t {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo
}

// speed formats a velocity for the telemetry column.
func speed(v float64) string {
	return fmt.Sprintf("%s %s", formatNum(v, 0), T("unit.mps"))
}

func altText(v float64) string {
	if math.IsInf(v, 1) {
		return "∞"
	}
	return fmtEng(v, T("unit.m"))
}

func apsisColor(v, top float64) color.NRGBA {
	switch {
	case math.IsInf(v, 1):
		return colWarn
	case v >= top:
		return colGood
	case v >= 0:
		return colWarn
	default:
		return colTextDim
	}
}

func machColor(m float64) color.NRGBA {
	if m > 0.8 && m < 1.4 {
		return colWarn // through the transonic region
	}
	return colTextDim
}

func gColor(g float64) color.NRGBA {
	switch {
	case g > 6:
		return colBad
	case g > 4:
		return colWarn
	default:
		return colTextDim
	}
}

func qColor(q float64, s *sim.Sim) color.NRGBA {
	peak, _ := s.MaxQ()
	if peak > 0 && q >= peak*0.98 && q > 0 {
		return colWarn
	}
	return colTextDim
}

func propColor(stage, active int) color.NRGBA {
	if stage == active {
		return colAccent
	}
	return colTextFaint
}
