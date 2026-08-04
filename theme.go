package main

import (
	"bytes"
	"image/color"
	"log"
	"math"

	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/gomonobold"
	"golang.org/x/image/font/gofont/goregular"
)

// The palette: a dark mission-control look. Everything is muted except the
// live telemetry, so the numbers that change are the ones that catch the eye.
var (
	colBG        = color.NRGBA{0x0d, 0x11, 0x17, 0xff}
	colPanel     = color.NRGBA{0x14, 0x1a, 0x23, 0xff}
	colPanelHi   = color.NRGBA{0x1c, 0x24, 0x30, 0xff}
	colBorder    = color.NRGBA{0x2a, 0x35, 0x45, 0xff}
	colText      = color.NRGBA{0xc8, 0xd4, 0xe0, 0xff}
	colTextDim   = color.NRGBA{0x76, 0x86, 0x99, 0xff}
	colTextFaint = color.NRGBA{0x4a, 0x57, 0x67, 0xff}
	colAccent    = color.NRGBA{0x4d, 0xa6, 0xff, 0xff}
	colAccentDim = color.NRGBA{0x27, 0x5a, 0x8f, 0xff}
	colGood      = color.NRGBA{0x3d, 0xd5, 0x98, 0xff}
	colWarn      = color.NRGBA{0xff, 0xb3, 0x4d, 0xff}
	colBad       = color.NRGBA{0xff, 0x5c, 0x5c, 0xff}
	colFlame     = color.NRGBA{0xff, 0x9a, 0x3c, 0xff}

	colPlanet   = color.NRGBA{0x27, 0x3d, 0x2f, 0xff}
	colAir      = color.NRGBA{0x4d, 0xa6, 0xff, 0x20}
	colTrail    = color.NRGBA{0x6f, 0xc8, 0xff, 0xff}
	colTrailOld = color.NRGBA{0x2c, 0x5b, 0x7d, 0xff}
	colMaxQ     = color.NRGBA{0x5e, 0xd6, 0xd6, 0xff}
	colPad      = color.NRGBA{0x8d, 0x9a, 0xa8, 0xff}
	colPadDeck  = color.NRGBA{0x5c, 0x66, 0x72, 0xff}
	colOrbit    = color.NRGBA{0x8a, 0x6f, 0xd0, 0xff}
	colTarget   = color.NRGBA{0x4f, 0x45, 0x77, 0xff}
	colGrid     = color.NRGBA{0x1e, 0x27, 0x33, 0xff}

	// Anything with no colour of its own — a body someone added in the editor.
	// Its rail is faint enough not to compete with the trajectory.
	colBody     = color.NRGBA{0x5a, 0x5f, 0x6b, 0xff}
	colRail     = color.NRGBA{0x2e, 0x3a, 0x4c, 0xff}
	colBodyText = color.NRGBA{0x93, 0x9c, 0xab, 0xff}

	// The predicted path, and the node panel's accent. Warmer than the trail so
	// that where the vehicle is going does not read as where it has been.
	colPred     = color.NRGBA{0xd0, 0x8a, 0x5a, 0xff}
	colNodeDone = color.NRGBA{0x54, 0x60, 0x6e, 0xff}
)

// bodyColors is what each body is painted with, keyed by the identifier sim
// carries. Which is to say: this is the one place that knows Mars is red, because
// the physics has no business knowing it.
//
// All of them are dim. The trajectory, the prediction and the markers are the
// things being read on this screen; a planet is scenery, and scenery that
// competes with the instruments is a planet drawn too brightly. Earth keeps the
// green it has always had — it is the ground under the launch pad in the close-up
// view, and land is what that should look like.
var bodyColors = map[string]color.NRGBA{
	"sun":     {0xc9, 0x9e, 0x3c, 0xff},
	"mercury": {0x6b, 0x63, 0x5c, 0xff},
	"venus":   {0xa8, 0x94, 0x63, 0xff},
	"earth":   colPlanet,
	"mars":    {0x5e, 0x33, 0x28, 0xff},
	"jupiter": {0x7d, 0x64, 0x48, 0xff},
	"saturn":  {0x8a, 0x7a, 0x54, 0xff},
	"uranus":  {0x45, 0x72, 0x78, 0xff},
	"neptune": {0x33, 0x4c, 0x8c, 0xff},

	"moon":     {0x5a, 0x5f, 0x6b, 0xff},
	"phobos":   {0x4c, 0x46, 0x42, 0xff},
	"deimos":   {0x4c, 0x46, 0x42, 0xff},
	"io":       {0x8a, 0x7e, 0x3e, 0xff},
	"europa":   {0x82, 0x7d, 0x74, 0xff},
	"ganymede": {0x6e, 0x66, 0x5e, 0xff},
	"callisto": {0x4f, 0x4c, 0x50, 0xff},
	"titan":    {0x8a, 0x6a, 0x3c, 0xff},
	"triton":   {0x74, 0x6e, 0x78, 0xff},

	"kerbin": colPlanet,
}

