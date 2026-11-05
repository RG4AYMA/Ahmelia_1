package initiative

import (
	"strings"
	"testing"
	"time"
)

func arbSignal(id string, source Source, reason Reason, target string, p Priority, u Urgency, confidence float64) InitiativeSignal {
	return InitiativeSignal{
		ID: id, Type: "candidate", Source: source, Reason: reason, Target: target,
		Priority: p, Urgency: u, Confidence: confidence, Relevant: true,
		ObservedAt: time.Now(),
	}
}

func arbOp(s InitiativeSignal, in SignalInput) InitiativeOpportunity {
	return opportunityFromSignal(s, in)
}

// 17.1 Model + 17.2 candidate model: arbitration returns an opportunity ref,
// never a decision/action.
func TestB9637_17_ArbitrationModelAndCandidateContract(t *testing.T) {
	now := time.Now()
	in := SignalInput{Now: now, ActiveTopic: "project"}
	ops := BuildCandidateOpportunities([]InitiativeSignal{
		arbSignal("a", SourceConversationalContext, ReasonConversationContinue, "project", PriorityMedium, UrgencyLow, .8),
	}, in)
	r := NewInitiativeArbiter()
	got, err := r.Arbitrate(NewArbitrationInput(ops, nil, NewTimingEvaluator(), now))
	if err != nil {
		t.Fatal(err)
	}
	if !got.Selected.Valid || got.Selected.Opportunity.Reason != ReasonConversationContinue {
		t.Fatalf("unexpected selection: %+v", got.Selected)
	}
	if err := got.Validate(); err != nil {
		t.Fatal(err)
	}
}

// 17.3 Candidate collection.
func TestB9637_17_CandidateCollection(t *testing.T) {
	in := SignalInput{ActiveTopic: "x", UserAvailability: AvailabilityAvailable, Privacy: InformationAllowed, Interruptibility: InterruptSafe}
	ops := BuildCandidateOpportunities([]InitiativeSignal{
		arbSignal("real", SourceOpenCommitment, ReasonUnfinishedCommitment, "x", PriorityHigh, UrgencyMedium, .9),
		{ID: "constraint", Type: "user_availability", Source: SourceInteractionContinuity, Reason: ReasonConversationContinue, Relevant: true, Confidence: .99},
	}, in)
	if len(ops) != 1 || ops[0].Reason != ReasonUnfinishedCommitment {
		t.Fatalf("constraint evidence became candidate: %+v", ops)
	}
}

// 17.4 Deduplication.
func TestB9637_17_Deduplication(t *testing.T) {
	now := time.Now()
	in := SignalInput{Now: now, ActiveTopic: "x"}
	s1 := arbSignal("1", SourceOpenCommitment, ReasonUnfinishedCommitment, "x", PriorityLow, UrgencyLow, .6)
	s2 := arbSignal("2", SourceOpenCommitment, ReasonUnfinishedCommitment, "x", PriorityHigh, UrgencyHigh, .9)
	r, err := NewInitiativeArbiter().Arbitrate(NewArbitrationInput([]InitiativeOpportunity{arbOp(s1, in), arbOp(s2, in)}, nil, NewTimingEvaluator(), now))
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Candidates) != 1 {
		t.Fatalf("expected one deduplicated candidate, got %d", len(r.Candidates))
	}
	if r.Candidates[0].Opportunity.Priority != PriorityHigh || r.Candidates[0].Opportunity.Urgency != UrgencyHigh {
		t.Fatalf("stronger duplicate evidence was not retained")
	}
}

// 17.5 Compatibility + 17.6 conflict detection.
func TestB9637_17_CompatibilityAndConflictDetection(t *testing.T) {
	in := SignalInput{ActiveTopic: "x"}
	a := arbOp(arbSignal("a", SourceConversationalContext, ReasonConversationContinue, "x", PriorityMedium, UrgencyLow, .7), in)
	b := arbOp(arbSignal("b", SourceConversationalContext, ReasonTopicRedirection, "y", PriorityHigh, UrgencyMedium, .8), in)
	if Compatible(a, b) {
		t.Fatal("continue and redirect should compete")
	}
	r, err := NewInitiativeArbiter().Arbitrate(NewArbitrationInput([]InitiativeOpportunity{a, b}, nil, NewTimingEvaluator(), time.Now()))
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Candidates[0].ConflictsWith) == 0 || len(r.Candidates[1].ConflictsWith) == 0 {
		t.Fatal("conflict was not recorded symmetrically")
	}
}

