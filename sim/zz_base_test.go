package sim

import ("fmt"; "os"; "testing")

func TestZZBaseline(t *testing.T) {
	f, err := os.Create(os.Getenv("BASE_OUT"))
	if err != nil { t.Fatal(err) }
	defer f.Close()
	for _, p := range Presets() {
		s := New(p.Cfg)
		s.RunToEnd()
		fmt.Fprintf(f, "%s T=%.17g pos=%.17g,%.17g vel=%.17g,%.17g m=%.17g dv=%.17g grav=%.17g drag=%.17g steer=%.17g stage=%d out=%d body=%d center=%d ev=%d\n",
			p.Name, s.St.T, s.St.Pos.X, s.St.Pos.Y, s.St.Vel.X, s.St.Vel.Y, s.Mass(),
			s.St.DeltaV, s.St.GravLoss, s.St.DragLoss, s.St.SteerLoss,
			s.St.Stage, int(s.St.Outcome), s.St.OutcomeBody, s.St.Center, len(s.Events))
		// and a long tail for the ones that go somewhere
		s2 := New(p.Cfg)
		s2.FastForward(min(p.Cfg.MaxTime, 4*86400))
		fmt.Fprintf(f, "%s.tail T=%.17g pos=%.17g,%.17g vel=%.17g,%.17g center=%d\n",
			p.Name, s2.St.T, s2.St.Pos.X, s2.St.Pos.Y, s2.St.Vel.X, s2.St.Vel.Y, s2.St.Center)
	}
}
