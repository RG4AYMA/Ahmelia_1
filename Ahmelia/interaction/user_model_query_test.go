package interaction

import (
	"testing"
	"time"
)

func testModelForQueries(t *testing.T) *UserModelStore {
	t.Helper()
	now := time.Now()
	src := UserModelSourceRef{Kind: UserSourceExplicitUser, ObservedAt: now}

	model := NewUserModel("user-1")
	model.Preferences = []UserPreference{
		{Key: "concise_responses", Value: "true", Confidence: 0.91, Provenance: src, Status: UserModelStatusActive, CreatedAt: now, UpdatedAt: now},
		{Key: "likes_tables", Value: "true", Confidence: 0.42, Provenance: src, Status: UserModelStatusUncertain, CreatedAt: now, UpdatedAt: now},
	}
	model.Goals = []UserGoal{
		{Key: "aprender Blender", Value: "aspirational", Confidence: 0.76, Provenance: src, Status: UserModelStatusActive, CreatedAt: now, UpdatedAt: now},
	}
	model.Knowledge = []UserKnowledgeItem{
		{Key: "Go", Value: "advanced", Confidence: 0.9, Provenance: src, Status: UserModelStatusActive, CreatedAt: now, UpdatedAt: now},
	}
	model.Capabilities = []UserCapability{
		{Key: "Blender", Value: "assisted", Confidence: 0.6, Provenance: src, Status: UserModelStatusActive, CreatedAt: now, UpdatedAt: now},
	}
	model.InteractionProfile.Values["directness"] = UserInteractionProfileValue{
		Value: "high", Confidence: 0.9, Provenance: src, Status: UserModelStatusActive, UpdatedAt: now,
	}
	model.InteractionProfile.Values["humor_tolerance"] = UserInteractionProfileValue{
		Value: "low", Confidence: 0.5, Provenance: src, Status: UserModelStatusUncertain, UpdatedAt: now,
	}

	store, err := NewUserModelStore(model)
	if err != nil {
		t.Fatalf("unexpected error building store: %v", err)
	}
	return store
}

func TestGetRelevantPreferences_RequiresTopics(t *testing.T) {
	store := testModelForQueries(t)
	if _, err := GetRelevantPreferences(store, UserModelActorCognition, UserModelQueryContext{}); err == nil {
		t.Fatal("expected an error for empty topics")
	}
}

