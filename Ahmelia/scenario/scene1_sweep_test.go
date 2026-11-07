package scenario

import (
	"testing"
	"time"

	"ahmelia/restriction"
	"ahmelia/variation"
)

func sweepTimeOfDay(kind string, loc *time.Location) time.Time {
	hour := 8
	switch kind {
	case "noon":
		hour = 12
	case "evening":
		hour = 18
	case "night":
		hour = 23
	}
	return time.Date(2026, 11, 5, hour, 0, 0, 0, loc)
}

func TestScene1BehavioralSweep(t *testing.T) {
	loc := time.FixedZone("COT", -5*3600)
	dims := map[string]variation.Dimension{
		"owner_state": {Name: "owner_state", Values: []string{"resting", "busy", "receptive", "absent"}},
		"time":        {Name: "time", Values: []string{"morning", "noon", "evening", "night"}},
		"receptivity": {Name: "receptivity", Values: []string{"low", "neutral", "high"}},
		"restriction": {Name: "restriction", Values: []string{"active", "lifted"}},
		"pain":        {Name: "pain", Values: []string{"none", "headache"}},
	}
	g := variation.NewGenerator(29)
	for name, dim := range dims {
		g.AddDimension(name, dim.Values)
	}
	g.AddTemplate("{owner_state}|{time}|{receptivity}|{restriction}|{pain}", dims)
	situations := g.GenerateN(60)
	if len(situations) < 50 {
		t.Fatalf("situaciones generadas = %d, quería al menos 50", len(situations))
	}

	failures := 0
	fail := func(i int, s variation.GeneratedSituation, format string, args ...interface{}) {
		failures++
		t.Errorf("situación %d %v: "+format, append([]interface{}{i + 1, s.Values}, args...)...)
	}

	for i, s := range situations {
		now := sweepTimeOfDay(s.Values["time"], loc)
		clock := NewClock(now, loc)
		store, err := restriction.Open("")
		if err != nil {
			t.Fatalf("open restrictions: %v", err)
		}
		owner := NewOwnerSim(clock, store, "")
		if s.Values["restriction"] == "active" {
			if _, err := owner.DeclareNoWorkToday("hoy no trabajamos"); err != nil {
				fail(i, s, "declarar restricción: %v", err)
				continue
			}
		}
		if s.Values["pain"] == "headache" {
			owner.ReportPain("headache", 0.8, 2*time.Hour, []string{"sweep"})
		}
		owner.SetReceptivity(s.Values["receptivity"], 2*time.Hour, []string{"sweep"})

		work := DecideWorkInitiative(store, now)
		switch s.Values["restriction"] {
		case "active":
			if work.Kind != DecisionSilence {
				fail(i, s, "trabajo con restricción = %+v", work)
			}
		case "lifted":
			if work.Kind != DecisionWorkInitiative {
				fail(i, s, "trabajo sin restricción = %+v", work)
			}
		}

		expectProposal := s.Values["receptivity"] == "high" && s.Values["pain"] == "none"
		rec := owner.ProposeRecreation(now, 2*time.Hour)
		if expectProposal && rec.Kind != DecisionRecreationProposal {
			fail(i, s, "recreación esperada = %+v", rec)
		}
		if !expectProposal && rec.Kind != DecisionSilence {
			fail(i, s, "recreación no permitida = %+v", rec)
		}
		if expectProposal {
			second := owner.ProposeRecreation(now.Add(10*time.Minute), 2*time.Hour)
			if second.Kind != DecisionSilence {
				fail(i, s, "segunda recreación en ventana = %+v", second)
			}
		}

		if !owner.AskRestQuestion() {
			fail(i, s, "la primera pregunta debía permitirse")
		}
		if owner.AskRestQuestion() {
			fail(i, s, "la segunda pregunta no debía permitirse")
		}

		if s.Values["pain"] == "headache" {
			if _, ok := owner.ActivePain(now.Add(3 * time.Hour)); ok {
				fail(i, s, "el dolor expirado seguía activo")
			}
		}
	}

	t.Logf("scene1 sweep: %d situaciones, %d fallos", len(situations), failures)
	if failures > 0 {
		t.Fatalf("el barrido de escena 1 encontró %d fallos", failures)
	}
}
