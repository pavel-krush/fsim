package main

import (
	"fmt"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/pavel-krush/fsim/sim"
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

	cam Camera

	// The camera. frame is whose coordinates the world is drawn in — -1 follows
	// the vehicle's own sphere of influence — and follow is what sits in the
	// middle of the screen: the vehicle, a body, or camFree for wherever it was
	// last dragged to. Splitting the two is what lets the view be pushed around
	// while a moon still holds still in it.
	frame   int
	follow  int
	freePos sim.Vec2 // centre in frame coordinates, while free
	// manualScale stops the zoom easing towards the automatic framing. Any camera
	// gesture sets it; C clears it.
	manualScale bool
	pendingZoom float64 // a scripted zoom, applied once the view size is known
	// groundHold is how much of a ground track the flown path is drawn as, from 1
	// on the pad to 0 once the view has pulled back off the planet. Set by
	// updateCamera, read by trackPoint.
	groundHold float64
	// frameShown is the frame the picture is being drawn in, noticed by handOver.
	frameShown int
	// camHold is where the view was, written from the new centre, and camHoldK how
	// much of it is still being held on to.
	camHold  sim.Vec2
	camHoldK float64
	// The frame just left, and what is still being shown of it: the offset that
	// keeps it where it was drawn, frozen at the crossing, and how much is left to
	// fade. ghostFrom is -1 when there is nothing to let go of.
	ghostFrom int
	ghostOff  sim.Vec2
	ghostK    float64

	dragging   bool
	dragAnchor sim.Vec2 // the world point grabbed at the press

	// The predicted path, recomputed on a timer rather than every frame: it is a
	// few hundred integrator steps and nothing about it changes in 200 ms.
	pred    []sim.PredPoint
	predAge float64
}

// camFree is the follow target of a camera that has been dragged: it centres on a
// point rather than on a thing.
const camFree = -2

// frameBody is the body at the origin of the drawn world.
func (f *FlightScreen) frameBody() int {
	if f.frame < 0 || f.frame >= len(f.s.Cfg.System.Bodies) {
		return f.s.St.Center
	}
	return f.frame
}

// frameEase is how long the picture takes to hand over from one frame to the next,
// in seconds of real time.
const frameEase = 0.6

// frameShift is how far body `from`'s origin sits from `center`'s at time t.
func (f *FlightScreen) frameShift(from, center int, t float64) sim.Vec2 {
	if from == center {
		return sim.Vec2{}
	}
	d, _ := f.s.Cfg.System.RelState(from, center, t)
	return d
}

// framePoint maps a position measured from body `from` into the frame the picture
// is drawn in, using where the bodies are at time t.
//
// The change of frame itself is instant, as a change of coordinates has to be: what
// must not move is the *picture*, and that is the camera's business. See handOver.
func (f *FlightScreen) framePoint(p sim.Vec2, from int, t float64) sim.Vec2 {
	return p.Add(f.frameShift(from, f.frameBody(), t))
}

// handOver is the change of frame, which happens when the vehicle crosses into or
// out of a sphere of influence. Everything drawn is written from the new centre from
// this instant on — including the camera, whose own centre is a point in the frame's
// coordinates, and a dragged view, which is stored as one.
//
// So the view is carried across too: the same point in space, written from the new
// centre, and then eased back to whatever the automatic framing wants. Without that
// the whole picture slides by the distance between the two bodies — 384,000 km on
// the way to the Moon — over a bookkeeping change that nothing in the flight marks.
// Easing the *world* instead, which was the first attempt, is worse: it slides the
// picture by the same amount and takes longer over it.
func (f *FlightScreen) handOver(dt float64) {
	// Decay first, so that the frame a change is noticed on holds the whole of the
	// old view rather than the first step's worth less.
	if f.camHoldK > 0 {
		f.camHoldK = math.Max(0, f.camHoldK-dt/frameEase)
	}
	if f.ghostK > 0 {
		f.ghostK = math.Max(0, f.ghostK-dt/ghostFade)
	}
	if want := f.frameBody(); want != f.frameShown {
		d := f.frameShift(f.frameShown, want, f.s.St.T)
		f.freePos = f.freePos.Add(d)
		f.camHold, f.camHoldK = f.cam.Center.Add(d), 1
		// And what is being let go of, held exactly where it was drawn: the offset
		// is taken once, here, and never recomputed — at this instant the two frames
		// agree, so nothing moves as the picture changes hands.
		f.ghostFrom, f.ghostOff, f.ghostK = f.frameShown, d, 1
		f.frameShown = want
	}
}

