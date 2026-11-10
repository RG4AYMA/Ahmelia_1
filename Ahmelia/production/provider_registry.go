package production

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"ahmelia/autonomy"
	"ahmelia/capability"
	"ahmelia/interaction"
	"ahmelia/tools"
)

// Provider is a domain adapter for Production. It turns a resolved Capability
// and the original goal into a production attempt. It does not decide intent,
// authorization, recovery policy, or verification authority.
//
// Keeping this contract in Production is intentional: the Engine can remain
// an orchestrator while Programming, Unity, Blender, multimedia, automation,
// and future domains register providers behind the same boundary.
type Provider interface {
	ID() string
	Execute(context.Context, ProviderRequest) ProviderResult
}

// RepairProvider reconciles an already materialized result. Repair is a provider
// implementation concern: Cognition decides that repair is required, while the
// provider decides how to reconcile its own existing artifact safely.
type RepairProvider interface {
	Provider
	Repair(context.Context, RepairIntent) ProviderResult
}

// ProviderRequest is the domain-neutral input handed to a production
// provider. Capability is the declarative contract selected by the resolver.
// CapabilityID repeats the selected identity explicitly so the execution
// boundary can prove reference coherence without introducing tools.Action.
type ProviderRequest struct {
	Input           string
	Target          string
	WorkID          string
	AttemptNumber   int
	RecoveryContext *tools.RecoveryContext
	// RecoveryEvidence is retained as a legacy compatibility field. New recovery
	// paths must preserve RecoveryContext instead of flattening evidence to text.
	RecoveryEvidence         string
	EffectRecorder           func([]byte) error
	EffectEvaluationRecorder func([]byte) error
	Project                  ProductionProject
	Parameters               map[string]string
	Capability               capability.Capability
	// CapabilityID is the execution-bound identity. It must equal
	// Capability.ID at the provider boundary.
	CapabilityID capability.ID
	// Operation is the resolved capability action, not the original natural
	// language operation, so provider matching stays provider-neutral.
	Operation string
	// ObjectiveFrame is the semantic contract built before execution. Providers
	// consume this structured meaning instead of reparsing the human sentence.
	ObjectiveFrame interaction.ObjectiveFrame
}

// Clone returns a provider-safe copy so a provider cannot mutate the semantic
// request map owned by the caller.
func (r ProviderRequest) Clone() ProviderRequest {
	r.Parameters = cloneProviderParams(r.Parameters)
	return r
}

