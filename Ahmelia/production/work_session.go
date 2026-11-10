package production

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"ahmelia/interaction"
)

// WorkStatus describes the lifecycle of an objective independently of domain.
type WorkStatus string

const (
	WorkPending   WorkStatus = "pending"
	WorkRunning   WorkStatus = "running"
	WorkBlocked   WorkStatus = "blocked"
	WorkCompleted WorkStatus = "completed"
	WorkFailed    WorkStatus = "failed"
	WorkCancelled WorkStatus = "cancelled"
)

// WorkEvent is an append-only fact about work. It is intentionally generic.
type WorkEvent struct {
	Time   time.Time `json:"time"`
	Type   string    `json:"type"`
	Detail string    `json:"detail,omitempty"`
}

// WorkAttempt is one execution attempt inside the canonical WorkSession.
// It is a record owned by Work, not a second work system: one Work can contain
// multiple attempts while Cognition/Recovery decides what should happen next.
type PendingExecution struct {
	Request       interaction.SemanticRequest `json:"request"`
	TaskID        string                      `json:"task_id,omitempty"`
	CapabilityID  string                      `json:"capability_id"`
	Operation     string                      `json:"operation"`
	RouteKind     string                      `json:"route_kind,omitempty"`
	ProviderID    string                      `json:"provider_id,omitempty"`
	AttemptNumber int                         `json:"attempt_number"`
	Reason        string                      `json:"reason,omitempty"`
}

type WorkAttempt struct {
	Number              int               `json:"number"`
	TaskID              string            `json:"task_id,omitempty"`
	SkillID             string            `json:"skill_id,omitempty"`
	CapabilityID        string            `json:"capability_id,omitempty"`
	Operation           string            `json:"operation,omitempty"`
	Provider            string            `json:"provider,omitempty"`
	JobID               string            `json:"job_id,omitempty"`
	ExecutionAttemptID  string            `json:"execution_attempt_id,omitempty"`
	EffectSpecification json.RawMessage   `json:"effect_specification,omitempty"`
	EffectEvaluations   []json.RawMessage `json:"effect_evaluations,omitempty"`
	Status              string            `json:"status"`
	StartedAt           time.Time         `json:"started_at"`
	FinishedAt          time.Time         `json:"finished_at,omitempty"`
	Result              *ActionResult     `json:"result,omitempty"`
	RecoveryAction      string            `json:"recovery_action,omitempty"`
	RecoveryReason      string            `json:"recovery_reason,omitempty"`
}

// WorkSession is persistent operational memory for one objective.
type WorkSession struct {
	ID               string                `json:"id"`
	Objective        interaction.Objective `json:"objective"`
	Status           WorkStatus            `json:"status"`
	GoalID           string                `json:"goal_id,omitempty"`
	PlanID           string                `json:"plan_id,omitempty"`
	Provider         string                `json:"provider,omitempty"`
	CurrentStep      string                `json:"current_step,omitempty"`
	Completed        []string              `json:"completed,omitempty"`
	Pending          []string              `json:"pending,omitempty"`
	Artifacts        []ProductionArtifact  `json:"artifacts,omitempty"`
	Events           []WorkEvent           `json:"events,omitempty"`
	LastActionResult *ActionResult         `json:"last_action_result,omitempty"`
	Attempts         []WorkAttempt         `json:"attempts,omitempty"`
	PendingExecution *PendingExecution     `json:"pending_execution,omitempty"`
	CreatedAt        time.Time             `json:"created_at"`
	UpdatedAt        time.Time             `json:"updated_at"`
}

// WorkStore persists production state independently of domain.
type WorkStore struct {
	mu       sync.RWMutex
	path     string
	sessions map[string]WorkSession
	lastID   string
	loadErr  error
}

var workSessionIDSequence atomic.Uint64

func NewWorkStore(path string) *WorkStore {
	ws := &WorkStore{path: path, sessions: make(map[string]WorkSession)}
	ws.load()
	return ws
}

func (s *WorkStore) Begin(req interaction.SemanticRequest) (WorkSession, error) {
	if s == nil {
		return WorkSession{}, fmt.Errorf("production work store: nil")
	}
	// UnixNano alone can collide on Windows, where clock resolution is often
	// coarser than one nanosecond. Two accepted Begin calls must never alias the
	// same canonical Work owner, even when they occur in the same clock tick.
	id := fmt.Sprintf("work_%d_%d", time.Now().UnixNano(), workSessionIDSequence.Add(1))
	now := time.Now().UTC()
	w := WorkSession{ID: id, Objective: req.Objective, Status: WorkPending, CreatedAt: now, UpdatedAt: now}

	s.mu.Lock()
	defer s.mu.Unlock()

	candidate := cloneSessions(s.sessions)
	candidate[id] = w
	if err := s.commitLocked(candidate); err != nil {
		return WorkSession{}, err
	}
	s.sessions = candidate
	s.lastID = id
	return w, nil
}

// AttachPlanning links a WorkSession to the canonical Goal/Plan owners without
// starting physical production. Work remains pending until Execution Authority
// actually hands the work to a provider.
func (s *WorkStore) AttachPlanning(id, goalID, planID string) error {
	return s.update(id, func(w *WorkSession) {
		w.GoalID, w.PlanID = strings.TrimSpace(goalID), strings.TrimSpace(planID)
		w.Events = append(w.Events, WorkEvent{Time: time.Now().UTC(), Type: "planned", Detail: "goal and plan attached"})
	})
}