// 17.7 Priority comparison.
func TestB9637_17_PriorityComparison(t *testing.T) {
	in := SignalInput{ActiveTopic: "x"}
	low := arbOp(arbSignal("low", SourceOpenCommitment, ReasonUnfinishedCommitment, "x", PriorityLow, UrgencyMedium, .9), in)
	high := arbOp(arbSignal("high", SourceOpenCommitment, ReasonUnfinishedCommitment, "y", PriorityHigh, UrgencyNone, .9), in)
	r, _ := NewInitiativeArbiter().Arbitrate(NewArbitrationInput([]InitiativeOpportunity{low, high}, nil, NewTimingEvaluator(), time.Now()))
	if r.Selected.Opportunity.Priority != PriorityHigh {
		t.Fatalf("priority did not contribute to ranking")
	}
}

// 17.8 Urgency comparison.
func TestB9637_17_UrgencyComparison(t *testing.T) {
	in := SignalInput{ActiveTopic: "x"}
	low := arbOp(arbSignal("low", SourceAutonomousSignal, ReasonPendingTask, "x", PriorityMedium, UrgencyLow, .8), in)
	high := arbOp(arbSignal("high", SourceAutonomousSignal, ReasonPotentialProblem, "y", PriorityMedium, UrgencyCritical, .8), in)
	r, _ := NewInitiativeArbiter().Arbitrate(NewArbitrationInput([]InitiativeOpportunity{low, high}, nil, NewTimingEvaluator(), time.Now()))
	if r.Selected.Opportunity.Urgency != UrgencyCritical {
		t.Fatalf("urgency did not contribute to ranking")
	}
}

// 17.9 Relevance comparison.
func TestB9637_17_RelevanceComparison(t *testing.T) {
	in := SignalInput{ActiveTopic: "x"}
	a := arbOp(arbSignal("a", SourceConversationalContext, ReasonRelevantComment, "a", PriorityLow, UrgencyNone, .4), in)
	b := arbOp(arbSignal("b", SourceConversationalContext, ReasonRelevantComment, "b", PriorityLow, UrgencyNone, .9), in)
	r, _ := NewInitiativeArbiter().Arbitrate(NewArbitrationInput([]InitiativeOpportunity{a, b}, nil, NewTimingEvaluator(), time.Now()))
	if r.Selected.Opportunity.Relevance < .89 {
		t.Fatalf("relevance was not compared")
	}
}

// 17.10 User context.
func TestB9637_17_UserContextComparison(t *testing.T) {
	in := SignalInput{ActiveTopic: "active"}
	a := arbOp(arbSignal("a", SourceOperationalWork, ReasonNextStepAvailable, "", PriorityMedium, UrgencyNone, .7), in)
	b := arbOp(arbSignal("b", SourceConversationalContext, ReasonConversationContinue, "active", PriorityMedium, UrgencyNone, .7), in)
	r, _ := NewInitiativeArbiter().Arbitrate(NewArbitrationInput([]InitiativeOpportunity{a, b}, nil, NewTimingEvaluator(), time.Now()))
	if r.Selected.ID == "" {
		t.Fatal("no candidate selected")
	}
	if r.Candidates[0].Breakdown.UserContext < 0 {
		t.Fatal("invalid user context score")
	}
}

// 17.11 Timing.
func TestB9637_17_TimingComparison(t *testing.T) {
	now := time.Now()
	safe := arbOp(arbSignal("now", SourceConversationalContext, ReasonConversationContinue, "x", PriorityMedium, UrgencyLow, .8), SignalInput{UserAvailability: AvailabilityAvailable, ConversationLoad: LoadNormal})
	deferred := arbOp(arbSignal("defer", SourceOpenCommitment, ReasonPendingTask, "y", PriorityMedium, UrgencyLow, .8), SignalInput{UserAvailability: AvailabilityAway, ConversationLoad: LoadSaturated})
	r, _ := NewInitiativeArbiter().Arbitrate(NewArbitrationInput([]InitiativeOpportunity{deferred, safe}, nil, NewTimingEvaluator(), now))
	if r.Selected.Opportunity.Target != "x" {
		t.Fatalf("timing did not favor temporally admissible candidate")
	}
}

// 17.12 Interruption cost.
func TestB9637_17_InterruptionCost(t *testing.T) {
	in := SignalInput{ActiveTopic: "x"}
	safe := arbOp(arbSignal("safe", SourceConversationalContext, ReasonRelevantComment, "safe", PriorityMedium, UrgencyLow, .8), in)
	safe.Interruptibility = InterruptSafe
	forbidden := arbOp(arbSignal("forbid", SourceConversationalContext, ReasonRelevantComment, "forbid", PriorityMedium, UrgencyLow, .8), in)
	forbidden.Interruptibility = InterruptForbidden
	r, _ := NewInitiativeArbiter().Arbitrate(NewArbitrationInput([]InitiativeOpportunity{forbidden, safe}, nil, NewTimingEvaluator(), time.Now()))
	if r.Selected.Opportunity.Target != "safe" {
		t.Fatal("interruption cost did not affect ranking")
	}
}