// holdView keeps the picture where it was through a hand-over, easing back to the
// framing the flight would have chosen for itself.
func (f *FlightScreen) holdView(natural sim.Vec2) sim.Vec2 {
	if f.camHoldK <= 0 {
		return natural
	}
	k := f.camHoldK * f.camHoldK * (3 - 2*f.camHoldK)
	return natural.Add(f.camHold.Sub(natural).Scale(k))
}

// snapFrame lands the hand-over immediately, for a scripted capture: a screenshot
// taken mid-glide is a screenshot of neither framing.
func (f *FlightScreen) snapFrame() {
	f.frameShown, f.camHoldK = f.frameBody(), 0
	f.ghostFrom, f.ghostK = -1, 0
}

// trackPoint maps a recorded sample into the drawn frame, turning it forward with
// the launch body's rotation first, so that a launch reads as a climb off the pad
// instead of the 6 km sideways drift the inertial frame shows: the vehicle carries
// the launch site's eastward velocity, which is real but unhelpful to look at. In
// orbit the track lags the ellipse by omega*T — that is the ground track, and it
// should.
//
// Only samples of the frame being drawn, and of the one just left, are drawn at
// all; see showTrack.
//
// The rotation belongs to the pad, so it is applied about the body the pad is on
// and only to samples measured from it — before the shift into the drawn frame,
// never after. Turning the *frame* body's rotation on everything, which is what
// this used to do, put a ground track where there is no ground: an ascent marker
// drawn in the Sun's frame ninety days into a transfer came out forty-six radians
// round the Sun, so max q ended up near the orbit of Venus.
func (f *FlightScreen) trackPoint(sm sim.Sample) sim.Vec2 {
	p := sm.Pos
	if lb := f.s.Cfg.LaunchBody; sm.Center == lb && f.groundHold > 0 {
		if w := f.s.Cfg.System.Bodies[lb].AngularVelocity(); w != 0 {
			p = p.Rotate(f.groundHold * w * (f.s.St.T - sm.T))
		}
	}
	if sm.Center != f.frameBody() {
		// The frame just left, held where it was drawn. See showTrack.
		return p.Add(f.ghostOff)
	}
	return f.framePoint(p, sm.Center, sm.T)
}

// ghostFade is how long the picture keeps showing the frame it has just left, in
// seconds of real time.
const ghostFade = 1.2

// showTrack says whether a recorded sample is drawn, and how solidly.
//
// A sample flown in the frame being drawn is drawn whole, and needs no mapping at
// all: it is already in the coordinates being drawn, so it cannot be smeared,
// kinked or displaced, and it never moves again.
//
// A sample flown in the frame just left is drawn fading, held by the offset frozen
// at the crossing so that it stays exactly where it was drawn. It cannot stay. The
// shape a leg has is the shape it has in *its own* frame, so held there it wears the
// wrong one for the frame it is now in: positions match at the seam and directions
// do not, and left up it reaches the sphere of influence and turns forty-five
// degrees while the launch markers sit a hundred and fifty thousand kilometres off
// the Earth. Redrawing it in the new frame instead is worse: the true path relative
// to a moving body is a spiral, and the revolutions flown while waiting for the Moon
// spread over the 229,000 km the Moon travelled meanwhile. Neither kept nor redrawn,
// then — let go of, over a second, from where it was.
//
// Anything older than that is not drawn at all. The graph screen keeps the flight.
func (f *FlightScreen) showTrack(sm sim.Sample) (weight float64, ok bool) {
	switch {
	case sm.Center == f.frameBody():
		return 1, true
	case f.ghostK > 0 && sm.Center == f.ghostFrom:
		return f.ghostK * f.ghostK * (3 - 2*f.ghostK), true
	}
	return 0, false
}

// vehiclePos is where the vehicle is in the drawn frame, now.
func (f *FlightScreen) vehiclePos() sim.Vec2 {
	return f.framePoint(f.s.St.Pos, f.s.St.Center, f.s.St.T)
}

