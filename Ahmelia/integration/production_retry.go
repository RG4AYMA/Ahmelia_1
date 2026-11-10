package integration

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"

	"ahmelia/interaction"
	"ahmelia/production"
)

// Recovery appends evidence to Objective.Text. The original objective and
// canonical frame remain immutable, including all acceptance conditions.
func (r *ProductionRuntime) productionRetryContract(workID string, req interaction.SemanticRequest) string {
	objective := req.Objective
	objective.Text = strings.TrimSpace(objective.Text)
	if objective.Text == "" {
		objective.Text = strings.TrimSpace(req.Raw)
	}
	if work, ok := r.work.Get(workID); ok {
		original := strings.TrimSpace(work.Objective.Text)
		// Only an appended suffix after the ENTIRE stored objective is recovery
		// metadata. A marker in the owner's original text is never discarded.
		if original != "" && strings.HasPrefix(objective.Text, original+"\n\n[Recovery evidence]\n") {
			objective.Text = original
		}
	}
	frame := req.Frame
	// Confidence and provenance describe interpretation, not acceptance.
	frame.Confidence, frame.Provenance = 0, ""
	data, _ := json.Marshal(struct {
		Objective interaction.Objective
		Frame     interaction.ObjectiveFrame
	}{objective, frame})
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

func (r *ProductionRuntime) retryBudgetForWork(workID string, req interaction.SemanticRequest, options production.ExecutionOptions) *production.RetryBudget {
	// The durable Work is authoritative over stale or freshly supplied options.
	if work, ok := r.work.Get(workID); ok && work.LastActionResult != nil && work.LastActionResult.RetryState != nil {
		return production.RestoreRetryBudget(*work.LastActionResult.RetryState)
	}
	if options.RetryState != nil {
		return production.RestoreRetryBudget(*options.RetryState)
	}
	return production.NewRetryBudget(options.RetryPolicy, r.productionRetryContract(workID, req))
}
