//go:build js

package main

import (
	"net/url"
	"os"
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
// Only the two flags that mean anything in a browser are accepted. -shot writes
// files and -camtrace prints to a console nobody has open.
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
}
