package interaction

import (
	"fmt"
	"strings"
	"time"
)

// InteractionReference is the interaction-layer projection of a semantic
// reference. It gives references a stable identity inside the current turn
// without making InteractionState the owner of reference history.
//
// ConversationState remains authoritative for historical turns. The semantic
// parser remains authoritative for the meaning of the reference. This type is
// only the coordination/read model needed by later correction, switching and
// continuity blocks.
type InteractionReference struct {
	ID         string
	Kind       ReferenceKind
	Expression string
	Value      string
	Resolved   bool
	Confidence float64
	SourceTurn uint64
	UpdatedAt  time.Time
}

// ActiveReferences returns the references attached to the currently active
// semantic request. The returned slice is detached from internal state.
func (s *InteractionState) ActiveReferences() []InteractionReference {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return projectInteractionReferences(s.snapshot.Semantic.References, s.snapshot.Metadata.TurnID, s.snapshot.Metadata.UpdatedAt)
}

func projectInteractionReferences(refs []Reference, turnID uint64, updatedAt time.Time) []InteractionReference {
	if len(refs) == 0 {
		return nil
	}
	out := make([]InteractionReference, 0, len(refs))
	for i, ref := range refs {
		expression := strings.TrimSpace(ref.Expression)
		if ref.Kind == ReferenceNone && expression == "" {
			continue
		}
		out = append(out, InteractionReference{
			ID:         fmt.Sprintf("turn-%d-ref-%d", turnID, i+1),
			Kind:       ref.Kind,
			Expression: expression,
			Value:      strings.TrimSpace(ref.Value),
			Resolved:   ref.Resolved,
			Confidence: referenceConfidence(ref),
			SourceTurn: turnID,
			UpdatedAt:  updatedAt,
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// referenceConfidence preserves the existing boolean Reference contract while
// exposing a numeric signal for consumers that need to compare references.
// A later resolver can provide richer confidence without changing this API.
func referenceConfidence(ref Reference) float64 {
	if ref.Resolved {
		return 1.0
	}
	return 0.0
}
