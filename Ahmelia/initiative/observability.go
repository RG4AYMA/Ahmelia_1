package initiative

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

// B96.37.20 — Initiative Observability & Decision Trace.
// Observability records evidence, decisions and lifecycle facts already
// produced by their owning authorities. It never authorizes, generates or
// executes an initiative and is not a replacement for ConversationHistory.
// EVENT != TRACE != STATE != DECISION != OUTCOME.

type TraceStatus string

const (
	TraceActive    TraceStatus = "ACTIVE"
	TraceCompleted TraceStatus = "COMPLETED"
	TraceExpired   TraceStatus = "EXPIRED"
)

type TraceEventType string

const (
	TraceEventSignal      TraceEventType = "SIGNAL"
	TraceEventOpportunity TraceEventType = "OPPORTUNITY"
	TraceEventEvaluation  TraceEventType = "EVALUATION"
	TraceEventArbitration TraceEventType = "ARBITRATION"
	TraceEventDecision    TraceEventType = "DECISION"
	TraceEventAction      TraceEventType = "ACTION"
	TraceEventTiming      TraceEventType = "TIMING"
	TraceEventLifecycle   TraceEventType = "LIFECYCLE"
	TraceEventOutcome     TraceEventType = "OUTCOME"
	TraceEventConstraint  TraceEventType = "CONSTRAINT"
	TraceEventReason      TraceEventType = "REASON"
	TraceEventProvenance  TraceEventType = "PROVENANCE"
	TraceEventCorrelation TraceEventType = "CORRELATION"
	TraceEventLegacy      TraceEventType = "LEGACY_MIGRATION"
)

type InitiativeTrace struct {
	ID            string       `json:"id"`
	InitiativeID  string       `json:"initiative_id"`
	OpportunityID string       `json:"opportunity_id,omitempty"`
	SignalIDs     []string     `json:"signal_ids,omitempty"`
	DecisionID    string       `json:"decision_id,omitempty"`
	ActionID      string       `json:"action_id,omitempty"`
	ScheduleID    string       `json:"schedule_id,omitempty"`
	OutcomeID     string       `json:"outcome_id,omitempty"`
	LifecycleID   string       `json:"lifecycle_id,omitempty"`
	CreatedAt     time.Time    `json:"created_at"`
	UpdatedAt     time.Time    `json:"updated_at"`
	Status        TraceStatus  `json:"status"`
	Source        Source       `json:"source"`
	Provenance    Provenance   `json:"provenance"`
	Events        []TraceEvent `json:"events,omitempty"`
}

type TraceEvent struct {
	ID          string                   `json:"id"`
	Type        TraceEventType           `json:"type"`
	At          time.Time                `json:"at"`
	Authority   string                   `json:"authority,omitempty"`
	Component   string                   `json:"component,omitempty"`
	Summary     string                   `json:"summary,omitempty"`
	Evidence    string                   `json:"evidence,omitempty"`
	Signal      *InitiativeSignal        `json:"signal,omitempty"`
	Opportunity *InitiativeOpportunity   `json:"opportunity,omitempty"`
	Decision    *InitiativeDecision      `json:"decision,omitempty"`
	Action      *ActionContract          `json:"action,omitempty"`
	Schedule    *Schedule                `json:"schedule,omitempty"`
	Lifecycle   *InitiativeLifecycle     `json:"lifecycle,omitempty"`
	Outcome     *InitiativeOutcomeRecord `json:"outcome,omitempty"`
	Arbitration *ArbitrationResult       `json:"arbitration,omitempty"`
	Evaluation  map[string]string        `json:"evaluation,omitempty"`
	Constraints []InitiativeConstraint   `json:"constraints,omitempty"`
	Correlation map[string]string        `json:"correlation,omitempty"`
	Provenance  Provenance               `json:"provenance"`
}

type TraceStore interface {
	Put(InitiativeTrace) error
	Get(string) (InitiativeTrace, bool)
	List() []InitiativeTrace
	Delete(string) error
}

type InMemoryTraceStore struct {
	mu    sync.RWMutex
	items map[string]InitiativeTrace
}

