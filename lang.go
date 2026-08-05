package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"log"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/pavel-krush/fsim/sim"
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

// eventLabel is the timeline caption for an event. Some of them are about a
// body, and pass its name through the format string.
func eventLabel(e sim.Event, sys *sim.System) string {
	switch e.Kind {
	case sim.EvSOIEnter:
		return fmt.Sprintf(T("event.soiEnter"), bodyName(sys.Bodies[e.Body].Name))
	case sim.EvSOIExit:
		return fmt.Sprintf(T("event.soiExit"), bodyName(sys.Bodies[e.Body].Name))
	}
	switch e.Kind {
	case sim.EvLiftoff:
		return T("event.liftoff")
	case sim.EvMaxQ:
		return T("event.maxQ")
	case sim.EvCutoff:
		return T("event.cutoff")
	case sim.EvSeparation:
		return T("event.separation")
	case sim.EvIgnition:
		return T("event.ignition")
	case sim.EvApoapsis:
		return T("event.apoapsis")
	case sim.EvOrbit:
		return T("event.orbit")
	case sim.EvEnd:
		return T("event.end")
	default:
		return ""
	}
}

// outcomeText is the verdict shown when the flight is over. Two of them name the
// body they are about.
func outcomeText(o sim.Outcome, body string) string {
	switch o {
	case sim.OutcomeCaptured:
		return fmt.Sprintf(T("outcome.captured"), body)
	case sim.OutcomeImpact:
		return fmt.Sprintf(T("outcome.impact"), body)
	}
	switch o {
	case sim.OutcomeOrbit:
		return T("outcome.orbit")
	case sim.OutcomeDecaying:
		return T("outcome.decaying")
	case sim.OutcomeSuborbital:
		return T("outcome.suborbital")
	case sim.OutcomeCrashed:
		return T("outcome.crashed")
	case sim.OutcomeReturned:
		return T("outcome.returned")
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

// bodyName turns a body identifier into display text. Like preset names, what a
// body is called is a presentation decision: sim only carries the identifier.
//
// Looked up rather than switched on, because there are seventeen of them and a
// switch would be seventeen lines of the same line. An identifier with no entry
// renders as itself, which is the same safety net T has.
func bodyName(key string) string {
	if key == "" {
		return ""
	}
	full := "body." + key
	if s := T(full); s != full {
		return s
	}
	return key
}

// presetName turns a preset identifier into display text. A lookup rather than a
// switch, for the same reason bodyName is one: thirteen cases is past the point
// where writing them out earns anything, and a missing entry renders as the
// identifier instead of as nothing.
func presetName(key string) string {
	full := "preset." + key
	if s := T(full); s != full {
		return s
	}
	return key
}
