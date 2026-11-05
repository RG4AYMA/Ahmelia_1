package initiative

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// B96.37.19 — Initiative Lifecycle Management.
// Lifecycle is operational state management only. It records what happened;
// B96.37.2 remains the sole authority for INTERVENE/WAIT/DEFER.
type LifecycleState string

const (
	LifecycleDetected    LifecycleState = "DETECTED"
	LifecycleEvaluating  LifecycleState = "EVALUATING"
	LifecycleCandidate   LifecycleState = "CANDIDATE"
	LifecycleSelected    LifecycleState = "SELECTED"
	LifecycleDecided     LifecycleState = "DECIDED"
	LifecycleScheduled   LifecycleState = "SCHEDULED"
	LifecycleDeferred    LifecycleState = "DEFERRED"
	LifecycleIntervening LifecycleState = "INTERVENING"
	LifecycleExecuting   LifecycleState = "EXECUTING"
	LifecycleOutcome     LifecycleState = "OUTCOME"
	LifecycleResolved    LifecycleState = "RESOLVED"
	LifecycleFailed      LifecycleState = "FAILED"
	LifecycleCancelled   LifecycleState = "CANCELLED"
	LifecycleExpired     LifecycleState = "EXPIRED"
)

type LifecycleEvent struct {
	From       LifecycleState `json:"from"`
	To         LifecycleState `json:"to"`
	At         time.Time      `json:"at"`
	Authority  string         `json:"authority,omitempty"`
	Reason     string         `json:"reason,omitempty"`
	Provenance Provenance     `json:"provenance"`
}

type InitiativeLifecycle struct {
	ID          string                `json:"id"`
	IdentityKey string                `json:"identity_key"`
	RelatedIDs  []string              `json:"related_ids,omitempty"`
	State       LifecycleState        `json:"state"`
	Opportunity InitiativeOpportunity `json:"opportunity"`
	Decision    *InitiativeDecision   `json:"decision,omitempty"`
	CreatedAt   time.Time             `json:"created_at"`
	UpdatedAt   time.Time             `json:"updated_at"`
	ResolvedAt  time.Time             `json:"resolved_at,omitempty"`
	ExpiresAt   time.Time             `json:"expires_at,omitempty"`
	Events      []LifecycleEvent      `json:"events,omitempty"`
	Provenance  Provenance            `json:"provenance"`
	Revision    uint64                `json:"revision"`
}

type LifecycleStore interface {
	Put(InitiativeLifecycle) error
	Get(string) (InitiativeLifecycle, bool)
	List() []InitiativeLifecycle
}

type InMemoryLifecycleStore struct {
	mu    sync.RWMutex
	items map[string]InitiativeLifecycle
}

func NewInMemoryLifecycleStore() *InMemoryLifecycleStore {
	return &InMemoryLifecycleStore{items: map[string]InitiativeLifecycle{}}
}
func (s *InMemoryLifecycleStore) Put(x InitiativeLifecycle) error {
	if err := ValidateLifecycle(x); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[x.ID] = x
	return nil
}
func (s *InMemoryLifecycleStore) Get(id string) (InitiativeLifecycle, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	x, ok := s.items[id]
	return x, ok
}
func (s *InMemoryLifecycleStore) List() []InitiativeLifecycle {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]InitiativeLifecycle, 0, len(s.items))
	for _, x := range s.items {
		out = append(out, x)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out
}

func NormalizeInitiativeIdentity(o InitiativeOpportunity) string {
	return strings.Join([]string{string(o.Source), string(o.Reason), normalize(o.Target), normalize(o.Context), normalize(o.InteractionState)}, "|")
}
func normalize(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(s)), " "))
}

func ValidateLifecycle(x InitiativeLifecycle) error {
	if strings.TrimSpace(x.ID) == "" {
		return errors.New("lifecycle requires id")
	}
	if strings.TrimSpace(x.IdentityKey) == "" {
		return errors.New("lifecycle requires identity_key")
	}
	if !validLifecycleState(x.State) {
		return fmt.Errorf("invalid lifecycle state %q", x.State)
	}
	if x.CreatedAt.IsZero() || x.UpdatedAt.IsZero() {
		return errors.New("lifecycle requires timestamps")
	}
	if x.Revision == 0 {
		return errors.New("lifecycle requires revision")
	}
	if !x.ExpiresAt.IsZero() && x.ExpiresAt.Before(x.CreatedAt) {
		return errors.New("lifecycle expires before creation")
	}
	return nil
}
func validLifecycleState(s LifecycleState) bool {
	switch s {
	case LifecycleDetected, LifecycleEvaluating, LifecycleCandidate, LifecycleSelected, LifecycleDecided, LifecycleScheduled, LifecycleDeferred, LifecycleIntervening, LifecycleExecuting, LifecycleOutcome, LifecycleResolved, LifecycleFailed, LifecycleCancelled, LifecycleExpired:
		return true
	}
	return false
}

