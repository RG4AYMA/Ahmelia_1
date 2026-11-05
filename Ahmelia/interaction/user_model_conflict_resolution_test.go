package interaction

import (
	"testing"
	"time"
)

func conflictItem(key, value, context string, kind UserModelSourceKind, confidence float64, evidence int, status UserModelItemStatus, at time.Time) UserModelConflictItem {
	return UserModelConflictItem{
		UserModelUpdateItem: UserModelUpdateItem{
			Key: key, Value: value, Confidence: confidence, EvidenceCount: evidence,
			Provenance: UserModelSourceRef{Kind: kind, ObservedAt: at},
			Status:     status, CreatedAt: at, UpdatedAt: at,
		},
		Context: context,
	}
}

func TestResolveUserModelConflict_ContextSplitKeepsBothScoped(t *testing.T) {
	t0 := time.Now()
	t1 := t0.Add(time.Hour)
	// The doc's own example: "deep" at work, "short" casually. Neither
	// outranks the other because they are not actually disagreeing about
	// the same thing.
	candidates := []UserModelConflictItem{
		conflictItem("response_depth", "deep", "technical_work", UserSourceExplicitUser, 0.9, 3, UserModelStatusActive, t0),
		conflictItem("response_depth", "short", "casual", UserSourceExplicitUser, 0.9, 3, UserModelStatusActive, t1),
	}

	outcome, err := ResolveUserModelConflict(candidates, DefaultUserModelConflictPolicy(), t1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outcome.Kind != UserModelConflictContextSplit {
		t.Fatalf("expected context_split, got %s", outcome.Kind)
	}
	if outcome.PartiallyUnresolved {
		t.Fatalf("expected both contexts to resolve cleanly")
	}
	if len(outcome.Superseded) != 0 {
		t.Fatalf("expected nothing superseded across genuinely different contexts, got %+v", outcome.Superseded)
	}
	if len(outcome.Resolved) != 2 {
		t.Fatalf("expected both context-scoped beliefs to remain live, got %d", len(outcome.Resolved))
	}
	foundWork, foundCasual := false, false
	for _, r := range outcome.Resolved {
		if r.Context == "technical_work" && r.Value == "deep" && r.Status == UserModelStatusActive {
			foundWork = true
		}
		if r.Context == "casual" && r.Value == "short" && r.Status == UserModelStatusActive {
			foundCasual = true
		}
	}
	if !foundWork || !foundCasual {
		t.Fatalf("expected both context-scoped active beliefs, got %+v", outcome.Resolved)
	}
}

func TestResolveUserModelConflict_ContextSplitPromotesHeldUncertainToActive(t *testing.T) {
	// Mirrors what B96.34.11 actually hands off: the prior belief stayed
	// Active, the new disagreeing one was forced Uncertain purely because
	// the contradiction looked unresolved. Once B96.34.13 sees they are in
	// different contexts, the held item should be promotable back to
	// Active — it has enough evidence, the uncertainty was never about its
	// own merit.
	t0 := time.Now()
	t1 := t0.Add(time.Hour)
	candidates := []UserModelConflictItem{
		conflictItem("response_depth", "deep", "technical_work", UserSourceExplicitUser, 0.91, 4, UserModelStatusActive, t0),
		conflictItem("response_depth", "short", "casual", UserSourceExplicitUser, 0.90, 3, UserModelStatusUncertain, t1),
	}

	outcome, err := ResolveUserModelConflict(candidates, DefaultUserModelConflictPolicy(), t1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, r := range outcome.Resolved {
		if r.Status != UserModelStatusActive {
			t.Fatalf("expected every resolved context-scoped belief to be active, got %+v", r)
		}
	}
}

func TestResolveUserModelConflict_ContextSplitDoesNotPromoteSingleWeakObservation(t *testing.T) {
	t0 := time.Now()
	t1 := t0.Add(time.Hour)
	candidates := []UserModelConflictItem{
		conflictItem("response_depth", "deep", "technical_work", UserSourceExplicitUser, 0.91, 4, UserModelStatusActive, t0),
		// A single inferred observation in a new context: the B96.34.9-style
		// single-observation guard must still hold even after a context
		// split resolves the apparent contradiction.
		conflictItem("response_depth", "short", "casual", UserSourceInferred, 0.85, 1, UserModelStatusUncertain, t1),
	}

	outcome, err := ResolveUserModelConflict(candidates, DefaultUserModelConflictPolicy(), t1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, r := range outcome.Resolved {
		if r.Context == "casual" && r.Status != UserModelStatusUncertain {
			t.Fatalf("expected the single weak observation to stay uncertain, got %+v", r)
		}
	}
}

func TestResolveUserModelConflict_GenuineContradictionSupersedesLowerPriority(t *testing.T) {
	t0 := time.Now()
	t1 := t0.Add(time.Hour)
	// Same (absent) context on both sides: this is exactly the
	// single-truth case B96.34.11 already ranks by source priority.
	candidates := []UserModelConflictItem{
		conflictItem("preferred_response_length", "short", "", UserSourceInferred, 0.86, 3, UserModelStatusActive, t0),
		conflictItem("preferred_response_length", "long", "", UserSourceExplicitUser, 0.95, 1, UserModelStatusActive, t1),
	}

	outcome, err := ResolveUserModelConflict(candidates, DefaultUserModelConflictPolicy(), t1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outcome.Kind != UserModelConflictGenuineContradiction {
		t.Fatalf("expected genuine_contradiction, got %s", outcome.Kind)
	}
	if len(outcome.Resolved) != 1 || outcome.Resolved[0].Value != "long" {
		t.Fatalf("expected the explicit statement to win, got %+v", outcome.Resolved)
	}
	if len(outcome.Superseded) != 1 || outcome.Superseded[0].Value != "short" || outcome.Superseded[0].Status != UserModelStatusSuperseded {
		t.Fatalf("expected the inferred belief to be marked superseded, not deleted, got %+v", outcome.Superseded)
	}
	if !outcome.Superseded[0].UpdatedAt.Equal(t1) {
		t.Fatalf("expected the superseded item's timestamp to be stamped with now, got %v", outcome.Superseded[0].UpdatedAt)
	}
}

func TestResolveUserModelConflict_UnresolvedWhenEvenlyMatched(t *testing.T) {
	t0 := time.Now()
	t1 := t0.Add(time.Hour)
	// Same priority tier, confidence gap under the default 0.20 margin:
	// neither outranks the other, so both must remain live but honestly
	// uncertain rather than one silently winning.
	candidates := []UserModelConflictItem{
		conflictItem("humor_tolerance", "low", "", UserSourceInferred, 0.88, 3, UserModelStatusActive, t0),
		conflictItem("humor_tolerance", "high", "", UserSourceInferred, 0.90, 3, UserModelStatusUncertain, t1),
	}

	outcome, err := ResolveUserModelConflict(candidates, DefaultUserModelConflictPolicy(), t1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outcome.Kind != UserModelConflictUnresolved {
		t.Fatalf("expected unresolved, got %s", outcome.Kind)
	}
	if len(outcome.Superseded) != 0 {
		t.Fatalf("expected nothing superseded on a genuine tie, got %+v", outcome.Superseded)
	}
	if len(outcome.Resolved) != 2 {
		t.Fatalf("expected both tied beliefs to remain live, got %d", len(outcome.Resolved))
	}
	for _, r := range outcome.Resolved {
		if r.Status != UserModelStatusUncertain {
			t.Fatalf("expected both tied beliefs to be uncertain (no sé), got %+v", r)
		}
	}
}

func TestResolveUserModelConflict_PartiallyUnresolvedContextSplit(t *testing.T) {
	t0 := time.Now()
	t1 := t0.Add(time.Hour)
	t2 := t0.Add(2 * time.Hour)
	candidates := []UserModelConflictItem{
		conflictItem("response_depth", "deep", "technical_work", UserSourceExplicitUser, 0.95, 2, UserModelStatusActive, t0),
		// The "casual" context itself has an internal, evenly matched tie.
		conflictItem("response_depth", "short", "casual", UserSourceInferred, 0.88, 3, UserModelStatusActive, t1),
		conflictItem("response_depth", "medium", "casual", UserSourceInferred, 0.89, 3, UserModelStatusUncertain, t2),
	}

	outcome, err := ResolveUserModelConflict(candidates, DefaultUserModelConflictPolicy(), t2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outcome.Kind != UserModelConflictContextSplit {
		t.Fatalf("expected context_split, got %s", outcome.Kind)
	}
	if !outcome.PartiallyUnresolved {
		t.Fatalf("expected the casual context's internal tie to be flagged")
	}
	if len(outcome.Groups) != 2 {
		t.Fatalf("expected two context groups, got %d", len(outcome.Groups))
	}
	for _, g := range outcome.Groups {
		if g.Context == "technical_work" && !g.Resolved {
			t.Fatalf("expected technical_work to resolve cleanly, got %+v", g)
		}
		if g.Context == "casual" && g.Resolved {
			t.Fatalf("expected casual to remain unresolved internally, got %+v", g)
		}
	}
}

func TestResolveUserModelConflict_AgreeingDuplicatesInSameContextAreNotAmbiguous(t *testing.T) {
	t0 := time.Now()
	t1 := t0.Add(time.Hour)
	// Two candidates in the same context that actually agree on value are
	// not a conflict at all — just a redundant live copy left over from
	// repeated evidence. Keep the latest, supersede the rest.
	candidates := []UserModelConflictItem{
		conflictItem("response_depth", "deep", "technical_work", UserSourceExplicitUser, 0.9, 2, UserModelStatusActive, t0),
		conflictItem("response_depth", "deep", "technical_work", UserSourceExplicitUser, 0.93, 3, UserModelStatusActive, t1),
	}

	outcome, err := ResolveUserModelConflict(candidates, DefaultUserModelConflictPolicy(), t1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outcome.Kind != UserModelConflictGenuineContradiction {
		t.Fatalf("expected a single context group to report as genuine_contradiction (trivially resolved), got %s", outcome.Kind)
	}
	if len(outcome.Resolved) != 1 || !outcome.Resolved[0].UpdatedAt.Equal(t1) {
		t.Fatalf("expected the most recently updated duplicate to remain live, got %+v", outcome.Resolved)
	}
	if len(outcome.Superseded) != 1 {
		t.Fatalf("expected the older duplicate to be superseded, got %+v", outcome.Superseded)
	}
}

func TestResolveUserModelConflict_RejectsFewerThanTwoCandidates(t *testing.T) {
	now := time.Now()
	single := []UserModelConflictItem{
		conflictItem("k", "v", "", UserSourceExplicitUser, 0.9, 1, UserModelStatusActive, now),
	}
	if _, err := ResolveUserModelConflict(single, DefaultUserModelConflictPolicy(), now); err == nil {
		t.Fatal("expected an error for fewer than two candidates")
	}
}

func TestResolveUserModelConflict_RejectsMismatchedKeys(t *testing.T) {
	now := time.Now()
	candidates := []UserModelConflictItem{
		conflictItem("a", "v1", "", UserSourceExplicitUser, 0.9, 1, UserModelStatusActive, now),
		conflictItem("b", "v2", "", UserSourceExplicitUser, 0.9, 1, UserModelStatusActive, now),
	}
	if _, err := ResolveUserModelConflict(candidates, DefaultUserModelConflictPolicy(), now); err == nil {
		t.Fatal("expected an error for mismatched keys")
	}
}

func TestResolveUserModelConflict_RejectsNonLiveCandidate(t *testing.T) {
	now := time.Now()
	candidates := []UserModelConflictItem{
		conflictItem("k", "v1", "", UserSourceExplicitUser, 0.9, 1, UserModelStatusSuperseded, now),
		conflictItem("k", "v2", "", UserSourceExplicitUser, 0.9, 1, UserModelStatusActive, now),
	}
	if _, err := ResolveUserModelConflict(candidates, DefaultUserModelConflictPolicy(), now); err == nil {
		t.Fatal("expected an error for a non-live candidate")
	}
}

func TestResolveUserModelConflict_RejectsInvalidPolicy(t *testing.T) {
	now := time.Now()
	candidates := []UserModelConflictItem{
		conflictItem("k", "v1", "", UserSourceExplicitUser, 0.9, 1, UserModelStatusActive, now),
		conflictItem("k", "v2", "", UserSourceExplicitUser, 0.9, 1, UserModelStatusActive, now),
	}
	bad := UserModelConflictPolicy{ContradictionConfidenceMargin: -0.1}
	if _, err := ResolveUserModelConflict(candidates, bad, now); err == nil {
		t.Fatal("expected an error for an out-of-range contradiction margin")
	}
}

func TestResolveUserModelConflict_RejectsInvalidCandidate(t *testing.T) {
	now := time.Now()
	candidates := []UserModelConflictItem{
		conflictItem("", "v1", "", UserSourceExplicitUser, 0.9, 1, UserModelStatusActive, now),
		conflictItem("", "v2", "", UserSourceExplicitUser, 0.9, 1, UserModelStatusActive, now),
	}
	if _, err := ResolveUserModelConflict(candidates, DefaultUserModelConflictPolicy(), now); err == nil {
		t.Fatal("expected an error for an empty key")
	}
}

func TestResolveUserModelConflict_DoesNotMutateInputSlice(t *testing.T) {
	t0 := time.Now()
	t1 := t0.Add(time.Hour)
	candidates := []UserModelConflictItem{
		conflictItem("k", "short", "", UserSourceInferred, 0.86, 3, UserModelStatusActive, t0),
		conflictItem("k", "long", "", UserSourceExplicitUser, 0.95, 1, UserModelStatusActive, t1),
	}

	_, err := ResolveUserModelConflict(candidates, DefaultUserModelConflictPolicy(), t1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if candidates[0].Value != "short" || candidates[0].Status != UserModelStatusActive {
		t.Fatalf("expected caller's slice to remain untouched, got %+v", candidates[0])
	}
}

func TestFindLiveConflicts_DetectsOnlyGenuineDisagreement(t *testing.T) {
	t0 := time.Now()
	t1 := t0.Add(time.Hour)
	t2 := t0.Add(2 * time.Hour)

	items := []UserModelUpdateItem{
		// Genuine live disagreement.
		{Key: "response_depth", Value: "deep", Confidence: 0.9, EvidenceCount: 2, Status: UserModelStatusActive, Provenance: UserModelSourceRef{Kind: UserSourceExplicitUser}, CreatedAt: t0, UpdatedAt: t0},
		{Key: "response_depth", Value: "short", Confidence: 0.9, EvidenceCount: 2, Status: UserModelStatusUncertain, Provenance: UserModelSourceRef{Kind: UserSourceExplicitUser}, CreatedAt: t1, UpdatedAt: t1},
		// Agreeing values at another key: not a conflict.
		{Key: "language", Value: "spanish", Confidence: 0.9, EvidenceCount: 2, Status: UserModelStatusActive, Provenance: UserModelSourceRef{Kind: UserSourceExplicitUser}, CreatedAt: t0, UpdatedAt: t0},
		{Key: "language", Value: "spanish", Confidence: 0.9, EvidenceCount: 2, Status: UserModelStatusActive, Provenance: UserModelSourceRef{Kind: UserSourceExplicitUser}, CreatedAt: t1, UpdatedAt: t1},
		// Settled history: superseded, not a live conflict.
		{Key: "focus_style", Value: "deep_work", Confidence: 0.9, EvidenceCount: 2, Status: UserModelStatusSuperseded, Provenance: UserModelSourceRef{Kind: UserSourceExplicitUser}, CreatedAt: t0, UpdatedAt: t0},
		{Key: "focus_style", Value: "shallow_work", Confidence: 0.9, EvidenceCount: 2, Status: UserModelStatusActive, Provenance: UserModelSourceRef{Kind: UserSourceExplicitUser}, CreatedAt: t2, UpdatedAt: t2},
	}

	conflicts := FindLiveConflicts(items)
	if len(conflicts) != 1 {
		t.Fatalf("expected exactly one key with a live conflict, got %d: %+v", len(conflicts), conflicts)
	}
	group, ok := conflicts["response_depth"]
	if !ok || len(group) != 2 {
		t.Fatalf("expected response_depth to be reported as conflicted, got %+v", conflicts)
	}
}

func TestValidateUserModelConflictItem(t *testing.T) {
	now := time.Now()
	valid := conflictItem("k", "v", "casual", UserSourceExplicitUser, 0.5, 1, UserModelStatusActive, now)
	if err := ValidateUserModelConflictItem(valid); err != nil {
		t.Fatalf("expected valid item to pass, got %v", err)
	}
	invalid := valid
	invalid.Key = ""
	if err := ValidateUserModelConflictItem(invalid); err == nil {
		t.Fatal("expected an error for an empty key")
	}
}
