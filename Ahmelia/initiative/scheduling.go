package initiative

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// B96.37.18 — Initiative Scheduling & Temporal Planning.
// Scheduling owns only temporal persistence and trigger management. It never
// authorizes INTERVENE, generates language, or bypasses B96.37.2.
// TRIGGER != DECISION; SCHEDULE != INTERVENTION; DEFERRED != PROMISE.
type ScheduleMode string

const (
	ScheduleNow       ScheduleMode = "NOW"
	ScheduleTime      ScheduleMode = "TIME_BASED"
	ScheduleEvent     ScheduleMode = "EVENT_BASED"
	ScheduleCondition ScheduleMode = "CONDITION_BASED"
	ScheduleDeferred  ScheduleMode = "DEFERRED"
)

type ScheduleState string

const (
	SchedulePending      ScheduleState = "PENDING"
	ScheduleTriggered    ScheduleState = "TRIGGERED"
	ScheduleReevaluating ScheduleState = "REEVALUATING"
	ScheduleCompleted    ScheduleState = "COMPLETED"
	ScheduleCancelled    ScheduleState = "CANCELLED"
	ScheduleExpired      ScheduleState = "EXPIRED"
)

type ScheduleTriggerType string

const (
	ScheduleTriggerTime         ScheduleTriggerType = "TIME"
	ScheduleTriggerEvent        ScheduleTriggerType = "EVENT"
	ScheduleTriggerCondition    ScheduleTriggerType = "CONDITION"
	ScheduleTriggerContext      ScheduleTriggerType = "CONTEXT"
	ScheduleTriggerAvailability ScheduleTriggerType = "AVAILABILITY"
	ScheduleTriggerCooldown     ScheduleTriggerType = "COOLDOWN"
	ScheduleTriggerCommitment   ScheduleTriggerType = "COMMITMENT"
	ScheduleTriggerExplicit     ScheduleTriggerType = "EXPLICIT"
)

type TemporalConstraint struct {
	NotBefore time.Time           `json:"not_before,omitempty"`
	Deadline  time.Time           `json:"deadline,omitempty"`
	TimeZone  string              `json:"time_zone,omitempty"`
	Trigger   ScheduleTriggerType `json:"trigger,omitempty"`
	Key       string              `json:"key,omitempty"`
	Value     string              `json:"value,omitempty"`
}

type Schedule struct {
	ID               string                `json:"id"`
	Mode             ScheduleMode          `json:"mode"`
	Trigger          ScheduleTriggerType   `json:"trigger"`
	TriggerKey       string                `json:"trigger_key,omitempty"`
	TriggerValue     string                `json:"trigger_value,omitempty"`
	Opportunity      InitiativeOpportunity `json:"opportunity"`
	SourceDecisionID string                `json:"source_decision_id,omitempty"`
	Priority         Priority              `json:"priority"`
	CreatedAt        time.Time             `json:"created_at"`
	NotBefore        time.Time             `json:"not_before,omitempty"`
	ExpiresAt        time.Time             `json:"expires_at,omitempty"`
	LastTriggeredAt  time.Time             `json:"last_triggered_at,omitempty"`
	LastEvaluatedAt  time.Time             `json:"last_evaluated_at,omitempty"`
	State            ScheduleState         `json:"state"`
	Dependencies     []string              `json:"dependencies,omitempty"`
	Constraints      []TemporalConstraint  `json:"constraints,omitempty"`
	Reason           string                `json:"reason,omitempty"`
	Provenance       Provenance            `json:"provenance"`
	Revision         uint64                `json:"revision"`
}

// ScheduledInitiative is the canonical persisted temporal object.
type ScheduledInitiative struct {
	Schedule Schedule `json:"schedule"`
}

type SchedulingPolicy struct {
	DefaultTTL time.Duration
	MaxTTL     time.Duration
	MaxPending int
}

