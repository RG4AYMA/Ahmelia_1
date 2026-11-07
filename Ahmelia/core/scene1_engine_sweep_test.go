package core

import (
	"path/filepath"
	"testing"
	"time"

	"ahmelia/agency"
	"ahmelia/variation"
)

func sweepMode(kind string) agency.Mode {
	switch kind {
	case "focus":
		return agency.ModeFocus
	case "rest":
		return agency.ModeRest
	default:
		return agency.ModeCompanion
	}
}

func TestScene1EngineBehavioralSweep(t *testing.T) {
	loc := time.FixedZone("COT", -5*3600)
	start := time.Date(2026, 11, 5, 8, 0, 0, 0, loc)
	dims := map[string]variation.Dimension{
		"mode":        {Name: "mode", Values: []string{"companion", "focus", "rest"}},
		"restriction": {Name: "restriction", Values: []string{"active", "lifted"}},
		"pain":        {Name: "pain", Values: []string{"none", "headache"}},
		"receptivity": {Name: "receptivity", Values: []string{"low", "neutral", "high"}},
	}
	g := variation.NewGenerator(53)
	for name, dim := range dims {
		g.AddDimension(name, dim.Values)
	}
	g.AddTemplate("{mode}|{restriction}|{pain}|{receptivity}", dims)
	situations := g.GenerateN(24)
	if len(situations) != 24 {
		t.Fatalf("situaciones generadas = %d, quería 24", len(situations))
	}

	failures := 0
	fail := func(i int, s variation.GeneratedSituation, format string, args ...interface{}) {
		failures++
		t.Errorf("situación %d %v: "+format, append([]interface{}{i + 1, s.Values}, args...)...)
	}

	for i, s := range situations {
		e := buildTestEngine(t)
		now := start
		e.sceneNow = func() time.Time { return now }
		dir := t.TempDir()
		e.enablePersistentModeAt(filepath.Join(dir, "persistent_mode.json"))
		if _, err := e.SetPersistentMode(sweepMode(s.Values["mode"]), "sweep"); err != nil {
			fail(i, s, "modo: %v", err)
			continue
		}
		e.enableOwnerRestrictionsAt(filepath.Join(dir, "owner_restrictions.json"))
		e.enableOwnerStateAt(filepath.Join(dir, "owner_state.json"))
		e.SetResponseCallback(func(string) {})

		if s.Values["restriction"] == "active" {
			e.Process("hoy no trabajamos")
		}
		if s.Values["pain"] == "headache" {
			e.Process("me duele la cabeza")
		}
		if err := e.SetOwnerReceptivity(s.Values["receptivity"], 24*time.Hour, []string{"sweep"}); err != nil {
			fail(i, s, "receptividad: %v", err)
		}

		modeAllows := s.Values["mode"] != "rest"
		restrictionOpen := s.Values["restriction"] != "active"
		wantAt := func(painActive bool) bool {
			return modeAllows && restrictionOpen && !painActive
		}
		check := func(at time.Time, want bool, label string) {
			if got := e.ownerAllowsInitiative(at); got != want {
				fail(i, s, "%s: iniciativa=%t, quería %t", label, got, want)
			}
			quota := e.agentGoalPlannerQuota()
			if want && quota != 1 {
				fail(i, s, "%s: cupo=%d, quería 1", label, quota)
			}
			if !want && quota != 0 {
				fail(i, s, "%s: cupo=%d, quería 0", label, quota)
			}
			if !want {
				if imps := e.buildImpulses(&processTurn{interactionHasWork: true}); len(imps) != 0 {
					fail(i, s, "%s: impulsos=%+v, quería ninguno", label, imps)
				}
			}
		}

		painAtStart := s.Values["pain"] == "headache"
		check(now, wantAt(painAtStart), "08:00")

		for _, hour := range []int{9, 12, 18, 23} {
			now = time.Date(2026, 11, 5, hour, 0, 0, 0, loc)
			e.Process("hola")
			painActive := painAtStart && now.Before(start.Add(2*time.Hour))
			check(now, wantAt(painActive), time.Date(2026, 11, 5, hour, 0, 0, 0, loc).Format("15:04"))
		}
		if got := e.OwnerReceptivity(now).Level; got != s.Values["receptivity"] {
			fail(i, s, "receptividad = %s", got)
		}
		expectedQuestions := 0
		if s.Values["restriction"] == "active" {
			expectedQuestions = 1
		}
		if got := e.OwnerQuestionsAsked(); got != expectedQuestions {
			fail(i, s, "preguntas = %d, quería %d", got, expectedQuestions)
		}

		next := time.Date(2026, 11, 6, 0, 0, 0, 0, loc)
		if err := e.SweepOwnerRestrictions(next); err != nil {
			fail(i, s, "sweep restricción: %v", err)
		}
		if err := e.SweepOwnerState(next); err != nil {
			fail(i, s, "sweep estado: %v", err)
		}
		check(next, modeAllows, "día siguiente")
	}

	t.Logf("scene1 engine sweep: %d situaciones, %d fallos", len(situations), failures)
	if failures > 0 {
		t.Fatalf("el barrido de motor encontró %d fallos", failures)
	}
}
