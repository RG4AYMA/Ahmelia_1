package interaction

import (
	"testing"
	"time"
)

func explicitItem(key, value string, confidence float64, evidence int, at time.Time) UserModelUpdateItem {
	return UserModelUpdateItem{
		Key: key, Value: value, Confidence: confidence, EvidenceCount: evidence,
		Provenance: UserModelSourceRef{Kind: UserSourceExplicitUser, ObservedAt: at},
		Status:     UserModelStatusActive, CreatedAt: at, UpdatedAt: at,
	}
}

func inferredItem(key, value string, confidence float64, evidence int, status UserModelItemStatus, at time.Time) UserModelUpdateItem {
	return UserModelUpdateItem{
		Key: key, Value: value, Confidence: confidence, EvidenceCount: evidence,
		Provenance: UserModelSourceRef{Kind: UserSourceInferred, ObservedAt: at},
		Status:     status, CreatedAt: at, UpdatedAt: at,
	}
}

func TestReconcileUserModelUpdate_EstablishesWhenNoPriorBelief(t *testing.T) {
	now := time.Now()
	proposal := explicitItem("technical_depth", "high", 0.95, 1, now)

	result, outcome, err := ReconcileUserModelUpdate(nil, proposal, DefaultUserModelUpdatePolicy())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outcome.Action != UserModelUpdateEstablish {
		t.Fatalf("expected establish, got %s", outcome.Action)
	}
	if len(result) != 1 || result[0].Value != "high" {
		t.Fatalf("expected the proposal to be stored as-is, got %+v", result)
	}
}

func TestReconcileUserModelUpdate_ReinforcesAgreeingBelief(t *testing.T) {
	t0 := time.Now()
	t1 := t0.Add(time.Hour)

	existing := []UserModelUpdateItem{
		inferredItem("technical_programming_experience", "high", 0.70, 2, UserModelStatusUncertain, t0),
	}
	proposal := inferredItem("technical_programming_experience", "high", 0.90, 2, UserModelStatusUncertain, t1)

	result, outcome, err := ReconcileUserModelUpdate(existing, proposal, DefaultUserModelUpdatePolicy())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outcome.Action != UserModelUpdateReinforce {
		t.Fatalf("expected reinforce, got %s", outcome.Action)
	}
	if len(result) != 1 {
		t.Fatalf("expected reinforcement to replace the single existing item, got %d items", len(result))
	}
	// Weighted average of 0.70 (weight 2) and 0.90 (weight 2) is 0.80 — a
	// single agreeing proposal must move confidence, not just keep the max.
	if got := result[0].Confidence; got < 0.79 || got > 0.81 {
		t.Fatalf("expected confidence to move toward 0.80, got %f", got)
	}
	if result[0].EvidenceCount != 4 {
		t.Fatalf("expected evidence to accumulate to 4, got %d", result[0].EvidenceCount)
	}
}

func TestReconcileUserModelUpdate_ExplicitSupersedesInferred(t *testing.T) {
	t0 := time.Now()
	t1 := t0.Add(time.Hour)

	existing := []UserModelUpdateItem{
		inferredItem("preferred_response_length", "short", 0.86, 3, UserModelStatusActive, t0),
	}
	proposal := explicitItem("preferred_response_length", "long", 0.95, 1, t1)

	result, outcome, err := ReconcileUserModelUpdate(existing, proposal, DefaultUserModelUpdatePolicy())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outcome.Action != UserModelUpdateSupersede {
		t.Fatalf("expected supersede, got %s", outcome.Action)
	}
	if outcome.Superseded == nil || outcome.Superseded.Status != UserModelStatusSuperseded {
		t.Fatalf("expected the old belief to be marked superseded, got %+v", outcome.Superseded)
	}
	if len(result) != 2 {
		t.Fatalf("expected the old belief to remain in history (not deleted), got %d items", len(result))
	}
	foundSuperseded, foundActive := false, false
	for _, it := range result {
		if it.Value == "short" && it.Status == UserModelStatusSuperseded {
			foundSuperseded = true
		}
		if it.Value == "long" && it.Status == UserModelStatusActive {
			foundActive = true
		}
	}
	if !foundSuperseded || !foundActive {
		t.Fatalf("expected both the superseded old value and the new active value in history, got %+v", result)
	}
}

