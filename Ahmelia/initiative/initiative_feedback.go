package initiative

import (
	"errors"
	"strings"
	"sync"
	"time"
)

// B96.37.15 observes what happened after an initiative. It records evidence;
// it does not learn, modify policy, or authorize future initiatives.
type InitiativeOutcome string

const (
	OutcomeUnknown     InitiativeOutcome = "UNKNOWN"
	OutcomeEngaged     InitiativeOutcome = "ENGAGED"
	OutcomeAccepted    InitiativeOutcome = "ACCEPTED"
	OutcomeRejected    InitiativeOutcome = "REJECTED"
	OutcomeIgnored     InitiativeOutcome = "IGNORED"
	OutcomeInterrupted InitiativeOutcome = "INTERRUPTED"
	OutcomeCompleted   InitiativeOutcome = "COMPLETED"
	OutcomeFailed      InitiativeOutcome = "FAILED"
	OutcomeExpired     InitiativeOutcome = "EXPIRED"
)

type ResponseClass string

const (
	ResponseNone         ResponseClass = "NONE"
	ResponseAffirmation  ResponseClass = "AFFIRMATION"
	ResponseRejection    ResponseClass = "REJECTION"
	ResponseContinuation ResponseClass = "CONTINUATION"
	ResponseCorrection   ResponseClass = "CORRECTION"
	ResponseInterruption ResponseClass = "INTERRUPTION"
	ResponseCompletion   ResponseClass = "COMPLETION"
	ResponseUnknown      ResponseClass = "UNKNOWN"
)

type InitiativeOutcomeRecord struct {
	ID               string            `json:"id"`
	InitiativeID     string            `json:"initiative_id"`
	Outcome          InitiativeOutcome `json:"outcome"`
	ResponseClass    ResponseClass     `json:"response_class"`
	Evidence         string            `json:"evidence,omitempty"`
	Confidence       float64           `json:"confidence"`
	InitiativeAction Action            `json:"initiative_action"`
	InitiativeReason Reason            `json:"initiative_reason"`
	ObservedAt       time.Time         `json:"observed_at"`
	ExpiresAt        time.Time         `json:"expires_at,omitempty"`
	Provenance       Provenance        `json:"provenance"`
}

type OutcomeObservation struct {
	Initiative    InitiativeDecision
	UserResponse  string
	ResponseClass ResponseClass
	Interrupted   bool
	Completed     bool
	Failed        bool
	Engaged       bool
	ObservedAt    time.Time
}

type OutcomePolicy struct{ FeedbackTTL time.Duration }

func DefaultOutcomePolicy() OutcomePolicy { return OutcomePolicy{FeedbackTTL: 30 * time.Minute} }

type OutcomeStore interface {
	Put(InitiativeOutcomeRecord) error
	Get(string) (InitiativeOutcomeRecord, bool)
	List() []InitiativeOutcomeRecord
}
type InMemoryOutcomeStore struct {
	mu    sync.RWMutex
	items map[string]InitiativeOutcomeRecord
}

func NewInMemoryOutcomeStore() *InMemoryOutcomeStore {
	return &InMemoryOutcomeStore{items: map[string]InitiativeOutcomeRecord{}}
}
func (s *InMemoryOutcomeStore) Put(x InitiativeOutcomeRecord) error {
	if strings.TrimSpace(x.ID) == "" {
		return errors.New("outcome requires id")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[x.ID] = x
	return nil
}
func (s *InMemoryOutcomeStore) Get(id string) (InitiativeOutcomeRecord, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	x, ok := s.items[id]
	return x, ok
}
func (s *InMemoryOutcomeStore) List() []InitiativeOutcomeRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	o := make([]InitiativeOutcomeRecord, 0, len(s.items))
	for _, x := range s.items {
		o = append(o, x)
	}
	return o
}

type OutcomeEvaluator struct {
	Store  OutcomeStore
	Policy OutcomePolicy
}

func NewOutcomeEvaluator(store OutcomeStore) *OutcomeEvaluator {
	if store == nil {
		store = NewInMemoryOutcomeStore()
	}
	return &OutcomeEvaluator{Store: store, Policy: DefaultOutcomePolicy()}
}

