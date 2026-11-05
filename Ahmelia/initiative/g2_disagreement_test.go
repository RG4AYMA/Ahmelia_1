package initiative

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"ahmelia/situation"
)

func TestG2DisagreementRequiresAuthoritativeFindingAndCarriesEvidence(t *testing.T) {
	now := time.Now()
	finding := situation.DecisionFinding{Kind: "contradicts_authoritative_evidence", Target: "color favorito", Confidence: .93, EvidenceIDs: []string{"semantic_turn:aa", "user-model-pref-1"}}
	in := SignalInput{Now: now, DecisionEvidence: situation.DecisionEvidence{Findings: []situation.DecisionFinding{finding}}, RequestKind: "question", Privacy: InformationAllowed, UserAvailability: AvailabilityAvailable, Interruptibility: InterruptSafe}
	signals := authoritativeDisagreementAdapter().CollectSnapshot(RuntimeSnapshot{Input: in, Now: now})
	if len(signals) != 1 || signals[0].Reason != ReasonContradictsAuthoritativeEvidence {
		t.Fatalf("signals=%+v", signals)
	}
	if len(signals[0].EvidenceIDs) != 2 {
		t.Fatalf("evidence ids=%v", signals[0].EvidenceIDs)
	}
	op := BuildOpportunity(signals, in)
	decision := DefaultPolicy().Evaluate(op, now)
	if decision.Decision != Intervene {
		t.Fatalf("decision=%s want INTERVENE", decision.Decision)
	}
	final, err := NewActionSelector().FinalizeDecision(decision, now)
	if err != nil {
		t.Fatal(err)
	}
	if final.Action != ActionDisagree {
		t.Fatalf("action=%s", final.Action)
	}
	if final.Provenance.OriginID != "semantic_turn:aa,user-model-pref-1" {
		t.Fatalf("decision provenance lost evidence ids: %q", final.Provenance.OriginID)
	}
}

func TestG2NoFindingNeverProducesDisagreement(t *testing.T) {
	now := time.Now()
	in := SignalInput{Now: now, DecisionEvidence: situation.DecisionEvidence{}, RequestKind: "question"}
	if signals := authoritativeDisagreementAdapter().CollectSnapshot(RuntimeSnapshot{Input: in, Now: now}); len(signals) != 0 {
		t.Fatalf("unexpected disagreement signals=%+v", signals)
	}
}

func TestG2AlreadyAuthorizedOrderBlocksDisagreement(t *testing.T) {
	now := time.Now()
	signal := InitiativeSignal{Source: SourceCognitionConflict, Reason: ReasonContradictsAuthoritativeEvidence, Relevant: true, Confidence: .94, EvidenceIDs: []string{"a", "b"}, ObservedAt: now, Priority: PriorityMedium, Urgency: UrgencyMedium}
	op := BuildOpportunity([]InitiativeSignal{signal}, SignalInput{Now: now, RequestKind: "production", Privacy: InformationAllowed, UserAvailability: AvailabilityAvailable, Interruptibility: InterruptSafe})
	decision := DefaultPolicy().Evaluate(op, now)
	if decision.Decision != Wait || decision.Timing != TimingWait {
		t.Fatalf("decision=%s timing=%s constraints=%+v", decision.Decision, decision.Timing, decision.Constraints)
	}
	var blocked bool
	for _, c := range decision.Constraints {
		if c.Code == "disagreement_blocked_authorized_order" && c.Active {
			blocked = true
		}
	}
	if !blocked {
		t.Fatal("authorized order did not block disagreement")
	}
}

func TestG2DisagreementContractReusesDirectionAndNoFinalText(t *testing.T) {
	now := time.Now()
	d := InitiativeDecision{
		ID: "d-g2", Decision: Intervene,
		Opportunity: InitiativeOpportunity{Source: SourceCognitionConflict, Reason: ReasonContradictsAuthoritativeEvidence, Target: "color favorito", Signal: InitiativeSignal{Source: SourceCognitionConflict, Reason: ReasonContradictsAuthoritativeEvidence, Relevant: true, Confidence: .93, EvidenceIDs: []string{"a", "b"}}},
		Source:      SourceCognitionConflict, Reason: ReasonContradictsAuthoritativeEvidence, Target: "color favorito", Timing: TimingNow, Priority: PriorityMedium, Urgency: UrgencyMedium, Confidence: .93, CreatedAt: now, PolicyVersion: "B96.37.2",
	}
	d.Action = NewActionSelector().Select(d.Opportunity, Intervene)
	contract, err := NewActionContractBuilder().Build(d)
	if err != nil {
		t.Fatal(err)
	}
	if contract.Spec.Category != ActionCategoryDirection || contract.Spec.ContentMode != ContentDisagreement {
		t.Fatalf("spec=%+v", contract.Spec)
	}
	proposal, err := BuildInterventionProposal(contract, d.Opportunity, d.Target)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(proposal.Context), "user-model-pref") {
		t.Fatalf("proposal leaked evidence payload into final content context: %q", proposal.Context)
	}
	// Structural invariant: the proposal carries semantic coordinates only;
	// final language belongs to Dialogue/Language and is not a field here.
	typ := reflect.TypeOf(InterventionProposal{})
	for _, forbidden := range []string{"Text", "FinalText", "Question", "Sentence"} {
		if _, ok := typ.FieldByName(forbidden); ok {
			t.Fatalf("proposal gained a final-language field %q", forbidden)
		}
	}
}
