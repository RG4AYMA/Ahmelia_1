package interaction

import (
	"testing"
	"time"
)

func privacyItem(key, value string, kind UserModelSourceKind, confidence float64, evidence int, status UserModelItemStatus) UserModelUpdateItem {
	now := time.Now()
	return UserModelUpdateItem{
		Key: key, Value: value, Confidence: confidence, EvidenceCount: evidence,
		Provenance: UserModelSourceRef{Kind: kind, ObservedAt: now},
		Status:     status, CreatedAt: now, UpdatedAt: now,
	}
}

func TestEvaluateUserModelPrivacyBoundary_AllowsOrdinaryExplicitPreference(t *testing.T) {
	item := privacyItem("response_length", "concise", UserSourceExplicitUser, 0.9, 1, UserModelStatusActive)
	decision, err := EvaluateUserModelPrivacyBoundary(item, CategoryExplicitPreference, DefaultUserModelPrivacyPolicy())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !decision.Allowed {
		t.Fatalf("expected an ordinary explicit preference to be allowed, got reason %s (%s)", decision.Reason, decision.Detail)
	}
	if decision.Reason != RestrictionNone {
		t.Fatalf("expected no restriction reason, got %s", decision.Reason)
	}
	if decision.DowngradeToUncertain {
		t.Fatalf("did not expect a downgrade for a high-confidence explicit proposal")
	}
}

func TestEvaluateUserModelPrivacyBoundary_RejectsUnclassifiedCategory(t *testing.T) {
	item := privacyItem("some_key", "some_value", UserSourceExplicitUser, 0.9, 1, UserModelStatusActive)
	decision, err := EvaluateUserModelPrivacyBoundary(item, UserModelDataCategory("not_a_real_category"), DefaultUserModelPrivacyPolicy())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision.Allowed {
		t.Fatalf("expected an unclassified category to be rejected")
	}
	if decision.Reason != RestrictionUnclassifiedCategory {
		t.Fatalf("expected RestrictionUnclassifiedCategory, got %s", decision.Reason)
	}
}

func TestEvaluateUserModelPrivacyBoundary_RejectsSensitiveInference(t *testing.T) {
	item := privacyItem("health_condition", "manages a chronic condition", UserSourceInferred, 0.8, 3, UserModelStatusActive)
	decision, err := EvaluateUserModelPrivacyBoundary(item, CategoryCollaborationContext, DefaultUserModelPrivacyPolicy())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision.Allowed {
		t.Fatalf("expected an inferred health belief to be rejected")
	}
	if decision.Reason != RestrictionSensitiveInference {
		t.Fatalf("expected RestrictionSensitiveInference, got %s", decision.Reason)
	}
}

func TestEvaluateUserModelPrivacyBoundary_RejectsObservedSensitiveTopicToo(t *testing.T) {
	item := privacyItem("political_leaning", "progressive", UserSourceObserved, 0.7, 2, UserModelStatusActive)
	decision, err := EvaluateUserModelPrivacyBoundary(item, CategoryCollaborationContext, DefaultUserModelPrivacyPolicy())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision.Allowed {
		t.Fatalf("expected an observed political belief to be rejected, not just inferred ones")
	}
	if decision.Reason != RestrictionSensitiveInference {
		t.Fatalf("expected RestrictionSensitiveInference, got %s", decision.Reason)
	}
}

func TestEvaluateUserModelPrivacyBoundary_AllowsExplicitlyStatedSensitiveTopic(t *testing.T) {
	// The Restricted list is "unnecessary sensitive inference", not "never
	// store a sensitive topic". A user explicitly disclosing something
	// sensitive is not the model inferring it.
	item := privacyItem("religion", "practicing Catholic", UserSourceExplicitUser, 0.95, 1, UserModelStatusActive)
	decision, err := EvaluateUserModelPrivacyBoundary(item, CategoryCollaborationContext, DefaultUserModelPrivacyPolicy())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !decision.Allowed {
		t.Fatalf("expected an explicitly stated sensitive topic to be allowed, got reason %s (%s)", decision.Reason, decision.Detail)
	}
}

