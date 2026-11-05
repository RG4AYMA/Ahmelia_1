package initiative

import (
	"strings"
	"testing"
	"time"
)

func testOpportunity() InitiativeOpportunity {
	now := time.Date(2026, 9, 7, 15, 0, 0, 0, time.UTC)
	s := InitiativeSignal{Source: SourceConversationalContext, Reason: ReasonConversationContinue, Relevant: true, Confidence: .9, Priority: PriorityHigh, Urgency: UrgencyLow, ObservedAt: now}
	return InitiativeOpportunity{Exists: true, Source: SourceConversationalContext, Reason: ReasonConversationContinue, Relevance: .9, Priority: PriorityHigh, Urgency: UrgencyLow, UserAvailability: AvailabilityAvailable, Privacy: InformationAllowed, Interruptibility: InterruptSafe, ConversationLoad: LoadNormal, Signal: s, Signals: []InitiativeSignal{s}}
}

func privacyCtx() PrivacyContext {
	return PrivacyContext{Classification: ClassificationPublic, SourceSensitivity: SourceSensitivityLow, Audience: AudienceUserOnly, Channel: "text", Permission: PermissionAllowed, ExplicitDisclosure: true, RequestedDisclosure: true, Now: time.Date(2026, 9, 7, 15, 0, 0, 0, time.UTC)}
}