func TestReconcileUserModelUpdate_HoldsWhenNeitherOutranks(t *testing.T) {
	t0 := time.Now()
	t1 := t0.Add(time.Hour)

	// Same priority tier (both inferred), and the new proposal's confidence
	// is not enough above the existing one to cross the margin.
	existing := []UserModelUpdateItem{
		inferredItem("humor_tolerance", "low", 0.88, 3, UserModelStatusActive, t0),
	}
	proposal := inferredItem("humor_tolerance", "high", 0.90, 3, UserModelStatusUncertain, t1)

	result, outcome, err := ReconcileUserModelUpdate(existing, proposal, DefaultUserModelUpdatePolicy())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outcome.Action != UserModelUpdateHoldForReconciliation {
		t.Fatalf("expected hold_for_reconciliation, got %s", outcome.Action)
	}
	if len(result) != 2 {
		t.Fatalf("expected both beliefs to remain live, got %d items", len(result))
	}
	for _, it := range result {
		if it.Value == "low" && it.Status != UserModelStatusActive {
			t.Fatalf("expected the prior belief to remain untouched and active, got %+v", it)
		}
		if it.Value == "high" && it.Status != UserModelStatusUncertain {
			t.Fatalf("expected the held proposal to be forced to uncertain, got %+v", it)
		}
	}
}

func TestReconcileUserModelUpdate_GuardsSingleObservationFromActive(t *testing.T) {
	now := time.Now()
	// A domain policy that (incorrectly) marked a single inferred
	// observation as Active must still be caught here.
	proposal := UserModelUpdateItem{
		Key: "focus_style", Value: "deep_work", Confidence: 0.95, EvidenceCount: 1,
		Provenance: UserModelSourceRef{Kind: UserSourceInferred, ObservedAt: now},
		Status:     UserModelStatusActive, CreatedAt: now, UpdatedAt: now,
	}

	result, outcome, err := ReconcileUserModelUpdate(nil, proposal, DefaultUserModelUpdatePolicy())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outcome.Action != UserModelUpdateEstablish {
		t.Fatalf("expected establish, got %s", outcome.Action)
	}
	if result[0].Status != UserModelStatusUncertain {
		t.Fatalf("expected a single inferred observation to be downgraded to uncertain, got %s", result[0].Status)
	}
}

func TestReconcileUserModelUpdate_RejectsInvalidProposal(t *testing.T) {
	now := time.Now()
	invalid := UserModelUpdateItem{
		Key: "", Value: "x", Confidence: 0.9,
		Provenance: UserModelSourceRef{Kind: UserSourceExplicitUser, ObservedAt: now},
		Status:     UserModelStatusActive, CreatedAt: now, UpdatedAt: now,
	}
	if _, _, err := ReconcileUserModelUpdate(nil, invalid, DefaultUserModelUpdatePolicy()); err == nil {
		t.Fatal("expected an error for an empty key")
	}
}

func TestReconcileUserModelUpdate_RejectsInvalidPolicy(t *testing.T) {
	now := time.Now()
	proposal := explicitItem("k", "v", 0.9, 1, now)
	bad := UserModelUpdatePolicy{ContradictionConfidenceMargin: 1.5}
	if _, _, err := ReconcileUserModelUpdate(nil, proposal, bad); err == nil {
		t.Fatal("expected an error for an out-of-range contradiction margin")
	}
}

func TestReconcileUserModelUpdate_DoesNotMutateExistingSlice(t *testing.T) {
	t0 := time.Now()
	t1 := t0.Add(time.Hour)
	existing := []UserModelUpdateItem{
		inferredItem("k", "old", 0.80, 2, UserModelStatusActive, t0),
	}
	proposal := explicitItem("k", "new", 0.95, 1, t1)

	_, _, err := ReconcileUserModelUpdate(existing, proposal, DefaultUserModelUpdatePolicy())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if existing[0].Value != "old" || existing[0].Status != UserModelStatusActive {
		t.Fatalf("expected the caller's slice to remain untouched, got %+v", existing[0])
	}
}

func TestValidateUserModelUpdateItem(t *testing.T) {
	now := time.Now()
	valid := explicitItem("k", "v", 0.5, 1, now)
	if err := ValidateUserModelUpdateItem(valid); err != nil {
		t.Fatalf("expected valid item to pass, got %v", err)
	}

	badConfidence := valid
	badConfidence.Confidence = 1.5
	if err := ValidateUserModelUpdateItem(badConfidence); err == nil {
		t.Fatal("expected an error for out-of-range confidence")
	}

	badEvidence := valid
	badEvidence.EvidenceCount = -1
	if err := ValidateUserModelUpdateItem(badEvidence); err == nil {
		t.Fatal("expected an error for negative evidence count")
	}
}