var lifecycleTransitions = map[LifecycleState]map[LifecycleState]bool{
	LifecycleDetected:    {LifecycleEvaluating: true, LifecycleCandidate: true, LifecycleCancelled: true, LifecycleExpired: true},
	LifecycleEvaluating:  {LifecycleCandidate: true, LifecycleSelected: true, LifecycleDecided: true, LifecycleDeferred: true, LifecycleCancelled: true, LifecycleExpired: true},
	LifecycleCandidate:   {LifecycleSelected: true, LifecycleEvaluating: true, LifecycleCancelled: true, LifecycleExpired: true},
	LifecycleSelected:    {LifecycleDecided: true, LifecycleCancelled: true, LifecycleExpired: true},
	LifecycleDecided:     {LifecycleScheduled: true, LifecycleDeferred: true, LifecycleIntervening: true, LifecycleOutcome: true, LifecycleCancelled: true, LifecycleExpired: true},
	LifecycleScheduled:   {LifecycleEvaluating: true, LifecycleDeferred: true, LifecycleCancelled: true, LifecycleExpired: true},
	LifecycleDeferred:    {LifecycleScheduled: true, LifecycleEvaluating: true, LifecycleOutcome: true, LifecycleResolved: true, LifecycleCancelled: true, LifecycleExpired: true},
	LifecycleIntervening: {LifecycleExecuting: true, LifecycleOutcome: true, LifecycleFailed: true, LifecycleCancelled: true},
	LifecycleExecuting:   {LifecycleOutcome: true, LifecycleResolved: true, LifecycleFailed: true, LifecycleCancelled: true},
	LifecycleOutcome:     {LifecycleResolved: true, LifecycleFailed: true, LifecycleCancelled: true, LifecycleExpired: true},
}

func CanTransitionLifecycle(from, to LifecycleState) bool {
	return from != to && lifecycleTransitions[from][to]
}

// LifecycleManager persists and validates state transitions; it never creates
// an InitiativeDecision and never authorizes a turn.
type LifecycleManager struct{ Store LifecycleStore }

func NewLifecycleManager(store LifecycleStore) *LifecycleManager {
	if store == nil {
		store = NewInMemoryLifecycleStore()
	}
	return &LifecycleManager{Store: store}
}

