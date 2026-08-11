package main

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pavel-krush/fsim/sim"
)

// noSavedSetup points the store at an empty directory for the duration of a test, so
// that what is on the machine running it cannot change the answer. Anything that counts
// the rows of the mission list needs it: with a setup saved there is one more row, and
// a test that passes on a fresh checkout and fails on the author's laptop is worse than
// no test.
func noSavedSetup(t *testing.T) {
	t.Helper()
	old := storeRoot
	storeRoot = t.TempDir()
	t.Cleanup(func() { storeRoot = old })
}

// Every preset has to survive the round trip, and "survive" means the flight is the
// same flight — not that the JSON looks similar. So each one is flown before and after
// and the final states are compared to the last bit. A configuration that reads back
// almost right is the worst possible outcome here: nothing complains and the numbers
// quietly differ.
func TestEveryPresetSurvivesBeingSaved(t *testing.T) {
	for _, p := range sim.Presets() {
		t.Run(p.Name, func(t *testing.T) {
			data, err := encodeConfig(p.Cfg)
			if err != nil {
				t.Fatalf("encode: %v", err)
			}
			back, err := decodeConfig(data)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}

			// A short flight rather than the whole mission: the ascent exercises the
			// air, the stages and the pitch programme, which is where a lost field
			// would show, and eight days of Apollo per preset is not worth the wait.
			want := flyFor(p.Cfg, 400)
			got := flyFor(back, 400)
			if want != got {
				t.Errorf("the flight differs after a round trip\n before %+v\n  after %+v", want, got)
			}
		})
	}
}

// flyFor advances a configuration to time t and returns what it is worth comparing.
func flyFor(cfg sim.Config, t float64) [7]float64 {
	s := sim.New(cfg)
	s.FastForward(t)
	return [7]float64{
		s.St.Pos.X, s.St.Pos.Y, s.St.Vel.X, s.St.Vel.Y,
		s.Mass(), s.St.DeltaV, float64(s.St.Stage),
	}
}

// The root's sphere of influence is +Inf, and json.Marshal refuses an infinity — so a
// normalized system could not be written at all until Mu and SOI were left out of it.
// This is that fact as a test, because the failure is a whole feature not working and
// the cause is one struct tag two packages away.
func TestASystemOnRailsCanBeWrittenAtAll(t *testing.T) {
	cfg := presetNamed(t, "apollo-lunar").Cfg
	cfg.EnsureSystem() // fills SOI with +Inf at the root, which is the trap
	if !math.IsInf(cfg.System.Bodies[0].SOI, 1) {
		t.Fatalf("the root's SOI is %v, so this test is no longer testing anything",
			cfg.System.Bodies[0].SOI)
	}
	if _, err := encodeConfig(cfg); err != nil {
		t.Fatalf("encode: %v", err)
	}
}

// What is derived must be derived again on the way in, whatever the file says. The
// file is the one input this program gets from a previous version of itself, so it is
// the one that has to be treated as capable of anything.
func TestDerivedValuesAreRebuiltOnLoad(t *testing.T) {
	cfg := presetNamed(t, "earth-falcon").Cfg
	data, err := encodeConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"Mu"`) || strings.Contains(string(data), `"SOI"`) {
		t.Error("the derived values were written to the file")
	}
	back, err := decodeConfig(data)
	if err != nil {
		t.Fatal(err)
	}
	if back.Body.Mu <= 0 {
		t.Errorf("Mu came back as %v", back.Body.Mu)
	}
}

// A file that cannot fly has to be refused, not loaded and then crashed into. The
// stale-index family lives here too: a launch body pointing past the end of the slice
// is exactly the shape of every crash this project has had.
func TestARuinedFileIsRefused(t *testing.T) {
	cases := []struct{ name, json string }{
		{"not json", `{`},
		{"no stages", `{"version":1,"config":{"Body":{"Radius":6371000,"Mass":5.97e24}}}`},
		{"no radius", `{"version":1,"config":{"Rocket":{"Stages":[{"DryMass":1}]}}}`},
		{"from the future", `{"version":99,"config":{}}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := decodeConfig([]byte(c.json)); err == nil {
				t.Error("accepted")
			}
		})
	}
}

// A launch body past the end of the bodies is repaired rather than refused: it is what
// EnsureSystem is for, and a setup saved by a build whose system was bigger is worth
// rescuing rather than throwing away.
func TestAStaleLaunchBodyIsClamped(t *testing.T) {
	cfg := presetNamed(t, "earth-falcon").Cfg
	cfg.LaunchBody = 9
	data, err := encodeConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	back, err := decodeConfig(data)
	if err != nil {
		t.Fatalf("a stale index should be clamped, not refused: %v", err)
	}
	if back.LaunchBody != 0 {
		t.Errorf("the launch body came back as %d", back.LaunchBody)
	}
}

