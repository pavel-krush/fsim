//go:build !js

package main

import (
	"os"
	"path/filepath"
)

// The saved setup as a file. It goes in the user's own configuration directory —
// ~/Library/Application Support/fsim on a Mac, ~/.config/fsim on Linux, AppData on
// Windows — rather than beside the binary or in the working directory: a rebuild must
// not lose it, and a program that drops files where it happens to have been started
// from is a program nobody trusts with a second one.
//
// storeRoot overrides that directory, and exists for the tests: writing into the real
// one from a test run would quietly replace whatever the person running the tests had
// saved, which is precisely the fault this whole file is here to prevent.
var storeRoot string

func storePath() (string, error) {
	root := storeRoot
	if root == "" {
		dir, err := os.UserConfigDir()
		if err != nil {
			return "", err
		}
		root = dir
	}
	return filepath.Join(root, "fsim", storeKey+".json"), nil
}

func storeWrite(data []byte) error {
	path, err := storePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	// Written through a temporary file in the same directory and renamed over the
	// old one, so that a failure half way through leaves the previous save intact
	// rather than a truncated file where a setup used to be.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

func storeRead() ([]byte, bool, error) {
	path, err := storePath()
	if err != nil {
		return nil, false, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		// Nothing saved yet is the ordinary state of a fresh install, not a fault.
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return data, true, nil
}