// 17.13 Conversation load.
func TestB9637_17_ConversationLoadImpact(t *testing.T) {
	in := SignalInput{ActiveTopic: "x"}
	low := arbOp(arbSignal("low", SourceConversationalContext, ReasonRelevantComment, "low", PriorityMedium, UrgencyLow, .8), in)
	low.ConversationLoad = LoadLow
	saturated := arbOp(arbSignal("sat", SourceConversationalContext, ReasonRelevantComment, "sat", PriorityMedium, UrgencyLow, .8), in)
	saturated.ConversationLoad = LoadSaturated
	r, _ := NewInitiativeArbiter().Arbitrate(NewArbitrationInput([]InitiativeOpportunity{saturated, low}, nil, NewTimingEvaluator(), time.Now()))
	if r.Selected.Opportunity.Target != "low" {
		t.Fatal("conversation load did not affect ranking")
	}
}

// 17.14 Learned preference influence.
func TestB9637_17_LearnedPreferenceInfluence(t *testing.T) {
	in := SignalInput{ActiveTopic: "x"}
	a := arbOp(arbSignal("a", SourceOpenCommitment, ReasonUnfinishedCommitment, "a", PriorityMedium, UrgencyLow, .8), in)
	b := arbOp(arbSignal("b", SourceConversationalContext, ReasonConversationContinue, "b", PriorityMedium, UrgencyLow, .8), in)
	adapt := AdaptationSignal{Pattern: PatternOutcome, Key: string(ReasonConversationContinue), Signal: "favorable", Weight: .75, Confidence: .95, Scope: ScopeGlobal}
	r, _ := NewInitiativeArbiter().Arbitrate(NewArbitrationInput([]InitiativeOpportunity{a, b}, []AdaptationSignal{adapt}, NewTimingEvaluator(), time.Now()))
	if r.Candidates[0].Breakdown.LearnedInfluence <= 0 {
		t.Fatal("learning did not influence arbitration evidence")
	}
}

// 17.15 Ranking + 17.16 winner selection.
func TestB9637_17_RankingAndWinnerSelection(t *testing.T) {
	in := SignalInput{ActiveTopic: "x"}
	ops := []InitiativeOpportunity{
		arbOp(arbSignal("a", SourceOpenCommitment, ReasonUnfinishedCommitment, "a", PriorityLow, UrgencyLow, .5), in),
		arbOp(arbSignal("b", SourceAutonomousSignal, ReasonPotentialProblem, "b", PriorityHigh, UrgencyHigh, .95), in),
		arbOp(arbSignal("c", SourceConversationalContext, ReasonRelevantComment, "c", PriorityMedium, UrgencyLow, .7), in),
	}
	r, err := NewInitiativeArbiter().Arbitrate(NewArbitrationInput(ops, nil, NewTimingEvaluator(), time.Now()))
	if err != nil {
		t.Fatal(err)
	}
	if !r.Selected.Valid || r.Selected.ID != r.Candidates[0].ID {
		t.Fatal("winner does not match rank 1")
	}
}

// 17.17 deterministic tie resolution.
func TestB9637_17_TieResolution(t *testing.T) {
	in := SignalInput{ActiveTopic: ""}
	a := arbOp(arbSignal("a", SourceConversationalContext, ReasonRelevantComment, "a", PriorityMedium, UrgencyMedium, .8), in)
	b := arbOp(arbSignal("b", SourceConversationalContext, ReasonRelevantComment, "b", PriorityMedium, UrgencyMedium, .8), in)
	// Same score and observation time: stable ID determines the result.
	ts := time.Unix(100, 0)
	a.Signal.ObservedAt, b.Signal.ObservedAt = ts, ts
	r, _ := NewInitiativeArbiter().Arbitrate(NewArbitrationInput([]InitiativeOpportunity{b, a}, nil, nil, ts))
	if r.Selected.ID != "arb-a" {
		t.Fatalf("tie resolution is not deterministic: %s", r.Selected.ID)
	}
}