func TestB963711_11_1PrivacyModel(t *testing.T) {
	p := DefaultDisclosurePolicy()
	a, err := EvaluateDisclosurePolicy(privacyCtx(), p)
	if err != nil || !a.DisclosureAllowed {
		t.Fatalf("privacy model rejected allowed context: %+v %v", a, err)
	}
}
func TestB963711_11_2InformationClassification(t *testing.T) {
	c := privacyCtx()
	c.Classification = ""
	c.WorkProject = true
	if got := ClassifyInformation(c); got != ClassificationConfidential {
		t.Fatalf("got %q", got)
	}
}
func TestB963711_11_3DisclosurePolicy(t *testing.T) {
	c := privacyCtx()
	c.Classification = ClassificationPrivate
	c.ExplicitDisclosure = false
	a, _ := EvaluateDisclosurePolicy(c, DefaultDisclosurePolicy())
	if a.DisclosureAllowed {
		t.Fatal("private disclosure escaped policy")
	}
}
func TestB963711_11_4SourceSensitivity(t *testing.T) {
	c := privacyCtx()
	c.SourceSensitivity = SourceSensitivityUnknown
	c.UserModel = true
	if got := SourceSensitivityFor(c); got != SourceSensitivityHigh {
		t.Fatalf("got %q", got)
	}
}
func TestB963711_11_5UserPrivateInformation(t *testing.T) {
	c := privacyCtx()
	c.UserPrivate = true
	c.Classification = ClassificationPrivate
	c.ExplicitDisclosure = false
	a := EvaluatePrivacy(c, DefaultDisclosurePolicy())
	if a.DisclosureAllowed {
		t.Fatal("user-private information disclosed")
	}
}
func TestB963711_11_6HistoricalMemoryBoundaries(t *testing.T) {
	c := privacyCtx()
	c.HistoricalMemory = true
	p := DefaultDisclosurePolicy()
	p.AllowHistoricalMemoryDisclosure = false
	a := EvaluatePrivacy(c, p)
	if a.DisclosureAllowed {
		t.Fatal("historical memory crossed disabled boundary")
	}
}
func TestB963711_11_7UserModelBoundaries(t *testing.T) {
	c := privacyCtx()
	c.UserModel = true
	p := DefaultDisclosurePolicy()
	p.AllowUserModelDisclosure = false
	a := EvaluatePrivacy(c, p)
	if a.DisclosureAllowed {
		t.Fatal("user model crossed disabled boundary")
	}
}
func TestB963711_11_8WorkProjectConfidentiality(t *testing.T) {
	c := privacyCtx()
	c.Classification = ""
	c.WorkProject = true
	c.Classification = ClassificationConfidential
	p := DefaultDisclosurePolicy()
	p.AllowWorkProjectDisclosure = false
	a := EvaluatePrivacy(c, p)
	if a.DisclosureAllowed {
		t.Fatal("work/project confidentiality escaped")
	}
}
func TestB963711_11_9ChannelBoundaries(t *testing.T) {
	p := DefaultDisclosurePolicy()
	if ChannelDisclosureAllowed("external", AudienceUserOnly, p) {
		t.Fatal("external channel should be blocked by default")
	}
}
func TestB963711_11_10AudienceEnvironment(t *testing.T) {
	c := privacyCtx()
	c.Audience = AudienceSharedWorkspace
	a := EvaluatePrivacy(c, DefaultDisclosurePolicy())
	if a.AudienceAllowed {
		t.Fatal("shared audience allowed by default")
	}
}
func TestB963711_11_11PermissionEvaluation(t *testing.T) {
	c := privacyCtx()
	c.Permission = PermissionDenied
	if got := EvaluatePermission(c, DefaultDisclosurePolicy()); got != PermissionDenied {
		t.Fatalf("got %q", got)
	}
}
func TestB963711_11_12DisclosureConstraints(t *testing.T) {
	c := privacyCtx()
	c.Classification = ClassificationSensitive
	c.ExplicitDisclosure = false
	a := EvaluatePrivacy(c, DefaultDisclosurePolicy())
	if a.DisclosureAllowed || a.Reason == "" {
		t.Fatalf("missing disclosure constraint: %+v", a)
	}
}
func TestB963711_11_13PrivacyConflictResolution(t *testing.T) {
	c := privacyCtx()
	c.Permission = PermissionDenied
	c.Audience = AudienceExternal
	if got := ResolvePrivacyConflict(c, DefaultDisclosurePolicy()); got != PrivacyConflictPermission {
		t.Fatalf("got %q", got)
	}
}
func TestB963711_11_14OpportunityEnrichment(t *testing.T) {
	c := privacyCtx()
	op := testOpportunity()
	op.Privacy = InformationAllowed
	enriched, a, err := EnrichPrivacyOpportunity(op, c, DefaultDisclosurePolicy(), c.Now)
	if err != nil || !a.Enriched || enriched.Sensitivity == "" {
		t.Fatalf("bad enrichment: %+v %+v %v", enriched, a, err)
	}
}
func TestB963711_11_15EngineIntegrationContract(t *testing.T) {
	c := privacyCtx()
	op := testOpportunity()
	op.Privacy = InformationPrivate
	c.Classification = ClassificationPrivate
	c.ExplicitDisclosure = false
	enriched, _, err := EnrichPrivacyOpportunity(op, c, DefaultDisclosurePolicy(), c.Now)
	if err != nil {
		t.Fatal(err)
	}
	if enriched.Privacy != InformationPrivate {
		t.Fatalf("privacy boundary lost: %q", enriched.Privacy)
	}
}
func TestB963711_11_16LegacyPrivacyMigration(t *testing.T) {
	cases := []InformationPolicy{InformationAllowed, InformationRestricted, InformationPrivate, InformationSensitive, InformationProhibited}
	for _, p := range cases {
		got, migrated := LegacyPrivacyMigration(p)
		if got != p || migrated {
			t.Fatalf("legacy migration changed canonical %q", p)
		}
	}
	if got, migrated := LegacyPrivacyMigration(InformationPolicy("legacy")); got != InformationRestricted || !migrated {
		t.Fatalf("legacy unknown not normalized: %q %v", got, migrated)
	}
}
func TestB963711_11_17UnitContractSurface(t *testing.T) {
	a := PrivacyAssessment{Level: PrivacyPublic, Classification: ClassificationPublic, SourceSensitivity: SourceSensitivityLow, Permission: PermissionAllowed, DisclosureAllowed: true, ChannelAllowed: true, AudienceAllowed: true, EvaluatedAt: time.Now()}
	if err := a.Validate(); err != nil {
		t.Fatal(err)
	}
}
func TestB963711_11_18E2EInitiativeBoundary(t *testing.T) {
	c := privacyCtx()
	c.Classification = ClassificationPrivate
	c.ExplicitDisclosure = false
	op := testOpportunity()
	op.Privacy = InformationPrivate
	op.Signals = []InitiativeSignal{op.Signal}
	enriched, _, err := EnrichPrivacyOpportunity(op, c, DefaultDisclosurePolicy(), c.Now)
	if err != nil {
		t.Fatal(err)
	}
	d := DefaultPolicy().Evaluate(enriched, c.Now)
	if d.Decision != Defer && d.Decision != Wait {
		t.Fatalf("privacy boundary should not permit intervention: %+v", d)
	}
}
func TestB963711_11_19ArchitectureAudit(t *testing.T) {
	text := "PrivacyAssessment DisclosurePolicy EvaluatePrivacy EnrichPrivacyOpportunity"
	for _, forbidden := range []string{"InitiativeDecision", "INTERVENE", "WAIT", "DEFER"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("privacy implementation surface contains decision authority marker %q", forbidden)
		}
	}
}

func TestB963711LegacyClassificationRoundTrip(t *testing.T) {
	for _, p := range []InformationPolicy{InformationAllowed, InformationRestricted, InformationPrivate, InformationSensitive, InformationProhibited} {
		if LegacyInformationPolicy(InformationClassificationFromLegacy(p)) != p {
			t.Fatalf("round trip failed for %q", p)
		}
	}
}
