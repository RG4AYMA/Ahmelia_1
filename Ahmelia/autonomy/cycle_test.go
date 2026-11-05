package autonomy

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type cycleObserverMock struct {
	observation string
	err         error
}

func (m *cycleObserverMock) Observe(context.Context) (string, error) {
	return m.observation, m.err
}

type cycleEvaluatorMock struct {
	relevant bool
	err      error
}

func (m *cycleEvaluatorMock) EvaluateRelevance(context.Context, string) (bool, error) {
	return m.relevant, m.err
}

type cyclePlannerMock struct {
	req ActionRequest
	has bool
	err error
}

func (m *cyclePlannerMock) GetNextAction(context.Context) (ActionRequest, bool, error) {
	return m.req, m.has, m.err
}

type cycleExecutorMock struct {
	response ActionResponse
}

func (m *cycleExecutorMock) ExecuteAction(context.Context, ActionRequest) ActionResponse {
	return m.response
}

type cycleDiagnoserMock struct {
	mu     sync.Mutex
	called int
	err    error
}

func (m *cycleDiagnoserMock) DiagnoseFailure(context.Context, ActionRequest, VerificationResult) error {
	m.mu.Lock()
	m.called++
	m.mu.Unlock()
	return m.err
}

func (m *cycleDiagnoserMock) Calls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.called
}

type cycleLearnerMock struct {
	mu     sync.Mutex
	called int
	err    error
}

func (m *cycleLearnerMock) RecordOutcome(context.Context, ActionRequest, VerificationResult) error {
	m.mu.Lock()
	m.called++
	m.mu.Unlock()
	return m.err
}

func (m *cycleLearnerMock) Calls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.called
}

type cycleUpdaterMock struct {
	mu     sync.Mutex
	called int
	err    error
	last   VerificationResult
}

func (m *cycleUpdaterMock) UpdateState(_ context.Context, _ ActionRequest, verif VerificationResult) error {
	m.mu.Lock()
	m.called++
	m.last = verif
	m.mu.Unlock()
	return m.err
}

func (m *cycleUpdaterMock) Calls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.called
}

func (m *cycleUpdaterMock) Last() VerificationResult {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.last
}

type cycleStateInspector struct{}

func (cycleStateInspector) InspectAction(context.Context, ActionRequest) (EnvironmentObservation, error) {
	return EnvironmentObservation{
		Confirmed: false,
		Details:   "el estado observado contradice la postcondición",
		Status:    ObservationContradicted,
	}, nil
}

type cycleSuccessExecutor struct{}

func (cycleSuccessExecutor) ExecuteAction(context.Context, ActionRequest) ActionResponse {
	return ActionResponse{RequestID: "act-feedback", Executed: true, Output: "ejecutado"}
}

type cycleFeedbackPlanner struct{}

func (cycleFeedbackPlanner) GetNextAction(context.Context) (ActionRequest, bool, error) {
	return ActionRequest{ID: "act-feedback", Action: "crear_carpeta", Target: "x"}, true, nil
}

func TestCycleCarriesEnvironmentalStateIntoExistingUpdater(t *testing.T) {
	state := New()
	updater := &cycleUpdaterMock{}
	cycle := NewCycle(state, nil, nil, cycleFeedbackPlanner{}, cycleSuccessExecutor{}, nil, nil, nil, updater)
	cycle.SetEnvironmentVerifier(NewEnvironmentVerifier(cycleStateInspector{}))

	if err := cycle.RunStep(context.Background()); err != nil {
		t.Fatalf("RunStep() error = %v", err)
	}
	got := updater.Last()
	if got.Success {
		t.Fatal("environment contradiction must not collapse to verification success")
	}
	if got.ObservationStatus != ObservationContradicted {
		t.Fatalf("observation status=%q, want %q", got.ObservationStatus, ObservationContradicted)
	}
	if got.ObservationDetails == "" {
		t.Fatal("observation details were lost before the existing updater authority")
	}
}

