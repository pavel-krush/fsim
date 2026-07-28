package main

import (
	"embed"
	"encoding/json"
	"log"

	"github.com/hajimehoshi/ebiten/v2"

	"fsim/sim"
)

// Interface text lives in assets/locale/<code>.json, keyed by dotted
// identifiers, and is embedded into the binary. Keeping the copy out of the
// screens means the whole wording of the program can be read and revised in
// one place, and a new language is one more file.
//
// The compiler cannot check a string key, so TestLocales stands in for it: it
// verifies that every language carries the same key set and that every key
// used anywhere in the source actually exists.

//go:embed assets/locale/*.json
var localeFS embed.FS

// Lang is the language the interface is drawn in.
type Lang int

const (
	RU Lang = iota
	EN
)

// localeFile is the asset each language loads from. The code is also what the
// -lang flag accepts.
var localeFile = map[Lang]string{
	RU: "assets/locale/ru.json",
	EN: "assets/locale/en.json",
}

// localeCode maps a command-line code onto a language.
var localeCode = map[string]Lang{"ru": RU, "en": EN}

// defaultLang is what the program starts in, and what a key missing from the
// selected language falls back to.
const defaultLang = EN

// lang is global on purpose. It is read from every draw call in the program
// and changes only when the user clicks the toggle.
var lang = defaultLang

// locales holds every language's table, loaded once at startup.
var locales = map[Lang]map[string]string{}

// loadLocales reads every embedded language file. A failure here is fatal:
// without text the interface is unusable, and the files ship inside the
// binary, so a failure means the build itself is broken.
func loadLocales() {
	for l, path := range localeFile {
		data, err := localeFS.ReadFile(path)
		if err != nil {
			log.Fatalf("locale %s: %v", path, err)
		}
		var table map[string]string
		if err := json.Unmarshal(data, &table); err != nil {
			log.Fatalf("locale %s: %v", path, err)
		}
		locales[l] = table
	}
}

// T looks up interface text by key in the current language.
//
// A missing key renders as the key itself rather than as an empty gap. That is
// the whole safety net: a typo shows up the moment the screen is opened, so
// nothing has to go looking for one ahead of time.
func T(key string) string {
	if s, ok := locales[lang][key]; ok {
		return s
	}
	if s, ok := locales[defaultLang][key]; ok {
		return s
	}
	return key
}

// langPickerW is the width the switch occupies in every bar that hosts it.
const langPickerW = 116

// langOrder is the order the languages appear in the picker, and it starts
// with the default. langName is each one written in its own script — a picker
// that translated its own options would be useless to whoever cannot read the
// language currently selected.
var (
	langOrder = []Lang{EN, RU}
	langName  = map[Lang]string{EN: "English", RU: "Русский"}
)

// LangPicker draws the language picker and applies a change.
func (u *UI) LangPicker(dst *ebiten.Image, r Rect) {
	names := make([]string, len(langOrder))
	sel := 0
	for i, l := range langOrder {
		names[i] = langName[l]
		if l == lang {
			sel = i
		}
	}
	if picked := u.Dropdown(dst, r, "language", names, sel); picked != sel {
		lang = langOrder[picked]
	}
}

// eventLabel is the timeline caption for an event kind.
func eventLabel(k sim.EventKind) string {
	switch k {
	case sim.EvLiftoff:
		return T("event.liftoff")
	case sim.EvMaxQ:
		return "MAX Q"
	case sim.EvCutoff:
		return T("event.cutoff")
	case sim.EvSeparation:
		return T("event.separation")
	case sim.EvIgnition:
		return T("event.ignition")
	case sim.EvApoapsis:
		return T("event.apoapsis")
	case sim.EvEnd:
		return T("event.end")
	default:
		return ""
	}
}

// outcomeText is the verdict shown when the flight is over.
func outcomeText(o sim.Outcome) string {
	switch o {
	case sim.OutcomeOrbit:
		return T("outcome.orbit")
	case sim.OutcomeDecaying:
		return T("outcome.decaying")
	case sim.OutcomeSuborbital:
		return T("outcome.suborbital")
	case sim.OutcomeCrashed:
		return T("outcome.crashed")
	case sim.OutcomeEscape:
		return T("outcome.escape")
	case sim.OutcomeTimeout:
		return T("outcome.timeout")
	default:
		return T("outcome.flying")
	}
}

// phaseText describes what the staging sequence is doing.
func phaseText(p sim.Phase) string {
	switch p {
	case sim.PhaseBurn:
		return T("phase.burn")
	case sim.PhaseSepWait:
		return T("phase.sepWait")
	case sim.PhaseIgnitionWait:
		return T("phase.ignitionWait")
	default:
		return T("phase.coast")
	}
}

// presetName turns a preset identifier into display text.
func presetName(key string) string {
	switch key {
	case "earth-falcon":
		return T("preset.earthFalcon")
	case "mars":
		return T("preset.mars")
	case "moon":
		return T("preset.moon")
	case "kerbin":
		return T("preset.kerbin")
	default:
		return key
	}
}