// ringBand is one annulus of a body's ring system, measured in the body's own
// radii, with how solid it looks.
type ringBand struct {
	inner, outer float64
	alpha        uint8
}

// bodyRings is decoration and nothing else: no mass, no shadow, no shepherding
// moons, and nothing in sim knows they exist. The radii are the real ones — the C,
// B and A rings, with the Cassini division as the gap between the last two — and
// they are drawn face-on because everything in this simulator shares one plane and
// a ring system lies in its planet's equator.
var bodyRings = map[string][]ringBand{
	"saturn": {
		{1.24, 1.52, 0x1e}, // C, thin and dark
		{1.52, 1.95, 0x52}, // B, the bright one
		{2.03, 2.27, 0x38}, // A, past the Cassini division
	},
}

// bodyPaint is the surface colour of a body, the rim that outlines it, and the dot
// it collapses into at a distance. The rim and the dot are derived rather than
// listed: eighteen bodies is where three hand-picked shades each stops being worth
// maintaining, and the ratios are what make them read as the same body.
func bodyPaint(name string) (surface, rim, dot color.NRGBA) {
	surface = colBody
	if c, ok := bodyColors[name]; ok {
		surface = c
	}
	return surface, lighten(surface, 0.22), lighten(surface, 0.5)
}

// ringExtent is how far a body's rings reach, in its own radii, or 1 for a body
// with none. The label placement needs it: a name printed at the planet's edge
// lands in the middle of the ring system.
func ringExtent(name string) float64 {
	out := 1.0
	for _, b := range bodyRings[name] {
		out = math.Max(out, b.outer)
	}
	return out
}

// The plot series colours, kept distinguishable in the graph screen.
var plotColors = []color.NRGBA{
	{0x4d, 0xa6, 0xff, 0xff},
	{0x3d, 0xd5, 0x98, 0xff},
	{0xff, 0xb3, 0x4d, 0xff},
	{0xff, 0x7a, 0xa2, 0xff},
	{0xa9, 0x8b, 0xff, 0xff},
	{0x5e, 0xd6, 0xd6, 0xff},
}

// Font faces. Proportional for labels, monospaced for anything numeric so the
// telemetry does not jitter as digits change.
var (
	fontUI     *text.GoTextFace
	fontUISm   *text.GoTextFace
	fontMono   *text.GoTextFace
	fontMonoSm *text.GoTextFace
	fontHead   *text.GoTextFace
	fontBig    *text.GoTextFace
)

func initFonts() {
	reg := mustSource(goregular.TTF)
	mono := mustSource(gomono.TTF)
	bold := mustSource(gomonobold.TTF)

	fontUI = &text.GoTextFace{Source: reg, Size: 13}
	fontUISm = &text.GoTextFace{Source: reg, Size: 11}
	fontMono = &text.GoTextFace{Source: mono, Size: 13}
	fontMonoSm = &text.GoTextFace{Source: mono, Size: 11}
	fontHead = &text.GoTextFace{Source: bold, Size: 13}
	fontBig = &text.GoTextFace{Source: bold, Size: 22}
}

func mustSource(ttf []byte) *text.GoTextFaceSource {
	s, err := text.NewGoTextFaceSource(bytes.NewReader(ttf))
	if err != nil {
		log.Fatalf("could not load font: %v", err)
	}
	return s
}