func TestUpdateItemAdapters(t *testing.T) {
	now := time.Now()
	src := UserModelSourceRef{Kind: UserSourceExplicitUser, ObservedAt: now}

	attr := AttributeUpdateItem(UserAttributeRecord{Key: "language", Value: "Spanish", Confidence: 1, Source: src, Status: UserModelStatusActive, CreatedAt: now, UpdatedAt: now})
	if attr.Key != "language" || attr.Value != "Spanish" {
		t.Fatalf("unexpected attribute adapter output: %+v", attr)
	}

	goal := GoalUpdateItem(UserGoalRecord{Goal: "aprender Blender", Category: UserGoalCategoryAspirational, Confidence: 0.76, Source: src, Status: UserModelStatusActive, CreatedAt: now, UpdatedAt: now})
	if goal.Key != "aprender Blender" || goal.Value != string(UserGoalCategoryAspirational) {
		t.Fatalf("unexpected goal adapter output: %+v", goal)
	}

	cap := CapabilityUpdateItem(UserCapabilityRecord{Capability: "Blender", Level: UserCapabilityAssisted, Confidence: 0.6, Source: src, Status: UserModelStatusUncertain, CreatedAt: now, UpdatedAt: now})
	if cap.Key != "Blender" || cap.Value != string(UserCapabilityAssisted) {
		t.Fatalf("unexpected capability adapter output: %+v", cap)
	}

	know := KnowledgeUpdateItem(UserKnowledgeRecord{Topic: "Go", Level: UserKnowledgeAdvanced, Confidence: 0.9, Source: src, Status: UserModelStatusActive, CreatedAt: now, UpdatedAt: now})
	if know.Key != "Go" || know.Value != string(UserKnowledgeAdvanced) {
		t.Fatalf("unexpected knowledge adapter output: %+v", know)
	}

	pattern := InteractionPatternUpdateItem(UserInteractionPatternRecord{Pattern: "incremental_workflow", Confidence: 0.88, EvidenceCount: 37, Source: src, Status: UserModelStatusActive, CreatedAt: now, UpdatedAt: now})
	if pattern.Key != "incremental_workflow" || pattern.EvidenceCount != 37 {
		t.Fatalf("unexpected pattern adapter output: %+v", pattern)
	}

	comm := CommunicationProfileUpdateItem(UserCommunicationProfileValue{Dimension: CommunicationDirectness, Value: "high", Confidence: 0.9, Source: src, Status: UserModelStatusActive, CreatedAt: now, UpdatedAt: now})
	if comm.Key != string(CommunicationDirectness) || comm.Value != "high" {
		t.Fatalf("unexpected communication profile adapter output: %+v", comm)
	}
}

func TestReconcileUserModelUpdate_ReinforcesCleanVariantOfSameExplicitBelief(t *testing.T) {
	t0 := time.Now()
	t1 := t0.Add(time.Hour)
	existing := []UserModelUpdateItem{
		explicitItem("favorite_color", "negro ya te lo había dicho", 0.72, 1, t0),
	}
	proposal := explicitItem("favorite_color", "negro", 0.72, 1, t1)

	result, outcome, err := ReconcileUserModelUpdate(existing, proposal, DefaultUserModelUpdatePolicy())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outcome.Action != UserModelUpdateReinforce {
		t.Fatalf("expected reinforce for a clean variant of the same explicit belief, got %s (%s)", outcome.Action, outcome.Reason)
	}
	if len(result) != 1 {
		t.Fatalf("expected one live belief after clean-variant reinforcement, got %d: %+v", len(result), result)
	}
	if result[0].Status != UserModelStatusActive || result[0].Value != "negro" {
		t.Fatalf("expected clean active value to remain canonical, got %+v", result[0])
	}
	if result[0].EvidenceCount != 2 {
		t.Fatalf("expected evidence to accumulate, got %d", result[0].EvidenceCount)
	}
}

func TestReconcileUserModelUpdate_DoesNotTreatRealCompoundValueAsCleanVariant(t *testing.T) {
	t0 := time.Now()
	existing := []UserModelUpdateItem{explicitItem("favorite_color", "azul", 0.72, 1, t0)}
	proposal := explicitItem("favorite_color", "azul y verde", 0.72, 1, t0.Add(time.Hour))
	result, outcome, err := ReconcileUserModelUpdate(existing, proposal, DefaultUserModelUpdatePolicy())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outcome.Action != UserModelUpdateHoldForReconciliation || len(result) != 2 {
		t.Fatalf("expected genuine disagreement to remain a conflict, got action=%s result=%+v", outcome.Action, result)
	}
}
