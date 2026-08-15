//go:build js

package main

import (
	"net/url"
	"os"
	"strconv"
	"strings"
	"syscall/js"

	"github.com/pavel-krush/fsim/sim"
)

// A page has no command line, so the query string is one:
//
//	?preset=apollo-mars&lang=ru
//
// becomes -preset apollo-mars -lang ru before flag.Parse ever looks at os.Args.
// That is the whole point of it — a link can say "here is the way to Mars, a
// hundred and eighty-six days, watch" rather than just "here is a simulator".
//
// Only the flags that mean anything in a browser are accepted: -preset, -lang,
// -fly and -scale. -shot writes files and -camtrace prints to a console nobody has
// open.
//
// A value that is not a preset or a language is dropped rather than passed on,
// because main treats both as fatal: a mistyped link would leave whoever clicked
// it looking at a blank page with the reason in a console they will never open.
// Dropped, it starts on the default instead, which is a working simulator.
func init() {
	loc := js.Global().Get("location")
	if !loc.Truthy() {
		return
	}
	q, err := url.ParseQuery(strings.TrimPrefix(loc.Get("search").String(), "?"))
	if err != nil {
		return
	}

	if name := q.Get("preset"); name != "" {
		for _, p := range sim.Presets() {
			if p.Name == name {
				os.Args = append(os.Args, "-preset", name)
				break
			}
		}
	}
	if code := q.Get("lang"); code != "" {
		if _, ok := localeCode[code]; ok {
			os.Args = append(os.Args, "-lang", code)
		}
	}
	// ?scale=1 renders one frame pixel per interface pixel, which is what to reach for on a
	// machine where the page is slow: the sharp default asks for the display's own density and
	// costs four times the pixels on a Retina screen. ?scale=3 goes the other way. Anything that
	// is not a number between a half and four is dropped rather than argued with.
	if v := q.Get("scale"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f >= 0.5 && f <= 4 {
			os.Args = append(os.Args, "-scale", v)
		}
	}
	// ?fly=1 goes straight to the pad, which is the whole of what a link wants to
	// do when it is showing someone a mission rather than handing them an editor.
	if v := q.Get("fly"); v == "1" || v == "true" {
		os.Args = append(os.Args, "-fly")
	}
}
