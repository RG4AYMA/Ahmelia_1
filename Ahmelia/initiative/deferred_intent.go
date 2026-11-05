package initiative

import (
	"errors"
	"strings"
	"sync"
	"time"
)

// B96.37.14 — Initiative Persistence & Deferred Intent.
// This layer preserves deferred opportunities as revocable, re-evaluable
// intentions. It never authorizes an initiative decision.
type DeferredIntentState string

const (
	DeferredActive       DeferredIntentState = "ACTIVE"
	DeferredDeferred     DeferredIntentState = "DEFERRED"
	DeferredReevaluating DeferredIntentState = "REEVALUATING"
	DeferredResolved     DeferredIntentState = "RESOLVED"
	DeferredCancelled    DeferredIntentState = "CANCELLED"
	DeferredExpired      DeferredIntentState = "EXPIRED"
)

type DeferralReason string

const (
	DeferralCooldown     DeferralReason = "cooldown"
	DeferralBusy         DeferralReason = "user_busy"
	DeferralTiming       DeferralReason = "timing"
	DeferralPrivacy      DeferralReason = "privacy"
	DeferralInterruption DeferralReason = "interruption"
	DeferralPolicy       DeferralReason = "policy"
	DeferralOther        DeferralReason = "other"
)

type RecheckCondition string

const (
	RecheckAvailabilityChange RecheckCondition = "availability_change"
	RecheckContextChange      RecheckCondition = "context_change"
	RecheckTopicChange        RecheckCondition = "topic_change"
	RecheckPriorityChange     RecheckCondition = "priority_change"
	RecheckUrgencyChange      RecheckCondition = "urgency_change"
	RecheckCooldownRecovery   RecheckCondition = "cooldown_recovery"
	RecheckExplicitTrigger    RecheckCondition = "explicit_trigger"
	RecheckUserResponse       RecheckCondition = "user_response"
)

type RecheckTrigger string

const (
	TriggerAvailability RecheckTrigger = "availability"
	TriggerContext      RecheckTrigger = "context"
	TriggerTopic        RecheckTrigger = "topic"
	TriggerPriority     RecheckTrigger = "priority"
	TriggerUrgency      RecheckTrigger = "urgency"
	TriggerCooldown     RecheckTrigger = "cooldown"
	TriggerUserResponse RecheckTrigger = "user_response"
	TriggerExplicit     RecheckTrigger = "explicit"
)

type DeferredIntent struct {
	ID                  string                `json:"id"`
	State               DeferredIntentState   `json:"state"`
	Source              Source                `json:"source"`
	Reason              Reason                `json:"reason"`
	DeferralReason      DeferralReason        `json:"deferral_reason"`
	Target              string                `json:"target,omitempty"`
	OriginalOpportunity InitiativeOpportunity `json:"original_opportunity"`
	CreatedAt           time.Time             `json:"created_at"`
	ExpiresAt           time.Time             `json:"expires_at,omitempty"`
	RecheckConditions   []RecheckCondition    `json:"recheck_conditions,omitempty"`
	LastEvaluatedAt     time.Time             `json:"last_evaluated_at,omitempty"`
	ResolvedAt          time.Time             `json:"resolved_at,omitempty"`
	Provenance          Provenance            `json:"provenance"`
	Revision            uint64                `json:"revision"`
}

// DeferredIntentPolicy contains persistence rules only; it never chooses a decision.
type DeferredIntentPolicy struct {
	DefaultTTL time.Duration
	MaxTTL     time.Duration
	MaxPending int
}

func DefaultDeferredIntentPolicy() DeferredIntentPolicy {
	return DeferredIntentPolicy{DefaultTTL: 10 * time.Minute, MaxTTL: 24 * time.Hour, MaxPending: 32}
}

func (d DeferredIntent) Valid() error {
	switch d.State {
	case DeferredActive, DeferredDeferred, DeferredReevaluating, DeferredResolved, DeferredCancelled, DeferredExpired:
	default:
		return errors.New("invalid deferred intent state")
	}
	if strings.TrimSpace(d.ID) == "" {
		return errors.New("deferred intent requires id")
	}
	if d.CreatedAt.IsZero() {
		return errors.New("deferred intent requires created_at")
	}
	if d.ExpiresAt.IsZero() {
		return errors.New("deferred intent requires expires_at")
	}
	if d.ExpiresAt.Before(d.CreatedAt) {
		return errors.New("deferred intent expires before creation")
	}
	if d.Revision == 0 {
		return errors.New("deferred intent requires revision")
	}
	return nil
}

func inferDeferralReason(d InitiativeDecision) DeferralReason {
	for _, c := range d.Constraints {
		switch c.Code {
		case "disclosure_not_authorized":
			return DeferralPrivacy
		case "timing_invalid", "cooldown_active":
			return DeferralTiming
		}
	}
	switch d.Opportunity.UserAvailability {
	case AvailabilityBusy, AvailabilityDistracted, AvailabilityAway, AvailabilityExplicitlyUnavailable:
		return DeferralBusy
	}
	if d.Opportunity.CooldownActive {
		return DeferralCooldown
	}
	if d.Timing == TimingInterrupt {
		return DeferralInterruption
	}
	return DeferralPolicy
}