func (s *WorkStore) AttachProduction(id, goalID, planID, provider string) error {
	if s == nil {
		return fmt.Errorf("production work store: nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.sessions[id]
	if !ok {
		return fmt.Errorf("production work store: work %q not found", id)
	}
	// Attaching a provider is a lifecycle mutation performed before BeginAttempt
	// by the runtime. It must not make an uncertain or still-running prior effect
	// look like a fresh ordinary production pass before the attempt gate rejects
	// a retry. Reconciliation/recovery must resolve that attempt first.
	for _, prior := range current.Attempts {
		if prior.Status == "unverified" || prior.Status == "running" {
			return fmt.Errorf("production work store: attempt %d has status %q and must be reconciled before attaching production", prior.Number, prior.Status)
		}
	}
	candidate := cloneSessions(s.sessions)
	w := candidate[id]
	// Empty planning identifiers mean "preserve the Core-owned
	// Work→Goal→Plan relationship". A provider adapter must never
	// erase canonical planning state merely because it does not own it.
	if strings.TrimSpace(goalID) != "" {
		w.GoalID = strings.TrimSpace(goalID)
	}
	if strings.TrimSpace(planID) != "" {
		w.PlanID = strings.TrimSpace(planID)
	}
	if strings.TrimSpace(provider) != "" {
		w.Provider = strings.TrimSpace(provider)
	}
	w.Status = WorkRunning
	w.Events = append(w.Events, WorkEvent{Time: time.Now().UTC(), Type: "started", Detail: "production started"})
	w.UpdatedAt = time.Now().UTC()
	candidate[id] = w
	if err := s.commitLocked(candidate); err != nil {
		return err
	}
	s.sessions = candidate
	return nil
}

func (s *WorkStore) RecordArtifact(id string, a ProductionArtifact) error {
	if !a.Valid() {
		return fmt.Errorf("production work store: invalid artifact")
	}
	a = EnrichArtifactIfAvailable(a)
	return s.update(id, func(w *WorkSession) {
		for i := range w.Artifacts {
			if (a.ID != "" && w.Artifacts[i].ID == a.ID) || filepath.Clean(w.Artifacts[i].Path) == filepath.Clean(a.Path) {
				w.Artifacts[i] = cloneArtifact(a)
				return
			}
		}
		w.Artifacts = append(w.Artifacts, cloneArtifact(a))
	})
}

// Block records that the operational work must stop. Applying this state is
// the WorkStore's responsibility; it does not execute or select a capability.
func (s *WorkStore) Block(id, detail string) error {
	return s.update(id, func(w *WorkSession) {
		w.Status = WorkBlocked
		if strings.TrimSpace(detail) != "" {
			w.Events = append(w.Events, WorkEvent{Time: time.Now().UTC(), Type: "blocked", Detail: detail})
		}
	})
}
func (s *WorkStore) Complete(id string, success bool, detail string) error {
	if s == nil {
		return fmt.Errorf("production work store: nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.sessions[id]
	if !ok {
		return fmt.Errorf("production work store: work %q not found", id)
	}
	// Legacy completion is still used by recovery/compatibility paths. It must
	// not bypass the canonical attempt lifecycle and close a Work whose physical
	// effect is uncertain, still running, or awaiting authorization.
	for _, attempt := range current.Attempts {
		switch attempt.Status {
		case "unverified", "running", "awaiting_authorization":
			return fmt.Errorf("production work store: cannot close work while attempt %d has status %q", attempt.Number, attempt.Status)
		}
	}
	candidate := cloneSessions(s.sessions)
	w := candidate[id]
	if success {
		w.Status = WorkCompleted
	} else {
		w.Status = WorkFailed
	}
	if detail != "" {
		w.Events = append(w.Events, WorkEvent{Time: time.Now().UTC(), Type: statusType(success), Detail: detail})
	}
	w.UpdatedAt = time.Now().UTC()
	candidate[id] = w
	if err := s.commitLocked(candidate); err != nil {
		return err
	}
	s.sessions = candidate
	return nil
}

func statusType(ok bool) string {
	if ok {
		return "completed"
	}
	return "failed"
}

func (s *WorkStore) SetProgress(id, current string, completed, pending []string) error {
	return s.update(id, func(w *WorkSession) {
		w.CurrentStep = current
		w.Completed = append([]string(nil), completed...)
		w.Pending = append([]string(nil), pending...)
	})
}

func (s *WorkStore) RecordActionResult(id string, result ActionResult) error {
	return s.update(id, func(w *WorkSession) {
		copy := cloneActionResult(result)
		w.LastActionResult = &copy
		w.Events = append(w.Events, WorkEvent{Time: time.Now().UTC(), Type: "action_result", Detail: result.Summary})
	})
}

// BeginAttempt appends a new execution attempt to the same WorkSession.
// The attempt ordinal is derived from the Work record, so retries never create
// a second Work owner.
func (s *WorkStore) SetPendingExecution(id string, pending PendingExecution) error {
	if s == nil {
		return fmt.Errorf("production work store: nil")
	}
	pending.CapabilityID = strings.TrimSpace(pending.CapabilityID)
	pending.Operation = strings.TrimSpace(pending.Operation)
	pending.RouteKind = strings.TrimSpace(pending.RouteKind)
	pending.ProviderID = strings.TrimSpace(pending.ProviderID)
	if pending.AttemptNumber < 1 || pending.CapabilityID == "" || pending.Operation == "" ||
		pending.RouteKind != "production" || pending.ProviderID == "" || strings.TrimSpace(pending.Request.Raw) == "" {
		return fmt.Errorf("production work store: pending execution identity is incomplete")
	}
	if pending.Reason == "" {
		pending.Reason = "authorization_required"
	}

	// Work, Attempt, and PendingExecution are one causal record. Validate and
	// persist their transition under the same store lock; do not publish a
	// pending authorization that points at a missing or different attempt.
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.sessions[id]
	if !ok {
		return fmt.Errorf("production work store: work %q not found", id)
	}
	if pending.AttemptNumber > len(current.Attempts) {
		return fmt.Errorf("production work store: pending execution references missing attempt %d", pending.AttemptNumber)
	}
	attempt := current.Attempts[pending.AttemptNumber-1]
	if attempt.CapabilityID != pending.CapabilityID || attempt.Operation != pending.Operation || attempt.Provider != pending.ProviderID {
		return fmt.Errorf("production work store: pending execution identity does not match its attempt")
	}
	if attempt.TaskID != "" && attempt.TaskID != pending.TaskID {
		return fmt.Errorf("production work store: pending task identity does not match its attempt")
	}
	if attempt.Status != "running" && attempt.Status != "awaiting_authorization" {
		return fmt.Errorf("production work store: attempt %d cannot await authorization from status %q", pending.AttemptNumber, attempt.Status)
	}
	if current.PendingExecution != nil {
		prior := current.PendingExecution
		if prior.AttemptNumber != pending.AttemptNumber || prior.CapabilityID != pending.CapabilityID || prior.Operation != pending.Operation || prior.RouteKind != pending.RouteKind || prior.ProviderID != pending.ProviderID || prior.TaskID != pending.TaskID {
			return fmt.Errorf("production work store: a different authorization is already pending")
		}
	}
	candidate := cloneSessions(s.sessions)
	w := candidate[id]
	p := pending
	w.PendingExecution = &p
	w.Attempts[pending.AttemptNumber-1].TaskID = pending.TaskID
	w.Attempts[pending.AttemptNumber-1].Status = "awaiting_authorization"
	w.Status = WorkBlocked
	w.Events = append(w.Events, WorkEvent{Time: time.Now().UTC(), Type: "execution_awaiting_authorization", Detail: fmt.Sprintf("attempt=%d capability=%s provider=%s", pending.AttemptNumber, pending.CapabilityID, pending.ProviderID)})
	w.UpdatedAt = time.Now().UTC()
	candidate[id] = w
	if err := s.commitLocked(candidate); err != nil {
		return err
	}
	s.sessions = candidate
	return nil
}

// SetAttemptTaskID binds a planned Task identity to the durable WorkAttempt
// before authorization suspension or provider dispatch. Existing interactive
// callers may leave taskID empty and retain the legacy behavior.
func (s *WorkStore) SetAttemptTaskID(id string, number int, taskID string) error {
	if s == nil {
		return fmt.Errorf("production work store: nil")
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return nil
	}
	if number < 1 {
		return fmt.Errorf("production work store: attempt number inválido")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.sessions[id]
	if !ok {
		return fmt.Errorf("production work store: work %q not found", id)
	}
	if number > len(current.Attempts) {
		return fmt.Errorf("production work store: attempt %d not found in work %q", number, id)
	}
	prior := current.Attempts[number-1]
	if prior.Status != "running" && prior.Status != "awaiting_authorization" {
		return fmt.Errorf("production work store: cannot bind task identity to attempt status %q", prior.Status)
	}
	if prior.TaskID != "" && prior.TaskID != taskID {
		return fmt.Errorf("production work store: attempt task identity mismatch")
	}
	candidate := cloneSessions(s.sessions)
	w := candidate[id]
	w.Attempts[number-1].TaskID = taskID
	w.Events = append(w.Events, WorkEvent{Time: time.Now().UTC(), Type: "attempt_task_identity", Detail: fmt.Sprintf("attempt=%d task=%s", number, taskID)})
	w.UpdatedAt = time.Now().UTC()
	candidate[id] = w
	if err := s.commitLocked(candidate); err != nil {
		return err
	}
	s.sessions = candidate
	return nil
}

func (s *WorkStore) RejectPendingExecution(id string, detail string) error {
	return s.update(id, func(w *WorkSession) {
		if w.PendingExecution != nil {
			n := w.PendingExecution.AttemptNumber
			if n >= 1 && n <= len(w.Attempts) {
				w.Attempts[n-1].Status = "authorization_denied"
				w.Attempts[n-1].FinishedAt = time.Now().UTC()
			}
		}
		w.PendingExecution = nil
		w.Status = WorkCancelled
		w.Events = append(w.Events, WorkEvent{Time: time.Now().UTC(), Type: "execution_authorization_denied", Detail: detail})
	})
}

func (s *WorkStore) ClearPendingExecution(id string) error {
	return s.update(id, func(w *WorkSession) { w.PendingExecution = nil })
}

func (s *WorkStore) PendingExecution(id string) (PendingExecution, bool) {
	w, ok := s.Get(id)
	if !ok || w.PendingExecution == nil {
		return PendingExecution{}, false
	}
	return *w.PendingExecution, true
}

func (s *WorkStore) BeginAttempt(id, capabilityID, operation, provider string, skillIDs ...string) (int, error) {
	if s == nil {
		return 0, fmt.Errorf("production work store: nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.sessions[id]
	if !ok {
		return 0, fmt.Errorf("production work store: work %q not found", id)
	}
	for _, prior := range current.Attempts {
		switch prior.Status {
		case "unverified":
			return 0, fmt.Errorf("production work store: attempt %d has an unknown outcome and must be reconciled before another attempt", prior.Number)
		case "running", "awaiting_authorization":
			return 0, fmt.Errorf("production work store: attempt %d is still active with status %q and cannot be followed by another attempt", prior.Number, prior.Status)
		}
	}
	candidate := cloneSessions(s.sessions)
	w := candidate[id]
	number := len(w.Attempts) + 1
	skillID := ""
	if len(skillIDs) > 0 {
		skillID = strings.TrimSpace(skillIDs[0])
	}
	w.Attempts = append(w.Attempts, WorkAttempt{
		Number: number, SkillID: skillID, CapabilityID: strings.TrimSpace(capabilityID), Operation: strings.TrimSpace(operation),
		Provider: strings.TrimSpace(provider), Status: "running", StartedAt: time.Now().UTC(),
	})
	w.Status = WorkRunning
	w.Events = append(w.Events, WorkEvent{Time: time.Now().UTC(), Type: "attempt_started", Detail: fmt.Sprintf("attempt=%d operation=%s provider=%s", number, operation, provider)})
	w.UpdatedAt = time.Now().UTC()
	candidate[id] = w
	if err := s.commitLocked(candidate); err != nil {
		return 0, err
	}
	s.sessions = candidate
	return number, nil
}

// FinalizeAttempt atomically persists the attempt outcome, last action result,
// and optional terminal Work status. This prevents partial lifecycle updates
// when one of several independent writes fails.
func (s *WorkStore) FinalizeAttempt(id string, number int, result ActionResult, recoveryAction, recoveryReason string, terminalStatus WorkStatus, detail string) error {
	if s == nil {
		return fmt.Errorf("production work store: nil")
	}
	if number < 1 {
		return fmt.Errorf("production work store: attempt number inválido")
	}
	if terminalStatus != "" && terminalStatus != WorkCompleted && terminalStatus != WorkFailed {
		return fmt.Errorf("production work store: invalid terminal work status %q", terminalStatus)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.sessions[id]
	if !ok {
		return fmt.Errorf("production work store: work %q not found", id)
	}
	if number > len(current.Attempts) {
		return fmt.Errorf("production work store: attempt %d not found for work %q", number, id)
	}
	candidate := cloneSessions(s.sessions)
	w := candidate[id]
	if result.Success && result.Verified && !result.Observed {
		return fmt.Errorf("production work store: verified success requires observation")
	}
	status := result.OutcomeStatus
	if !status.Valid() {
		if result.Success && result.Verified && result.Observed {
			status = OutcomeSucceeded
		} else if result.Err != nil || !result.Success {
			status = OutcomeFailed
		} else {
			status = OutcomeUnknown
		}
	}
	// Enforce epistemic closure invariants at the canonical lifecycle owner,
	// not only in current callers. A caller must not close Work as completed
	// from a provider claim without both observation and verification, nor mark
	// an unknown outcome terminal before reconciliation.
	if status == OutcomeSucceeded && (!result.Success || !result.Observed || !result.Verified) {
		return fmt.Errorf("production work store: succeeded outcome requires success, observation, and verification")
	}
	if status == OutcomeFailed && result.Success {
		return fmt.Errorf("production work store: failed outcome cannot report success")
	}
	if status == OutcomeUnknown && terminalStatus != "" {
		return fmt.Errorf("production work store: unknown outcome cannot close work before reconciliation")
	}
	if terminalStatus == WorkCompleted && status != OutcomeSucceeded {
		return fmt.Errorf("production work store: completed work requires a verified succeeded outcome")
	}
	if terminalStatus == WorkFailed && status != OutcomeFailed {
		return fmt.Errorf("production work store: failed work requires a known failed outcome")
	}
	result.WorkID, result.AttemptNumber, result.OutcomeStatus = id, number, status
	resultCopy := cloneActionResult(result)
	attempt := &w.Attempts[number-1]
	attempt.FinishedAt = time.Now().UTC()
	switch status {
	case OutcomeSucceeded:
		attempt.Status = "succeeded"
	case OutcomeUnknown:
		attempt.Status = "unverified"
	default:
		attempt.Status = "failed"
	}
	attempt.Result = &resultCopy
	attempt.RecoveryAction = strings.TrimSpace(recoveryAction)
	attempt.RecoveryReason = strings.TrimSpace(recoveryReason)
	w.PendingExecution = nil
	w.LastActionResult = &resultCopy
	eventDetail := fmt.Sprintf("attempt=%d status=%s", number, attempt.Status)
	if strings.TrimSpace(detail) != "" {
		eventDetail += " detail=" + strings.TrimSpace(detail)
	}
	if attempt.RecoveryAction != "" {
		eventDetail += " recovery=" + attempt.RecoveryAction
	}
	w.Events = append(w.Events, WorkEvent{Time: time.Now().UTC(), Type: "attempt_completed", Detail: eventDetail})
	if terminalStatus != "" {
		w.Status = terminalStatus
		eventType := string(terminalStatus)
		w.Events = append(w.Events, WorkEvent{Time: time.Now().UTC(), Type: eventType, Detail: strings.TrimSpace(detail)})
	}
	w.UpdatedAt = time.Now().UTC()
	candidate[id] = w
	if err := s.commitLocked(candidate); err != nil {
		return err
	}
	s.sessions = candidate
	return nil
}

// FinalizeRepairAttempt atomically commits the verified replacement objective,
// attempt result, LastActionResult and terminal Work status. A repair must not
// publish a new objective separately from the execution evidence that justifies it.
func (s *WorkStore) FinalizeRepairAttempt(id string, number int, result ActionResult, replacement, detail string) error {
	replacement = strings.TrimSpace(replacement)
	if replacement == "" {
		return fmt.Errorf("production work store: repair replacement objective is empty")
	}
	if result.OutcomeStatus != OutcomeSucceeded || !result.Success || !result.Observed || !result.Verified {
		return fmt.Errorf("production work store: repair finalization requires a verified successful outcome")
	}
	if s == nil {
		return fmt.Errorf("production work store: nil")
	}
	if number < 1 {
		return fmt.Errorf("production work store: attempt number inválido")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.sessions[id]
	if !ok {
		return fmt.Errorf("production work store: work %q not found", id)
	}
	if number > len(current.Attempts) {
		return fmt.Errorf("production work store: attempt %d not found for work %q", number, id)
	}
	candidate := cloneSessions(s.sessions)
	w := candidate[id]
	result.WorkID, result.AttemptNumber = id, number
	result.OutcomeStatus = OutcomeSucceeded
	resultCopy := cloneActionResult(result)
	attempt := &w.Attempts[number-1]
	attempt.Status = "succeeded"
	attempt.FinishedAt = time.Now().UTC()
	attempt.Result = &resultCopy
	attempt.RecoveryAction = "repair_verified"
	attempt.RecoveryReason = strings.TrimSpace(detail)
	w.LastActionResult = &resultCopy
	w.PendingExecution = nil
	w.Objective.Text = replacement
	w.Status = WorkCompleted
	now := time.Now().UTC()
	w.Events = append(w.Events,
		WorkEvent{Time: now, Type: "attempt_completed", Detail: fmt.Sprintf("attempt=%d status=succeeded recovery=repair_verified", number)},
		WorkEvent{Time: now, Type: "objective_repaired", Detail: strings.TrimSpace(detail)},
		WorkEvent{Time: now, Type: "completed", Detail: strings.TrimSpace(result.Summary)},
	)
	w.UpdatedAt = now
	candidate[id] = w
	if err := s.commitLocked(candidate); err != nil {
		return err
	}
	s.sessions = candidate
	return nil
}

// CompleteAttempt materializes one attempt's structured outcome and the
// recovery policy result that followed it. It never decides recovery itself.
func (s *WorkStore) ResumeAttempt(id string, number int) error {
	if s == nil {
		return fmt.Errorf("production work store: nil")
	}
	if number < 1 {
		return fmt.Errorf("production work store: attempt number inválido")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.sessions[id]
	if !ok {
		return fmt.Errorf("production work store: work %q not found", id)
	}
	if number > len(current.Attempts) {
		return fmt.Errorf("production work store: attempt %d not found for work %q", number, id)
	}
	attempt := current.Attempts[number-1]
	if attempt.Status != "awaiting_authorization" {
		return fmt.Errorf("production work store: attempt %d cannot resume from status %q", number, attempt.Status)
	}
	if current.PendingExecution == nil || current.PendingExecution.AttemptNumber != number ||
		current.PendingExecution.CapabilityID != attempt.CapabilityID ||
		current.PendingExecution.Operation != attempt.Operation ||
		current.PendingExecution.ProviderID != attempt.Provider ||
		current.PendingExecution.TaskID != attempt.TaskID {
		return fmt.Errorf("production work store: attempt %d has no matching pending authorization", number)
	}

	// Authorization is a one-way transition into execution. Consume the
	// pending authorization and mark the attempt running in the same durable
	// commit, before dispatch. A crash after the external effect can therefore
	// never make a later "yes" replay the same authorization as if execution
	// had not begun; the running attempt must be reconciled instead.
	candidate := cloneSessions(s.sessions)
	w := candidate[id]
	w.Attempts[number-1].Status = "running"
	w.Attempts[number-1].FinishedAt = time.Time{}
	w.PendingExecution = nil
	w.Status = WorkRunning
	w.Events = append(w.Events, WorkEvent{Time: time.Now().UTC(), Type: "authorization_consumed_attempt_started", Detail: fmt.Sprintf("attempt=%d capability=%s", number, attempt.CapabilityID)})
	w.UpdatedAt = time.Now().UTC()
	candidate[id] = w
	if err := s.commitLocked(candidate); err != nil {
		return err
	}
	s.sessions = candidate
	return nil
}

// SetAttemptExecutionIdentity persists the provider-neutral job and the deterministic
// physical-attempt identity before dispatch. Recovery can therefore relate an
// uncertain WorkAttempt to the execution lease that was intended to run it.
func (s *WorkStore) SetAttemptExecutionIdentity(id string, number int, jobID, executionAttemptID string) error {
	if number < 1 {
		return fmt.Errorf("production work store: attempt number inválido")
	}
	jobID = strings.TrimSpace(jobID)
	executionAttemptID = strings.TrimSpace(executionAttemptID)
	if jobID == "" || executionAttemptID == "" {
		return fmt.Errorf("production work store: execution identity incompleta")
	}
	return s.update(id, func(w *WorkSession) {
		if number > len(w.Attempts) {
			return
		}
		w.Attempts[number-1].JobID = jobID
		w.Attempts[number-1].ExecutionAttemptID = executionAttemptID
		w.Events = append(w.Events, WorkEvent{Time: time.Now().UTC(), Type: "attempt_execution_identity", Detail: fmt.Sprintf("attempt=%d job=%s execution=%s", number, jobID, executionAttemptID)})
	})
}

// SetAttemptEffectSpecification persists the already-existing domain effect
// specification before the provider can mutate the external world. The store
// intentionally keeps it opaque: Programming owns the schema; Work owns only
// the durable causal association to the attempt.
func (s *WorkStore) SetAttemptEffectSpecification(id string, number int, specification []byte) error {
	if number < 1 {
		return fmt.Errorf("production work store: attempt number inválido")
	}
	if len(specification) == 0 || !json.Valid(specification) {
		return fmt.Errorf("production work store: invalid effect specification")
	}
	copySpec := append([]byte(nil), specification...)
	return s.update(id, func(w *WorkSession) {
		if number > len(w.Attempts) {
			return
		}
		w.Attempts[number-1].EffectSpecification = copySpec
		w.Events = append(w.Events, WorkEvent{Time: time.Now().UTC(), Type: "attempt_effect_specification_persisted", Detail: fmt.Sprintf("attempt=%d", number)})
	})
}

// RecordAttemptEffectEvaluation persists one opaque programming-domain effect
// evaluation under the WorkAttempt that owns the expected effect. Work keeps
// the durable operational association, while Programming owns the evaluation
// schema. Evaluations are append-only so repeated observations are preserved.
func (s *WorkStore) RecordAttemptEffectEvaluation(id string, number int, evaluation []byte) error {
	if number < 1 {
		return fmt.Errorf("production work store: attempt number inválido")
	}
	if len(evaluation) == 0 || !json.Valid(evaluation) {
		return fmt.Errorf("production work store: invalid effect evaluation")
	}
	copyEvaluation := append([]byte(nil), evaluation...)
	return s.update(id, func(w *WorkSession) {
		if number > len(w.Attempts) {
			return
		}
		attempt := &w.Attempts[number-1]
		attempt.EffectEvaluations = append(attempt.EffectEvaluations, copyEvaluation)
		w.Events = append(w.Events, WorkEvent{Time: time.Now().UTC(), Type: "attempt_effect_evaluation_recorded", Detail: fmt.Sprintf("attempt=%d", number)})
	})
}

func (s *WorkStore) CompleteAttempt(id string, number int, result ActionResult, recoveryAction, recoveryReason string) error {
	// Legacy callers must use the same epistemic lifecycle contract as the
	// canonical finalizer. Keeping a second, weaker completion path would let
	// an old route publish a "verified" success without an actual observation.
	return s.FinalizeAttempt(id, number, result, recoveryAction, recoveryReason, "", "legacy completion")
}

// ReconcileUnknownAttempt resolves a previously uncertain physical attempt only
// when a fresh, structured observation establishes the effect's outcome. It
// does not close Work; a later planner decision may continue or stop the work.
func (s *WorkStore) ReconcileUnknownAttempt(id string, number int, result ActionResult) error {
	if s == nil {
		return fmt.Errorf("production work store: nil")
	}
	if number < 1 {
		return fmt.Errorf("production work store: attempt number inválido")
	}
	if !result.Observed {
		return fmt.Errorf("production work store: reconciliation requires a fresh observation")
	}
	switch result.OutcomeStatus {
	case OutcomeSucceeded:
		if !result.Success || !result.Verified {
			return fmt.Errorf("production work store: successful reconciliation requires success and verification")
		}
	case OutcomeFailed:
		if result.Success || strings.TrimSpace(result.FailureReason) == "" && strings.TrimSpace(result.ObservationDetails) == "" {
			return fmt.Errorf("production work store: failed reconciliation requires evidence that the expected effect is absent or contradicted")
		}
	default:
		return fmt.Errorf("production work store: reconciliation requires a known succeeded or failed outcome")
	}
	if result.WorkID != "" && result.WorkID != id {
		return fmt.Errorf("production work store: reconciliation work identity mismatch")
	}
	if result.AttemptNumber != 0 && result.AttemptNumber != number {
		return fmt.Errorf("production work store: reconciliation attempt identity mismatch")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.sessions[id]
	if !ok {
		return fmt.Errorf("production work store: work %q not found", id)
	}
	if number > len(current.Attempts) || current.Attempts[number-1].Status != "unverified" {
		return fmt.Errorf("production work store: attempt %d is not awaiting reconciliation", number)
	}
	candidate := cloneSessions(s.sessions)
	w := candidate[id]
	copyResult := cloneActionResult(result)
	copyResult.WorkID = id
	copyResult.AttemptNumber = number
	attempt := &w.Attempts[number-1]
	attempt.Status = "succeeded"
	if result.OutcomeStatus == OutcomeFailed {
		attempt.Status = "failed"
	}
	attempt.Result = &copyResult
	attempt.FinishedAt = time.Now().UTC()
	lastResult := cloneActionResult(copyResult)
	w.LastActionResult = &lastResult
	w.Events = append(w.Events, WorkEvent{Time: time.Now().UTC(), Type: "attempt_reconciled", Detail: fmt.Sprintf("attempt=%d outcome=%s", number, result.OutcomeStatus)})
	w.UpdatedAt = time.Now().UTC()
	candidate[id] = w
	if err := s.commitLocked(candidate); err != nil {
		return err
	}
	s.sessions = candidate
	return nil
}

func (s *WorkStore) LastAttempt(id string) (WorkAttempt, bool) {
	w, ok := s.Get(id)
	if !ok || len(w.Attempts) == 0 {
		return WorkAttempt{}, false
	}
	return cloneWorkAttempt(w.Attempts[len(w.Attempts)-1]), true
}

func (s *WorkStore) UpdateObjective(id, objective, detail string) error {
	objective = strings.TrimSpace(objective)
	if objective == "" {
		return fmt.Errorf("production work store: objective vacío")
	}
	return s.update(id, func(w *WorkSession) {
		w.Objective.Text = objective
		if detail != "" {
			w.Events = append(w.Events, WorkEvent{Time: time.Now().UTC(), Type: "objective_repaired", Detail: detail})
		}
	})
}

func (s *WorkStore) Get(id string) (WorkSession, bool) {
	if s == nil || strings.TrimSpace(id) == "" {
		return WorkSession{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	w, ok := s.sessions[id]
	if !ok {
		return WorkSession{}, false
	}
	return cloneWork(w), true
}

func (s *WorkStore) Last() (WorkSession, bool) {
	if s == nil {
		return WorkSession{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	w, ok := s.sessions[s.lastID]
	if !ok {
		return WorkSession{}, false
	}
	return cloneWork(w), true
}

// PendingAuthorizations returns canonical work owners awaiting explicit user
// approval. It is narrower than Last/history: callers may recover an owner
// only when an actual pending execution contract exists.
func (s *WorkStore) PendingAuthorizations() []WorkSession {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	var pending []WorkSession
	for _, work := range s.sessions {
		if work.PendingExecution != nil {
			pending = append(pending, cloneWork(work))
		}
	}
	sort.Slice(pending, func(i, j int) bool { return pending[i].UpdatedAt.After(pending[j].UpdatedAt) })
	return pending
}

func (s *WorkStore) FindArtifact(query string) (ProductionArtifact, bool) {
	w, ok := s.Last()
	if !ok {
		return ProductionArtifact{}, false
	}
	q := strings.ToLower(strings.TrimSpace(query))
	for _, a := range w.Artifacts {
		if strings.EqualFold(a.Name, q) || strings.EqualFold(filepath.Base(a.Path), q) || strings.EqualFold(a.ID, q) ||
			(q != "" && strings.Contains(strings.ToLower(a.Name), q)) {
			return a, true
		}
	}
	return ProductionArtifact{}, false
}

func (s *WorkStore) update(id string, fn func(*WorkSession)) error {
	if s == nil {
		return fmt.Errorf("production work store: nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.sessions[id]; !ok {
		return fmt.Errorf("production work store: work %q not found", id)
	}

	candidate := cloneSessions(s.sessions)
	w := candidate[id]
	fn(&w)
	w.UpdatedAt = time.Now().UTC()
	candidate[id] = w

	if err := s.commitLocked(candidate); err != nil {
		return err
	}
	s.sessions = candidate
	return nil
}

// commitLocked makes candidate the next durable state. The caller must hold s.mu.
// The live state is published only after the filesystem accepts the candidate.
func (s *WorkStore) commitLocked(candidate map[string]WorkSession) error {
	if s == nil {
		return fmt.Errorf("production work store: nil")
	}
	if s.loadErr != nil {
		return fmt.Errorf("production work store: refusing to overwrite state that failed to load: %w", s.loadErr)
	}
	if s.path == "" {
		return nil
	}
	data, err := json.MarshalIndent(candidate, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0755); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *WorkStore) load() {
	if s == nil || s.path == "" {
		return
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		if !os.IsNotExist(err) {
			s.loadErr = fmt.Errorf("read persisted state %q: %w", s.path, err)
		}
		return
	}
	var sessions map[string]WorkSession
	if err := json.Unmarshal(data, &sessions); err != nil {
		s.loadErr = fmt.Errorf("decode persisted state %q: %w", s.path, err)
		return
	}
	if sessions == nil {
		s.loadErr = fmt.Errorf("decode persisted state %q: expected a JSON object, got null", s.path)
		return
	}
	changed := false
	for id, w := range sessions {
		if strings.TrimSpace(id) == "" || strings.TrimSpace(w.ID) == "" || w.ID != id {
			s.loadErr = fmt.Errorf("decode persisted state %q: invalid work identity key=%q id=%q", s.path, id, w.ID)
			s.sessions = make(map[string]WorkSession)
			s.lastID = ""
			return
		}

		// A process restart destroys the in-memory execution context. A persisted
		// attempt marked running therefore cannot safely remain "running": the
		// store no longer knows whether the external effect happened before the
		// process stopped. Convert only that restart boundary state into the same
		// epistemic state used for other unknown outcomes. Authorization is
		// different: its pending execution is explicit and remains awaiting
		// authorization rather than being treated as an interrupted effect.
		for i := range w.Attempts {
			a := &w.Attempts[i]
			if a.Status != "running" || w.PendingExecution != nil && w.PendingExecution.AttemptNumber == a.Number {
				continue
			}
			a.Status = "unverified"
			if a.FinishedAt.IsZero() {
				a.FinishedAt = time.Now().UTC()
			}
			w.LastActionResult = cloneActionResultPtr(a.Result)
			w.Events = append(w.Events, WorkEvent{Time: time.Now().UTC(), Type: "attempt_interrupted_on_restart", Detail: fmt.Sprintf("attempt=%d converted from running to unverified; reconciliation required", a.Number)})
			w.Status = WorkRunning
			w.UpdatedAt = time.Now().UTC()
			changed = true
		}
		s.sessions[id] = w
		if s.lastID == "" || w.UpdatedAt.After(s.sessions[s.lastID].UpdatedAt) {
			s.lastID = id
		}
	}
	if changed {
		// Persist the restart normalization only after every persisted identity
		// has passed validation. This makes the boundary durable: a later restart
		// cannot resurrect the same interrupted attempt as "running".
		if err := s.commitLocked(s.sessions); err != nil {
			s.loadErr = fmt.Errorf("persist restart reconciliation state %q: %w", s.path, err)
		}
	}
}

// LoadError reports why persisted state could not be restored. A missing file
// is a valid first-run state; a corrupt or unreadable existing file is not.
func (s *WorkStore) LoadError() error {
	if s == nil {
		return fmt.Errorf("production work store: nil")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.loadErr
}

func cloneSessions(in map[string]WorkSession) map[string]WorkSession {
	out := make(map[string]WorkSession, len(in))
	for id, w := range in {
		out[id] = cloneWork(w)
	}
	return out
}

func cloneActionResultPtr(result *ActionResult) *ActionResult {
	if result == nil {
		return nil
	}
	copy := cloneActionResult(*result)
	return &copy
}

func cloneWork(w WorkSession) WorkSession {
	w.Completed = append([]string(nil), w.Completed...)
	w.Pending = append([]string(nil), w.Pending...)
	w.Artifacts = cloneArtifacts(w.Artifacts)
	w.Events = append([]WorkEvent(nil), w.Events...)
	w.Attempts = cloneWorkAttempts(w.Attempts)
	if w.PendingExecution != nil {
		p := *w.PendingExecution
		w.PendingExecution = &p
	}
	if w.LastActionResult != nil {
		copy := cloneActionResult(*w.LastActionResult)
		w.LastActionResult = &copy
	}
	return w
}

// RecentResult returns the most recent meaningful result/failure detail for the active work.
func (s *WorkStore) RecentResult() (string, bool) {
	w, ok := s.Last()
	if !ok {
		return "", false
	}
	for i := len(w.Events) - 1; i >= 0; i-- {
		if strings.TrimSpace(w.Events[i].Detail) != "" {
			return w.Events[i].Detail, true
		}
	}
	return "", false
}

func cloneActionResult(r ActionResult) ActionResult {
	if r.RetryState != nil {
		state := r.RetryState.Clone()
		r.RetryState = &state
	}
	r.ProgressChecks = append([]ProgressCheck(nil), r.ProgressChecks...)
	r.Artifacts = cloneArtifacts(r.Artifacts)
	if r.RecoveryContext != nil {
		r.RecoveryContext = r.RecoveryContext.Clone()
	}
	if r.Recovery != nil {
		copy := *r.Recovery
		r.Recovery = &copy
	}
	return r
}

func cloneWorkAttempts(in []WorkAttempt) []WorkAttempt {
	if len(in) == 0 {
		return nil
	}
	out := make([]WorkAttempt, len(in))
	for i, attempt := range in {
		out[i] = cloneWorkAttempt(attempt)
	}
	return out
}

func cloneWorkAttempt(attempt WorkAttempt) WorkAttempt {
	if attempt.EffectSpecification != nil {
		attempt.EffectSpecification = append([]byte(nil), attempt.EffectSpecification...)
	}
	if len(attempt.EffectEvaluations) > 0 {
		evaluations := attempt.EffectEvaluations
		attempt.EffectEvaluations = make([]json.RawMessage, len(evaluations))
		for i, evaluation := range evaluations {
			attempt.EffectEvaluations[i] = json.RawMessage(string(evaluation))
		}
	}
	if attempt.Result != nil {
		copy := cloneActionResult(*attempt.Result)
		attempt.Result = &copy
	}
	return attempt
}

// List devuelve una copia de solo lectura de todas las sesiones, ordenadas por
// creación. Es la fuente única de intentos: la competencia se PROYECTA de aquí
// (no se registra en un segundo almacén).
func (s *WorkStore) List() []WorkSession {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]WorkSession, 0, len(s.sessions))
	for _, w := range s.sessions {
		out = append(out, w)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out
}
