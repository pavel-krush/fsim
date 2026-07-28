package main

import (
	"bytes"
	"image/color"
	"log"

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
	colPlanetHi = color.NRGBA{0x3a, 0x57, 0x44, 0xff}
	colAir      = color.NRGBA{0x4d, 0xa6, 0xff, 0x20}
	colTrail    = color.NRGBA{0x6f, 0xc8, 0xff, 0xff}
	colTrailOld = color.NRGBA{0x2c, 0x5b, 0x7d, 0xff}
	colMaxQ     = color.NRGBA{0x5e, 0xd6, 0xd6, 0xff}
	colPad      = color.NRGBA{0x8d, 0x9a, 0xa8, 0xff}
	colPadDeck  = color.NRGBA{0x5c, 0x66, 0x72, 0xff}
	colOrbit    = color.NRGBA{0x8a, 0x6f, 0xd0, 0xff}
	colTarget   = color.NRGBA{0x4f, 0x45, 0x77, 0xff}
	colGrid     = color.NRGBA{0x1e, 0x27, 0x33, 0xff}
)

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
		log.Fatalf("не удалось загрузить шрифт: %v", err)
	}
	return s
}