func newTestCycle(state *AutonomousState, executor ExecutorAdapter, learner CycleLearner, diagnoser CycleDiagnoser, updater CycleUpdater) *Cycle {
	return NewCycle(
		state,
		&cycleObserverMock{observation: "relevant_event"},
		&cycleEvaluatorMock{relevant: true},
		&cyclePlannerMock{
			req: ActionRequest{ID: "act-1", Action: "crear_carpeta", Target: "files"},
			has: true,
		},
		executor,
		NewActionVerifier(),
		diagnoser,
		learner,
		updater,
	)
}

func TestCycleRunStepSuccess(t *testing.T) {
	state := New()
	learner := &cycleLearnerMock{}
	updater := &cycleUpdaterMock{}
	diagnoser := &cycleDiagnoserMock{}

	cycle := newTestCycle(
		state,
		&cycleExecutorMock{response: ActionResponse{RequestID: "act-1", Executed: true, Output: "ok"}},
		learner,
		diagnoser,
		updater,
	)

	if err := cycle.RunStep(context.Background()); err != nil {
		t.Fatalf("RunStep() error = %v", err)
	}
	if got := learner.Calls(); got != 1 {
		t.Fatalf("learner calls = %d, want 1", got)
	}
	if got := updater.Calls(); got != 1 {
		t.Fatalf("updater calls = %d, want 1", got)
	}
	if got := diagnoser.Calls(); got != 0 {
		t.Fatalf("diagnoser calls = %d, want 0", got)
	}
	if got := state.Execution(); got != ExecIdle {
		t.Fatalf("final state = %s, want %s", got, ExecIdle)
	}
}

func TestCycleRunStepFailure(t *testing.T) {
	state := New()
	learner := &cycleLearnerMock{}
	updater := &cycleUpdaterMock{}
	diagnoser := &cycleDiagnoserMock{}

	cycle := newTestCycle(
		state,
		&cycleExecutorMock{response: ActionResponse{RequestID: "act-1", Executed: false, Error: errors.New("not executable")}},
		learner,
		diagnoser,
		updater,
	)

	if err := cycle.RunStep(context.Background()); err != nil {
		t.Fatalf("RunStep() error = %v", err)
	}
	if got := diagnoser.Calls(); got != 1 {
		t.Fatalf("diagnoser calls = %d, want 1", got)
	}
	if got := learner.Calls(); got != 0 {
		t.Fatalf("learner calls = %d, want 0", got)
	}
	if got := updater.Calls(); got != 1 {
		t.Fatalf("updater calls = %d, want 1", got)
	}
	if got := state.Execution(); got != ExecIdle {
		t.Fatalf("final state = %s, want %s", got, ExecIdle)
	}
}

func TestCycleRunStepIrrelevant(t *testing.T) {
	state := New()
	planner := &cyclePlannerMock{has: true}

	cycle := NewCycle(
		state,
		&cycleObserverMock{observation: "noise"},
		&cycleEvaluatorMock{relevant: false},
		planner,
		&cycleExecutorMock{},
		NewActionVerifier(),
		nil,
		nil,
		nil,
	)

	if err := cycle.RunStep(context.Background()); err != nil {
		t.Fatalf("RunStep() error = %v", err)
	}
	if planner.has != true {
		t.Fatalf("planner mock was unexpectedly modified")
	}
	if got := state.Execution(); got != ExecIdle {
		t.Fatalf("final state = %s, want %s", got, ExecIdle)
	}
}

func TestCycleRunStepPlannerErrorReturnsToIdle(t *testing.T) {
	state := New()
	plannerErr := errors.New("planning failure")

	cycle := NewCycle(
		state,
		&cycleObserverMock{observation: "relevant_event"},
		&cycleEvaluatorMock{relevant: true},
		&cyclePlannerMock{err: plannerErr},
		nil,
		NewActionVerifier(),
		nil,
		nil,
		nil,
	)

	if err := cycle.RunStep(context.Background()); err == nil {
		t.Fatal("RunStep() error = nil, want planner error")
	}
	if got := state.Execution(); got != ExecIdle {
		t.Fatalf("final state = %s, want %s", got, ExecIdle)
	}
}

