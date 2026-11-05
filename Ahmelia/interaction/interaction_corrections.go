package interaction

import (
	"strings"
	"time"
)

// InteractionCorrection is the interaction-layer projection of a semantic
// correction. It makes the repair explicit to consumers of the active
// interaction without introducing a second correction history.
//
// The semantic request remains authoritative for interpretation and
// ConversationState remains authoritative for historical turns. This type is
// only the current interaction read model.
type InteractionCorrection struct {
	IsCorrection bool
	IsNegation   bool
	Target       string
	Replacement  string
	Reason       string
	Confidence   float64
	SourceTurn   uint64
	Applied      bool
}

// ActiveCorrection returns the correction currently participating in the
// interaction. It is detached from internal state and therefore safe for
// callers to inspect or modify locally.
func (s *InteractionState) ActiveCorrection() InteractionCorrection {
	if s == nil {
		return InteractionCorrection{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return projectInteractionCorrection(
		s.snapshot.Semantic.Correction,
		s.snapshot.Semantic.CorrectionTarget,
		s.snapshot.Metadata.TurnID,
		s.snapshot.Metadata.UpdatedAt,
	)
}

func projectInteractionCorrection(c CorrectionInfo, target string, turnID uint64, _ time.Time) InteractionCorrection {
	if !c.IsCorrection {
		return InteractionCorrection{}
	}
	target = strings.TrimSpace(target)
	if target == "" {
		target = strings.TrimSpace(c.Target)
	}
	replacement := strings.TrimSpace(c.Replacement)
	return InteractionCorrection{
		IsCorrection: true,
		IsNegation:   c.IsNegation,
		Target:       target,
		Replacement:  replacement,
		Reason:       strings.TrimSpace(c.Reason),
		Confidence:   c.Confidence,
		SourceTurn:   turnID,
		Applied:      replacement != "",
	}
}
