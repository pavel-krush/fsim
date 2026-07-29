package main

import (
	"math"
	"testing"

	"fsim/sim"
)

// closeTo is the assertion helper. It is not called close: that is a builtin
// here, and shadowing it in a package that also owns channels is asking for
// trouble.
func closeTo(t *testing.T, name string, got, want, tol float64) {
	t.Helper()
	if math.Abs(got-want) > tol*math.Max(1, math.Abs(want)) {
		t.Errorf("%s = %g, want %g", name, got, want)
	}
}

// gasIdx finds a gas by formula so the tests read as chemistry rather than as
// indices into a slice.
func gasIdx(t *testing.T, name string) int {
	t.Helper()
	for i := range sim.Gases {
		if sim.Gases[i].Name == name {
			return i
		}
	}
	t.Fatalf("no gas called %q", name)
	return -1
}

func mixture(t *testing.T, pairs ...any) *sim.Atmosphere {
	t.Helper()
	at := &sim.Atmosphere{Fractions: make([]float64, len(sim.Gases))}
	for i := 0; i+1 < len(pairs); i += 2 {
		// Accept ints as well: writing 1 rather than 1.0 in a variadic any is
		// far too easy, and the type assertion would panic instead of failing.
		var v float64
		switch n := pairs[i+1].(type) {
		case float64:
			v = n
		case int:
			v = float64(n)
		default:
			t.Fatalf("fraction for %v is %T, want a number", pairs[i], pairs[i+1])
		}
		at.Fractions[gasIdx(t, pairs[i].(string))] = v
	}
	return at
}

func total(at *sim.Atmosphere) float64 {
	var s float64
	for _, v := range at.Fractions {
		s += v
	}
	return s
}

func frac(t *testing.T, at *sim.Atmosphere, name string) float64 {
	t.Helper()
	return at.Fractions[gasIdx(t, name)]
}

// Editing one gas rescales the others to fill what is left, keeping their
// proportions to each other. That is the whole point: no arithmetic by hand.
func TestBalanceKeepsTheProportionsOfTheRest(t *testing.T) {
	at := mixture(t, "N2", 0.78, "O2", 0.21, "Ar", 0.01)
	o2 := gasIdx(t, "O2")

	at.Fractions[o2] = 0.30
	balanceGases(at, o2)

	closeTo(t, "total", total(at), 1, 1e-12)
	closeTo(t, "O2", frac(t, at, "O2"), 0.30, 1e-12)
	// N2 and Ar were 78:1 before and must still be.
	closeTo(t, "N2:Ar ratio", frac(t, at, "N2")/frac(t, at, "Ar"), 78, 1e-9)
	closeTo(t, "N2", frac(t, at, "N2"), 0.70*78/79, 1e-12)
}

// A hundred per cent of one gas means there is nothing else, which is the
// right answer even though it throws the other values away.
func TestBalanceAtFullPurgesTheRest(t *testing.T) {
	at := mixture(t, "N2", 0.78, "O2", 0.21, "Ar", 0.01)
	o2 := gasIdx(t, "O2")

	at.Fractions[o2] = 1
	balanceGases(at, o2)

	closeTo(t, "total", total(at), 1, 1e-12)
	closeTo(t, "O2", frac(t, at, "O2"), 1, 1e-12)
	for i, v := range at.Fractions {
		if i != o2 && v != 0 {
			t.Errorf("%s left at %g in a pure mixture", sim.Gases[i].Name, v)
		}
	}
}

// Coming back down from a pure gas is the case that traps a naive rescale:
// there is nothing to scale against, so the remainder would vanish and the
// mixture would be stuck at a hundred per cent for ever.
func TestBalanceCanComeBackFromAPureGas(t *testing.T) {
	at := mixture(t, "O2", 1)
	o2 := gasIdx(t, "O2")

	at.Fractions[o2] = 0.6
	balanceGases(at, o2)

	closeTo(t, "total", total(at), 1, 1e-12)
	closeTo(t, "O2", frac(t, at, "O2"), 0.6, 1e-12)
	if frac(t, at, "N2") <= 0 {
		t.Error("the remainder went nowhere: the mixture is stuck pure")
	}
}

// Zeroing a gas is the mirror case and has to be just as harmless.
func TestBalanceAtZeroFillsWithTheRest(t *testing.T) {
	at := mixture(t, "N2", 0.78, "O2", 0.21, "Ar", 0.01)
	o2 := gasIdx(t, "O2")

	at.Fractions[o2] = 0
	balanceGases(at, o2)

	closeTo(t, "total", total(at), 1, 1e-12)
	closeTo(t, "O2", frac(t, at, "O2"), 0, 1e-12)
	closeTo(t, "N2:Ar ratio", frac(t, at, "N2")/frac(t, at, "Ar"), 78, 1e-9)
}

// Whatever is thrown at it, the mixture must come out summing to one and with
// no negative parts.
func TestBalanceAlwaysLeavesAValidMixture(t *testing.T) {
	cases := [][]any{
		{"N2", 0.78, "O2", 0.21, "Ar", 0.01},
		{"CO2", 0.95, "N2", 0.05},
		{"H2", 1.0},
		{"N2", 0.5},
	}
	for _, c := range cases {
		for _, v := range []float64{0, 0.001, 0.5, 0.999, 1} {
			for i := range sim.Gases {
				at := mixture(t, c...)
				at.Fractions[i] = v
				balanceGases(at, i)

				if s := total(at); math.Abs(s-1) > 1e-9 {
					t.Fatalf("%s set to %g: total came out %g", sim.Gases[i].Name, v, s)
				}
				for j, f := range at.Fractions {
					if f < 0 {
						t.Fatalf("%s set to %g: %s went negative (%g)",
							sim.Gases[i].Name, v, sim.Gases[j].Name, f)
					}
				}
			}
		}
	}
}

// The picker only reports a composition as loaded when it really is, and does
// not care whether it was written as fractions or as percentages.
func TestSameMixtureIgnoresScale(t *testing.T) {
	earth := sim.Compositions()[0]
	if earth.Name != "earth" {
		t.Fatalf("expected earth first, got %q", earth.Name)
	}

	scaled := make([]float64, len(earth.Fractions))
	for i, v := range earth.Fractions {
		scaled[i] = v * 100
	}
	if !sameMixture(scaled, earth.Fractions) {
		t.Error("the same mixture written as percentages should still match")
	}

	off := append([]float64(nil), earth.Fractions...)
	off[gasIdx(t, "O2")] += 0.05
	if sameMixture(off, earth.Fractions) {
		t.Error("a mixture with five per cent more oxygen is not Earth air")
	}
	if sameMixture(make([]float64, len(sim.Gases)), earth.Fractions) {
		t.Error("an empty mixture matched Earth air")
	}
}
