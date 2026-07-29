package main

import "testing"

// The mission clock has to carry, which it did not: splitting the minutes off
// before rounding the seconds turned 11999.98 s into "T+199:60.0". It only
// showed up after the flight was allowed to keep orbiting for hours.
func TestClockCarries(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{0, "T+00:00.0"},
		{9.94, "T+00:09.9"},
		{59.94, "T+00:59.9"},
		{59.98, "T+01:00.0"},
		{60, "T+01:00.0"},
		{480.5, "T+08:00.5"},
		{3599.97, "T+60:00.0"},
		{11999.98, "T+200:00.0"},
	}
	for _, c := range cases {
		if got := fmtClock(c.in); got != c.want {
			t.Errorf("fmtClock(%g) = %q, want %q", c.in, got, c.want)
		}
	}
}

// Negative time cannot happen, but the readout must not produce nonsense if it
// ever does.
func TestClockIgnoresNegativeTime(t *testing.T) {
	if got := fmtClock(-5); got != "T+00:00.0" {
		t.Errorf("fmtClock(-5) = %q, want the clock pinned at zero", got)
	}
}