func DefaultSchedulingPolicy() SchedulingPolicy {
	return SchedulingPolicy{DefaultTTL: 10 * time.Minute, MaxTTL: 24 * time.Hour, MaxPending: 64}
}

func (s Schedule) Valid() error {
	if strings.TrimSpace(s.ID) == "" {
		return errors.New("schedule requires id")
	}
	switch s.Mode {
	case ScheduleNow, ScheduleTime, ScheduleEvent, ScheduleCondition, ScheduleDeferred:
	default:
		return errors.New("invalid schedule mode")
	}
	switch s.Trigger {
	case ScheduleTriggerTime, ScheduleTriggerEvent, ScheduleTriggerCondition, ScheduleTriggerContext, ScheduleTriggerAvailability, ScheduleTriggerCooldown, ScheduleTriggerCommitment, ScheduleTriggerExplicit:
	default:
		return errors.New("invalid schedule trigger")
	}
	switch s.State {
	case SchedulePending, ScheduleTriggered, ScheduleReevaluating, ScheduleCompleted, ScheduleCancelled, ScheduleExpired:
	default:
		return errors.New("invalid schedule state")
	}
	if s.CreatedAt.IsZero() {
		return errors.New("schedule requires created_at")
	}
	if s.Revision == 0 {
		return errors.New("schedule requires revision")
	}
	if !s.ExpiresAt.IsZero() && s.ExpiresAt.Before(s.CreatedAt) {
		return errors.New("schedule expires before creation")
	}
	if !s.NotBefore.IsZero() && !s.ExpiresAt.IsZero() && s.ExpiresAt.Before(s.NotBefore) {
		return errors.New("schedule expires before not_before")
	}
	return nil
}

type ScheduleStore interface {
	Put(Schedule) error
	Get(string) (Schedule, bool)
	List() []Schedule
	Delete(string) error
}

type InMemoryScheduleStore struct {
	mu    sync.RWMutex
	items map[string]Schedule
}

func NewInMemoryScheduleStore() *InMemoryScheduleStore {
	return &InMemoryScheduleStore{items: map[string]Schedule{}}
}
func (s *InMemoryScheduleStore) Put(x Schedule) error {
	if err := x.Valid(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[x.ID] = x
	return nil
}
func (s *InMemoryScheduleStore) Get(id string) (Schedule, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	x, ok := s.items[id]
	return x, ok
}
func (s *InMemoryScheduleStore) List() []Schedule {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Schedule, 0, len(s.items))
	for _, x := range s.items {
		out = append(out, x)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out
}
func (s *InMemoryScheduleStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.items, id)
	return nil
}

type InitiativeScheduler struct {
	Store  ScheduleStore
	Policy SchedulingPolicy
}

func NewInitiativeScheduler(store ScheduleStore) *InitiativeScheduler {
	if store == nil {
		store = NewInMemoryScheduleStore()
	}
	return &InitiativeScheduler{Store: store, Policy: DefaultSchedulingPolicy()}
}