func NewFlightScreen(s *sim.Sim) *FlightScreen {
	f := &FlightScreen{s: s, frame: -1, follow: -1, groundHold: 1}
	f.snapFrame()
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
		f.lookAt(-1)
		f.manualScale = false
	}
	if u.keyPressed(ebiten.KeyTab) {
		// Round the bodies and back to the vehicle. From a free view this lands on
		// the vehicle first, which is the way back most of the time.
		next := f.follow + 1
		if f.follow == camFree || next >= len(f.s.Cfg.System.Bodies) {
			next = -1
		}
		f.lookAt(next)
	}
}

// lookAt points the camera at the vehicle (-1) or at a body, and draws the world
// in that body's frame so the thing being watched holds still.
func (f *FlightScreen) lookAt(target int) {
	if target >= len(f.s.Cfg.System.Bodies) {
		target = -1
	}
	f.follow, f.frame = target, target
	f.dragging = false
}

// autoScale is the framing the flight would choose for itself: a window holding
// the vehicle and the ground under it, widening to the whole orbit once there is
// one.
func (f *FlightScreen) autoScale(view Rect) float64 {
	b := f.s.Center()
	pos := f.s.St.Pos
	r := pos.Len()

	span := math.Max(f.s.Altitude()*2.6, 1500)
	o := sim.ComputeOrbit(pos, f.s.St.Vel, b.Mu)
	switch {
	case o.Bound() && o.PeriapsisAlt(b.Radius) > 0:
		// A closed orbit that clears the ground: pull back far enough to see the
		// whole ellipse, which is the interesting picture at that point.
		span = math.Max(span, o.Apoapsis*2.3)
	case o.Bound() && o.Apoapsis > r:
		span = math.Max(span, (o.Apoapsis-b.Radius)*2.4)
	}
	span = math.Min(span, b.Radius*24)

	if f.follow >= 0 {
		// Framed on a body instead: its sphere of influence is what an approach
		// is aiming at, so that is what fills the view.
		fb := &f.s.Cfg.System.Bodies[f.follow]
		span = fb.Radius * 6
		if fb.SOI > 0 && !math.IsInf(fb.SOI, 1) {
			span = math.Min(fb.SOI*2.6, fb.Radius*400)
		}
	}
	return math.Min(view.W, view.H) / span
}

// updateCamera places the camera for this frame. Scale, centre and rotation are
// three separate decisions, and each of them is the user's the moment the user
// touches it.
func (f *FlightScreen) updateCamera(a *App, view Rect) {
	f.handOver(a.ui.DT)
	f.cam.View = view
	want := f.autoScale(view)

	switch {
	case f.pendingZoom > 0:
		f.cam.Scale, f.manualScale = want*f.pendingZoom, true
		f.pendingZoom = 0
	case f.cam.Scale == 0:
		f.cam.Scale = want
	case !f.manualScale:
		// Eased in log space so that a factor of two takes the same time whatever
		// the scale, and never snapped: a step in the automatic span — the orbit
		// closing, say — should glide rather than jump.
		f.cam.Scale = math.Exp(expLerp(math.Log(f.cam.Scale), math.Log(want), 2.5, a.ui.DT))
	}

	// Frame everything off the scale actually in force rather than off the
	// automatic span, so a zoom in progress does not jolt the composition.
	effSpan := math.Min(view.W, view.H) / f.cam.Scale
	b := f.s.Center()

	// How far the view has pulled back, from "standing on a planet" to "looking
	// at one". It decides both how much of the vehicle's own position the centre
	// follows and how hard the camera holds the local vertical.
	u := clamp((effSpan/b.Radius-0.5)/2.1, 0, 1)
	u = u * u * (3 - 2*u)

	// The same ramp decides how much of a ground track the flown path is drawn as.
	// It is only a ground track while the picture is about the ground: see
	// trackPoint. Following a body rather than the vehicle means there is no
	// "standing on it" to speak of, and a track relative to a surface the picture
	// is not looking at is worth nothing.
	f.groundHold = 0
	if f.follow == -1 && f.frameBody() == f.s.Cfg.LaunchBody {
		f.groundHold = 1 - u
	}

	switch {
	case f.follow == camFree:
		f.cam.Center = f.holdView(f.freePos)
	case f.follow >= 0:
		f.cam.Center = f.holdView(f.framePoint(sim.Vec2{}, f.follow, f.s.St.T))
	default:
		// The vehicle, with the centre sliding towards the planet's middle as the
		// view widens. The blend has to stay pinned at zero while zoomed in: the
		// target is thousands of kilometres away, so even a part in ten thousand
		// would shove the pad off a 1.5 km wide view. Close in, the vehicle sits
		// a little below centre so it has sky to climb into.
		drawn := f.vehiclePos()
		f.cam.Center = f.holdView(drawn.Unit().Scale((drawn.Len() + 0.16*effSpan) * (1 - u)))
	}

	// Rotation tracks the local vertical only while following the vehicle, and
	// only while close in. Held all the way out the picture would spin with the
	// orbit — a full turn every five seconds at ×1000 — and in a body's frame or a
	// dragged view there is no "up" to speak of. At u = 0 this is exactly "point
	// the vertical at the top of the screen", the way it has always been; the
	// shortest-path delta is what stops the wrap through pi flipping the world.
	want = math.Pi / 2
	hold := 1.0
	if f.follow == -1 {
		want, hold = f.vehiclePos().Angle(), 1-u
	}
	f.cam.Rot += hold * angleDelta(f.cam.Rot, want)
}

