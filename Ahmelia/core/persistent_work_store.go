package core

import (
	"ahmelia/interaction"
	"ahmelia/production"
)

// PersistentWorkStore is the Core-owned adapter around the single existing
// persistent WorkStore implementation. It provides one live owner to Engine
// and ProductionRuntime without creating a second JSON persistence format.
type PersistentWorkStore struct {
	store *production.WorkStore
}

func NewPersistentWorkStore(store *production.WorkStore) *PersistentWorkStore {
	return &PersistentWorkStore{store: store}
}

func (s *PersistentWorkStore) Begin(req interaction.SemanticRequest) (production.WorkSession, error) {
	return s.store.Begin(req)
}
func (s *PersistentWorkStore) AttachPlanning(id, goalID, planID string) error {
	return s.store.AttachPlanning(id, goalID, planID)
}
func (s *PersistentWorkStore) AttachProduction(id, goalID, planID, provider string) error {
	return s.store.AttachProduction(id, goalID, planID, provider)
}
func (s *PersistentWorkStore) RecordArtifact(id string, artifact production.ProductionArtifact) error {
	return s.store.RecordArtifact(id, artifact)
}
func (s *PersistentWorkStore) Block(id, detail string) error { return s.store.Block(id, detail) }
func (s *PersistentWorkStore) Complete(id string, success bool, detail string) error {
	return s.store.Complete(id, success, detail)
}
func (s *PersistentWorkStore) SetProgress(id, current string, completed, pending []string) error {
	return s.store.SetProgress(id, current, completed, pending)
}
func (s *PersistentWorkStore) RecordActionResult(id string, result production.ActionResult) error {
	return s.store.RecordActionResult(id, result)
}
func (s *PersistentWorkStore) SetPendingExecution(id string, pending production.PendingExecution) error {
	return s.store.SetPendingExecution(id, pending)
}
func (s *PersistentWorkStore) RejectPendingExecution(id, detail string) error {
	return s.store.RejectPendingExecution(id, detail)
}
func (s *PersistentWorkStore) ClearPendingExecution(id string) error {
	return s.store.ClearPendingExecution(id)
}
func (s *PersistentWorkStore) PendingExecution(id string) (production.PendingExecution, bool) {
	return s.store.PendingExecution(id)
}
func (s *PersistentWorkStore) BeginAttempt(id, capabilityID, operation, provider string, skillIDs ...string) (int, error) {
	return s.store.BeginAttempt(id, capabilityID, operation, provider, skillIDs...)
}
func (s *PersistentWorkStore) SetAttemptTaskID(id string, number int, taskID string) error {
	return s.store.SetAttemptTaskID(id, number, taskID)
}
func (s *PersistentWorkStore) SetAttemptExecutionIdentity(id string, number int, jobID, executionAttemptID string) error {
	return s.store.SetAttemptExecutionIdentity(id, number, jobID, executionAttemptID)
}
func (s *PersistentWorkStore) SetAttemptEffectSpecification(id string, number int, specification []byte) error {
	return s.store.SetAttemptEffectSpecification(id, number, specification)
}
func (s *PersistentWorkStore) RecordAttemptEffectEvaluation(id string, number int, evaluation []byte) error {
	return s.store.RecordAttemptEffectEvaluation(id, number, evaluation)
}
func (s *PersistentWorkStore) ResumeAttempt(id string, number int) error {
	return s.store.ResumeAttempt(id, number)
}
func (s *PersistentWorkStore) CompleteAttempt(id string, number int, result production.ActionResult, recoveryAction, recoveryReason string) error {
	return s.store.CompleteAttempt(id, number, result, recoveryAction, recoveryReason)
}
func (s *PersistentWorkStore) FinalizeAttempt(id string, number int, result production.ActionResult, recoveryAction, recoveryReason string, terminalStatus production.WorkStatus, detail string) error {
	return s.store.FinalizeAttempt(id, number, result, recoveryAction, recoveryReason, terminalStatus, detail)
}
func (s *PersistentWorkStore) FinalizeRepairAttempt(id string, number int, result production.ActionResult, replacement, detail string) error {
	return s.store.FinalizeRepairAttempt(id, number, result, replacement, detail)
}
func (s *PersistentWorkStore) ReconcileUnknownAttempt(id string, number int, result production.ActionResult) error {
	return s.store.ReconcileUnknownAttempt(id, number, result)
}
func (s *PersistentWorkStore) LastAttempt(id string) (production.WorkAttempt, bool) {
	return s.store.LastAttempt(id)
}
func (s *PersistentWorkStore) UpdateObjective(id, objective, detail string) error {
	return s.store.UpdateObjective(id, objective, detail)
}
func (s *PersistentWorkStore) Get(id string) (production.WorkSession, bool) { return s.store.Get(id) }
func (s *PersistentWorkStore) Last() (production.WorkSession, bool)         { return s.store.Last() }
func (s *PersistentWorkStore) PendingAuthorizations() []production.WorkSession {
	if s == nil || s.store == nil {
		return nil
	}
	return s.store.PendingAuthorizations()
}
func (s *PersistentWorkStore) FindArtifact(query string) (production.ProductionArtifact, bool) {
	return s.store.FindArtifact(query)
}
func (s *PersistentWorkStore) RecentResult() (string, bool) { return s.store.RecentResult() }

// List exposes the persisted sessions to the composition-root feed without
// leaking the underlying store (P2). The adapter remains the single owner.
func (s *PersistentWorkStore) List() []production.WorkSession {
	if s == nil || s.store == nil {
		return nil
	}
	return s.store.List()
}

func (s *PersistentWorkStore) Raw() *production.WorkStore { return s.store }
