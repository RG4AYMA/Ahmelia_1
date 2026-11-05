package core

import (
	"strings"
	"testing"
	"time"

	conversationalinitiative "ahmelia/initiative"
)

func TestB963711EngineWiresPrivacyPolicy(t *testing.T) {
	isolateTestWorkspace(t)
	e := NewEngine(nil, nil, nil, nil, nil)
	if e.initiativePrivacyPolicy == nil {
		t.Fatal("B96.37.11 privacy policy is not initialized")
	}
	if e.initiativePriorityUrgencyPolicy == nil || e.conversationalInitiative == nil {
		t.Fatal("B96.37 chain is incomplete")
	}
}

func TestB963711PrivacyRunsAfterPriorityUrgencyBeforeDecision(t *testing.T) {
	isolateTestWorkspace(t)
	e := NewEngine(nil, nil, nil, nil, nil)
	now := time.Date(2026, 9, 7, 15, 0, 0, 0, time.UTC)
	op := conversationalinitiative.InitiativeOpportunity{Exists: true, Source: conversationalinitiative.SourceConversationalContext, Reason: conversationalinitiative.ReasonWarning, Relevance: .95, Priority: conversationalinitiative.PriorityHigh, Urgency: conversationalinitiative.UrgencyCritical, UserAvailability: conversationalinitiative.AvailabilityAvailable, Privacy: conversationalinitiative.InformationPrivate, Interruptibility: conversationalinitiative.InterruptSafe, ConversationLoad: conversationalinitiative.LoadNormal}
	op.Signal = conversationalinitiative.InitiativeSignal{Relevant: true, Source: op.Source, Reason: op.Reason, Confidence: .95, Priority: op.Priority, Urgency: op.Urgency, ObservedAt: now}
	op.Signals = []conversationalinitiative.InitiativeSignal{op.Signal}
	pu, _, err := e.initiativePriorityUrgencyPolicy.Enrich(op, conversationalinitiative.SignalInput{Now: now}, now)
	if err != nil {
		t.Fatal(err)
	}
	ctx := conversationalinitiative.PrivacyContext{Classification: conversationalinitiative.ClassificationPrivate, Audience: conversationalinitiative.AudienceUserOnly, Channel: "text", Permission: conversationalinitiative.PermissionAllowed, ExplicitDisclosure: false, RequestedDisclosure: true, Now: now}
	enriched, assessment, err := conversationalinitiative.EnrichPrivacyOpportunity(pu, ctx, *e.initiativePrivacyPolicy, now)
	if err != nil {
		t.Fatal(err)
	}
	if assessment.DisclosureAllowed {
		t.Fatal("private information crossed disclosure boundary")
	}
	d := e.conversationalInitiative.Evaluate(enriched, now)
	if d.Decision == conversationalinitiative.Intervene {
		t.Fatalf("privacy boundary must prevent intervention: %+v", d)
	}
	if d.Priority != conversationalinitiative.PriorityHigh || d.Urgency != conversationalinitiative.UrgencyCritical {
		t.Fatalf("privacy changed priority/urgency unexpectedly: %+v", d)
	}
}

func TestB963711ArchitectureAudit(t *testing.T) {
	source := "initiativePrivacyPolicy EnrichPrivacyOpportunity EvaluateDisclosurePolicy"
	for _, forbidden := range []string{"initiativePrivacyPolicy.Evaluate", "initiativePrivacyPolicy.Decide", "InitiativeDecision"} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("privacy layer contains decision authority marker %q", forbidden)
		}
	}
}
