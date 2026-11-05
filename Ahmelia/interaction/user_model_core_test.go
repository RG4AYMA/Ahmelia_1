package interaction

import (
	"testing"
	"time"
)

func TestUserModelCoreStartsEmptyAndValid(t *testing.T) {
	m := NewUserModel("user-1")
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	if m.UserID != "user-1" || len(m.Attributes) != 0 || len(m.Preferences) != 0 || len(m.Goals) != 0 || len(m.Capabilities) != 0 || len(m.Knowledge) != 0 || len(m.InteractionPatterns) != 0 {
		t.Fatalf("expected empty formal model, got %+v", m)
	}
	if m.InteractionProfile.Values == nil {
		t.Fatal("interaction profile map must be initialized")
	}
}

func TestUserModelValidationRejectsInvalidStructure(t *testing.T) {
	m := NewUserModel("user-1")
	m.Attributes = append(m.Attributes, UserAttribute{Key: "", Value: "x", Confidence: 0.5, Status: UserModelStatusActive, Provenance: UserModelSourceRef{Kind: UserSourceObserved}})
	if err := m.Validate(); err == nil {
		t.Fatal("expected validation error for empty attribute key")
	}

	m = NewUserModel("user-1")
	m.Confidence = 1.1
	if err := m.Validate(); err == nil {
		t.Fatal("expected validation error for invalid model confidence")
	}
}

func TestUserModelAccessPolicyIsExplicit(t *testing.T) {
	p := UserModelAccessPolicy{}
	for _, actor := range []UserModelActorKind{UserModelActorInteraction, UserModelActorCognition, UserModelActorMemory, UserModelActorSystem} {
		if !p.Allow(UserModelAccessRequest{Actor: actor, Mode: UserModelAccessRead}) {
			t.Fatalf("expected %q to read", actor)
		}
	}
	for _, actor := range []UserModelActorKind{UserModelActorUser, UserModelActorExternal} {
		if p.Allow(UserModelAccessRequest{Actor: actor, Mode: UserModelAccessRead}) {
			t.Fatalf("did not expect %q to have implicit model read access", actor)
		}
	}
	if !p.Allow(UserModelAccessRequest{Actor: UserModelActorMemory, Mode: UserModelAccessModify}) ||
		!p.Allow(UserModelAccessRequest{Actor: UserModelActorSystem, Mode: UserModelAccessModify}) {
		t.Fatal("expected only model update boundaries to modify")
	}
	if p.Allow(UserModelAccessRequest{Actor: UserModelActorCognition, Mode: UserModelAccessModify}) {
		t.Fatal("cognition must not mutate the user model directly")
	}
}

func TestUserModelSnapshotIsDetached(t *testing.T) {
	now := time.Now()
	m := NewUserModel("user-1")
	m.InteractionProfile.Values["response_length"] = UserInteractionProfileValue{
		Value: "medium", Confidence: 0.8, Status: UserModelStatusActive,
		Provenance: UserModelSourceRef{Kind: UserSourceExplicitUser}, UpdatedAt: now,
	}
	view := m.Snapshot()
	view.Model.InteractionProfile.Values["response_length"] = UserInteractionProfileValue{Value: "changed"}
	if m.InteractionProfile.Values["response_length"].Value != "medium" {
		t.Fatal("snapshot mutated original interaction profile")
	}
}

func TestUserModelStoreEnforcesBoundary(t *testing.T) {
	store, err := NewUserModelStore(NewUserModel("user-1"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Read(UserModelAccessRequest{Actor: UserModelActorCognition, Mode: UserModelAccessRead}); err != nil {
		t.Fatal(err)
	}
	if err := store.Replace(UserModelAccessRequest{Actor: UserModelActorCognition, Mode: UserModelAccessModify}, NewUserModel("user-1")); err == nil {
		t.Fatal("cognition must not directly replace the model")
	}
	updated := NewUserModel("user-1")
	updated.UpdatedAt = time.Now()
	if err := store.Replace(UserModelAccessRequest{Actor: UserModelActorMemory, Mode: UserModelAccessModify}, updated); err != nil {
		t.Fatal(err)
	}
	if err := store.Replace(UserModelAccessRequest{Actor: UserModelActorSystem, Mode: UserModelAccessModify}, NewUserModel("other-user")); err == nil {
		t.Fatal("replace must not change user identity")
	}
}
