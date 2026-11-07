package core

// owner_state.go — ESTADO EFÍMERO DEL PROPIETARIO (ESCENA 1).
//
// Dolor, cansancio y receptividad son lecturas de corta vida. Viven en un store
// separado del UserModel, con TTL y privacidad; el motor las usa para no
// proponer trabajo ni actividades no urgentes mientras el propietario no está
// disponible. Nunca bloquean una orden explícita.

import (
	"strings"
	"time"

	"ahmelia/ownerstate"
)

// EnableOwnerState turns on the ephemeral owner readings in the live process.
func (e *Engine) EnableOwnerState() {
	e.enableOwnerStateAt(ownerstate.StorePath)
}

func (e *Engine) enableOwnerStateAt(path string) {
	if e == nil {
		return
	}
	store, err := ownerstate.Open(path)
	e.deliberationMu.Lock()
	defer e.deliberationMu.Unlock()
	if err != nil {
		e.ownerState = nil
		e.ownerStateOn = false
		e.lastOwnerStateNote = "no se pudo cargar el estado del propietario: " + err.Error()
		return
	}
	store.SetClock(e.nowTime)
	e.ownerState = store
	e.ownerStateOn = true
	e.lastOwnerStateNote = strings.Join(store.Warnings(), "; ")
}

func (e *Engine) ownerStateStore() *ownerstate.Store {
	if e == nil {
		return nil
	}
	e.deliberationMu.Lock()
	defer e.deliberationMu.Unlock()
	if !e.ownerStateOn {
		return nil
	}
	return e.ownerState
}

// OwnerStateActive returns the ephemeral reading while its TTL is valid.
func (e *Engine) OwnerStateActive(now time.Time) (ownerstate.State, bool) {
	store := e.ownerStateStore()
	if store == nil {
		return ownerstate.State{}, false
	}
	return store.ActiveState(now)
}

// OwnerQuiet is true when a sensitive ephemeral state is active. It closes
// optional initiative, not the owner's explicit orders.
func (e *Engine) OwnerQuiet(now time.Time) bool {
	_, ok := e.OwnerStateActive(now)
	return ok
}

// SetOwnerEphemeralState records a sensitive reading with TTL.
func (e *Engine) SetOwnerEphemeralState(state ownerstate.State) error {
	store := e.ownerStateStore()
	if store == nil {
		return ownerstate.ErrInvalid
	}
	return store.SetState(state)
}

// ClearOwnerState drops the ephemeral reading.
func (e *Engine) ClearOwnerState() error {
	store := e.ownerStateStore()
	if store == nil {
		return nil
	}
	return store.ClearState()
}

// SetOwnerReceptivity records the short-lived receptivity reading.
func (e *Engine) SetOwnerReceptivity(level string, ttl time.Duration, evidence []string) error {
	store := e.ownerStateStore()
	if store == nil {
		return ownerstate.ErrInvalid
	}
	return store.SetReceptivity(level, ttl, evidence)
}

// OwnerReceptivity returns the active reading, or neutral when stale/absent.
func (e *Engine) OwnerReceptivity(now time.Time) ownerstate.Receptivity {
	store := e.ownerStateStore()
	if store == nil {
		return ownerstate.Receptivity{Level: "neutral"}
	}
	return store.Receptivity(now)
}

// AskOwnerRestQuestion consumes the single question budget.
func (e *Engine) AskOwnerRestQuestion() bool {
	store := e.ownerStateStore()
	if store == nil {
		return false
	}
	return store.AskQuestion()
}

// OwnerQuestionsAsked returns the persisted question counter.
func (e *Engine) OwnerQuestionsAsked() int {
	store := e.ownerStateStore()
	if store == nil {
		return 0
	}
	return store.QuestionsAsked()
}

// maybeProposeRecreation offers one optional recreation suggestion only when
// the owner is explicitly receptive, no sensitive state is active, the mode
// allows initiative and the proposal window is open. It is a suggestion, not an
// action, and it never proposes work.
func (e *Engine) maybeProposeRecreation(now time.Time, window time.Duration) string {
	store := e.ownerStateStore()
	if store == nil || e.OwnerQuiet(now) || !e.persistentModeAllowsInitiative() {
		return ""
	}
	if store.Receptivity(now).Level != "high" || !store.ProposalWindowOpen(now, window) {
		return ""
	}
	_ = store.MarkProposal(now)
	return " Si te apetece, podemos hacer una pausa y escuchar música; tú decides."
}

// SweepOwnerState clears expired readings and persists the transition.
func (e *Engine) SweepOwnerState(now time.Time) error {
	store := e.ownerStateStore()
	if store == nil {
		return nil
	}
	return store.Sweep(now)
}

// LastOwnerStateNote reports load warnings (for example a quarantined file).
func (e *Engine) LastOwnerStateNote() string {
	if e == nil {
		return ""
	}
	e.deliberationMu.Lock()
	defer e.deliberationMu.Unlock()
	return e.lastOwnerStateNote
}
