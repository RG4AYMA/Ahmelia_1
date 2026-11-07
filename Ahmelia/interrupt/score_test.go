package interrupt

import (
	"testing"
	"time"
)

func testEvent(sev Severity, urgency time.Duration, con Consequence, blast BlastRadius) Event {
	return Event{
		ID:          "evt-1",
		Kind:        "database_down",
		Severity:    sev,
		Urgency:     urgency,
		Consequence: con,
		BlastRadius: blast,
		Privacy:     PrivacyShared,
		Source:      "monitor",
		ObservedAt:  time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC),
		Evidence:    []string{"probe:connection_refused"},
	}
}

func TestScoreIsMonotone(t *testing.T) {
	base := testEvent(SeverityMedium, time.Hour, ConsequenceCompensable, BlastProject)
	baseScore, err := Assess(base)
	if err != nil {
		t.Fatalf("assess base: %v", err)
	}

	severities := []Severity{SeverityInfo, SeverityLow, SeverityMedium, SeverityHigh, SeverityCritical}
	last := -1.0
	for _, sev := range severities {
		e := base
		e.Severity = sev
		s, err := Assess(e)
		if err != nil {
			t.Fatalf("severity %s: %v", sev, err)
		}
		if s.Total < last {
			t.Fatalf("severidad %s bajó el total: %.3f < %.3f", sev, s.Total, last)
		}
		last = s.Total
	}

	urgencies := []time.Duration{24 * time.Hour, 6 * time.Hour, time.Hour, 15 * time.Minute, 5 * time.Minute, 0}
	last = -1.0
	for _, u := range urgencies {
		e := base
		e.Urgency = u
		s, err := Assess(e)
		if err != nil {
			t.Fatalf("urgencia %s: %v", u, err)
		}
		if s.Total < last {
			t.Fatalf("urgencia %s bajó el total: %.3f < %.3f", u, s.Total, last)
		}
		last = s.Total
	}

	consequences := []Consequence{ConsequenceReversible, ConsequenceCompensable, ConsequenceIrreversible}
	last = -1.0
	for _, c := range consequences {
		e := base
		e.Consequence = c
		s, err := Assess(e)
		if err != nil {
			t.Fatalf("consecuencia %s: %v", c, err)
		}
		if s.Total < last {
			t.Fatalf("consecuencia %s bajó el total: %.3f < %.3f", c, s.Total, last)
		}
		last = s.Total
	}

	for _, blast := range []BlastRadius{BlastSelf, BlastProject, BlastOwner, BlastThirdParty} {
		e := base
		e.BlastRadius = blast
		s, err := Assess(e)
		if err != nil {
			t.Fatalf("blast %s: %v", blast, err)
		}
		if s.Total != baseScore.Total {
			t.Fatalf("el coste %s cambió el total: %.3f != %.3f", blast, s.Total, baseScore.Total)
		}
	}
}

func TestCriticalPredicateUsesConsequenceNotLabel(t *testing.T) {
	cases := []struct {
		name     string
		event    Event
		critical bool
		escalate bool
	}{
		{
			name:     "irreversible imminent high interrupts",
			event:    testEvent(SeverityHigh, 10*time.Minute, ConsequenceIrreversible, BlastProject),
			critical: true,
		},
		{
			name:     "critical label reversible does not interrupt",
			event:    testEvent(SeverityCritical, 5*time.Minute, ConsequenceReversible, BlastProject),
			critical: false,
		},
		{
			name: "critical label compensable without evidence does not interrupt",
			event: func() Event {
				e := testEvent(SeverityCritical, 5*time.Minute, ConsequenceCompensable, BlastProject)
				e.Evidence = nil
				return e
			}(),
			critical: false,
		},
		{
			name:     "critical label compensable with evidence interrupts",
			event:    testEvent(SeverityCritical, 5*time.Minute, ConsequenceCompensable, BlastProject),
			critical: true,
		},
		{
			name:     "irreversible low label is escalated not lost",
			event:    testEvent(SeverityLow, 10*time.Minute, ConsequenceIrreversible, BlastThirdParty),
			critical: false,
			escalate: true,
		},
		{
			name:     "medium reversible stays below",
			event:    testEvent(SeverityMedium, 30*time.Minute, ConsequenceReversible, BlastSelf),
			critical: false,
			escalate: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, err := Assess(tc.event)
			if err != nil {
				t.Fatalf("assess: %v", err)
			}
			if s.Critical != tc.critical {
				t.Fatalf("critical = %t, quería %t (%s)", s.Critical, tc.critical, s)
			}
			if s.Escalated != tc.escalate {
				t.Fatalf("escalated = %t, quería %t (%s)", s.Escalated, tc.escalate, s)
			}
		})
	}
}

func TestScoreRejectsInvalidEvent(t *testing.T) {
	e := testEvent(Severity("turbo"), time.Minute, ConsequenceReversible, BlastSelf)
	if _, err := Assess(e); err == nil {
		t.Fatal("una severidad desconocida debía fallar")
	}
}