func (m *LifecycleManager) Create(op InitiativeOpportunity, now time.Time) (InitiativeLifecycle, error) {
	if m == nil || m.Store == nil {
		return InitiativeLifecycle{}, errors.New("lifecycle manager unavailable")
	}
	if now.IsZero() {
		now = time.Now()
	}
	if !op.Exists {
		return InitiativeLifecycle{}, errors.New("cannot create lifecycle without opportunity")
	}
	key := NormalizeInitiativeIdentity(op)
	for _, v := range m.Store.List() {
		if v.IdentityKey == key && !terminalLifecycle(v.State) {
			v.RelatedIDs = appendUnique(v.RelatedIDs, v.ID)
			return v, nil
		}
	}
	id := "initiative-" + newID(now)
	x := InitiativeLifecycle{ID: id, IdentityKey: key, State: LifecycleDetected, Opportunity: op, CreatedAt: now, UpdatedAt: now, ExpiresAt: op.ExpiresAt, Revision: 1, Provenance: Provenance{Component: "initiative.LifecycleManager", Subsystem: "B96.37.19", SignalType: "detected", OriginID: op.Signal.ID, CreatedAt: now}}
	x.Events = []LifecycleEvent{{From: "", To: LifecycleDetected, At: now, Authority: "B96.37.19", Reason: "initiative lifecycle created", Provenance: x.Provenance}}
	return x, m.Store.Put(x)
}
func (m *LifecycleManager) Transition(id string, to LifecycleState, now time.Time, authority, reason string) (InitiativeLifecycle, error) {
	if m == nil || m.Store == nil {
		return InitiativeLifecycle{}, errors.New("lifecycle manager unavailable")
	}
	if now.IsZero() {
		now = time.Now()
	}
	x, ok := m.Store.Get(id)
	if !ok {
		return x, errors.New("initiative lifecycle not found")
	}
	if x.State == to {
		return x, nil
	}
	if !CanTransitionLifecycle(x.State, to) {
		return x, fmt.Errorf("invalid lifecycle transition %s -> %s", x.State, to)
	}
	from := x.State
	x.State = to
	x.UpdatedAt = now
	x.Revision++
	if terminalLifecycle(to) {
		x.ResolvedAt = now
	}
	x.Events = append(x.Events, LifecycleEvent{From: from, To: to, At: now, Authority: authority, Reason: reason, Provenance: Provenance{Component: "initiative.LifecycleManager", Subsystem: "B96.37.19", SignalType: string(to), OriginID: id, CreatedAt: now}})
	if err := m.Store.Put(x); err != nil {
		return x, err
	}
	return x, nil
}
func (m *LifecycleManager) RecordDecision(id string, d InitiativeDecision, now time.Time) (InitiativeLifecycle, error) {
	if now.IsZero() {
		now = time.Now()
	}
	x, ok := m.Store.Get(id)
	if !ok {
		return x, errors.New("initiative lifecycle not found")
	}
	cp := d
	x.Decision = &cp
	x.UpdatedAt = now
	x.Revision++
	if err := m.Store.Put(x); err != nil {
		return x, err
	}
	return x, nil
}

// FindByDecisionID returns the active/most-recent lifecycle owning a canonical
// decision. It is a correlation helper only; it never creates or evaluates a
// decision.
func (m *LifecycleManager) FindByDecisionID(decisionID string) (InitiativeLifecycle, bool) {
	if m == nil || m.Store == nil || strings.TrimSpace(decisionID) == "" {
		return InitiativeLifecycle{}, false
	}
	var best InitiativeLifecycle
	found := false
	for _, x := range m.Store.List() {
		if x.Decision == nil || x.Decision.ID != decisionID {
			continue
		}
		if !found || x.UpdatedAt.After(best.UpdatedAt) {
			best, found = x, true
		}
	}
	return best, found
}

func (m *LifecycleManager) FindDuplicate(op InitiativeOpportunity) (InitiativeLifecycle, bool) {
	if m == nil || m.Store == nil {
		return InitiativeLifecycle{}, false
	}
	key := NormalizeInitiativeIdentity(op)
	var best InitiativeLifecycle
	found := false
	for _, x := range m.Store.List() {
		if x.IdentityKey == key && !terminalLifecycle(x.State) {
			if !found || x.UpdatedAt.After(best.UpdatedAt) {
				best = x
				found = true
			}
		}
	}
	return best, found
}
func (m *LifecycleManager) Orphans(now time.Time) []InitiativeLifecycle {
	if now.IsZero() {
		now = time.Now()
	}
	out := []InitiativeLifecycle{}
	for _, x := range m.Store.List() {
		if terminalLifecycle(x.State) {
			continue
		}
		if x.ExpiresAt.IsZero() && x.State == LifecycleDeferred {
			out = append(out, x)
		}
	}
	return out
}
func (m *LifecycleManager) Expire(now time.Time) []InitiativeLifecycle {
	if now.IsZero() {
		now = time.Now()
	}
	out := []InitiativeLifecycle{}
	for _, x := range m.Store.List() {
		if terminalLifecycle(x.State) || x.ExpiresAt.IsZero() || now.Before(x.ExpiresAt) {
			continue
		}
		if y, e := m.Transition(x.ID, LifecycleExpired, now, "B96.37.19", "lifecycle expiration"); e == nil {
			out = append(out, y)
		}
	}
	return out
}
func terminalLifecycle(s LifecycleState) bool {
	return s == LifecycleResolved || s == LifecycleFailed || s == LifecycleCancelled || s == LifecycleExpired
}
func appendUnique(xs []string, v string) []string {
	for _, x := range xs {
		if x == v {
			return xs
		}
	}
	return append(xs, v)
}
