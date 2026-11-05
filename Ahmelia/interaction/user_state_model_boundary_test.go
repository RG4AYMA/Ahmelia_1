package interaction

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
	"time"
)

func TestDescribeUserStateForModelDropsRawSignals(t *testing.T) {
	now := time.Now()
	s := UserState{Frustration: 0.93, Enthusiasm: 0.87, InteractionNeed: NeedAssistance}
	ref := DescribeUserStateForModel(s, now)

	if !ref.ObservedAt.Equal(now) {
		t.Fatalf("expected observed_at %v, got %v", now, ref.ObservedAt)
	}
	if ref.InteractionNeed != NeedAssistance {
		t.Fatalf("expected interaction need to carry across, got %q", ref.InteractionNeed)
	}
	if ref.Note == "" {
		t.Fatal("expected a non-empty audit note")
	}
	// The whole point of the boundary: no raw numeric signal leaks through as
	// a field on the snapshot ref. UserStateSnapshotRef simply has no such
	// fields, so this is enforced by the type itself; here we just confirm
	// the note is a coarse description, not a serialization of the state.
	if ref.Note == "0.93" {
		t.Fatal("note must not be a raw signal value")
	}
}

func TestDescribeUserStateForModelNoSalientSignal(t *testing.T) {
	ref := DescribeUserStateForModel(UserState{}, time.Now())
	if ref.Note != "no salient state signal" {
		t.Fatalf("expected neutral note for empty state, got %q", ref.Note)
	}
}

func TestUserStateSourceRefIsAlwaysObserved(t *testing.T) {
	ref := UserStateSnapshotRef{ObservedAt: time.Now(), Note: "elevated urgency signal"}
	src, err := UserStateSourceRef(ref, "turn-42", "ev-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if src.Kind != UserSourceObserved {
		t.Fatalf("expected UserSourceObserved, got %q", src.Kind)
	}
	if src.SourceID != "turn-42" || src.EvidenceID != "ev-1" {
		t.Fatalf("expected source/evidence ids to carry through, got %+v", src)
	}
	if !src.ObservedAt.Equal(ref.ObservedAt) {
		t.Fatal("expected observed_at to carry through")
	}
}

func TestUserStateSourceRefRejectsMissingSourceID(t *testing.T) {
	ref := UserStateSnapshotRef{ObservedAt: time.Now()}
	if _, err := UserStateSourceRef(ref, "", "ev-1"); err == nil {
		t.Fatal("expected error for missing source id")
	}
}

func TestUserStateSourceRefRejectsZeroObservedAt(t *testing.T) {
	if _, err := UserStateSourceRef(UserStateSnapshotRef{}, "turn-1", ""); err == nil {
		t.Fatal("expected error for zero observed_at")
	}
}

func TestRejectPersistentTraitFromSingleStateBlocksActive(t *testing.T) {
	if err := RejectPersistentTraitFromSingleState(UserModelStatusActive, 1); err == nil {
		t.Fatal("expected a single state-derived observation to be rejected for Active status")
	}
	if err := RejectPersistentTraitFromSingleState(UserModelStatusActive, 0); err == nil {
		t.Fatal("expected zero evidence to also be rejected for Active status")
	}
}

func TestRejectPersistentTraitFromSingleStateAllowsUncertain(t *testing.T) {
	if err := RejectPersistentTraitFromSingleState(UserModelStatusUncertain, 1); err != nil {
		t.Fatalf("expected Uncertain from a single observation to be allowed, got %v", err)
	}
}

func TestRejectPersistentTraitFromSingleStateAllowsActiveWithAccumulatedEvidence(t *testing.T) {
	if err := RejectPersistentTraitFromSingleState(UserModelStatusActive, 5); err != nil {
		t.Fatalf("expected Active with accumulated evidence to be allowed, got %v", err)
	}
}

// TestUserStateFileNeverReferencesModelMutationAPI is the executable form of
// B96.34.9's core rule: user_state.go computes UserState for the current
// turn and must never itself reach into UserModel construction, evaluation,
// or storage. This mirrors the B96.22.31 architecture-audit precedent
// (TestB9631InteractionHasNoDomainImplementationImports) but polices an
// intra-package seam rather than a cross-package import.
func TestUserStateFileNeverReferencesModelMutationAPI(t *testing.T) {
	forbiddenIdents := map[string]bool{
		"UserModel":                              true,
		"NewUserModel":                           true,
		"UserModelStore":                         true,
		"NewUserModelStore":                      true,
		"UserAttributeRecord":                    true,
		"UserPreferenceRecord":                   true,
		"UserGoalRecord":                         true,
		"UserCapabilityRecord":                   true,
		"UserKnowledgeRecord":                    true,
		"UserInteractionPatternRecord":           true,
		"UserCommunicationProfileValue":          true,
		"EvaluateUserAttributeUpdate":            true,
		"EvaluateUserPreferenceUpdate":           true,
		"EvaluateUserGoalUpdate":                 true,
		"EvaluateUserCapabilityUpdate":           true,
		"EvaluateUserKnowledgeUpdate":            true,
		"EvaluateUserCommunicationProfileUpdate": true,
		"ReinforceUserInteractionPattern":        true,
	}

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "user_state.go", nil, 0)
	if err != nil {
		t.Fatalf("parse user_state.go: %v", err)
	}

	ast.Inspect(f, func(n ast.Node) bool {
		if ident, ok := n.(*ast.Ident); ok {
			if forbiddenIdents[ident.Name] {
				t.Fatalf("user_state.go references %q, which belongs to the UserModel mutation API; B96.34.9 requires all state-to-model traffic to go through user_state_model_boundary.go", ident.Name)
			}
		}
		return true
	})
}

// TestUserModelCoreFileNeverReferencesUserStateType guards the seam from the
// other direction: user_model_core.go documents the separation in prose but
// must not import the UserState type into any of its structural definitions
// or logic. Only user_state_model_boundary.go is permitted to reference both
// sides.
func TestUserModelCoreFileNeverReferencesUserStateType(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "user_model_core.go", nil, 0)
	if err != nil {
		t.Fatalf("parse user_model_core.go: %v", err)
	}

	ast.Inspect(f, func(n ast.Node) bool {
		if ident, ok := n.(*ast.Ident); ok && ident.Name == "UserState" {
			t.Fatal("user_model_core.go references UserState directly; the boundary must be crossed only through user_state_model_boundary.go")
		}
		return true
	})
}