// handleCamera reads the panning and zooming gestures. It runs at the end of the
// frame, after every widget has had its chance at the click: the flight plan panel
// sits inside the view, and a press on it must not also grab the world.
func (f *FlightScreen) handleCamera(u *UI, view Rect) {
	if u.hover(view) && u.Wheel != 0 {
		under := f.cam.Unproject(u.MX, u.MY)
		f.cam.Scale = clamp(f.cam.Scale*math.Exp(u.Wheel*0.18), 1e-12, 1e4)
		f.manualScale = true
		if f.follow == camFree {
			// Keep the point under the pointer where it was. Following something
			// means it stays in the middle instead, which is the whole of what
			// following is for.
			after := f.cam.Unproject(u.MX, u.MY)
			f.freePos = f.freePos.Add(under.Sub(after))
		}
	}

	if u.Click && !u.consumed && u.hover(view) {
		f.dragging, f.dragAnchor = true, f.cam.Unproject(u.MX, u.MY)
	}
	if !u.Down {
		f.dragging = false
	}
	if !f.dragging {
		return
	}

	shift := f.dragAnchor.Sub(f.cam.Unproject(u.MX, u.MY))
	if shift.Len() == 0 {
		return
	}
	if f.follow != camFree {
		// Taking over from wherever the camera happened to be, so the picture does
		// not jump on the first pixel of the drag.
		f.freePos, f.follow = f.cam.Center, camFree
		f.manualScale = true
	}
	f.freePos = f.freePos.Add(shift)
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
	f.drawPrediction(clip, cam, a.ui.DT)

	padX, padY, padLabelled := f.drawPad(clip, cam)
	f.drawTrail(clip, cam)
	f.drawEventMarkers(clip, cam, padX, padY, padLabelled)
	f.drawVehicle(clip, cam, tm)
	f.drawScaleBar(clip, view, cam)
	f.drawViewHUD(clip, view, tm)
	f.drawNodePanel(a, clip, view)
	f.drawCamPicker(a, clip, view)
	f.handleCamera(u, view)
}

// nodePanelW is the width of the manoeuvre panel. It fits a time, a direction, a
// delta-v and a delete button on one row, which is the whole of what a node is.
const nodePanelW = 392