func TestGetRelevantPreferences_FiltersByTopicAndStatus(t *testing.T) {
	store := testModelForQueries(t)
	out, err := GetRelevantPreferences(store, UserModelActorCognition, UserModelQueryContext{Topics: []string{"concise"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 1 || out[0].Key != "concise_responses" {
		t.Fatalf("expected only the active concise_responses preference, got %+v", out)
	}

	// likes_tables is Uncertain and should not appear unless requested.
	outWithUncertain, err := GetRelevantPreferences(store, UserModelActorCognition, UserModelQueryContext{Topics: []string{"tables"}, IncludeUncertain: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(outWithUncertain) != 1 || outWithUncertain[0].Key != "likes_tables" {
		t.Fatalf("expected likes_tables when IncludeUncertain is set, got %+v", outWithUncertain)
	}

	outWithoutUncertain, err := GetRelevantPreferences(store, UserModelActorCognition, UserModelQueryContext{Topics: []string{"tables"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(outWithoutUncertain) != 0 {
		t.Fatalf("expected no result for an uncertain preference without IncludeUncertain, got %+v", outWithoutUncertain)
	}
}

func TestGetRelevantGoals(t *testing.T) {
	store := testModelForQueries(t)
	out, err := GetRelevantGoals(store, UserModelActorCognition, UserModelQueryContext{Topics: []string{"blender"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 1 || out[0].Key != "aprender Blender" {
		t.Fatalf("expected the Blender goal, got %+v", out)
	}

	if _, err := GetRelevantGoals(store, UserModelActorCognition, UserModelQueryContext{}); err == nil {
		t.Fatal("expected an error for empty topics")
	}
}

func TestGetRelevantUserKnowledge(t *testing.T) {
	store := testModelForQueries(t)
	out, err := GetRelevantUserKnowledge(store, UserModelActorCognition, "Go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 1 || out[0].Key != "Go" {
		t.Fatalf("expected the Go knowledge item, got %+v", out)
	}

	if _, err := GetRelevantUserKnowledge(store, UserModelActorCognition, "   "); err == nil {
		t.Fatal("expected an error for an empty topic")
	}
}

func TestGetCapabilityLevel(t *testing.T) {
	store := testModelForQueries(t)
	level, found, contested, err := GetCapabilityLevel(store, UserModelActorCognition, "Blender")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found || level.Value != "assisted" {
		t.Fatalf("expected to find the Blender capability, got %+v found=%v", level, found)
	}
	if contested {
		t.Fatal("expected a single live belief to not be contested")
	}

	_, found, _, err = GetCapabilityLevel(store, UserModelActorCognition, "Unreal")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found {
		t.Fatal("expected no capability recorded for Unreal")
	}

	if _, _, _, err := GetCapabilityLevel(store, UserModelActorCognition, ""); err == nil {
		t.Fatal("expected an error for an empty domain")
	}
}

func TestGetCapabilityLevel_FlagsContestedBeliefs(t *testing.T) {
	store := testModelForQueries(t)
	view, _ := store.Read(UserModelAccessRequest{Actor: UserModelActorCognition, Mode: UserModelAccessRead})
	model := view.Model
	// Simulate what B96.34.11's hold_for_reconciliation leaves behind: two
	// live beliefs for the same domain with disagreeing values.
	model.Capabilities = append(model.Capabilities, UserCapability{
		Key: "Blender", Value: "independent", Confidence: 0.6,
		Provenance: UserModelSourceRef{Kind: UserSourceInferred, ObservedAt: time.Now()},
		Status:     UserModelStatusUncertain, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})
	contestedStore, err := NewUserModelStore(model)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, found, contested, err := GetCapabilityLevel(contestedStore, UserModelActorCognition, "Blender")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected the active belief to still be found")
	}
	if !contested {
		t.Fatal("expected the disagreeing uncertain belief to be flagged as contested")
	}
}

func TestGetCommunicationProfile(t *testing.T) {
	store := testModelForQueries(t)

	all, err := GetCommunicationProfile(store, UserModelActorCognition, UserModelQueryContext{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(all) != 1 || all["directness"].Value != "high" {
		t.Fatalf("expected only the active directness dimension with empty context, got %+v", all)
	}

	withUncertain, err := GetCommunicationProfile(store, UserModelActorCognition, UserModelQueryContext{IncludeUncertain: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(withUncertain) != 2 {
		t.Fatalf("expected both dimensions with IncludeUncertain, got %+v", withUncertain)
	}

	scoped, err := GetCommunicationProfile(store, UserModelActorCognition, UserModelQueryContext{Topics: []string{"directness"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(scoped) != 1 {
		t.Fatalf("expected only the requested dimension, got %+v", scoped)
	}
}

func TestUserModelQueries_RespectReadBoundary(t *testing.T) {
	store := testModelForQueries(t)
	if _, err := GetRelevantPreferences(store, UserModelActorExternal, UserModelQueryContext{Topics: []string{"concise"}}); err == nil {
		t.Fatal("expected an external actor to be denied")
	}
	if _, _, _, err := GetCapabilityLevel(store, UserModelActorExternal, "Blender"); err == nil {
		t.Fatal("expected an external actor to be denied")
	}
}

func TestCommunicationProfileValueToInteractionProfile(t *testing.T) {
	now := time.Now()
	src := UserModelSourceRef{Kind: UserSourceExplicitUser, ObservedAt: now}
	value := UserCommunicationProfileValue{
		Dimension: CommunicationDirectness, Value: "high", Confidence: 0.9,
		Source: src, Status: UserModelStatusActive, CreatedAt: now, UpdatedAt: now,
	}
	projected := CommunicationProfileValueToInteractionProfile(value)
	if projected.Value != "high" || projected.Confidence != 0.9 || projected.Status != UserModelStatusActive {
		t.Fatalf("unexpected projection: %+v", projected)
	}
}

func TestDetectUserModelQueryDoesNotCrossSelfOwnership(t *testing.T) {
	r := Analyze("¿Tú tienes color favorito?", Context{Language: "es"})
	if r.Subject != UserObservationSubjectSelf {
		t.Fatalf("subject=%q want self", r.Subject)
	}
	if _, ok := DetectUserModelQuery(r); ok {
		t.Fatal("self question must not route into user model")
	}
}