func defaultRecheckConditions(d InitiativeDecision) []RecheckCondition {
	out := []RecheckCondition{RecheckContextChange, RecheckTopicChange, RecheckAvailabilityChange, RecheckCooldownRecovery, RecheckPriorityChange, RecheckUrgencyChange, RecheckUserResponse}
	if d.Opportunity.CooldownActive {
		out = append(out, RecheckExplicitTrigger)
	}
	return uniqueConditions(out)
}
func uniqueConditions(in []RecheckCondition) []RecheckCondition {
	seen := map[RecheckCondition]bool{}
	out := make([]RecheckCondition, 0, len(in))
	for _, v := range in {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

// NewDeferredIntent creates persistence state from a DEFER decision only.
func NewDeferredIntent(d InitiativeDecision, now time.Time, policy DeferredIntentPolicy) (DeferredIntent, error) {
	if d.Decision != Defer {
		return DeferredIntent{}, errors.New("only DEFER decisions may create deferred intents")
	}
	if now.IsZero() {
		now = time.Now()
	}
	if policy.DefaultTTL <= 0 {
		policy.DefaultTTL = DefaultDeferredIntentPolicy().DefaultTTL
	}
	if policy.MaxTTL <= 0 {
		policy.MaxTTL = DefaultDeferredIntentPolicy().MaxTTL
	}
	ttl := policy.DefaultTTL
	if d.ExpiresAt.After(now) {
		ttl = d.ExpiresAt.Sub(now)
	}
	if ttl > policy.MaxTTL {
		ttl = policy.MaxTTL
	}
	x := DeferredIntent{ID: "deferred-" + newID(now), State: DeferredDeferred, Source: d.Source, Reason: d.Reason, DeferralReason: inferDeferralReason(d), Target: d.Target, OriginalOpportunity: d.Opportunity, CreatedAt: now, ExpiresAt: now.Add(ttl), RecheckConditions: defaultRecheckConditions(d), LastEvaluatedAt: now, Provenance: Provenance{Component: "initiative.DeferredIntent", Subsystem: "B96.37.14", SignalType: string(d.Reason), OriginID: d.ID, CreatedAt: now}, Revision: 1}
	return x, x.Valid()
}

// DeferredIntentStore is the persistence boundary. Implementations may be durable.
type DeferredIntentStore interface {
	Put(DeferredIntent) error
	Get(string) (DeferredIntent, bool)
	List() []DeferredIntent
	Delete(string) error
}

type InMemoryDeferredIntentStore struct {
	mu    sync.RWMutex
	items map[string]DeferredIntent
}

func NewInMemoryDeferredIntentStore() *InMemoryDeferredIntentStore {
	return &InMemoryDeferredIntentStore{items: map[string]DeferredIntent{}}
}
func (s *InMemoryDeferredIntentStore) Put(d DeferredIntent) error {
	if err := d.Valid(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[d.ID] = d
	return nil
}
func (s *InMemoryDeferredIntentStore) Get(id string) (DeferredIntent, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	d, ok := s.items[id]
	return d, ok
}
func (s *InMemoryDeferredIntentStore) List() []DeferredIntent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]DeferredIntent, 0, len(s.items))
	for _, d := range s.items {
		out = append(out, d)
	}
	return out
}
func (s *InMemoryDeferredIntentStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.items, id)
	return nil
}

type DeferredIntentManager struct {
	Store  DeferredIntentStore
	Policy DeferredIntentPolicy
}

func NewDeferredIntentManager(store DeferredIntentStore) *DeferredIntentManager {
	if store == nil {
		store = NewInMemoryDeferredIntentStore()
	}
	return &DeferredIntentManager{Store: store, Policy: DefaultDeferredIntentPolicy()}
}

func (m *DeferredIntentManager) PersistDecision(d InitiativeDecision, now time.Time) (DeferredIntent, bool, error) {
	if m == nil || m.Store == nil {
		return DeferredIntent{}, false, errors.New("deferred intent manager unavailable")
	}
	if d.Decision != Defer {
		return DeferredIntent{}, false, nil
	}
	x, err := NewDeferredIntent(d, now, m.Policy)
	if err != nil {
		return x, false, err
	}
	if m.Policy.MaxPending > 0 {
		pending := 0
		for _, v := range m.Store.List() {
			if v.State == DeferredDeferred || v.State == DeferredActive || v.State == DeferredReevaluating {
				pending++
			}
		}
		if pending >= m.Policy.MaxPending {
			return DeferredIntent{}, false, errors.New("deferred intent capacity reached")
		}
	}
	created := true
	for _, v := range m.Store.List() {
		if v.State == DeferredDeferred && sameIntentIdentity(v, x) {
			x.ID = v.ID
			x.Revision = v.Revision + 1
			created = false
		}
	}
	err = m.Store.Put(x)
	return x, created && err == nil, err
}
func sameIntentIdentity(a, b DeferredIntent) bool {
	return a.Source == b.Source && a.Reason == b.Reason && a.Target == b.Target && strings.TrimSpace(a.OriginalOpportunity.Context) == strings.TrimSpace(b.OriginalOpportunity.Context)
}