// drawNodePanel is the flight plan: what burns are scheduled, and the controls to
// change them. It lives in the trajectory view rather than the telemetry column
// because it is the one thing on this screen that is edited rather than read, and
// because the column has no room left.
func (f *FlightScreen) drawNodePanel(a *App, dst *ebiten.Image, view Rect) {
	u := a.ui
	nodes := f.s.Cfg.Nodes

	rows := len(nodes)
	for i := range nodes {
		if nodes[i].Frame == sim.BurnPitch {
			rows++
		}
	}
	h := 30 + float64(rows)*24 + 28
	r := Rect{view.Right() - nodePanelW - 12, view.Bottom() - h - 12, nodePanelW, h}
	panel(dst, r, colPanel)

	u.SectionHeader(dst, Rect{r.X + 10, r.Y + 6, r.W - 20, 18}, T("flight.secPlan"))
	if len(nodes) > 0 {
		drawText(dst, T("flight.planColumns"), fontUISm, r.Right()-14, r.Y+8, colTextFaint, alignRight)
	}

	c := &rowCursor{x: r.X + 10, y: r.Y + 28, w: r.W - 20}
	remove := -1
	for i := range nodes {
		n := &nodes[i]
		done := f.s.St.NodesDone&(1<<uint(i)) != 0

		row := c.next(24)
		// A node that has already run is history: it is dimmed rather than
		// hidden, because "this is where that burn happened" is worth keeping.
		if done {
			drawText(dst, fmtClock(n.T), fontMonoSm, row.X, row.Y+5, colNodeDone, alignLeft)
			drawText(dst, nodeFrameName(n.Frame), fontUISm, row.X+112, row.Y+5, colNodeDone, alignLeft)
			mark := ""
			if n.Separate {
				mark = " ⤓"
			}
			drawText(dst, fmt.Sprintf("%.0f %s%s", n.DeltaV, T("unit.mps"), mark), fontMonoSm,
				row.Right()-24, row.Y+5, colNodeDone, alignRight)
		} else {
			u.NumField(dst, Rect{row.X, row.Y, 104, 20}, "", &n.T,
				NumOpt{Unit: T("unit.s"), Dec: 0, Min: 0, Max: 1e9})
			if u.Button(dst, Rect{row.X + 108, row.Y, 96, 20}, nodeFrameName(n.Frame), ButtonNormal) {
				n.Frame = (n.Frame + 1) % (sim.BurnPitch + 1)
			}
			u.NumField(dst, Rect{row.X + 208, row.Y, 104, 20}, "", &n.DeltaV,
				NumOpt{Unit: T("unit.mps"), Dec: 0, Min: 0, Max: 1e6})
			// Whether the stage this burn used goes overboard when it is done. A
			// spent booster carried through a coast has to go before the engine
			// above it can fire, and there is nowhere else to say so.
			u.Checkbox(dst, Rect{row.Right() - 44, row.Y, 20, 20}, "", &n.Separate)
		}
		if u.Button(dst, Rect{row.Right() - 18, row.Y + 1, 18, 18}, "×", ButtonDanger) {
			remove = i
		}
		if n.Frame == sim.BurnPitch {
			sub := c.next(24)
			u.NumField(dst, Rect{sub.X + 108, sub.Y, 96, 20}, "", &n.Pitch,
				NumOpt{Unit: "°", Min: -90, Max: 90, Dec: 0})
		}
	}

	if remove >= 0 {
		// The nodes above shift down inside the same array, so a focused field
		// would carry on editing what is now a different burn. Everything else
		// the delete has to repair — the running index, the spent mask, an engine
		// that is on for a burn that no longer exists — belongs to the state, so
		// the simulation does it.
		u.cancel()
		f.s.RemoveNode(remove)
	}

	add := c.next(24)
	if len(f.s.Cfg.Nodes) < 8 && u.Button(dst, Rect{add.X, add.Y, 120, 20}, T("flight.addNode"), ButtonNormal) {
		u.cancel()
		f.s.Cfg.Nodes = append(f.s.Cfg.Nodes, sim.Node{
			T: math.Round(f.s.St.T + 120), Frame: sim.BurnPrograde, DeltaV: 50,
		})
	}
	if f.s.St.Node >= 0 {
		drawText(dst, fmt.Sprintf(T("flight.burning"), f.s.St.NodeDV,
			f.s.Cfg.Nodes[f.s.St.Node].DeltaV), fontUISm,
			add.Right(), add.Y+4, colPred, alignRight)
	}
}

// nodeFrameName is what a burn direction is called on screen.
func nodeFrameName(fr sim.BurnFrame) string {
	switch fr {
	case sim.BurnRetrograde:
		return T("node.retrograde")
	case sim.BurnRadialOut:
		return T("node.radialOut")
	case sim.BurnRadialIn:
		return T("node.radialIn")
	case sim.BurnPitch:
		return T("node.pitch")
	default:
		return T("node.prograde")
	}
}

