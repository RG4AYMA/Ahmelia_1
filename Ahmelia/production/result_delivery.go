package production

import (
	"fmt"
	"time"

	"ahmelia/tools"
)

// B95.16 — Result & Delivery
// Production exposes the verified outcome without creating a second Result
// model. tools.Result / tools.VerifiedResult remain authoritative.

type DeliveryStatus string

const (
	DeliveryPending   DeliveryStatus = "pending"
	DeliveryDelivered DeliveryStatus = "delivered"
	DeliveryFailed    DeliveryStatus = "failed"
)

type ProductionResult struct {
	ProductionID string
	GoalID       string
	TaskID       string
	ActionID     string
	Success      bool
	Output       string
	Reason       string
	Evidence     []tools.Attempt
	DeliveredAt  time.Time
}

// ResultDelivery turns a verified execution into a production-facing result.
// It does not execute, verify, or persist memory by itself.
type ResultDelivery struct{}

func NewResultDelivery() *ResultDelivery { return &ResultDelivery{} }

func (d *ResultDelivery) Deliver(productionID, goalID, taskID string, verified tools.VerifiedResult) (ProductionResult, error) {
	if d == nil {
		return ProductionResult{}, fmt.Errorf("production: result delivery unavailable")
	}
	if productionID == "" || goalID == "" || taskID == "" {
		return ProductionResult{}, fmt.Errorf("production: production, goal y task son obligatorios")
	}
	if verified.ActionID == "" {
		return ProductionResult{}, fmt.Errorf("production: verified result action ID vacío")
	}
	if !verified.Success {
		return ProductionResult{}, fmt.Errorf("production: no se puede entregar un resultado no verificado: %s", verified.Reason)
	}
	output := ""
	if len(verified.Attempts) > 0 {
		output = verified.Attempts[len(verified.Attempts)-1].Execution.Output
	}
	return ProductionResult{ProductionID: productionID, GoalID: goalID, TaskID: taskID, ActionID: verified.ActionID, Success: true, Output: output, Reason: verified.Reason, Evidence: append([]tools.Attempt(nil), verified.Attempts...), DeliveredAt: time.Now()}, nil
}

// DeliveryStatusOf classifies a production result without changing it.
func DeliveryStatusOf(r ProductionResult) DeliveryStatus {
	if r.DeliveredAt.IsZero() {
		return DeliveryPending
	}
	if r.Success {
		return DeliveryDelivered
	}
	return DeliveryFailed
}
