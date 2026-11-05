package interaction

import (
	"testing"
	"time"
)

func TestRunUserModelUpdateE2E_EstablishesNewBelief(t *testing.T) {
	store, err := NewUserModelStore(NewUserModel("user-1"))
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	now := time.Now()
	signal := UserModelUpdateSignal{
		Domain:   UserModelDomainPreference,
		Category: CategoryExplicitPreference,
		Item: UserModelUpdateItem{
			Key: "concise_responses", Value: "true", Confidence: 0.9, EvidenceCount: 1,
			Status:     UserModelStatusActive,
			Provenance: UserModelSourceRef{Kind: UserSourceExplicitUser, ObservedAt: now},
			CreatedAt:  now, UpdatedAt: now,
		},
	}

	out, err := RunUserModelUpdateE2E(store, UserModelActorMemory, signal,
		DefaultUserModelPrivacyPolicy(), DefaultUserModelUpdatePolicy(), DefaultUserModelConflictPolicy(), now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !out.Applied || out.Rejected {
		t.Fatalf("expected applied outcome, got %+v", out)
	}
	if !out.Valid {
		t.Fatalf("expected valid outcome, errors: %v", out.Errors)
	}
	if out.Reconciliation.Action != UserModelUpdateEstablish {
		t.Fatalf("expected establish action, got %s", out.Reconciliation.Action)
	}
	if len(out.Model.Model.Preferences) != 1 {
		t.Fatalf("expected 1 preference stored, got %d", len(out.Model.Model.Preferences))
	}
	if out.Model.Model.Preferences[0].Key != "concise_responses" {
		t.Fatalf("unexpected stored preference: %+v", out.Model.Model.Preferences[0])
	}
}

func TestRunUserModelUpdateE2E_RejectsInferredSensitiveTopic(t *testing.T) {
	store, err := NewUserModelStore(NewUserModel("user-1"))
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	now := time.Now()
	signal := UserModelUpdateSignal{
		Domain:   UserModelDomainAttribute,
		Category: CategoryCollaborationContext,
		Item: UserModelUpdateItem{
			Key: "health_status", Value: "managing a diagnosis", Confidence: 0.8, EvidenceCount: 3,
			Status:     UserModelStatusActive,
			Provenance: UserModelSourceRef{Kind: UserSourceInferred, ObservedAt: now},
			CreatedAt:  now, UpdatedAt: now,
		},
	}

	out, err := RunUserModelUpdateE2E(store, UserModelActorMemory, signal,
		DefaultUserModelPrivacyPolicy(), DefaultUserModelUpdatePolicy(), DefaultUserModelConflictPolicy(), now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Applied || !out.Rejected {
		t.Fatalf("expected rejection, got %+v", out)
	}
	if out.Privacy.Reason != RestrictionSensitiveInference {
		t.Fatalf("expected sensitive-inference rejection, got %q", out.Privacy.Reason)
	}

	view, err := store.Read(UserModelAccessRequest{Actor: UserModelActorMemory, Mode: UserModelAccessRead})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(view.Model.Attributes) != 0 {
		t.Fatalf("model must remain untouched on rejection, got %+v", view.Model.Attributes)
	}
}

func TestRunUserModelUpdateE2E_HoldThenUnresolvedTieStaysLive(t *testing.T) {
	store, err := NewUserModelStore(NewUserModel("user-1"))
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	base := time.Now()

	first := UserModelUpdateSignal{
		Domain: UserModelDomainPreference, Category: CategoryExplicitPreference,
		Item: UserModelUpdateItem{
			Key: "response_length", Value: "short", Confidence: 0.6, EvidenceCount: 1,
			Status:     UserModelStatusActive,
			Provenance: UserModelSourceRef{Kind: UserSourceInferred, ObservedAt: base},
			CreatedAt:  base, UpdatedAt: base,
		},
	}
	if _, err := RunUserModelUpdateE2E(store, UserModelActorMemory, first,
		DefaultUserModelPrivacyPolicy(), DefaultUserModelUpdatePolicy(), DefaultUserModelConflictPolicy(), base); err != nil {
		t.Fatalf("first update: %v", err)
	}

	later := base.Add(time.Hour)
	second := UserModelUpdateSignal{
		Domain: UserModelDomainPreference, Category: CategoryExplicitPreference,
		Item: UserModelUpdateItem{
			Key: "response_length", Value: "long", Confidence: 0.65, EvidenceCount: 1,
			Status:     UserModelStatusActive,
			Provenance: UserModelSourceRef{Kind: UserSourceInferred, ObservedAt: later},
			CreatedAt:  later, UpdatedAt: later,
		},
	}
	out, err := RunUserModelUpdateE2E(store, UserModelActorMemory, second,
		DefaultUserModelPrivacyPolicy(), DefaultUserModelUpdatePolicy(), DefaultUserModelConflictPolicy(), later)
	if err != nil {
		t.Fatalf("second update: %v", err)
	}
	if !out.Applied {
		t.Fatalf("expected applied outcome, got %+v", out)
	}
	if out.Reconciliation.Action != UserModelUpdateHoldForReconciliation {
		t.Fatalf("expected hold_for_reconciliation, got %s", out.Reconciliation.Action)
	}
	if out.Conflict == nil {
		t.Fatalf("expected B96.34.13 to have run")
	}
	// Same priority tier (both inferred), confidence gap 0.05 < the 0.20
	// default margin: no honest winner, so both stay live.
	if out.Conflict.Kind != UserModelConflictUnresolved {
		t.Fatalf("expected unresolved tie, got %s", out.Conflict.Kind)
	}

	live := 0
	for _, p := range out.Model.Model.Preferences {
		if p.Key == "response_length" && isLiveUserFactStatus(p.Status) {
			live++
		}
	}
	if live != 2 {
		t.Fatalf("expected both beliefs to remain live pending B96.34.13, got %d", live)
	}
}

func TestRunUserModelUpdateE2E_ContextSplitKeepsBothBeliefsActive(t *testing.T) {
	store, err := NewUserModelStore(NewUserModel("user-1"))
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	base := time.Now()

	first := UserModelUpdateSignal{
		Domain: UserModelDomainPreference, Category: CategoryExplicitPreference,
		Item: UserModelUpdateItem{
			Key: "response_depth", Value: "short", Confidence: 0.9, EvidenceCount: 2,
			Status:     UserModelStatusActive,
			Provenance: UserModelSourceRef{Kind: UserSourceExplicitUser, ObservedAt: base},
			CreatedAt:  base, UpdatedAt: base,
		},
		// No Context: this belief predates any conflict, so B96.34.15 has
		// nothing to record for it yet — see the Known limitation note.
	}
	if _, err := RunUserModelUpdateE2E(store, UserModelActorMemory, first,
		DefaultUserModelPrivacyPolicy(), DefaultUserModelUpdatePolicy(), DefaultUserModelConflictPolicy(), base); err != nil {
		t.Fatalf("first update: %v", err)
	}

	later := base.Add(time.Hour)
	second := UserModelUpdateSignal{
		Domain: UserModelDomainPreference, Category: CategoryExplicitPreference,
		Item: UserModelUpdateItem{
			Key: "response_depth", Value: "deep", Confidence: 0.9, EvidenceCount: 2,
			Status:     UserModelStatusActive,
			Provenance: UserModelSourceRef{Kind: UserSourceExplicitUser, ObservedAt: later},
			CreatedAt:  later, UpdatedAt: later,
		},
		Context: "technical_work",
	}
	out, err := RunUserModelUpdateE2E(store, UserModelActorMemory, second,
		DefaultUserModelPrivacyPolicy(), DefaultUserModelUpdatePolicy(), DefaultUserModelConflictPolicy(), later)
	if err != nil {
		t.Fatalf("second update: %v", err)
	}
	if out.Conflict == nil {
		t.Fatalf("expected B96.34.13 to have run")
	}
	// The unscoped first belief ("") and the scoped second belief
	// ("technical_work") land in different context groups, so this is a
	// split, not a contradiction, even though the first belief's original
	// "casual" intent (never persisted) is not what B96.34.15 recovers.
	if out.Conflict.Kind != UserModelConflictContextSplit {
		t.Fatalf("expected context split, got %s", out.Conflict.Kind)
	}

	active := 0
	for _, p := range out.Model.Model.Preferences {
		if p.Key == "response_depth" && p.Status == UserModelStatusActive {
			active++
		}
	}
	if active != 2 {
		t.Fatalf("expected both context-scoped beliefs to remain active, got %d", active)
	}
}

func TestRunUserModelUpdateE2E_RejectionLeavesModelUntouchedAcrossDomains(t *testing.T) {
	store, err := NewUserModelStore(NewUserModel("user-1"))
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	now := time.Now()
	signal := UserModelUpdateSignal{
		Domain:   UserModelDomainGoal,
		Category: UserModelDataCategory("not_a_real_category"),
		Item: UserModelUpdateItem{
			Key: "ship_ahmelia_v4", Value: "long_term", Confidence: 0.7, EvidenceCount: 2,
			Status:     UserModelStatusActive,
			Provenance: UserModelSourceRef{Kind: UserSourceExplicitUser, ObservedAt: now},
			CreatedAt:  now, UpdatedAt: now,
		},
	}

	out, err := RunUserModelUpdateE2E(store, UserModelActorMemory, signal,
		DefaultUserModelPrivacyPolicy(), DefaultUserModelUpdatePolicy(), DefaultUserModelConflictPolicy(), now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !out.Rejected || out.Applied {
		t.Fatalf("expected rejection for unclassified category, got %+v", out)
	}
	if out.Privacy.Reason != RestrictionUnclassifiedCategory {
		t.Fatalf("expected unclassified-category rejection, got %q", out.Privacy.Reason)
	}
	if len(out.Model.Model.Goals) != 0 {
		t.Fatalf("model must remain untouched on rejection, got %+v", out.Model.Model.Goals)
	}
}

func TestRunUserModelUpdateE2E_CleansDirtyExplicitPreferenceVariant(t *testing.T) {
	t0 := time.Now()
	store, err := NewUserModelStore(UserModel{
		UserID: "ayma240",
		Preferences: []UserPreference{{
			Key: "favorite_color", Value: "negro ya te lo había dicho", Confidence: 0.72,
			Status: UserModelStatusActive, Provenance: UserModelSourceRef{Kind: UserSourceExplicitUser, SourceID: "old"},
			CreatedAt: t0, UpdatedAt: t0,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	item, accepted, err := EvaluateUserPreferenceUpdate(UserPreferenceUpdate{
		Key: "favorite_color", Value: "negro", Strength: 1, Confidence: 0.72,
		Source: UserModelSourceRef{Kind: UserSourceExplicitUser, SourceID: "new", ObservedAt: t0.Add(time.Minute)},
		Now:    t0.Add(time.Minute),
	}, DefaultUserPreferencePolicy())
	if err != nil || !accepted {
		t.Fatalf("expected clean explicit preference to be accepted: accepted=%v err=%v item=%+v", accepted, err, item)
	}
	out, err := RunUserModelUpdateE2E(
		store, UserModelActorSystem,
		UserModelUpdateSignal{Domain: UserModelDomainPreference, Category: CategoryExplicitPreference, Item: PreferenceUpdateItem(item)},
		DefaultUserModelPrivacyPolicy(), DefaultUserModelUpdatePolicy(), DefaultUserModelConflictPolicy(), t0.Add(time.Minute),
	)
	if err != nil {
		t.Fatal(err)
	}
	if out.Reconciliation.Action != UserModelUpdateReinforce {
		t.Fatalf("expected dirty variant to reinforce, got %+v", out.Reconciliation)
	}
	view, err := store.Read(UserModelAccessRequest{Actor: UserModelActorCognition, Mode: UserModelAccessRead})
	if err != nil {
		t.Fatal(err)
	}
	var found *UserPreference
	for i := range view.Model.Preferences {
		if view.Model.Preferences[i].Key == "favorite_color" && view.Model.Preferences[i].Status == UserModelStatusActive {
			found = &view.Model.Preferences[i]
		}
	}
	if found == nil || found.Value != "negro" {
		t.Fatalf("expected one clean active favorite_color, got %+v", view.Model.Preferences)
	}
}