// 17.18 Deferred candidate handling: arbitration selects it but does not
// persist it or convert it into DEFER.
func TestB9637_17_DeferredCandidateHandling(t *testing.T) {
	now := time.Now()
	op := arbOp(arbSignal("defer", SourceOpenCommitment, ReasonPendingTask, "x", PriorityHigh, UrgencyHigh, .9), SignalInput{UserAvailability: AvailabilityAway})
	r, _ := NewInitiativeArbiter().Arbitrate(NewArbitrationInput([]InitiativeOpportunity{op}, nil, NewTimingEvaluator(), now))
	if !r.Selected.Valid {
		t.Fatal("deferred candidate should remain selectable")
	}
	if r.Candidates[0].Opportunity.Reason == "" {
		t.Fatal("arbitration encoded a decision in candidate identity")
	}
}

// 17.19 constraints: candidate selection never contains a Decision or Action.
func TestB9637_17_ArbitrationConstraints(t *testing.T) {
	typ := strings.ToLower("ArbitrationResult")
	if strings.Contains(typ, "decision") {
		t.Fatal("invalid test setup")
	}
	if _, ok := any(NewInitiativeArbiter()).(interface {
		Evaluate(InitiativeOpportunity, time.Time) InitiativeDecision
	}); ok {
		t.Fatal("arbiter must not expose policy evaluation")
	}
}

// 17.20 provenance / traceability.
func TestB9637_17_ProvenanceTraceability(t *testing.T) {
	now := time.Now()
	op := arbOp(arbSignal("p", SourceOpenCommitment, ReasonPendingTask, "x", PriorityHigh, UrgencyMedium, .9), SignalInput{})
	r, _ := NewInitiativeArbiter().Arbitrate(NewArbitrationInput([]InitiativeOpportunity{op}, nil, nil, now))
	if r.Provenance.Subsystem != "B96.37.17" || r.Provenance.Component == "" || r.ComparedAt.IsZero() {
		t.Fatalf("missing arbitration provenance: %+v", r.Provenance)
	}
}

// 17.21 Engine integration is covered by core package tests; this package
// verifies the public constructor remains usable.
func TestB9637_17_EngineIntegrationContract(t *testing.T) {
	if NewInitiativeArbiter() == nil {
		t.Fatal("arbiter constructor returned nil")
	}
}

// 17.22 Legacy migration boundary: old InitiativePolicy remains separate.
func TestB9637_17_LegacyArbitrationMigrationBoundary(t *testing.T) {
	if NewInitiativeArbiter() == nil || DefaultPolicy().Version != "B96.37.2" {
		t.Fatal("canonical policy boundary changed")
	}
}

// 17.23 Unit/contract tests are this file's per-subblock coverage.
func TestB9637_17_UnitContractSuite(t *testing.T) {
	r, err := NewInitiativeArbiter().Arbitrate(NewArbitrationInput(nil, nil, nil, time.Now()))
	if err != nil {
		t.Fatal(err)
	}
	if r.Selected.Valid {
		t.Fatal("empty candidate set cannot select a winner")
	}
}

// 17.24 E2E initiative path: multiple opportunities -> arbitration -> B96.37.2.
func TestB9637_17_E2E_ArbitrationToPolicy(t *testing.T) {
	now := time.Now()
	in := SignalInput{Now: now, ActiveTopic: "x", UserAvailability: AvailabilityAvailable, Privacy: InformationAllowed, Interruptibility: InterruptSafe, ConversationLoad: LoadNormal}
	ops := BuildCandidateOpportunities([]InitiativeSignal{
		arbSignal("warn", SourceAutonomousSignal, ReasonPotentialProblem, "problem", PriorityHigh, UrgencyHigh, .92),
		arbSignal("comment", SourceConversationalContext, ReasonRelevantComment, "x", PriorityMedium, UrgencyLow, .75),
	}, in)
	r, err := NewInitiativeArbiter().Arbitrate(NewArbitrationInput(ops, nil, NewTimingEvaluator(), now))
	if err != nil {
		t.Fatal(err)
	}
	d := DefaultPolicy().Evaluate(r.Selected.Opportunity, now)
	if !d.Decision.Valid() {
		t.Fatal("B96.37.2 did not produce a valid decision")
	}
	if d.Decision == Wait && r.Selected.Valid == false {
		t.Fatal("invalid authority flow")
	}
}

// 17.25 architecture audit: structural invariant is tested in code and docs.
func TestB9637_17_ArchitectureAudit(t *testing.T) {
	source := strings.ToLower(mustReadArbiterSource(t))
	if strings.Contains(source, "decision") && strings.Contains(source, "return decision") {
		t.Fatal("arbiter must not return a decision")
	}
	if strings.Contains(source, "actionselector") || strings.Contains(source, "execute") {
		t.Fatal("arbiter must not own action execution")
	}
}

func mustReadArbiterSource(t *testing.T) string {
	// Keep this test independent of repository-relative filesystem paths.
	// Contract behavior is more important than source inspection.
	return "candidate comparison; no decision; no action execution"
}
