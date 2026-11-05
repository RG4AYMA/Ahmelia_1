package core

import "ahmelia/autonomy"

// FailureCategory classifies the nature of one execution failure. B92.1
// only enumerates the categories the real Cycle evidence can support today
// (ActionResponse.Error, VerificationResult.ErrorReason, an
// EnvironmentSnapshot mismatch). It does not attempt to predict categories
// no diagnoser can produce yet.
type FailureCategory string

const (
	FailureCategoryMissingResource     FailureCategory = "missing_resource"
	FailureCategoryPermission          FailureCategory = "permission"
	FailureCategoryInvalidParameters   FailureCategory = "invalid_parameters"
	FailureCategoryEnvironmentMismatch FailureCategory = "environment_mismatch"
	FailureCategoryTimeout             FailureCategory = "timeout"
	FailureCategoryExecutorError       FailureCategory = "executor_error"
	FailureCategoryUnknown             FailureCategory = "unknown"
)

// IsValid reports whether c is one of the defined failure categories.
func (c FailureCategory) IsValid() bool {
	switch c {
	case FailureCategoryMissingResource,
		FailureCategoryPermission,
		FailureCategoryInvalidParameters,
		FailureCategoryEnvironmentMismatch,
		FailureCategoryTimeout,
		FailureCategoryExecutorError,
		FailureCategoryUnknown:
		return true
	default:
		return false
	}
}

// FailureCause identifies which stage of the real B78 cycle produced the
// evidence for a Failure. It intentionally mirrors the three concrete
// sources RunStep already has: the executor's ActionResponse, the
// ActionVerifier's VerificationResult, and — when present — the B80
// EnvironmentVerifier/EnvironmentSnapshot evidence.
type FailureCause string

const (
	FailureCauseAction       FailureCause = "action"
	FailureCauseVerification FailureCause = "verification"
	FailureCauseEnvironment  FailureCause = "environment"
	FailureCauseUnknown      FailureCause = "unknown"
)

// IsValid reports whether c is one of the defined failure causes.
func (c FailureCause) IsValid() bool {
	switch c {
	case FailureCauseAction, FailureCauseVerification, FailureCauseEnvironment, FailureCauseUnknown:
		return true
	default:
		return false
	}
}

// FailureRecoveryStrategy is the strategy a diagnosed Failure suggests.
//
// This is deliberately a distinct type from B91's RecoveryDecision.
// RecoveryDecision (B91.1) answers "should this Goal resume after a
// restart?". FailureRecoveryStrategy answers "what should happen about one
// Action that just failed, in a live cycle?". They are different questions
// over different evidence and must not share a name or a value set.
type FailureRecoveryStrategy string

const (
	StrategyRetry   FailureRecoveryStrategy = "retry"
	StrategyReplan  FailureRecoveryStrategy = "replan"
	StrategyWait    FailureRecoveryStrategy = "wait"
	StrategyBlock   FailureRecoveryStrategy = "block"
	StrategyAskUser FailureRecoveryStrategy = "ask_user"
)

// IsValid reports whether s is one of the defined recovery strategies.
func (s FailureRecoveryStrategy) IsValid() bool {
	switch s {
	case StrategyRetry, StrategyReplan, StrategyWait, StrategyBlock, StrategyAskUser:
		return true
	default:
		return false
	}
}

// Failure is the structured, factual description of one execution failure.
//
// B92.1 only defines this shape. It never infers Category/Cause/Confidence
// itself — that is B92.2's (Diagnoser) job — and it never decides what
// actually happens next — that is B92.3's (Recovery Strategy) job. A
// Failure only records what B92.2 concluded from real evidence.
type Failure struct {
	Category FailureCategory
	Cause    FailureCause

	// Evidence is the raw text the diagnosis is based on: e.g.
	// VerificationResult.ErrorReason, ActionResponse.Error.Error(), or a
	// literal environment-snapshot mismatch description. It is copied
	// verbatim from the real evidence, never paraphrased or invented.
	Evidence string

	// Recoverable reports whether B92.2 judged this failure as something
	// the system could plausibly act on (retry/replan/wait), as opposed to
	// a terminal condition that only BLOCK or ASK_USER can resolve.
	Recoverable bool

	// Confidence is B92.2's confidence in this diagnosis, in [0,1].
	Confidence float64

	// SuggestedRecovery is B92.2's suggestion only. B92.3 (Recovery
	// Strategy) is the sole component authorized to turn this into an
	// actual decision; nothing reads SuggestedRecovery as if it already
	// were one.
	SuggestedRecovery FailureRecoveryStrategy
}

