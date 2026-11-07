package core

import (
	"testing"
	"time"

	"ahmelia/variation"
)

// TestOwnerRestrictionSurfaceVariation exercises the same owner decision over
// generated surface forms and times. The parser normalizes whitespace and
// punctuation; the behavior must not depend on the exact sentence of the
// specification.
func TestOwnerRestrictionSurfaceVariation(t *testing.T) {
	phrases := []string{
		"hoy no trabajamos",
		"hoy no trabajo",
		"no trabajamos hoy",
		"no trabajo hoy",
		"hoy descansamos",
		"hoy descanso",
		"descansamos hoy",
		"descanso hoy",
	}
	paddings := []string{"", " ", "\t"}
	punctuations := []string{"", ".", "!"}

	g := variation.NewGenerator(11)
	for name, values := range map[string][]string{
		"phrase":      phrases,
		"padding":     paddings,
		"punctuation": punctuations,
	} {
		g.AddDimension(name, values)
	}
	g.AddTemplate("{padding}{phrase}{punctuation}", map[string]variation.Dimension{
		"padding":     {Name: "padding", Values: paddings},
		"phrase":      {Name: "phrase", Values: phrases},
		"punctuation": {Name: "punctuation", Values: punctuations},
	})

	situations := g.GenerateN(50)
	if len(situations) < 50 {
		t.Fatalf("situaciones generadas = %d, quería 50", len(situations))
	}

	for i, situation := range situations {
		e := &Engine{}
		e.enableOwnerRestrictionsAt("")
		e.onResponse = func(string) {}
		now := time.Date(2026, 11, 5, 8, 0, 0, 0, time.Local)
		e.sceneNow = func() time.Time { return now }

		turn := &processTurn{input: situation.Text}
		if !e.ownerRestrictionPhase(turn) {
			t.Fatalf("situación %d no consumida: %q (valores=%v)", i+1, situation.Text, situation.Values)
		}
		if !e.OwnerWorkInitiativeRestricted(now) {
			t.Fatalf("situación %d no cerró la iniciativa: %q", i+1, situation.Text)
		}

		now = time.Date(2026, 11, 6, 0, 0, 0, 0, time.Local)
		if err := e.SweepOwnerRestrictions(now); err != nil {
			t.Fatalf("situación %d: sweep: %v", i+1, err)
		}
		if e.OwnerWorkInitiativeRestricted(now) {
			t.Fatalf("situación %d no reabrió al día siguiente: %q", i+1, situation.Text)
		}
	}
}