// The slot itself: written where it was asked for, read back, and absent before
// anything has been saved. Pointed at a temporary directory, because a test that
// replaced the setup of whoever ran it would be the very fault this file prevents.
func TestTheSlotHoldsWhatWasPutInIt(t *testing.T) {
	noSavedSetup(t)
	dir := storeRoot

	if _, ok, err := loadConfig(); ok || err != nil {
		t.Fatalf("a fresh install has nothing saved: ok=%v err=%v", ok, err)
	}

	cfg := presetNamed(t, "kerbin-mun").Cfg
	if err := saveConfig(cfg); err != nil {
		t.Fatalf("save: %v", err)
	}
	path, err := storePath()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(filepath.Dir(path)) != dir {
		t.Errorf("saved to %s, outside the directory it was pointed at", path)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("no file at %s: %v", path, err)
	}

	back, ok, err := loadConfig()
	if err != nil || !ok {
		t.Fatalf("load: ok=%v err=%v", ok, err)
	}
	if flyFor(cfg, 400) != flyFor(back, 400) {
		t.Error("the loaded setup flies differently")
	}

	// A second save replaces the first rather than accumulating files: one slot.
	if err := saveConfig(presetNamed(t, "titan-ascent").Cfg); err != nil {
		t.Fatal(err)
	}
	files, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		names := make([]string, len(files))
		for i, f := range files {
			names[i] = f.Name()
		}
		t.Errorf("%d files in the store: %v", len(files), names)
	}
}

// Loading has to reset the body selection, for the reason loading a preset does: the
// editor is holding an index into a slice that has just been replaced. That was a hard
// crash going from the solar system to a single planet, and a saved setup is the same
// jump with a different button on it.
func TestLoadingASavedSetupClearsTheSelection(t *testing.T) {
	noSavedSetup(t)

	if err := saveConfig(presetNamed(t, "earth-falcon").Cfg); err != nil {
		t.Fatal(err)
	}

	a := &App{ui: NewUI(), cfg: presetNamed(t, "apollo-lunar").Cfg}
	s := NewSetupScreen(0)
	s.selBody = 9 // looking at a moon of the solar system
	s.load(a)

	if s.noteBad {
		t.Fatalf("load said: %s", s.note)
	}
	if s.selBody >= len(a.cfg.System.Bodies) {
		t.Errorf("the selection is %d in a system of %d", s.selBody, len(a.cfg.System.Bodies))
	}
}

// The saved setup gets a row in the mission list, because a feature nobody can find is
// not a feature — and the row is last, so that every index above it still means the
// preset it always meant.
func TestTheSavedSetupGetsARowOfItsOwn(t *testing.T) {
	noSavedSetup(t)

	presets := sim.Presets()

	// Nothing saved: the list is exactly the presets.
	if n := NewPresetScreen(0).rows(); n != len(presets) {
		t.Errorf("%d rows with nothing saved, want %d", n, len(presets))
	}

	saved := presetNamed(t, "titan-ascent").Cfg
	if err := saveConfig(saved); err != nil {
		t.Fatal(err)
	}

	s := NewPresetScreen(0)
	if n := s.rows(); n != len(presets)+1 {
		t.Fatalf("%d rows with a setup saved, want %d", n, len(presets)+1)
	}

	// A preset row still selects its own preset: nothing shifted. A click selects and a
	// second click on the same row opens it, which is what these pairs are.
	a := &App{ui: NewUI()}
	s.pick(a, 1)
	s.pick(a, 1)
	if a.cfg.Rocket.Payload != presets[1].Cfg.Rocket.Payload {
		t.Error("row 1 no longer picks the second preset")
	}

	// And the last row opens what was saved.
	a.screen = ScreenPresets
	s.pick(a, len(presets))
	s.pick(a, len(presets))
	if a.screen != ScreenSetup {
		t.Errorf("the saved row left the screen at %v", a.screen)
	}
	if flyFor(a.cfg, 400) != flyFor(saved, 400) {
		t.Error("the saved row flies something else")
	}

	// With its own slices: editing the loaded setup must not reach back into the
	// file's copy, which is what sharing a config would do.
	a.cfg.Rocket.Stages[0].DryMass += 1000
	again, _, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if again.Rocket.Stages[0].DryMass != saved.Rocket.Stages[0].DryMass {
		t.Error("editing the loaded setup changed the stored one")
	}
}

// A file written before the air belonged to the bodies still has to load, with its air
// on the body it described. There is exactly one such file per person who used the
// version that wrote it, and losing their atmosphere silently is worse than refusing.
func TestAVersionOneFileKeepsItsAir(t *testing.T) {
	// A single-planet setup, which is the shape that has no system in it at all.
	single := `{"version":1,"config":{
		"Body":{"Name":"earth","Radius":6371000,"MassSource":0,"Mass":5.97237e24},
		"Atmo":{"Fractions":[0.78,0.21,0,0.01,0,0,0,0],
			"Layers":[{"BaseAlt":0,"Lapse":-0.0065}],
			"SurfaceTemp":288.15,"SurfacePressure":101325,"Top":140000},
		"Rocket":{"Diameter":3,"Cd":0.4,"Stages":[{"DryMass":1000,"PropMass":9000,
			"ThrustVac":200000,"IspVac":300,"IspSL":280,"Throttle":1}]}}}`

	cfg, err := decodeConfig([]byte(single))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if cfg.Body.Atmo.IsVacuum() {
		t.Fatal("the air was dropped on the way in")
	}
	if got := cfg.Body.Atmo.Top; got != 140000 {
		t.Errorf("the ceiling came back as %g", got)
	}
	// And it is the launch body's air, so the simulation finds it.
	s := sim.New(cfg)
	if got := s.AtmoTop(); got != 140000 {
		t.Errorf("the flight launched into a ceiling of %g", got)
	}
	if s.Center().Atmo.State(0).Density <= 1 {
		t.Errorf("surface density came out at %g", s.Center().Atmo.State(0).Density)
	}
}