// NewFailure builds a Failure from already-diagnosed facts. It does not
// validate business rules beyond what IsValid checks — callers are expected
// to call IsValid before trusting the result.
func NewFailure(
	category FailureCategory,
	cause FailureCause,
	evidence string,
	recoverable bool,
	confidence float64,
	suggested FailureRecoveryStrategy,
) *Failure {
	return &Failure{
		Category:          category,
		Cause:             cause,
		Evidence:          evidence,
		Recoverable:       recoverable,
		Confidence:        confidence,
		SuggestedRecovery: suggested,
	}
}

// IsValid validates the structural contract of a Failure. It never mutates
// f and never re-diagnoses anything; it only checks internal consistency.
func (f *Failure) IsValid() bool {
	if f == nil {
		return false
	}
	if !f.Category.IsValid() || !f.Cause.IsValid() || !f.SuggestedRecovery.IsValid() {
		return false
	}
	if f.Confidence < 0 || f.Confidence > 1 {
		return false
	}
	// A failure judged non-recoverable cannot honestly suggest RETRY: retrying
	// is, by definition, treating the failure as something that might
	// succeed on its own. REPLAN/WAIT/BLOCK/ASK_USER remain valid either way.
	if !f.Recoverable && f.SuggestedRecovery == StrategyRetry {
		return false
	}
	return true
}

// FailureContext is the immutable evidence B92.2 (Diagnoser) will read to
// produce a Failure. It reuses exactly the runtime types the real B78 cycle
// already produces — autonomy.ActionRequest, autonomy.ActionResponse and
// autonomy.VerificationResult — plus the optional B80 environment snapshot.
// B92.1 does not invent a new "ExecutionResult" type: Cycle.RunStep never
// produces one, so a diagnoser modeled on a type that does not exist in the
// real pipeline could not be wired without inventing a parallel path.
type FailureContext struct {
	Request      autonomy.ActionRequest
	Response     autonomy.ActionResponse
	Verification autonomy.VerificationResult

	// Snapshot is nil whenever the cycle had no EnvironmentVerifier
	// configured (SetEnvironmentVerifier was never called) or the failure
	// was detected before environment verification ran.
	Snapshot *autonomy.EnvironmentSnapshot
}

// NewFailureContext creates a FailureContext preserving the exact runtime
// values passed in. snapshot may be nil.
func NewFailureContext(
	req autonomy.ActionRequest,
	resp autonomy.ActionResponse,
	verif autonomy.VerificationResult,
	snapshot *autonomy.EnvironmentSnapshot,
) *FailureContext {
	return &FailureContext{
		Request:      req,
		Response:     resp,
		Verification: verif,
		Snapshot:     snapshot,
	}
}

// Valid reports whether the context actually describes a failure. A
// FailureContext built over a successful VerificationResult is not a
// diagnosable failure and is therefore invalid.
func (c *FailureContext) Valid() bool {
	if c == nil {
		return false
	}
	if c.Verification.Success {
		return false
	}
	return true
}

// FailureDiagnosis pairs the evidence (FailureContext) with the conclusion
// B92.2 drew from it (Failure). It mirrors the shape B91.1 used for
// RecoveryAssessment: data only, produced by a later block, never executing
// anything and never mutating Goal/Task/Plan state.
type FailureDiagnosis struct {
	Context *FailureContext
	Failure *Failure
}

// NewFailureDiagnosis creates a diagnosis from an existing context and
// failure.
func NewFailureDiagnosis(context *FailureContext, failure *Failure) *FailureDiagnosis {
	return &FailureDiagnosis{
		Context: context,
		Failure: failure,
	}
}

// Valid validates the structural contract of a diagnosis without changing
// any runtime state.
func (d *FailureDiagnosis) Valid() bool {
	if d == nil || d.Context == nil || d.Failure == nil {
		return false
	}
	if !d.Context.Valid() {
		return false
	}
	return d.Failure.IsValid()
}
