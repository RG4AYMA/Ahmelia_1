package interaction

import (
	"context"
	"fmt"
	"strings"

	"ahmelia/shared"
)

// InitiativeSimilarityGuard is an optional, evidence-only check that detects
// when a candidate autonomous initiative response is a semantic restatement
// of a recent assistant turn, even when the text is not literally identical.
// It never authorizes or blocks an action by itself — the caller decides what
// to do with the boolean result, and must fail open (allow the turn) whenever
// this guard is unavailable or errors, exactly like the existing literal
// isDuplicateInitiativeResponse check in core/engine.go already does for its
// own failure modes.
type InitiativeSimilarityGuard struct {
	Provider      EmbeddingProvider
	Threshold     float64 // cosine similarity at/above which two turns count as duplicates
	LookbackTurns int     // how many trailing assistant turns to compare against
}

// NewInitiativeSimilarityGuard builds a guard with the accepted defaults:
// Threshold=0.86, LookbackTurns=3. These defaults are deliberately more
// conservative (harder to trigger) than the Phase C proposal pilot's 0.78,
// because a false positive here silently deletes a real conversational turn,
// which is worse than a missed duplicate.
func NewInitiativeSimilarityGuard(provider EmbeddingProvider) *InitiativeSimilarityGuard {
	return &InitiativeSimilarityGuard{Provider: provider, Threshold: 0.86, LookbackTurns: 3}
}

// IsSemanticDuplicate returns true only when Provider is available AND the
// candidate's embedding scores at/above Threshold cosine similarity against
// any of the last LookbackTurns non-empty assistant utterances in snap.Turns.
//
// Error contract (must be respected exactly by the caller):
//   - err != nil means the check could not be performed (provider down,
//     timeout, empty candidate normalizes to ""). The caller MUST treat this
//     as "not a duplicate" and fall back to the existing literal check only.
//   - This guard must never be able to block an initiative turn on its own
//     failure — only on a positive, successful similarity match.
func (g *InitiativeSimilarityGuard) IsSemanticDuplicate(ctx context.Context, candidate string, snap ConversationSnapshot) (bool, error) {
	if g == nil || g.Provider == nil {
		return false, fmt.Errorf("initiative similarity guard unavailable")
	}
	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return false, fmt.Errorf("empty candidate")
	}
	candVec, err := g.Provider.Embed(ctx, shared.Normalize(candidate))
	if err != nil {
		return false, err
	}
	lookback := g.LookbackTurns
	if lookback <= 0 {
		lookback = 3
	}
	threshold := g.Threshold
	if threshold <= 0 {
		threshold = 0.86
	}
	n := len(snap.Turns)
	start := n - lookback
	if start < 0 {
		start = 0
	}
	for _, t := range snap.Turns[start:] {
		prev := strings.TrimSpace(t.Assistant)
		if prev == "" {
			continue
		}
		prevVec, embedErr := g.Provider.Embed(ctx, shared.Normalize(prev))
		if embedErr != nil {
			// Skip only this one comparison turn; do not abort the whole
			// check because one historical embedding call failed.
			continue
		}
		score, ok := cosineSimilarity(candVec, prevVec)
		if ok && score >= threshold {
			return true, nil
		}
	}
	return false, nil
}
