package production

import (
	"fmt"
)

// B95.20 — Integration & Validation
//
// B95 closes by making the production contract auditable without introducing
// another execution authority. The validator is deliberately structural:
// runtime E2E behaviour remains covered by production tests so validation can
// report architecture/configuration failures deterministically.

type ValidationSeverity string

const (
	ValidationError ValidationSeverity = "error"
	ValidationWarn  ValidationSeverity = "warning"
)

type ValidationFinding struct {
	ID       string
	Severity ValidationSeverity
	Message  string
}

type ValidationReport struct {
	Ready    bool
	Findings []ValidationFinding
}

// ValidateProduction checks the minimum composition required for the B95
// production pipeline. It never executes a tool and never changes state.
func ValidateProduction(loop *ProductionLoop, tasks *TaskSystem, delivery *ResultDelivery) ValidationReport {
	r := ValidationReport{Ready: true}
	if loop == nil {
		r.add("LOOP", ValidationError, "ProductionLoop is nil")
		return r
	}
	if loop.production == nil {
		r.add("PRODUCTION", ValidationError, "Production context is nil")
	}
	if tasks == nil {
		r.add("TASKS", ValidationError, "TaskSystem is nil")
	}
	if loop.gateway == nil {
		r.add("EXECUTION", ValidationError, "ExecutionGateway is nil")
	}
	if loop.authorization == nil {
		r.add("AUTHORIZATION", ValidationError, "AuthorizationGate is nil")
	}
	if loop.observer == nil {
		r.add("OBSERVATION", ValidationError, "Observer is nil")
	}
	if loop.verifier == nil {
		r.add("VERIFICATION", ValidationError, "VerificationService is nil")
	}
	if delivery == nil || loop.delivery == nil {
		r.add("DELIVERY", ValidationError, "ResultDelivery is nil")
	}
	if loop.production != nil && loop.production.Goal == nil {
		r.add("GOAL", ValidationError, "Production has no Goal")
	}
	if loop.production != nil && loop.production.Plan == nil {
		r.add("PLAN", ValidationWarn, "Production has no Plan bound")
	}
	return r
}

func (r *ValidationReport) add(id string, severity ValidationSeverity, message string) {
	r.Findings = append(r.Findings, ValidationFinding{ID: id, Severity: severity, Message: message})
	if severity == ValidationError {
		r.Ready = false
	}
}

func (r ValidationReport) Error() error {
	if r.Ready {
		return nil
	}
	for _, f := range r.Findings {
		if f.Severity == ValidationError {
			return fmt.Errorf("production validation %s: %s", f.ID, f.Message)
		}
	}
	return nil
}