func (s *InitiativeScheduler) ScheduleDecision(d InitiativeDecision, now time.Time) (Schedule, bool, error) {
	if s == nil || s.Store == nil {
		return Schedule{}, false, errors.New("scheduler unavailable")
	}
	if now.IsZero() {
		now = time.Now()
	}
	if d.Decision != Defer {
		return Schedule{}, false, nil
	}
	if s.Policy.MaxPending > 0 {
		n := 0
		for _, x := range s.Store.List() {
			if x.State == SchedulePending || x.State == ScheduleTriggered || x.State == ScheduleReevaluating {
				n++
			}
		}
		if n >= s.Policy.MaxPending {
			return Schedule{}, false, errors.New("schedule capacity reached")
		}
	}
	ttl := s.Policy.DefaultTTL
	if ttl <= 0 {
		ttl = DefaultSchedulingPolicy().DefaultTTL
	}
	maxTTL := s.Policy.MaxTTL
	if maxTTL <= 0 {
		maxTTL = DefaultSchedulingPolicy().MaxTTL
	}
	if !d.ExpiresAt.IsZero() && d.ExpiresAt.After(now) {
		ttl = d.ExpiresAt.Sub(now)
	}
	if ttl > maxTTL {
		ttl = maxTTL
	}
	mode, trigger, key, value := deriveSchedule(d)
	x := Schedule{ID: "schedule-" + newID(now), Mode: mode, Trigger: trigger, TriggerKey: key, TriggerValue: value, Opportunity: d.Opportunity, SourceDecisionID: d.ID, Priority: d.Priority, CreatedAt: now, NotBefore: now, ExpiresAt: now.Add(ttl), State: SchedulePending, Reason: string(d.Reason), Provenance: Provenance{Component: "initiative.InitiativeScheduler", Subsystem: "B96.37.18", SignalType: "deferred_schedule", OriginID: d.ID, CreatedAt: now}, Revision: 1}
	if d.Opportunity.ExpiresAt.After(now) && d.Opportunity.ExpiresAt.Before(x.ExpiresAt) {
		x.ExpiresAt = d.Opportunity.ExpiresAt
	}
	for _, v := range s.Store.List() {
		if v.State == SchedulePending && sameScheduleIdentity(v, x) {
			x.ID = v.ID
			x.Revision = v.Revision + 1
			return x, false, s.Store.Put(x)
		}
	}
	return x, true, s.Store.Put(x)
}

func deriveSchedule(d InitiativeDecision) (ScheduleMode, ScheduleTriggerType, string, string) {
	o := d.Opportunity
	if !o.CooldownActive && o.UserAvailability == AvailabilityAvailable && d.Timing == TimingNow {
		return ScheduleNow, ScheduleTriggerExplicit, "immediate", "ready"
	}
	if o.CooldownActive {
		return ScheduleDeferred, ScheduleTriggerCooldown, "cooldown", "recovery"
	}
	switch o.UserAvailability {
	case AvailabilityBusy, AvailabilityDistracted, AvailabilityAway, AvailabilityExplicitlyUnavailable:
		return ScheduleDeferred, ScheduleTriggerAvailability, "availability", "available"
	}
	if strings.TrimSpace(o.Target) != "" {
		return ScheduleCondition, ScheduleTriggerCondition, "target", o.Target
	}
	return ScheduleDeferred, ScheduleTriggerContext, "context", strings.TrimSpace(o.Context)
}

func sameScheduleIdentity(a, b Schedule) bool {
	return a.Trigger == b.Trigger && a.TriggerKey == b.TriggerKey && a.TriggerValue == b.TriggerValue && a.SourceDecisionID == b.SourceDecisionID && strings.TrimSpace(a.Opportunity.Context) == strings.TrimSpace(b.Opportunity.Context)
}

// Due returns schedules that are temporally eligible for a time-based check.
// It only marks lifecycle state; it does not authorize intervention.
func (s *InitiativeScheduler) Due(now time.Time) []Schedule {
	if s == nil || s.Store == nil {
		return nil
	}
	if now.IsZero() {
		now = time.Now()
	}
	out := []Schedule{}
	for _, x := range s.Store.List() {
		if x.State != SchedulePending || expiredSchedule(x, now) {
			continue
		}
		if x.NotBefore.IsZero() || !now.Before(x.NotBefore) {
			x.State = ScheduleTriggered
			x.LastTriggeredAt = now
			x.Revision++
			if s.Store.Put(x) == nil {
				out = append(out, x)
			}
		}
	}
	return out
}

func (s *InitiativeScheduler) Trigger(t ScheduleTriggerType, key, value string, now time.Time) []Schedule {
	if s == nil || s.Store == nil {
		return nil
	}
	if now.IsZero() {
		now = time.Now()
	}
	out := []Schedule{}
	for _, x := range s.Store.List() {
		if x.State != SchedulePending || expiredSchedule(x, now) || x.Trigger != t {
			continue
		}
		if key != "" && x.TriggerKey != key {
			continue
		}
		if value != "" && x.TriggerValue != "" && x.TriggerValue != value {
			continue
		}
		x.State = ScheduleTriggered
		x.LastTriggeredAt = now
		x.Revision++
		if s.Store.Put(x) == nil {
			out = append(out, x)
		}
	}
	return out
}