func (m *DeferredIntentManager) MarkReevaluating(id string, now time.Time) (DeferredIntent, error) {
	d, ok := m.Store.Get(id)
	if !ok {
		return d, errors.New("deferred intent not found")
	}
	if expired(d, now) {
		return m.transition(d, DeferredExpired, now)
	}
	d.State = DeferredReevaluating
	d.LastEvaluatedAt = now
	d.Revision++
	return d, m.Store.Put(d)
}
func (m *DeferredIntentManager) Reevaluate(id string, op InitiativeOpportunity, now time.Time, policy Policy) (InitiativeDecision, DeferredIntent, error) {
	d, ok := m.Store.Get(id)
	if !ok {
		return InitiativeDecision{}, d, errors.New("deferred intent not found")
	}
	if now.IsZero() {
		now = time.Now()
	}
	if expired(d, now) {
		d, _ = m.transition(d, DeferredExpired, now)
		return InitiativeDecision{Decision: Wait, Timing: TimingWait, CreatedAt: now, PolicyVersion: "B96.37.2"}, d, nil
	}
	d.State = DeferredReevaluating
	d.LastEvaluatedAt = now
	d.Revision++
	_ = m.Store.Put(d)
	decision := policy.Evaluate(op, now)
	if decision.Decision == Defer {
		d.State = DeferredDeferred
		d.OriginalOpportunity = op
		d.LastEvaluatedAt = now
		d.Revision++
		_ = m.Store.Put(d)
		return decision, d, nil
	}
	if decision.Decision == Intervene {
		d.State = DeferredResolved
		d.ResolvedAt = now
	} else {
		d.State = DeferredResolved
		d.ResolvedAt = now
	}
	d.Revision++
	_ = m.Store.Put(d)
	return decision, d, nil
}
func (m *DeferredIntentManager) Cancel(id string, now time.Time) (DeferredIntent, error) {
	d, ok := m.Store.Get(id)
	if !ok {
		return d, errors.New("deferred intent not found")
	}
	return m.transition(d, DeferredCancelled, now)
}
func (m *DeferredIntentManager) Resolve(id string, now time.Time) (DeferredIntent, error) {
	d, ok := m.Store.Get(id)
	if !ok {
		return d, errors.New("deferred intent not found")
	}
	return m.transition(d, DeferredResolved, now)
}
func (m *DeferredIntentManager) Expire(now time.Time) []DeferredIntent {
	if now.IsZero() {
		now = time.Now()
	}
	out := []DeferredIntent{}
	for _, d := range m.Store.List() {
		if (d.State == DeferredDeferred || d.State == DeferredActive || d.State == DeferredReevaluating) && expired(d, now) {
			if x, e := m.transition(d, DeferredExpired, now); e == nil {
				out = append(out, x)
			}
		}
	}
	return out
}
func (m *DeferredIntentManager) transition(d DeferredIntent, state DeferredIntentState, now time.Time) (DeferredIntent, error) {
	if now.IsZero() {
		now = time.Now()
	}
	d.State = state
	d.Revision++
	if state == DeferredResolved || state == DeferredCancelled || state == DeferredExpired {
		d.ResolvedAt = now
	}
	return d, m.Store.Put(d)
}
func expired(d DeferredIntent, now time.Time) bool {
	return !d.ExpiresAt.IsZero() && !now.Before(d.ExpiresAt)
}

// TriggerMatches reports whether a change is relevant to an intent.
func TriggerMatches(d DeferredIntent, t RecheckTrigger) bool {
	var c RecheckCondition
	switch t {
	case TriggerAvailability:
		c = RecheckAvailabilityChange
	case TriggerContext:
		c = RecheckContextChange
	case TriggerTopic:
		c = RecheckTopicChange
	case TriggerPriority:
		c = RecheckPriorityChange
	case TriggerUrgency:
		c = RecheckUrgencyChange
	case TriggerCooldown:
		c = RecheckCooldownRecovery
	case TriggerUserResponse:
		c = RecheckUserResponse
	case TriggerExplicit:
		c = RecheckExplicitTrigger
	default:
		return false
	}
	for _, x := range d.RecheckConditions {
		if x == c {
			return true
		}
	}
	return false
}

// ReevaluateDeferred preserves the existing canonical policy authority while
// B96.37.14 owns only the lifecycle of the pending intent.
func ReevaluateDeferredIntent(m *DeferredIntentManager, id string, op InitiativeOpportunity, now time.Time) (InitiativeDecision, DeferredIntent, error) {
	if m == nil {
		return InitiativeDecision{}, DeferredIntent{}, errors.New("deferred intent manager unavailable")
	}
	return m.Reevaluate(id, op, now, DefaultPolicy())
}