// predHorizon is how far ahead to predict: a couple of orbits, or far enough to
// see what the last planned burn does, whichever is further.
func (f *FlightScreen) predHorizon() float64 {
	o := sim.ComputeOrbit(f.s.St.Pos, f.s.St.Vel, f.s.Center().Mu)
	horizon := 2 * 3600.0
	if o.Bound() && o.Period > 0 {
		horizon = 2 * o.Period
	}
	for i := range f.s.Cfg.Nodes {
		if d := f.s.Cfg.Nodes[i].T - f.s.St.T; d > 0 {
			// A burn is planned because it changes the trajectory into something
			// the current orbit's period says nothing about. Four days past it is
			// enough to see a transfer arrive somewhere.
			horizon = math.Max(horizon, d+4*86400)
		}
	}
	return clamp(horizon, 600, 10*86400)
}

// drawPrediction traces where the vehicle is going, planned burns included. The
// osculating ellipse only answers "what if nothing happens", and a flight plan
// is a statement that something is about to.
//
// Drawn in the non-rotating frame, unlike the trail: a future path turned
// backwards by the ground's rotation would be nonsense. That is also why it only
// appears once the vehicle is out of the air, where the ascent's ground-frame
// reading stops being the useful one.
func (f *FlightScreen) drawPrediction(dst *ebiten.Image, cam *Camera, dt float64) {
	// Only while coasting. During the ascent the pitch programme is flying and a
	// prediction of it says nothing useful — and it is the expensive case: with a
	// burn in progress every predicted step is a fixed 0.02 s one, which came to
	// 650 ms of work twice a second in a system of eighteen bodies. That was the
	// stutter that showed up on the way out of the atmosphere, which is exactly
	// where the altitude test below starts letting predictions through.
	if f.s.St.Done || f.s.St.Phase != sim.PhaseCoast || f.s.Altitude() <= f.s.Cfg.Atmo.Top {
		f.pred = nil
		return
	}

	f.predAge += dt
	// Half a second. A long plan is tens of thousands of integrator steps —
	// a burn runs at the fixed step in the prediction too — and nothing about the
	// answer changes in that time.
	if f.pred == nil || f.predAge > 0.5 {
		f.pred = f.s.Predict(f.predHorizon(), 400)
		f.predAge = 0
	}

	// Same reason the trail thins itself: at orbital zoom hundreds of points land
	// on the same pixel, and every one of them is still a separate draw.
	var px, py float64
	first := true
	for i, p := range f.pred {
		x, y := cam.Project(f.framePoint(p.Pos, p.Center, p.T))
		if first {
			px, py, first = x, y, false
			continue
		}
		if math.Abs(x-px)+math.Abs(y-py) < 1.2 && i < len(f.pred)-1 {
			continue
		}
		line(dst, px, py, x, y, 1, colPred)
		px, py = x, y
	}
}

// trailWindow is the shortest stretch of flight, in seconds, that stays drawn
// behind the vehicle. Longer than any ascent in the presets, so nothing is
// trimmed on the way up.
const trailWindow = 900

// trailSpan is how far back the trail actually reaches. A fixed number of seconds
// cannot serve both ends of this: fifteen minutes covers an ascent and is a tenth
// of a pixel of an interplanetary cruise, where it left the flown path invisible
// and the vehicle apparently drawn from nowhere.
//
// What the window is really guarding against is *revolutions* — a trail that wraps
// the same orbit over and over is one smear — so the bound is one period of the
// orbit the vehicle is on. A trajectory that is not coming back round has no
// revolutions to repeat, and gets the whole flight.
func (f *FlightScreen) trailSpan() float64 {
	o := f.s.Telemetry().Orbit
	if !o.Bound() || o.Period <= 0 {
		return math.Inf(1)
	}
	return math.Max(o.Period, trailWindow)
}

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

	// Names last, in a pass of their own, so that a moon's cannot land on top of
	// its planet's: at system scale the two are the same pixel. Index order gives
	// the priority — the Sun and the planets are declared before the moons, and a
	// moon two pixels from its planet is not worth naming.
	var taken []Rect
	for i := range sys.Bodies {
		f.drawBodyLabel(dst, view, cam, i, &taken)
	}
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

	surface, rim, dot := bodyPaint(b.Name)

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
		ring(dst, x, y, rpx, 1.5, rim)
		drawRings(dst, b.Name, x, y, rpx, lighten(surface, 0.38))

	default:
		// A dot. Anything smaller than a couple of pixels would otherwise
		// vanish, and a moon you cannot see is a moon you cannot aim at. Brighter
		// than the surface it stands for, because two pixels of dull red is two
		// pixels of nothing — and sized by how big the body actually is, so that
		// the Sun and Phobos are not the same speck.
		if !view.Inset(-8).Contains(x, y) {
			return
		}
		circle(dst, x, y, dotRadius(b.Radius), dot)
	}
}