func (s *InitiativeScheduler) BeginReevaluation(id string, now time.Time) (Schedule, error) {
	x, ok := s.Store.Get(id)
	if !ok {
		return x, errors.New("schedule not found")
	}
	if now.IsZero() {
		now = time.Now()
	}
	if expiredSchedule(x, now) {
		x.State = ScheduleExpired
		x.Revision++
		return x, s.Store.Put(x)
	}
	if x.State != ScheduleTriggered {
		return x, fmt.Errorf("schedule %s is not triggered", id)
	}
	x.State = ScheduleReevaluating
	x.LastEvaluatedAt = now
	x.Revision++
	return x, s.Store.Put(x)
}

// CompleteReevaluation records the lifecycle result only. The decision itself
// must already have been produced by B96.37.2 and is not created here.
func (s *InitiativeScheduler) CompleteReevaluation(id string, d InitiativeDecision, now time.Time) (Schedule, error) {
	x, ok := s.Store.Get(id)
	if !ok {
		return x, errors.New("schedule not found")
	}
	if now.IsZero() {
		now = time.Now()
	}
	if d.Decision == Defer {
		x.State = SchedulePending
		x.Opportunity = d.Opportunity
		x.LastEvaluatedAt = now
		x.Revision++
		return x, s.Store.Put(x)
	}
	x.State = ScheduleCompleted
	x.LastEvaluatedAt = now
	x.Revision++
	return x, s.Store.Put(x)
}
func (s *InitiativeScheduler) Cancel(id string, now time.Time) (Schedule, error) {
	return s.transition(id, ScheduleCancelled, now)
}
func (s *InitiativeScheduler) Expire(now time.Time) []Schedule {
	if s == nil || s.Store == nil {
		return nil
	}
	if now.IsZero() {
		now = time.Now()
	}
	out := []Schedule{}
	for _, x := range s.Store.List() {
		if (x.State == SchedulePending || x.State == ScheduleTriggered || x.State == ScheduleReevaluating) && expiredSchedule(x, now) {
			x.State = ScheduleExpired
			x.Revision++
			if s.Store.Put(x) == nil {
				out = append(out, x)
			}
		}
	}
	return out
}
func (s *InitiativeScheduler) transition(id string, state ScheduleState, now time.Time) (Schedule, error) {
	x, ok := s.Store.Get(id)
	if !ok {
		return x, errors.New("schedule not found")
	}
	if now.IsZero() {
		now = time.Now()
	}
	x.State = state
	x.Revision++
	return x, s.Store.Put(x)
}
func expiredSchedule(x Schedule, now time.Time) bool {
	return !x.ExpiresAt.IsZero() && !now.Before(x.ExpiresAt)
}

// TriggerForRecheck bridges B96.37.14's recheck vocabulary without moving
// authority: a matching trigger only causes reevaluation to become eligible.
func TriggerForRecheck(c RecheckTrigger) ScheduleTriggerType {
	switch c {
	case TriggerAvailability:
		return ScheduleTriggerAvailability
	case TriggerContext:
		return ScheduleTriggerContext
	case TriggerTopic:
		return ScheduleTriggerContext
	case TriggerPriority:
		return ScheduleTriggerCondition
	case TriggerUrgency:
		return ScheduleTriggerCondition
	case TriggerCooldown:
		return ScheduleTriggerCooldown
	case TriggerUserResponse:
		return ScheduleTriggerEvent
	case TriggerExplicit:
		return ScheduleTriggerExplicit
	}
	return ScheduleTriggerExplicit
}
