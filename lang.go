package main

import (
	"github.com/hajimehoshi/ebiten/v2"

	"fsim/sim"
)

// Interface language. With exactly two languages, keeping both variants side
// by side at the call site beats a keyed table: nothing can drift out of sync,
// nothing can be missing, and the surrounding code stays readable.

// Lang is the language the interface is drawn in.
type Lang int

const (
	RU Lang = iota
	EN
)

// lang is global on purpose. It is read from every draw call in the program
// and changes only when the user clicks the toggle.
var lang = RU

// t picks the variant for the current language.
func t(ru, en string) string {
	if lang == EN {
		return en
	}
	return ru
}

// langToggleW is the width the switch occupies in every bar that hosts it.
const langToggleW = 96

// LangToggle draws the two-part language switch and applies a click.
func (u *UI) LangToggle(dst *ebiten.Image, r Rect) {
	half := r.W / 2
	ru := Rect{r.X, r.Y, half, r.H}
	en := Rect{r.X + half, r.Y, r.W - half, r.H}

	style := func(l Lang) ButtonStyle {
		if lang == l {
			return ButtonActive
		}
		return ButtonNormal
	}
	if u.Button(dst, ru, "РУС", style(RU)) {
		lang = RU
	}
	if u.Button(dst, en, "ENG", style(EN)) {
		lang = EN
	}
}

// eventLabel is the timeline caption for an event kind.
func eventLabel(k sim.EventKind) string {
	switch k {
	case sim.EvLiftoff:
		return t("СТАРТ", "LIFTOFF")
	case sim.EvMaxQ:
		return "MAX Q"
	case sim.EvCutoff:
		return t("ОТСЕЧКА", "CUTOFF")
	case sim.EvSeparation:
		return t("РАЗДЕЛЕНИЕ", "SEPARATION")
	case sim.EvIgnition:
		return t("ЗАЖИГАНИЕ", "IGNITION")
	case sim.EvApoapsis:
		return t("АПОГЕЙ", "APOAPSIS")
	case sim.EvEnd:
		return t("КОНЕЦ", "END")
	default:
		return ""
	}
}

// outcomeText is the verdict shown when the flight is over.
func outcomeText(o sim.Outcome) string {
	switch o {
	case sim.OutcomeOrbit:
		return t("ОРБИТА ДОСТИГНУТА", "ORBIT ACHIEVED")
	case sim.OutcomeDecaying:
		return t("ОРБИТА НЕУСТОЙЧИВА — ПЕРИГЕЙ В АТМОСФЕРЕ",
			"ORBIT UNSTABLE — PERIAPSIS INSIDE THE ATMOSPHERE")
	case sim.OutcomeSuborbital:
		return t("СУБОРБИТАЛЬНЫЙ ПОЛЁТ", "SUBORBITAL FLIGHT")
	case sim.OutcomeCrashed:
		return t("АВАРИЯ — УДАР О ПОВЕРХНОСТЬ", "CRASHED INTO THE SURFACE")
	case sim.OutcomeEscape:
		return t("УХОД С ОРБИТЫ ПЛАНЕТЫ", "ESCAPED THE PLANET")
	case sim.OutcomeTimeout:
		return t("ЛИМИТ ВРЕМЕНИ ИСЧЕРПАН", "TIME LIMIT REACHED")
	default:
		return t("ПОЛЁТ", "IN FLIGHT")
	}
}

// phaseText describes what the staging sequence is doing.
func phaseText(p sim.Phase) string {
	switch p {
	case sim.PhaseBurn:
		return t("РАБОТА ДВИГАТЕЛЯ", "ENGINE BURNING")
	case sim.PhaseSepWait:
		return t("ОЖИДАНИЕ РАЗДЕЛЕНИЯ", "AWAITING SEPARATION")
	case sim.PhaseIgnitionWait:
		return t("ОЖИДАНИЕ ЗАЖИГАНИЯ", "AWAITING IGNITION")
	default:
		return t("ПАССИВНЫЙ ПОЛЁТ", "COASTING")
	}
}

// presetName turns a preset identifier into display text.
func presetName(key string) string {
	switch key {
	case "earth-falcon":
		return t("Земля / Falcon-9", "Earth / Falcon-9")
	case "mars":
		return t("Марс / лёгкий носитель", "Mars / light launcher")
	case "moon":
		return t("Луна / без атмосферы", "Moon / no atmosphere")
	case "kerbin":
		return t("Кербин / KSP-подобный", "Kerbin / KSP-like")
	default:
		return key
	}
}