func cloneProviderParams(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// RepairIntent preserves the complete prior materialized result so a provider
// can reconcile an existing artifact instead of blindly creating a duplicate.
type RepairIntent struct {
	WorkID         string
	Target         string
	Replacement    string
	PreviousResult ActionResult
	Confidence     float64
	Reason         string
}

// OutcomeStatus preserves whether the provider can actually establish the
// business outcome independently from whether the physical call returned
// without an error. Unknown is intentional: execution may have happened while
// the environment was not observable enough to establish success or failure.
type OutcomeStatus string

const (
	OutcomeUnknown   OutcomeStatus = "unknown"
	OutcomeSucceeded OutcomeStatus = "succeeded"
	OutcomeFailed    OutcomeStatus = "failed"
)

func (s OutcomeStatus) Valid() bool {
	switch s {
	case OutcomeUnknown, OutcomeSucceeded, OutcomeFailed:
		return true
	default:
		return false
	}
}

// ProviderResult contains domain-neutral production facts. Providers may
// populate Summary/FailureReason, verification status, and generic artifacts
// without teaching the Engine about their internal object model.
type ProviderResult struct {
	ProgressChecks        []ProgressCheck // observed by the provider harness, never model prose
	Success               bool
	GenerationSource      string
	VerificationProfile   string
	ObjectiveStatus       string
	ObjectiveContractJSON string
	OutcomeStatus         OutcomeStatus
	Verified              bool
	Observed              bool
	Summary               string
	ObservationDetails    string
	// ObservationFacts carries the structured postcondition evidence produced
	// by the provider's independent observer. It is the raw material of the
	// expectation verdict; prose is never enough.
	ObservationFacts []ObservedPostconditionFact
	// ExpectationID/Status/Summary identify the durable act declared before
	// execution and its harness verdict. Status is a string at this boundary
	// so production does not own the expectation package vocabulary.
	ExpectationID      string
	ExpectationStatus  string
	ExpectationSummary string
	// ExpectationError makes a failed declaration or verdict write visible
	// without inventing a confirmed outcome.
	ExpectationError   string
	VerificationReason string
	VerificationSource string
	OutcomeCode        string
	InputRequired      bool
	ProductionID       string
	Producer           string
	Project            ProductionProject
	Artifacts          []ProductionArtifact
	FailedStage        string
	FailureReason      string
	RecoveryContext    *tools.RecoveryContext
	Err                error
}

// AuthorityExecutionResult is the canonical result at the Engine execution
// authority boundary. It deliberately separates control-flow handling from
// physical execution and behavioral verification. A single boolean cannot
// safely represent these states.
type AuthorityExecutionResult struct {
	Handled                   bool                    `json:"handled"`
	Executed                  bool                    `json:"executed"`
	Succeeded                 bool                    `json:"succeeded"`
	OutcomeStatus             OutcomeStatus           `json:"outcome_status"`
	Observed                  bool                    `json:"observed"`
	Verified                  bool                    `json:"verified"`
	AuthorizationRequired     bool                    `json:"authorization_required,omitempty"`
	AuthorizationCapabilityID capability.ID           `json:"authorization_capability_id,omitempty"`
	AuthorizationReason       string                  `json:"authorization_reason,omitempty"`
	WorkID                    string                  `json:"work_id,omitempty"`
	AttemptNumber             int                     `json:"attempt_number,omitempty"`
	BackgroundJobID           string                  `json:"background_job_id,omitempty"`
	OutcomeCode               string                  `json:"outcome_code,omitempty"`
	FailureReason             string                  `json:"failure_reason,omitempty"`
	Recovery                  *tools.RecoveryDecision `json:"recovery,omitempty"`
	RecoveryContext           *tools.RecoveryContext  `json:"recovery_context,omitempty"`
	Summary                   string                  `json:"summary,omitempty"`
}

// AuthorityExecutionResult implements autonomy.AuthorityOutcome so the exact
// canonical result can cross into the Cycle without making autonomy import
// production or introducing a duplicate evidence/result structure.
func (r AuthorityExecutionResult) AuthorityHandled() bool         { return r.Handled }
func (r AuthorityExecutionResult) AuthorityExecuted() bool        { return r.Executed }
func (r AuthorityExecutionResult) AuthoritySucceeded() bool       { return r.Succeeded }
func (r AuthorityExecutionResult) AuthorityOutcomeStatus() string { return string(r.OutcomeStatus) }
func (r AuthorityExecutionResult) AuthorityObserved() bool        { return r.Observed }
func (r AuthorityExecutionResult) AuthorityVerified() bool        { return r.Verified }
func (r AuthorityExecutionResult) AuthorityWorkID() string        { return r.WorkID }
func (r AuthorityExecutionResult) AuthorityAttemptNumber() int    { return r.AttemptNumber }
func (r AuthorityExecutionResult) AuthorityOutcomeCode() string   { return r.OutcomeCode }
func (r AuthorityExecutionResult) AuthorityFailureReason() string { return r.FailureReason }
func (r AuthorityExecutionResult) AuthoritySummary() string       { return r.Summary }

var _ autonomy.AuthorityOutcome = AuthorityExecutionResult{}

func AuthorityResultFromAction(result ActionResult) AuthorityExecutionResult {
	return AuthorityExecutionResult{
		Handled: true, Executed: result.Executed, Succeeded: result.Success, OutcomeStatus: result.OutcomeStatus,
		Observed: result.Observed, Verified: result.Verified, AttemptNumber: result.AttemptNumber,
		AuthorizationRequired:     result.AuthorizationRequired,
		AuthorizationCapabilityID: result.AuthorizationCapabilityID,
		AuthorizationReason:       result.AuthorizationReason, WorkID: result.WorkID,
		OutcomeCode: result.OutcomeCode, FailureReason: result.FailureReason,
		Recovery: result.Recovery, RecoveryContext: result.RecoveryContext, Summary: result.Summary,
	}
}

// ObservedPostconditionFact records one resource-specific fact used to evaluate
// an expected effect. It is durable evidence data, not a narrative summary.
type ObservedPostconditionFact struct {
	Subject   string `json:"subject"`
	Predicate string `json:"predicate"`
	Expected  string `json:"expected"`
	Observed  string `json:"observed"`
}

// ActionResult is the structured outcome of production work. It contains
// operational facts only; it does not contain a conversational sentence or
// language-generation decision. The caller decides how to communicate it.
type ActionResult struct {
	RetryState     *RetryState     `json:"retry_state,omitempty"`
	ProgressChecks []ProgressCheck `json:"progress_checks,omitempty"`
	// Executed means the provider crossed the physical execution boundary.
	// It is distinct from Success (business outcome) and Verified (behavioral evidence).
	Executed              bool                        `json:"executed,omitempty"`
	GenerationSource      string                      `json:"generation_source,omitempty"`
	VerificationProfile   string                      `json:"verification_profile,omitempty"`
	ObjectiveStatus       string                      `json:"objective_status,omitempty"`
	ObjectiveContractJSON string                      `json:"objective_contract_json,omitempty"`
	Success               bool                        `json:"success"`
	OutcomeStatus         OutcomeStatus               `json:"outcome_status,omitempty"`
	Verified              bool                        `json:"verified"`
	Observed              bool                        `json:"observed,omitempty"`
	Summary               string                      `json:"summary,omitempty"`
	ObservationDetails    string                      `json:"observation_details,omitempty"`
	ObservationActionID   string                      `json:"observation_action_id,omitempty"`
	ObservationOperation  string                      `json:"observation_operation,omitempty"`
	ObservationTarget     string                      `json:"observation_target,omitempty"`
	ObservationSource     string                      `json:"observation_source,omitempty"`
	ObservationCapturedAt time.Time                   `json:"observation_captured_at,omitempty"`
	ObservationFacts      []ObservedPostconditionFact `json:"observation_facts,omitempty"`
	ExpectationID         string                      `json:"expectation_id,omitempty"`
	ExpectationStatus     string                      `json:"expectation_status,omitempty"`
	ExpectationSummary    string                      `json:"expectation_summary,omitempty"`
	ExpectationError      string                      `json:"expectation_error,omitempty"`
	VerificationReason    string                      `json:"verification_reason,omitempty"`
	VerificationSource    string                      `json:"verification_source,omitempty"`
	OutcomeCode           string                      `json:"outcome_code,omitempty"`
	InputRequired         bool                        `json:"input_required,omitempty"`
	WorkID                string                      `json:"work_id,omitempty"`
	AttemptNumber         int                         `json:"attempt_number,omitempty"`
	ProductionID          string                      `json:"production_id,omitempty"`
	Producer              string                      `json:"producer,omitempty"`
	Project               ProductionProject           `json:"project,omitempty"`
	Artifacts             []ProductionArtifact        `json:"artifacts,omitempty"`
	FailedStage           string                      `json:"failed_stage,omitempty"`
	FailureReason         string                      `json:"failure_reason,omitempty"`
	Err                   error                       `json:"-"`

	// AuthorizationRequired is a structured handoff to the identity/permission
	// owner. Production never asks or answers the user itself.
	AuthorizationRequired     bool          `json:"authorization_required,omitempty"`
	AuthorizationCapabilityID capability.ID `json:"authorization_capability_id,omitempty"`
	AuthorizationReason       string        `json:"authorization_reason,omitempty"`

	// Recovery is a policy result. It is returned upward; ProductionRuntime
	// never applies Retry/Replan/Block/AskUser itself.
	Recovery        *tools.RecoveryDecision `json:"recovery,omitempty"`
	RecoveryContext *tools.RecoveryContext  `json:"recovery_context,omitempty"`
}

func (r ActionResult) Confidence() float64 {
	if r.OutcomeStatus == OutcomeSucceeded || (r.Success && r.Verified) {
		return 0.98
	}
	if r.OutcomeStatus == OutcomeUnknown || (r.Success && !r.Verified) {
		return 0.50
	}
	return 0.85
}

// ProviderRegistry maps resolved capabilities to their production provider.
// It is intentionally separate from capability.Registry: Registry answers
// "what can Ahmelia do?", while this registry answers "which implementation
// can produce it right now?".
type ProviderRegistry struct {
	mu       sync.RWMutex
	byID     map[capability.ID]map[string]Provider
	byDomain map[capability.Domain]map[string]Provider
}

// NewProviderRegistry creates an empty runtime provider registry.
func NewProviderRegistry() *ProviderRegistry {
	return &ProviderRegistry{
		byID:     make(map[capability.ID]map[string]Provider),
		byDomain: make(map[capability.Domain]map[string]Provider),
	}
}

// RegisterCapabilities adds or refreshes an implementation under its stable
// provider ID. Different provider IDs may implement the same capability and
// remain selectable alternatives; re-registering the same ID updates that
// implementation, preserving deterministic fixture/runtime replacement.
func (r *ProviderRegistry) RegisterCapabilities(p Provider, ids ...capability.ID) error {
	if r == nil {
		return fmt.Errorf("production provider registry: registry nil")
	}
	if p == nil || p.ID() == "" {
		return fmt.Errorf("production provider registry: provider inválido")
	}
	if len(ids) == 0 {
		return fmt.Errorf("production provider registry: ninguna capability")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for _, id := range ids {
		if id == "" {
			return fmt.Errorf("production provider registry: capability ID vacío")
		}
	}
	for _, id := range ids {
		if r.byID[id] == nil {
			r.byID[id] = make(map[string]Provider)
		}
		r.byID[id][p.ID()] = p
	}
	return nil
}

// RegisterDomain binds a provider as the default implementation for a
// domain. Capability-specific bindings always take precedence at Resolve.
func (r *ProviderRegistry) RegisterDomain(domain capability.Domain, p Provider) error {
	if r == nil {
		return fmt.Errorf("production provider registry: registry nil")
	}
	if domain == "" {
		return fmt.Errorf("production provider registry: domain vacío")
	}
	if p == nil || p.ID() == "" {
		return fmt.Errorf("production provider registry: provider inválido")
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.byDomain[domain] == nil {
		r.byDomain[domain] = make(map[string]Provider)
	}
	r.byDomain[domain][p.ID()] = p
	return nil
}

// Resolve finds the most specific provider for a capability: exact
// capability binding first, then the domain default. It succeeds only when
// that set contains exactly one provider; ambiguous sets require selection.
func (r *ProviderRegistry) Resolve(c capability.Capability) (Provider, bool) {
	candidates := r.Candidates(c)
	if len(candidates) != 1 {
		return nil, false
	}
	return candidates[0], true
}

// Candidates returns all implementations for one stable capability. Exact
// capability bindings take precedence over domain defaults; IDs are sorted so
// callers can make explicit, reproducible selections without taking the first
// registration or silently changing providers.
func (r *ProviderRegistry) Candidates(c capability.Capability) []Provider {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()

	providers := r.byID[c.ID]
	if len(providers) == 0 {
		providers = r.byDomain[c.Domain]
	}
	out := make([]Provider, 0, len(providers))
	for _, p := range providers {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID() < out[j].ID() })
	return out
}

// ResolveFor resolves an explicitly selected instrument for a capability.
// It never falls back to another provider when the requested ID is invalid.
func (r *ProviderRegistry) ResolveFor(c capability.Capability, providerID string) (Provider, bool) {
	for _, p := range r.Candidates(c) {
		if p.ID() == strings.TrimSpace(providerID) {
			return p, true
		}
	}
	return nil, false
}

// ResolveProvider returns the provider registered under its stable provider ID.
// Repair uses the producer recorded in the previous ActionResult so it can
// reconcile the already-materialized artifact without rediscovering a new domain.
func (r *ProviderRegistry) ResolveProvider(id string) (Provider, bool) {
	if r == nil {
		return nil, false
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, providers := range r.byID {
		if p := providers[id]; p != nil {
			return p, true
		}
	}
	for _, providers := range r.byDomain {
		if p := providers[id]; p != nil {
			return p, true
		}
	}
	return nil, false
}

// List returns registered providers in deterministic order.
func (r *ProviderRegistry) List() []Provider {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()

	seen := map[string]Provider{}
	for _, providers := range r.byID {
		for _, p := range providers {
			seen[p.ID()] = p
		}
	}
	for _, providers := range r.byDomain {
		for _, p := range providers {
			seen[p.ID()] = p
		}
	}
	out := make([]Provider, 0, len(seen))
	for _, p := range seen {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID() < out[j].ID() })
	return out
}