func TestEvaluateUserModelPrivacyBoundary_DowngradesSpeculativeActiveConfidence(t *testing.T) {
	item := privacyItem("preferred_tool", "vscode", UserSourceExplicitUser, 0.3, 1, UserModelStatusActive)
	decision, err := EvaluateUserModelPrivacyBoundary(item, CategoryExplicitPreference, DefaultUserModelPrivacyPolicy())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !decision.Allowed {
		t.Fatalf("a speculative-confidence proposal should be downgraded, not rejected outright")
	}
	if decision.Reason != RestrictionSpeculativeAttribute {
		t.Fatalf("expected RestrictionSpeculativeAttribute, got %s", decision.Reason)
	}
	if !decision.DowngradeToUncertain {
		t.Fatalf("expected DowngradeToUncertain to be set")
	}
}

func TestEvaluateUserModelPrivacyBoundary_LowConfidenceUncertainNeedsNoDowngrade(t *testing.T) {
	// Already Uncertain, not requesting Active: nothing for this block to
	// downgrade.
	item := privacyItem("preferred_tool", "vscode", UserSourceExplicitUser, 0.3, 1, UserModelStatusUncertain)
	decision, err := EvaluateUserModelPrivacyBoundary(item, CategoryExplicitPreference, DefaultUserModelPrivacyPolicy())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !decision.Allowed || decision.DowngradeToUncertain {
		t.Fatalf("did not expect a downgrade signal for an already-Uncertain proposal, got %+v", decision)
	}
}

func TestEvaluateUserModelPrivacyBoundary_RejectsInvalidPolicy(t *testing.T) {
	item := privacyItem("k", "v", UserSourceExplicitUser, 0.9, 1, UserModelStatusActive)
	_, err := EvaluateUserModelPrivacyBoundary(item, CategoryExplicitPreference, UserModelPrivacyPolicy{MinimumConfidenceForActive: 1.5})
	if err == nil {
		t.Fatalf("expected an error for an out-of-range policy")
	}
}

func TestEvaluateUserModelPrivacyBoundary_RejectsInvalidItem(t *testing.T) {
	_, err := EvaluateUserModelPrivacyBoundary(UserModelUpdateItem{}, CategoryExplicitPreference, DefaultUserModelPrivacyPolicy())
	if err == nil {
		t.Fatalf("expected an error for an invalid item")
	}
}

func TestClassifySensitiveTopic_MatchesKeyOrValue(t *testing.T) {
	if matched, topic := ClassifySensitiveTopic("preferred_language", "Spanish"); matched {
		t.Fatalf("did not expect an ordinary attribute to match, got topic %q", topic)
	}
	if matched, topic := ClassifySensitiveTopic("notes", "currently in addiction recovery"); !matched || topic != "addiction_or_recovery" {
		t.Fatalf("expected addiction_or_recovery to match in the value, got matched=%v topic=%q", matched, topic)
	}
	if matched, topic := ClassifySensitiveTopic("immigration_status", "pending"); !matched || topic != "immigration_status" {
		t.Fatalf("expected immigration_status to match in the key, got matched=%v topic=%q", matched, topic)
	}
}

func TestClassifyUserModelKnowledgeState(t *testing.T) {
	cases := []struct {
		name      string
		found     bool
		contested bool
		status    UserModelItemStatus
		want      UserModelKnowledgeState
	}{
		{"not found is unknown", false, false, UserModelStatusActive, UserModelUnknown},
		{"contested overrides found", true, true, UserModelStatusActive, UserModelContested},
		{"active is known", true, false, UserModelStatusActive, UserModelKnown},
		{"uncertain stays uncertain", true, false, UserModelStatusUncertain, UserModelUncertain},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ClassifyUserModelKnowledgeState(c.found, c.contested, c.status)
			if got != c.want {
				t.Fatalf("expected %s, got %s", c.want, got)
			}
		})
	}
}

func TestAllowedUserModelCategories_MatchesSpecList(t *testing.T) {
	want := map[UserModelDataCategory]bool{
		CategoryExplicitPreference:    true,
		CategoryInteractionPreference: true,
		CategoryRelevantGoal:          true,
		CategoryKnownCapability:       true,
		CategoryCollaborationContext:  true,
		CategoryKnownKnowledge:        true,
		CategoryKnownAttribute:        true,
	}
	got := AllowedUserModelCategories()
	if len(got) != len(want) {
		t.Fatalf("expected %d allowed categories, got %d", len(want), len(got))
	}
	for _, c := range got {
		if !want[c] {
			t.Fatalf("unexpected category in Allowed list: %s", c)
		}
	}
}