func TestCycleRunStepDiagnoserErrorReturnsToIdle(t *testing.T) {
	state := New()
	diagnoser := &cycleDiagnoserMock{err: errors.New("diagnosis failure")}
	updater := &cycleUpdaterMock{}

	cycle := newTestCycle(
		state,
		&cycleExecutorMock{response: ActionResponse{RequestID: "act-1", Executed: false, Error: errors.New("execution failure")}},
		nil,
		diagnoser,
		updater,
	)

	if err := cycle.RunStep(context.Background()); err == nil {
		t.Fatal("RunStep() error = nil, want diagnosis error")
	}
	if got := state.Execution(); got != ExecIdle {
		t.Fatalf("final state = %s, want %s", got, ExecIdle)
	}
	if got := updater.Calls(); got != 0 {
		t.Fatalf("updater calls = %d, want 0 because diagnosis failed", got)
	}
}

func TestCycleRunStepUpdaterErrorReturnsToIdle(t *testing.T) {
	state := New()
	updater := &cycleUpdaterMock{err: errors.New("update failure")}

	cycle := newTestCycle(
		state,
		&cycleExecutorMock{response: ActionResponse{RequestID: "act-1", Executed: true, Output: "ok"}},
		nil,
		nil,
		updater,
	)

	if err := cycle.RunStep(context.Background()); err == nil {
		t.Fatal("RunStep() error = nil, want updater error")
	}
	if got := state.Execution(); got != ExecIdle {
		t.Fatalf("final state = %s, want %s", got, ExecIdle)
	}
}

type cycleAuthorityOutcomeMock struct{}

func (cycleAuthorityOutcomeMock) AuthorityHandled() bool         { return true }
func (cycleAuthorityOutcomeMock) AuthorityExecuted() bool        { return true }
func (cycleAuthorityOutcomeMock) AuthoritySucceeded() bool       { return true }
func (cycleAuthorityOutcomeMock) AuthorityOutcomeStatus() string { return "succeeded" }
func (cycleAuthorityOutcomeMock) AuthorityObserved() bool        { return true }
func (cycleAuthorityOutcomeMock) AuthorityVerified() bool        { return true }
func (cycleAuthorityOutcomeMock) AuthorityWorkID() string        { return "work-cycle-authority" }
func (cycleAuthorityOutcomeMock) AuthorityAttemptNumber() int    { return 7 }
func (cycleAuthorityOutcomeMock) AuthorityOutcomeCode() string   { return "verified" }
func (cycleAuthorityOutcomeMock) AuthorityFailureReason() string { return "" }
func (cycleAuthorityOutcomeMock) AuthoritySummary() string       { return "executed and verified" }

type cycleAuthorityExecutor struct{}

func (cycleAuthorityExecutor) ExecuteAction(context.Context, ActionRequest) ActionResponse {
	return ActionResponse{RequestID: "act-feedback", Executed: true, Output: "ejecutado", AuthorityResult: cycleAuthorityOutcomeMock{}}
}

func TestCyclePreservesAuthorityResultWhenEnvironmentalVerificationContradicts(t *testing.T) {
	updater := &cycleUpdaterMock{}
	cycle := NewCycle(New(), nil, nil, cycleFeedbackPlanner{}, cycleAuthorityExecutor{}, nil, nil, nil, updater)
	cycle.SetEnvironmentVerifier(NewEnvironmentVerifier(cycleStateInspector{}))
	if err := cycle.RunStep(context.Background()); err != nil {
		t.Fatalf("RunStep() error = %v", err)
	}
	got := updater.Last()
	if got.Success || got.ObservationStatus != ObservationContradicted {
		t.Fatalf("expected environmental contradiction, got %+v", got)
	}
	if got.AuthorityResult == nil {
		t.Fatal("environmental verification composition discarded the canonical authority result")
	}
	if got.AuthorityResult.AuthorityWorkID() != "work-cycle-authority" || got.AuthorityResult.AuthorityAttemptNumber() != 7 {
		t.Fatalf("authority identity changed across environmental verification: work=%q attempt=%d", got.AuthorityResult.AuthorityWorkID(), got.AuthorityResult.AuthorityAttemptNumber())
	}
}