func NewInMemoryTraceStore() *InMemoryTraceStore {
	return &InMemoryTraceStore{items: map[string]InitiativeTrace{}}
}
func (s *InMemoryTraceStore) Put(x InitiativeTrace) error {
	if s == nil {
		return errors.New("trace store unavailable")
	}
	if strings.TrimSpace(x.ID) == "" || strings.TrimSpace(x.InitiativeID) == "" {
		return errors.New("trace requires id and initiative id")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[x.ID] = cloneTrace(x)
	return nil
}
func (s *InMemoryTraceStore) Get(id string) (InitiativeTrace, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	x, ok := s.items[id]
	if !ok {
		return InitiativeTrace{}, false
	}
	return cloneTrace(x), true
}
func (s *InMemoryTraceStore) List() []InitiativeTrace {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]InitiativeTrace, 0, len(s.items))
	for _, x := range s.items {
		out = append(out, cloneTrace(x))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out
}
func (s *InMemoryTraceStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.items, id)
	return nil
}

type TracePolicy struct {
	TTL       time.Duration
	MaxEvents int
}

func DefaultTracePolicy() TracePolicy { return TracePolicy{TTL: 24 * time.Hour, MaxEvents: 256} }

type InitiativeObservability struct {
	Store  TraceStore
	Policy TracePolicy
	mu     sync.Mutex
	now    func() time.Time
}

func NewInitiativeObservability(store TraceStore) *InitiativeObservability {
	if store == nil {
		store = NewInMemoryTraceStore()
	}
	return &InitiativeObservability{Store: store, Policy: DefaultTracePolicy(), now: time.Now}
}

func (o *InitiativeObservability) Create(initiativeID string, source Source, provenance Provenance, now time.Time) (InitiativeTrace, error) {
	if o == nil || o.Store == nil {
		return InitiativeTrace{}, errors.New("observability unavailable")
	}
	if strings.TrimSpace(initiativeID) == "" {
		return InitiativeTrace{}, errors.New("trace requires initiative id")
	}
	if now.IsZero() {
		now = o.now()
	}
	x := InitiativeTrace{ID: "trace-" + newID(now), InitiativeID: initiativeID, CreatedAt: now, UpdatedAt: now, Status: TraceActive, Source: source, Provenance: provenance}
	if o.Policy.TTL > 0 { /* retention is evaluated by Expire; no semantic field is added to history */
	}
	if err := o.Store.Put(x); err != nil {
		return InitiativeTrace{}, err
	}
	return x, nil
}

