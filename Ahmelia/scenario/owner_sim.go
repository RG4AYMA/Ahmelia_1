package scenario

import (
	"time"

	"ahmelia/ownerstate"
	"ahmelia/restriction"
)

// OwnerState and Receptivity are the production ephemeral types. Keeping the
// aliases preserves the scene API while the storage moves to ownerstate.
type OwnerState = ownerstate.State
type Receptivity = ownerstate.Receptivity

// OwnerSim is a deterministic stand-in for the owner. It owns the ephemeral
// state, the receptivity reading and the question budget through ownerstate;
// the durable prohibitions live in the restriction store.
type OwnerSim struct {
	clock        *Clock
	restrictions *restriction.Store
	state        *ownerstate.Store
}

// NewOwnerSim builds an owner double. statePath may be empty for an in-memory
// scene; otherwise the counter and ephemeral state survive a restart.
func NewOwnerSim(clock *Clock, store *restriction.Store, statePath string) *OwnerSim {
	state, err := ownerstate.Open(statePath)
	if err != nil {
		state, _ = ownerstate.Open("")
	}
	o := &OwnerSim{clock: clock, restrictions: store, state: state}
	o.wireClock()
	return o
}

// LoadOwnerSim restores the persisted owner state. A missing or corrupt file
// yields an empty owner (a corrupt file adds a warning); a read error is
// returned.
func LoadOwnerSim(path string, clock *Clock, store *restriction.Store) (*OwnerSim, error) {
	state, err := ownerstate.Open(path)
	if err != nil {
		return nil, err
	}
	o := &OwnerSim{clock: clock, restrictions: store, state: state}
	o.wireClock()
	return o, nil
}

func (o *OwnerSim) wireClock() {
	if o != nil && o.clock != nil && o.state != nil {
		o.state.SetClock(o.clock.Now)
	}
}

// DeclareNoWorkToday admits the bounded work-initiative restriction.
func (o *OwnerSim) DeclareNoWorkToday(statement string) (restriction.OwnerRestriction, error) {
	if o == nil || o.restrictions == nil {
		return restriction.OwnerRestriction{}, restriction.ErrInvalid
	}
	now := o.clock.Now()
	return o.restrictions.AdmitWorkInitiative(
		statement, restriction.SourceExplicitOwner, "owner", now, o.clock.Location(),
		[]string{"turn:owner", "statement:" + statement},
	)
}

// ReportPain records an ephemeral, sensitive owner state with a TTL.
func (o *OwnerSim) ReportPain(kind string, intensity float64, ttl time.Duration, evidence []string) {
	if o == nil || o.state == nil {
		return
	}
	if ttl <= 0 {
		ttl = 2 * time.Hour
	}
	now := o.clock.Now()
	_ = o.state.SetState(OwnerState{
		Kind:       kind,
		Intensity:  intensity,
		Source:     "explicit",
		ObservedAt: now,
		ExpiresAt:  now.Add(ttl),
		Privacy:    ownerstate.PrivacySensitive,
		Persist:    ownerstate.PersistEphemeral,
		Evidence:   append([]string(nil), evidence...),
	})
}

// ClearPain drops the ephemeral state immediately.
func (o *OwnerSim) ClearPain() {
	if o == nil || o.state == nil {
		return
	}
	_ = o.state.ClearState()
}

// ActivePain returns the state only while its TTL is valid.
func (o *OwnerSim) ActivePain(now time.Time) (OwnerState, bool) {
	if o == nil || o.state == nil {
		return OwnerState{}, false
	}
	return o.state.ActiveState(now)
}

// SetReceptivity records a short-lived receptivity reading.
func (o *OwnerSim) SetReceptivity(level string, ttl time.Duration, evidence []string) {
	if o == nil || o.state == nil {
		return
	}
	_ = o.state.SetReceptivity(level, ttl, evidence)
}

// Receptivity returns the active reading, or neutral when stale/absent.
func (o *OwnerSim) Receptivity(now time.Time) Receptivity {
	if o == nil || o.state == nil {
		return Receptivity{Level: "neutral"}
	}
	return o.state.Receptivity(now)
}

// AskRestQuestion allows at most one question about the reason for rest.
func (o *OwnerSim) AskRestQuestion() bool {
	if o == nil || o.state == nil {
		return false
	}
	return o.state.AskQuestion()
}

// QuestionsAsked returns the persisted counter.
func (o *OwnerSim) QuestionsAsked() int {
	if o == nil || o.state == nil {
		return 0
	}
	return o.state.QuestionsAsked()
}

// Restrictions exposes the durable store for policy decisions.
func (o *OwnerSim) Restrictions() *restriction.Store {
	if o == nil {
		return nil
	}
	return o.restrictions
}

// Save persists the counter and the ephemeral readings. Empty path is a no-op.
func (o *OwnerSim) Save() error {
	if o == nil || o.state == nil {
		return nil
	}
	return o.state.Save()
}