type cycleUnknownAuthorityOutcomeMock struct{ cycleAuthorityOutcomeMock }

func (cycleUnknownAuthorityOutcomeMock) AuthorityOutcomeStatus() string { return "unknown" }

type cycleUnknownAuthorityExecutor struct{}

func (cycleUnknownAuthorityExecutor) ExecuteAction(context.Context, ActionRequest) ActionResponse {
	return ActionResponse{RequestID: "act-feedback", Executed: true, Output: "efecto incierto", AuthorityResult: cycleUnknownAuthorityOutcomeMock{}}
}

func TestCycleDoesNotDiagnoseUnknownOutcomeAsKnownFailure(t *testing.T) {
	diagnoser := &cycleDiagnoserMock{}
	updater := &cycleUpdaterMock{}
	cycle := NewCycle(New(), nil, nil, cycleFeedbackPlanner{}, cycleUnknownAuthorityExecutor{}, nil, diagnoser, nil, updater)
	if err := cycle.RunStep(context.Background()); err != nil {
		t.Fatalf("RunStep() error = %v", err)
	}
	if got := diagnoser.Calls(); got != 0 {
		t.Fatalf("unknown outcome was sent to failure diagnosis %d times", got)
	}
	got := updater.Last()
	if got.AuthorityResult == nil || got.AuthorityResult.AuthorityOutcomeStatus() != "unknown" {
		t.Fatalf("unknown authority result was not preserved for state update: %+v", got)
	}
	if got.Success {
		t.Fatal("unknown outcome must not be promoted to success")
	}
}

type cycleStructuredEvidenceInspector struct{}

func (cycleStructuredEvidenceInspector) InspectAction(_ context.Context, req ActionRequest) (EnvironmentObservation, error) {
	return EnvironmentObservation{
		Confirmed:  true,
		Status:     ObservationConfirmed,
		Details:    "resource identity and postcondition observed",
		ActionID:   req.ID,
		Operation:  req.Action,
		Target:     req.Target,
		Source:     "test.filesystem",
		ObservedAt: time.Now().UTC(),
		Facts: []ObservationFact{{
			Subject: req.Target, Predicate: "filesystem.object_type", Expected: "directory", Observed: "directory",
		}},
	}, nil
}

func TestCyclePreservesStructuredEnvironmentalEvidenceIntoUpdater(t *testing.T) {
	updater := &cycleUpdaterMock{}
	cycle := NewCycle(New(), nil, nil, cycleFeedbackPlanner{}, cycleSuccessExecutor{}, nil, nil, nil, updater)
	cycle.SetEnvironmentVerifier(NewEnvironmentVerifier(cycleStructuredEvidenceInspector{}))
	if err := cycle.RunStep(context.Background()); err != nil {
		t.Fatalf("RunStep() error = %v", err)
	}
	got := updater.Last()
	if !got.Success || got.ObservationStatus != ObservationConfirmed {
		t.Fatalf("expected confirmed verification, got success=%t status=%q", got.Success, got.ObservationStatus)
	}
	obs := got.Observation
	if obs.ActionID != "act-feedback" || obs.Operation != "crear_carpeta" || obs.Target != "x" || obs.Source != "test.filesystem" || obs.ObservedAt.IsZero() {
		t.Fatalf("observation provenance was lost or changed across cycle: %+v", obs)
	}
	if len(obs.Facts) != 1 || obs.Facts[0].Subject != "x" || obs.Facts[0].Predicate != "filesystem.object_type" || obs.Facts[0].Expected != obs.Facts[0].Observed {
		t.Fatalf("structured facts were lost or changed across cycle: %+v", obs.Facts)
	}
}
