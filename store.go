package main

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/pavel-krush/fsim/sim"
)

// Saving a setup, so that an evening of editing survives closing the window.
//
// Everything the editor produces lives in one sim.Config — the system of bodies, the
// air, the vehicle, the pitch programme and the flight plan — so one file is the whole
// of it. There is a single slot rather than a library of named ones: the fault being
// fixed is losing work, not the absence of a collection.
//
// What is written is the *inputs*. Mu and SOI are derived, and carry a json:"-" for it
// in sim/body.go, which is what keeps the file honest — and incidentally what keeps it
// writable at all, since the root's sphere of influence is +Inf and JSON has no way to
// spell that. On the way back in, EnsureSystem derives them again.

// storeVersion is the format of what is written. Version 1 kept one atmosphere in the
// configuration, for the launch body; version 2 keeps one per body, where it belongs.
// A file from a later version says so instead of being silently misread.
const storeVersion = 2

// storeKey names the slot. It is the localStorage key in a browser and part of the
// path natively, so it is one string in one place.
const storeKey = "fsim.setup"

// stored is the file's shape. The configuration is nested rather than inlined so that
// the version is unambiguous even if Config ever grows a field called "version".
type stored struct {
	Version int        `json:"version"`
	Config  sim.Config `json:"config"`
}

// encodeConfig writes a configuration out. Indented, because a saved setup is a text
// file the user owns and may reasonably want to read, diff or hand-edit.
func encodeConfig(cfg sim.Config) ([]byte, error) {
	data, err := json.MarshalIndent(stored{Version: storeVersion, Config: cfg}, "", "\t")
	if err != nil {
		// The one way this happens is a NaN or an infinity in a field, which JSON
		// cannot spell. Saying so beats writing a truncated file.
		return nil, err
	}
	return append(data, '\n'), nil
}

// decodeConfig reads one back, deriving what was left out and refusing what cannot
// fly. A stored setup is the one input to this program that a previous version of it
// wrote, so it is the one that has to be treated as untrusted.
func decodeConfig(data []byte) (sim.Config, error) {
	var st stored
	if err := json.Unmarshal(data, &st); err != nil {
		return sim.Config{}, err
	}
	if st.Version > storeVersion {
		return sim.Config{}, fmt.Errorf("saved by a newer version (%d)", st.Version)
	}
	cfg := st.Config
	if st.Version < 2 {
		migrateAir(data, &cfg)
	}
	// EnsureSystem is the whole of the repair: it builds a one-body system out of
	// Body when there is no system, normalises the tree, clamps the launch body into
	// range and mirrors it back. A decoded config cannot hold a NaN — JSON has no
	// literal for one — so what is left to check is structure.
	cfg.EnsureSystem()
	if err := validConfig(cfg); err != nil {
		return sim.Config{}, err
	}
	return cfg, nil
}

// migrateAir moves a version 1 file's single atmosphere onto the body it described.
//
// The field it comes from does not exist in Config any more, so it is read out of the
// same bytes a second time through a shape that still has it. Cheap, and it keeps the
// dead field out of the live struct — a compatibility shim in the type everything else
// uses would outlive the compatibility.
func migrateAir(data []byte, cfg *sim.Config) {
	var old struct {
		Config struct {
			Atmo       sim.Atmosphere
			LaunchBody int
		}
	}
	if err := json.Unmarshal(data, &old); err != nil || old.Config.Atmo.IsVacuum() {
		return
	}
	i := old.Config.LaunchBody
	switch {
	case i >= 0 && i < len(cfg.System.Bodies):
		cfg.System.Bodies[i].Atmo = old.Config.Atmo
	default:
		// No system in the file: it was a single planet, and Body is what built it.
		cfg.Body.Atmo = old.Config.Atmo
	}
}

// validConfig is what a configuration has to have to be flown at all. It is not a
// judgement about whether the vehicle reaches orbit — plenty of editable nonsense
// flies badly, and that is the user's business — only about whether the simulation
// can start.
func validConfig(cfg sim.Config) error {
	switch {
	case len(cfg.System.Bodies) == 0:
		return errors.New("no bodies")
	case cfg.Body.Radius <= 0:
		return errors.New("the launch body has no radius")
	case len(cfg.Rocket.Stages) == 0:
		return errors.New("the vehicle has no stages")
	case len(cfg.Nodes) > sim.MaxNodes:
		return fmt.Errorf("%d burns, more than the %d a plan holds", len(cfg.Nodes), sim.MaxNodes)
	}
	return nil
}

// saveConfig puts the configuration in the slot.
func saveConfig(cfg sim.Config) error {
	data, err := encodeConfig(cfg)
	if err != nil {
		return err
	}
	return storeWrite(data)
}

// loadConfig takes it back out. The second return is false when there is simply
// nothing saved, which is not an error and should not be reported as one.
func loadConfig() (sim.Config, bool, error) {
	data, ok, err := storeRead()
	if err != nil || !ok {
		return sim.Config{}, ok, err
	}
	cfg, err := decodeConfig(data)
	return cfg, true, err
}
