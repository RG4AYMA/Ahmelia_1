package integration

import (
	"ahmelia/interaction"
	"ahmelia/production"
)

// WorkStorePort is the minimal persistent-work contract consumed by the
// production runtime. It deliberately exposes state operations only; it does
// not expose capability selection, authorization, execution, or recovery.
//
// The concrete storage remains production.WorkStore for compatibility, while
// Core can own the live instance through an adapter without creating a second
// persistence implementation.
type WorkStorePort interface {
	Begin(interaction.SemanticRequest) (production.WorkSession, error)
	AttachPlanning(string, string, string) error
	AttachProduction(string, string, string, string) error
	RecordArtifact(string, production.ProductionArtifact) error
	Block(string, string) error
	Complete(string, bool, string) error
	SetProgress(string, string, []string, []string) error
	RecordActionResult(string, production.ActionResult) error
	BeginAttempt(string, string, string, string, ...string) (int, error)
	SetAttemptTaskID(string, int, string) error
	SetAttemptExecutionIdentity(string, int, string, string) error
	SetAttemptEffectSpecification(string, int, []byte) error
	RecordAttemptEffectEvaluation(string, int, []byte) error
	ResumeAttempt(string, int) error
	SetPendingExecution(string, production.PendingExecution) error
	RejectPendingExecution(string, string) error
	ClearPendingExecution(string) error
	PendingExecution(string) (production.PendingExecution, bool)
	CompleteAttempt(string, int, production.ActionResult, string, string) error
	FinalizeAttempt(string, int, production.ActionResult, string, string, production.WorkStatus, string) error
	FinalizeRepairAttempt(string, int, production.ActionResult, string, string) error
	ReconcileUnknownAttempt(string, int, production.ActionResult) error
	LastAttempt(string) (production.WorkAttempt, bool)
	UpdateObjective(string, string, string) error
	Get(string) (production.WorkSession, bool)
	Last() (production.WorkSession, bool)
	FindArtifact(string) (production.ProductionArtifact, bool)
	RecentResult() (string, bool)
}