func ClassifyOutcome(o OutcomeObservation) (InitiativeOutcome, float64, string) {
	if o.Failed {
		return OutcomeFailed, .98, "initiative or downstream task explicitly failed"
	}
	if o.Interrupted {
		return OutcomeInterrupted, .95, "interaction was explicitly interrupted"
	}
	if o.Completed {
		return OutcomeCompleted, .95, "initiative-associated task or commitment completed"
	}
	if o.ResponseClass == ResponseRejection {
		return OutcomeRejected, .97, "user explicitly rejected the initiative"
	}
	if o.ResponseClass == ResponseAffirmation {
		return OutcomeAccepted, .98, "user explicitly accepted the initiative"
	}
	if o.Engaged || o.ResponseClass == ResponseContinuation {
		return OutcomeEngaged, .82, "user continued engaging with the initiative"
	}
	if strings.TrimSpace(o.UserResponse) == "" {
		return OutcomeUnknown, 0, "no response observed; absence is not rejection"
	}
	return OutcomeUnknown, .35, "response observed but outcome is not sufficiently classified"
}

func (e *OutcomeEvaluator) Evaluate(o OutcomeObservation) (InitiativeOutcomeRecord, error) {
	if e == nil || e.Store == nil {
		return InitiativeOutcomeRecord{}, errors.New("outcome evaluator unavailable")
	}
	now := o.ObservedAt
	if now.IsZero() {
		now = time.Now()
	}
	if o.Initiative.ID == "" {
		return InitiativeOutcomeRecord{}, errors.New("initiative outcome requires initiative id")
	}
	out, conf, ev := ClassifyOutcome(o)
	x := InitiativeOutcomeRecord{ID: "outcome-" + newID(now), InitiativeID: o.Initiative.ID, Outcome: out, ResponseClass: o.ResponseClass, Evidence: ev, Confidence: conf, InitiativeAction: o.Initiative.Action, InitiativeReason: o.Initiative.Reason, ObservedAt: now, Provenance: Provenance{Component: "initiative.OutcomeEvaluator", Subsystem: "B96.37.15", SignalType: string(out), OriginID: o.Initiative.ID, CreatedAt: now}}
	if e.Policy.FeedbackTTL > 0 {
		x.ExpiresAt = now.Add(e.Policy.FeedbackTTL)
	}
	return x, e.Store.Put(x)
}

// ExpireUnanswered records the end of an observation window as IGNORED.
// It must be called by the lifecycle/observation scheduler; silence before
// the window closes remains UNKNOWN.
func (e *OutcomeEvaluator) ExpireUnanswered(d InitiativeDecision, now time.Time) (InitiativeOutcomeRecord, error) {
	if e == nil || e.Store == nil {
		return InitiativeOutcomeRecord{}, errors.New("outcome evaluator unavailable")
	}
	if now.IsZero() {
		now = time.Now()
	}
	x := InitiativeOutcomeRecord{ID: "outcome-" + newID(now), InitiativeID: d.ID, Outcome: OutcomeIgnored, ResponseClass: ResponseNone, Evidence: "observation window expired without a user response; ignored is not rejection", Confidence: .55, InitiativeAction: d.Action, InitiativeReason: d.Reason, ObservedAt: now, Provenance: Provenance{Component: "initiative.OutcomeEvaluator", Subsystem: "B96.37.15", SignalType: string(OutcomeIgnored), OriginID: d.ID, CreatedAt: now}}
	if e.Policy.FeedbackTTL > 0 {
		x.ExpiresAt = now.Add(e.Policy.FeedbackTTL)
	}
	return x, e.Store.Put(x)
}

// ObserveResponse is deliberately conservative: text alone never turns silence into rejection.
func (e *OutcomeEvaluator) ObserveResponse(d InitiativeDecision, response string, class ResponseClass, now time.Time) (InitiativeOutcomeRecord, error) {
	return e.Evaluate(OutcomeObservation{Initiative: d, UserResponse: response, ResponseClass: class, ObservedAt: now})
}