// drawRings paints a body's rings, if it has any. Pure decoration: the physics
// does not know about them, and neither does anything that has to be correct.
//
// Face-on, as concentric bands, which is what a plane seen from above gives — the
// same convention that makes every orbit in this simulator a circle rather than an
// ellipse foreshortened by a viewing angle there is no room for.
func drawRings(dst *ebiten.Image, name string, x, y, rpx float64, c color.NRGBA) {
	// Under a few pixels the planet is a speck and its rings would be mush.
	if rpx < 3 || rpx > maxRingPx/3 {
		return
	}
	for _, band := range bodyRings[name] {
		w := (band.outer - band.inner) * rpx
		if w < 0.5 {
			continue
		}
		mid := (band.inner + band.outer) / 2 * rpx
		ring(dst, x, y, mid, w, color.NRGBA{c.R, c.G, c.B, band.alpha})
	}
}

// dotRadius is how big a body draws when it is too small to draw at all: a
// logarithm of its real size, because the range from Phobos to the Sun is five
// orders of magnitude and a linear scale would make everything but the Sun a
// single pixel.
func dotRadius(radius float64) float64 {
	if radius <= 0 {
		return 1.5
	}
	return clamp(1.6+math.Log10(radius/1e6)*0.6, 1.4, 3.4)
}

