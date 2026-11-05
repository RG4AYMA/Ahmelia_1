package programming

import (
	"fmt"
	"strings"
	"time"
)

// EffectEvaluation records a descriptive evaluation of one expected
// programming change against one concrete primary observation, in the
// context of one WorkAttempt.
//
// The WorkID and AttemptNumber identify the operational context to which the
// expected change belongs. They do not assert that the attempt caused the
// observed state. Observed is a snapshot of evidence; Judgment is the
// descriptive compatibility result produced from Expected + Observed.
//
// This contract deliberately contains no execution result, completion state,
// authorization, recovery decision, retry instruction, or causal attribution.
type EffectEvaluation struct {
	ID               string                `json:"id"`
	WorkID           string                `json:"work_id"`
	AttemptNumber    int                   `json:"attempt_number"`
	ExpectedChangeID string                `json:"expected_change_id"`
	Observed         ObservedFile          `json:"observed"`
	Judgment         CompatibilityJudgment `json:"judgment"`
	EvaluatedAt      time.Time             `json:"evaluated_at"`
}

// Validate checks the identity and temporal invariants of an evaluation.
// It deliberately does not infer causality or validate execution state.
func (e EffectEvaluation) Validate() error {
	if strings.TrimSpace(e.ID) == "" {
		return fmt.Errorf("programming effect evaluation: ID vacío")
	}
	if strings.TrimSpace(e.WorkID) == "" {
		return fmt.Errorf("programming effect evaluation %q: WorkID vacío", e.ID)
	}
	if e.AttemptNumber < 1 {
		return fmt.Errorf("programming effect evaluation %q: AttemptNumber inválido", e.ID)
	}
	if strings.TrimSpace(e.ExpectedChangeID) == "" {
		return fmt.Errorf("programming effect evaluation %q: ExpectedChangeID vacío", e.ID)
	}
	if strings.TrimSpace(e.Observed.Source) == "" {
		return fmt.Errorf("programming effect evaluation %q: observation source vacío", e.ID)
	}
	if strings.TrimSpace(e.Observed.Path) == "" {
		return fmt.Errorf("programming effect evaluation %q: observation path vacío", e.ID)
	}
	if e.Observed.ObservedAt.IsZero() {
		return fmt.Errorf("programming effect evaluation %q: observation timestamp vacío", e.ID)
	}
	if e.EvaluatedAt.IsZero() {
		return fmt.Errorf("programming effect evaluation %q: evaluation timestamp vacío", e.ID)
	}
	return nil
}