func (o *InitiativeObservability) RebindInitiative(traceID, initiativeID string, at time.Time) (InitiativeTrace, error) {
	if o == nil || o.Store == nil {
		return InitiativeTrace{}, errors.New("observability unavailable")
	}
	if strings.TrimSpace(initiativeID) == "" {
		return InitiativeTrace{}, errors.New("initiative id required")
	}
	if at.IsZero() {
		at = o.now()
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	x, ok := o.Store.Get(traceID)
	if !ok {
		return InitiativeTrace{}, errors.New("trace not found")
	}
	x.InitiativeID = initiativeID
	x.UpdatedAt = at
	if err := o.Store.Put(x); err != nil {
		return InitiativeTrace{}, err
	}
	return x, nil
}

func (o *InitiativeObservability) Append(traceID string, event TraceEvent) (InitiativeTrace, error) {
	if o == nil || o.Store == nil {
		return InitiativeTrace{}, errors.New("observability unavailable")
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	x, ok := o.Store.Get(traceID)
	if !ok {
		return InitiativeTrace{}, errors.New("trace not found")
	}
	if x.Status == TraceExpired {
		return x, errors.New("trace expired")
	}
	if event.At.IsZero() {
		event.At = o.now()
	}
	if event.ID == "" {
		event.ID = "event-" + newID(event.At)
	}
	if event.Provenance.CreatedAt.IsZero() {
		event.Provenance.CreatedAt = event.At
	}
	x.Events = append(x.Events, event)
	switch event.Type {
	case TraceEventSignal:
		if event.Signal != nil && event.Signal.ID != "" {
			x.SignalIDs = appendUniqueString(x.SignalIDs, event.Signal.ID)
		}
	case TraceEventOpportunity:
		if event.Opportunity != nil && event.Opportunity.ID != "" {
			x.OpportunityID = event.Opportunity.ID
		} else if x.OpportunityID == "" {
			x.OpportunityID = event.ID
		}
	case TraceEventDecision:
		if event.Decision != nil {
			x.DecisionID = event.Decision.ID
		}
	case TraceEventAction:
		if event.Action != nil {
			x.ActionID = event.Action.ID
			if x.ActionID == "" {
				x.ActionID = "action-" + event.Action.DecisionID
			}
		}
	case TraceEventLifecycle:
		if event.Lifecycle != nil {
			x.LifecycleID = event.Lifecycle.ID
		}
	case TraceEventOutcome:
		if event.Outcome != nil {
			x.OutcomeID = event.Outcome.ID
		}
	case TraceEventTiming:
		if event.Schedule != nil {
			x.ScheduleID = event.Schedule.ID
		}
	}
	if o.Policy.MaxEvents > 0 && len(x.Events) > o.Policy.MaxEvents {
		x.Events = append([]TraceEvent(nil), x.Events[len(x.Events)-o.Policy.MaxEvents:]...)
	}
	x.UpdatedAt = event.At
	if err := o.Store.Put(x); err != nil {
		return InitiativeTrace{}, err
	}
	return x, nil
}

func (o *InitiativeObservability) RecordSignal(traceID string, s InitiativeSignal, at time.Time) (InitiativeTrace, error) {
	return o.Append(traceID, TraceEvent{Type: TraceEventSignal, At: at, Authority: "B96.37.3/B96.37.4", Summary: s.Type, Evidence: s.Value, Signal: &s, Provenance: s.Provenance})
}
func (o *InitiativeObservability) RecordOpportunity(traceID string, op InitiativeOpportunity, at time.Time) (InitiativeTrace, error) {
	return o.Append(traceID, TraceEvent{Type: TraceEventOpportunity, At: at, Authority: "B96.37.3", Summary: string(op.Reason), Opportunity: &op, Provenance: op.Signal.Provenance})
}
func (o *InitiativeObservability) RecordEvaluation(traceID, authority string, e map[string]string, at time.Time) (InitiativeTrace, error) {
	return o.Append(traceID, TraceEvent{Type: TraceEventEvaluation, At: at, Authority: authority, Evaluation: copyMap(e), Provenance: Provenance{Component: "initiative.InitiativeObservability", Subsystem: "B96.37.20", CreatedAt: at}})
}
func (o *InitiativeObservability) RecordArbitration(traceID string, a ArbitrationResult, at time.Time) (InitiativeTrace, error) {
	return o.Append(traceID, TraceEvent{Type: TraceEventArbitration, At: at, Authority: "B96.37.17", Summary: "candidate comparison", Arbitration: &a, Provenance: a.Provenance})
}
func (o *InitiativeObservability) RecordDecision(traceID string, d InitiativeDecision, at time.Time) (InitiativeTrace, error) {
	return o.Append(traceID, TraceEvent{Type: TraceEventDecision, At: at, Authority: "B96.37.2", Summary: string(d.Decision), Decision: &d, Constraints: append([]InitiativeConstraint(nil), d.Constraints...), Provenance: d.Provenance})
}
func (o *InitiativeObservability) RecordAction(traceID string, a ActionContract, at time.Time) (InitiativeTrace, error) {
	return o.Append(traceID, TraceEvent{Type: TraceEventAction, At: at, Authority: "B96.37.7", Summary: string(a.Action), Action: &a, Provenance: Provenance{Component: "initiative.InitiativeObservability", Subsystem: "B96.37.20", OriginID: a.DecisionID, CreatedAt: at}})
}
func (o *InitiativeObservability) RecordTiming(traceID, authority string, timing Timing, assessment string, cooldown bool, availability UserAvailability, interruptibility Interruptibility, scheduledAt, triggeredAt time.Time, at time.Time) (InitiativeTrace, error) {
	return o.Append(traceID, TraceEvent{Type: TraceEventTiming, At: at, Authority: authority, Summary: string(timing), Evaluation: map[string]string{"timing": string(timing), "assessment": assessment, "cooldown": boolString(cooldown), "availability": string(availability), "interruptibility": string(interruptibility), "scheduled_at": timeString(scheduledAt), "triggered_at": timeString(triggeredAt)}, Provenance: Provenance{Component: "initiative.InitiativeObservability", Subsystem: "B96.37.20", CreatedAt: at}})
}
func (o *InitiativeObservability) RecordLifecycle(traceID string, l InitiativeLifecycle, at time.Time) (InitiativeTrace, error) {
	return o.Append(traceID, TraceEvent{Type: TraceEventLifecycle, At: at, Authority: "B96.37.19", Summary: string(l.State), Lifecycle: &l, Provenance: Provenance{Component: "initiative.InitiativeObservability", Subsystem: "B96.37.20", OriginID: l.ID, CreatedAt: at}})
}
func (o *InitiativeObservability) RecordOutcome(traceID string, out InitiativeOutcomeRecord, at time.Time) (InitiativeTrace, error) {
	return o.Append(traceID, TraceEvent{Type: TraceEventOutcome, At: at, Authority: "B96.37.15", Summary: string(out.Outcome), Outcome: &out, Provenance: out.Provenance})
}
func (o *InitiativeObservability) RecordConstraints(traceID, authority string, c []InitiativeConstraint, at time.Time) (InitiativeTrace, error) {
	return o.Append(traceID, TraceEvent{Type: TraceEventConstraint, At: at, Authority: authority, Constraints: append([]InitiativeConstraint(nil), c...), Provenance: Provenance{Component: "initiative.InitiativeObservability", Subsystem: "B96.37.20", CreatedAt: at}})
}
func (o *InitiativeObservability) RecordReason(traceID, authority, reason, requiredCondition string, at time.Time) (InitiativeTrace, error) {
	return o.Append(traceID, TraceEvent{Type: TraceEventReason, At: at, Authority: authority, Summary: reason, Evidence: requiredCondition, Provenance: Provenance{Component: "initiative.InitiativeObservability", Subsystem: "B96.37.20", CreatedAt: at}})
}
func (o *InitiativeObservability) Correlate(traceID string, ids map[string]string, at time.Time) (InitiativeTrace, error) {
	return o.Append(traceID, TraceEvent{Type: TraceEventCorrelation, At: at, Correlation: copyMap(ids), Provenance: Provenance{Component: "initiative.InitiativeObservability", Subsystem: "B96.37.20", CreatedAt: at}})
}
func (o *InitiativeObservability) RecordProvenance(traceID string, p Provenance, at time.Time) (InitiativeTrace, error) {
	return o.Append(traceID, TraceEvent{Type: TraceEventProvenance, At: at, Provenance: p})
}
func (o *InitiativeObservability) MigrateLegacyObservation(traceID, source, detail string, at time.Time) (InitiativeTrace, error) {
	return o.Append(traceID, TraceEvent{Type: TraceEventLegacy, At: at, Authority: "legacy-observability", Summary: source, Evidence: detail, Provenance: Provenance{Component: "initiative.InitiativeObservability", Subsystem: "B96.37.20", SignalType: "legacy_migration", CreatedAt: at}})
}

func (o *InitiativeObservability) Complete(traceID string, at time.Time) (InitiativeTrace, error) {
	return o.setStatus(traceID, TraceCompleted, at)
}
func (o *InitiativeObservability) Expire(at time.Time) int {
	if o == nil || o.Store == nil {
		return 0
	}
	if at.IsZero() {
		at = o.now()
	}
	n := 0
	for _, x := range o.Store.List() {
		if x.Status == TraceCompleted || x.Status == TraceExpired {
			continue
		}
		if o.Policy.TTL > 0 && x.CreatedAt.Add(o.Policy.TTL).Before(at) {
			x.Status = TraceExpired
			x.UpdatedAt = at
			_ = o.Store.Put(x)
			n++
		}
	}
	return n
}
func (o *InitiativeObservability) setStatus(id string, status TraceStatus, at time.Time) (InitiativeTrace, error) {
	if o == nil || o.Store == nil {
		return InitiativeTrace{}, errors.New("observability unavailable")
	}
	if at.IsZero() {
		at = o.now()
	}
	x, ok := o.Store.Get(id)
	if !ok {
		return InitiativeTrace{}, errors.New("trace not found")
	}
	x.Status = status
	x.UpdatedAt = at
	if err := o.Store.Put(x); err != nil {
		return InitiativeTrace{}, err
	}
	return x, nil
}

// Diagnostic queries are read-only and never feed authority decisions.
func (o *InitiativeObservability) Find(id string) (InitiativeTrace, bool) {
	if o == nil || o.Store == nil {
		return InitiativeTrace{}, false
	}
	return o.Store.Get(id)
}
func (o *InitiativeObservability) ByInitiative(id string) []InitiativeTrace {
	out := []InitiativeTrace{}
	if o == nil || o.Store == nil {
		return out
	}
	for _, x := range o.Store.List() {
		if x.InitiativeID == id {
			out = append(out, x)
		}
	}
	return out
}
func (o *InitiativeObservability) ByDecision(id string) []InitiativeTrace {
	return o.queryID(func(x InitiativeTrace) bool { return x.DecisionID == id })
}
func (o *InitiativeObservability) BySchedule(id string) []InitiativeTrace {
	return o.queryID(func(x InitiativeTrace) bool { return x.ScheduleID == id })
}

func (o *InitiativeObservability) ByOutcome(id string) []InitiativeTrace {
	return o.queryID(func(x InitiativeTrace) bool { return x.OutcomeID == id })
}
func (o *InitiativeObservability) Pending() []InitiativeTrace {
	out := []InitiativeTrace{}
	if o == nil || o.Store == nil {
		return out
	}
	for _, x := range o.Store.List() {
		if x.Status == TraceActive {
			out = append(out, x)
		}
	}
	return out
}
func (o *InitiativeObservability) queryID(fn func(InitiativeTrace) bool) []InitiativeTrace {
	out := []InitiativeTrace{}
	if o == nil || o.Store == nil {
		return out
	}
	for _, x := range o.Store.List() {
		if fn(x) {
			out = append(out, x)
		}
	}
	return out
}

func boolString(v bool) string {
	if v {
		return "true"
	}
	return "false"
}
func timeString(v time.Time) string {
	if v.IsZero() {
		return ""
	}
	return v.UTC().Format(time.RFC3339Nano)
}
func appendUniqueString(xs []string, value string) []string {
	for _, x := range xs {
		if x == value {
			return xs
		}
	}
	return append(xs, value)
}

func copyMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
func cloneTrace(x InitiativeTrace) InitiativeTrace {
	// Trace queries are explicitly read-only. A JSON round-trip is used here
	// as a defensive deep copy because TraceEvent contains nested pointers,
	// maps and slices; a shallow slice copy would still expose store-owned
	// state to diagnostic callers.
	b, err := json.Marshal(x)
	if err == nil {
		var out InitiativeTrace
		if json.Unmarshal(b, &out) == nil {
			return out
		}
	}
	// All current trace fields are JSON-safe. Keep a defensive fallback for
	// future non-serializable additions rather than returning shared slices.
	x.SignalIDs = append([]string(nil), x.SignalIDs...)
	x.Events = append([]TraceEvent(nil), x.Events...)
	for i := range x.Events {
		x.Events[i].Evaluation = copyMap(x.Events[i].Evaluation)
		x.Events[i].Correlation = copyMap(x.Events[i].Correlation)
		x.Events[i].Constraints = append([]InitiativeConstraint(nil), x.Events[i].Constraints...)
	}
	return x
}

func (o *InitiativeObservability) RecordScheduling(traceID string, s Schedule, at time.Time) (InitiativeTrace, error) {
	return o.Append(traceID, TraceEvent{Type: TraceEventTiming, At: at, Authority: "B96.37.18", Summary: string(s.Trigger), Schedule: &s, Evaluation: map[string]string{"mode": string(s.Mode), "state": string(s.State), "trigger": string(s.Trigger)}, Provenance: s.Provenance})
}

func (o *InitiativeObservability) RecordLifecycleTransition(traceID string, event LifecycleEvent, lifecycle InitiativeLifecycle, at time.Time) (InitiativeTrace, error) {
	return o.Append(traceID, TraceEvent{Type: TraceEventLifecycle, At: at, Authority: event.Authority, Summary: string(event.To), Lifecycle: &lifecycle, Evidence: event.Reason, Provenance: event.Provenance})
}
