//go:build js

package main

import (
	"errors"
	"fmt"
	"syscall/js"
)

// The saved setup in localStorage, because a page has no filesystem. It is per-origin,
// so a build served from a laptop and the published page keep separate setups — which
// is the right way round: the one you are editing is the one you are looking at.
//
// Every call here has to survive the browser throwing. localStorage is denied outright
// in some privacy modes, and setItem throws on quota — a few kilobytes of setup will
// not reach a quota, but "will not" is not "cannot", and a page that dies on a save is
// worse than one that says it could not save. A js.Value call raises a Go panic on a
// JS exception, so both are recovered into errors.

func localStorage() (js.Value, error) {
	// Reading the property can itself throw where storage is blocked by policy.
	ls, err := jsTry(func() js.Value { return js.Global().Get("localStorage") })
	if err != nil {
		return js.Undefined(), err
	}
	if !ls.Truthy() {
		return js.Undefined(), errors.New("this browser is not offering localStorage")
	}
	return ls, nil
}

// jsTry turns a JS exception into an error instead of a panic.
func jsTry(f func() js.Value) (v js.Value, err error) {
	defer func() {
		if r := recover(); r != nil {
			if e, ok := r.(*js.Error); ok {
				err = fmt.Errorf("%s", e.Value.Get("message").String())
				return
			}
			err = fmt.Errorf("%v", r)
		}
	}()
	return f(), nil
}

func storeWrite(data []byte) error {
	ls, err := localStorage()
	if err != nil {
		return err
	}
	_, err = jsTry(func() js.Value {
		ls.Call("setItem", storeKey, string(data))
		return js.Undefined()
	})
	return err
}

func storeRead() ([]byte, bool, error) {
	ls, err := localStorage()
	if err != nil {
		return nil, false, err
	}
	v, err := jsTry(func() js.Value { return ls.Call("getItem", storeKey) })
	if err != nil {
		return nil, false, err
	}
	// A key that was never set reads as null, which is nothing saved rather than a
	// fault — the same distinction the native side draws from IsNotExist.
	if !v.Truthy() {
		return nil, false, nil
	}
	return []byte(v.String()), true, nil
}