// drawBodyLabel names a body, unless something already has that patch of screen.
func (f *FlightScreen) drawBodyLabel(dst *ebiten.Image, view Rect, cam *Camera, i int, taken *[]Rect) {
	b := &f.s.Cfg.System.Bodies[i]
	x, y := cam.Project(f.framePoint(sim.Vec2{}, i, f.s.St.T))
	rpx := cam.Len(b.Radius)

	// Nothing on the body underfoot: the name would sit in the middle of the
	// ground it is standing on.
	if rpx >= math.Min(view.W, view.H)/3 || !view.Inset(-8).Contains(x, y) {
		return
	}

	name := bodyName(b.Name)
	w := textWidth(name, fontUISm)
	var r Rect
	if rpx < 4 {
		// A dot shares its patch of screen with the launch pad's own label, which
		// is drawn above it. Go underneath.
		r = Rect{x - w/2, y + 6, w, fontUISm.Size + 2}
	} else {
		// Past the rings, if there are any: at the planet's own edge the name
		// would be printed across the middle of them.
		r = Rect{x + rpx*ringExtent(b.Name) + 6, y - 8, w, fontUISm.Size + 2}
	}
	for _, t := range *taken {
		if r.X < t.Right() && t.X < r.Right() && r.Y < t.Bottom() && t.Y < r.Bottom() {
			return
		}
	}
	*taken = append(*taken, r)
	drawText(dst, name, fontUISm, r.X, r.Y, colBodyText, alignLeft)
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

	surface, rim, _ := bodyPaint(b.Name)

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
	band(0, 1.5, rim)
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

	// Once in orbit the flight has no end, so neither would the trail: it would
	// wrap the planet again and again until the whole picture is one smear. One
	// revolution of the current orbit is the bound; see trailSpan.
	first := 0
	if cutoff := f.s.St.T - f.trailSpan(); cutoff > 0 {
		first = sampleAt(h, cutoff)
	}
	n := len(h)
	if n-first < 2 {
		return
	}

	// The pen lifts over anything flown in another frame rather than drawing a line
	// across the hole it leaves: the two ends of that hole are hundreds of thousands
	// of kilometres apart and the vehicle did not fly between them in a straight line.
	var px, py float64
	pen := false
	for i := first; i < n; i++ {
		w, ok := f.showTrack(h[i])
		if !ok {
			pen = false
			continue
		}
		x, y := cam.Project(f.trackPoint(h[i]))
		if !pen {
			px, py, pen = x, y, true
			continue
		}
		if i < n-1 && math.Abs(x-px)+math.Abs(y-py) < minSeg {
			continue
		}
		// Older samples fade out, so the recent path stays legible even after
		// the trajectory has wrapped a long way around the planet. And a leg the
		// picture is letting go of fades out as a whole, on top of that.
		t := float64(i-first) / float64(n-first)
		c := color.NRGBA{
			uint8(float64(colTrailOld.R) + (float64(colTrail.R)-float64(colTrailOld.R))*t),
			uint8(float64(colTrailOld.G) + (float64(colTrail.G)-float64(colTrailOld.G))*t),
			uint8(float64(colTrailOld.B) + (float64(colTrail.B)-float64(colTrailOld.B))*t),
			uint8(255 * w),
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
		// A marker sits on the trail, so it comes and goes with it: whole in the
		// frame being drawn, fading with the one just left, gone after that.
		w, ok := f.showTrack(hist[i])
		if !ok {
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
		c.A = uint8(255 * w)
		ring(dst, x, y, 4, 1.5, c)

		label := eventLabel(e, &f.s.Cfg.System)
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

	if f.s.Settled() {
		y += 22
		verdict := outcomeText(f.s.St.Outcome, bodyName(f.s.Cfg.System.Bodies[f.s.St.OutcomeBody].Name))
		vc := colBad
		switch f.s.St.Outcome {
		case sim.OutcomeOrbit, sim.OutcomeCaptured, sim.OutcomeReturned:
			vc = colGood
		case sim.OutcomeDecaying:
			vc = colWarn
		}
		drawText(dst, verdict, fontHead, x, y, vc, alignLeft)
	}

	// A warp the current regime cannot deliver is worth saying out loud, or the
	// setting looks broken: inside the atmosphere the step cannot grow, so a
	// million times real time is not on offer at any price.
	if f.s.WarpLimited && !f.paused {
		drawText(dst, T("flight.warpLimited"), fontUISm, view.Right()-14, view.Y+30, colWarn, alignRight)
	}
}

// camPickerW is the width of the camera's target picker.
const camPickerW = 150

// drawCamPicker is the camera's target: the vehicle, or any body in the system.
// Cycling with Tab is fine for two bodies and hopeless for eighteen.
func (f *FlightScreen) drawCamPicker(a *App, dst *ebiten.Image, view Rect) {
	u := a.ui
	bodies := f.s.Cfg.System.Bodies

	items := make([]string, len(bodies)+1)
	items[0] = T("flight.camVehicle")
	for i := range bodies {
		items[i+1] = bodyName(bodies[i].Name)
	}
	sel := 0
	if f.follow >= 0 {
		sel = f.follow + 1
	}
	// A dragged view is following nothing, and the picker has to say so: showing
	// "the vehicle" while the camera sits half way to the Moon is a lie about the
	// one thing this control exists to report. The free state gets its own entry
	// at the top, which shifts everything else by one while it lasts.
	free := f.follow == camFree
	if free {
		items = append([]string{T("flight.camFree")}, items...)
		sel = 0
	}

	r := Rect{view.Right() - 14 - camPickerW, view.Y + 10, camPickerW, 22}
	if picked := u.Dropdown(dst, r, "camera", items, sel); picked != sel {
		if free {
			picked--
		}
		f.lookAt(picked - 1)
	}

	if u.Button(dst, Rect{r.X, r.Bottom() + 4, camPickerW, 20}, T("flight.camReset"), ButtonNormal) {
		f.lookAt(-1)
		f.manualScale = false
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
	row(T("common.maxQ"), fmt.Sprintf(T("flight.maxQAt"), fmtEng(q, T("unit.pa")), formatNum(qAlt/1000, 1)), colTextDim)
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
		f.lookAt(-1)
		f.manualScale = false
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
