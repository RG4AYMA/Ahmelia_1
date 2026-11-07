package core

// owner_restrictions.go — ESCENA 1: PROHIBICIÓN TEMPORAL DEL PROPIETARIO.
//
// Una restricción no es un modo ni un hecho del propietario. Es un enunciado
// acotado que sobrevive al turno, con procedencia, vencimiento y un levante
// explícito que queda en el historial. Este archivo conecta el store durable
// con la única puerta que decide si Ahmelia puede tomar iniciativa:
//
//   - el modo persistente elegido por el propietario (paso 3.4);
//   - la restricción temporal declarada en la escena 1.
//
// La restricción de trabajo solo resta iniciativa propia. No bloquea una orden
// autorizada del propietario: eso corresponde a una restricción de tipo
// action_ban, que exige confirmación. El modelo no crea, activa ni levanta
// restricciones; como mucho produce una lectura que el arnés aplica de forma
// provisional y acotada.
//
// Si el archivo no se puede leer, el mecanismo queda apagado. El arnés no
// inventa una prohibición que el propietario no declaró.

import (
	"strings"
	"time"

	"ahmelia/restriction"
)

// EnableOwnerRestrictions turns on the owner's durable restrictions in the live
// process. Like the other opt-in mechanisms, tests do not touch runtime state.
func (e *Engine) EnableOwnerRestrictions() {
	e.enableOwnerRestrictionsAt(restriction.StorePath)
}

func (e *Engine) enableOwnerRestrictionsAt(path string) {
	if e == nil {
		return
	}
	store, err := restriction.Open(path)
	e.deliberationMu.Lock()
	defer e.deliberationMu.Unlock()
	if err != nil {
		e.ownerRestrictions = nil
		e.ownerRestrictionsOn = false
		e.lastOwnerRestrictionNote = "no se pudo cargar la restricción del propietario: " + err.Error()
		return
	}
	e.ownerRestrictions = store
	e.ownerRestrictionsOn = true
	e.lastOwnerRestrictionNote = strings.Join(store.Warnings(), "; ")
}

// ownerRestrictionsEnabled reports whether the durable mechanism was loaded.
// It distinguishes "disabled" from "enabled with no active restriction".
func (e *Engine) ownerRestrictionsEnabled() bool {
	if e == nil {
		return false
	}
	e.deliberationMu.Lock()
	defer e.deliberationMu.Unlock()
	return e.ownerRestrictionsOn && e.ownerRestrictions != nil
}

// ownerWorkInitiativeRestrictedNow is the clock-taking wrapper for callers
// that do not pass an instant. It uses the scene clock when one is installed.
func (e *Engine) ownerWorkInitiativeRestrictedNow() bool {
	return e.OwnerWorkInitiativeRestricted(e.nowTime())
}

// nowTime is the engine clock. Scene runs inject a simulated clock; the live
// engine uses wall time.
func (e *Engine) nowTime() time.Time {
	if e != nil && e.sceneNow != nil {
		return e.sceneNow()
	}
	return time.Now()
}

// OwnerWorkInitiativeRestricted is the scene-1 gate. True means Ahmelia's own
// work initiative is closed until the restriction expires or the owner lifts
// it. An empty or disabled mechanism never restricts.
func (e *Engine) OwnerWorkInitiativeRestricted(now time.Time) bool {
	if e == nil {
		return false
	}
	e.deliberationMu.Lock()
	enabled := e.ownerRestrictionsOn && e.ownerRestrictions != nil
	store := e.ownerRestrictions
	e.deliberationMu.Unlock()
	if !enabled || store == nil {
		return false
	}
	return store.WorkInitiativeActive(now)
}

// SetWorkInitiativeRestriction admits the bounded restriction used by scene 1:
// it expires at the end of the local day and only subtracts Ahmelia's own work
// initiative. It is the owner gate's entry point, never a model tool.
func (e *Engine) SetWorkInitiativeRestriction(statement, turn string, now time.Time) (restriction.OwnerRestriction, error) {
	return e.setWorkInitiativeRestriction(
		statement, restriction.SourceExplicitOwner, turn, now,
		[]string{"owner", "turn:" + strings.TrimSpace(turn)},
	)
}

// SetProvisionalWorkInitiativeRestriction admits the same bounded restriction
// after a confirmed model reading. The source is confirmed_inference, the
// evidence carries the model trace and the expiry is still the end of the day:
// the model never creates a durable or open-ended prohibition.
func (e *Engine) SetProvisionalWorkInitiativeRestriction(statement, turn string, now time.Time, evidence []string) (restriction.OwnerRestriction, error) {
	return e.setWorkInitiativeRestriction(statement, restriction.SourceConfirmedInference, turn, now, evidence)
}

func (e *Engine) setWorkInitiativeRestriction(statement string, source restriction.Source, turn string, now time.Time, evidence []string) (restriction.OwnerRestriction, error) {
	if e == nil {
		return restriction.OwnerRestriction{}, restriction.ErrInvalid
	}
	e.deliberationMu.Lock()
	enabled := e.ownerRestrictionsOn && e.ownerRestrictions != nil
	store := e.ownerRestrictions
	e.deliberationMu.Unlock()
	if !enabled || store == nil {
		return restriction.OwnerRestriction{}, restriction.ErrInvalid
	}
	return store.AdmitWorkInitiative(statement, source, turn, now, now.Location(), evidence)
}

// LiftOwnerRestriction ends a restriction explicitly and keeps the record in
// history. Only the owner gate calls it.
func (e *Engine) LiftOwnerRestriction(id, by string, at time.Time) (restriction.OwnerRestriction, error) {
	if e == nil {
		return restriction.OwnerRestriction{}, restriction.ErrNotFound
	}
	e.deliberationMu.Lock()
	enabled := e.ownerRestrictionsOn && e.ownerRestrictions != nil
	store := e.ownerRestrictions
	e.deliberationMu.Unlock()
	if !enabled || store == nil {
		return restriction.OwnerRestriction{}, restriction.ErrNotFound
	}
	return store.Lift(id, by, at)
}

// SweepOwnerRestrictions marks expired records and persists the transition.
// Callers can rely on the read gate without sweeping; this is for audits and
// startup so the history says "expired" instead of leaving a stale active row.
func (e *Engine) SweepOwnerRestrictions(now time.Time) error {
	if e == nil {
		return nil
	}
	e.deliberationMu.Lock()
	enabled := e.ownerRestrictionsOn && e.ownerRestrictions != nil
	store := e.ownerRestrictions
	e.deliberationMu.Unlock()
	if !enabled || store == nil {
		return nil
	}
	return store.Sweep(now)
}

// ActiveOwnerRestrictions returns a copy of the restrictions that govern now.
func (e *Engine) ActiveOwnerRestrictions(now time.Time) []restriction.OwnerRestriction {
	if e == nil {
		return nil
	}
	e.deliberationMu.Lock()
	enabled := e.ownerRestrictionsOn && e.ownerRestrictions != nil
	store := e.ownerRestrictions
	e.deliberationMu.Unlock()
	if !enabled || store == nil {
		return nil
	}
	return store.Active(now)
}

// OwnerRestrictionHistory returns every record, including lifted and expired.
func (e *Engine) OwnerRestrictionHistory() []restriction.OwnerRestriction {
	if e == nil {
		return nil
	}
	e.deliberationMu.Lock()
	enabled := e.ownerRestrictionsOn && e.ownerRestrictions != nil
	store := e.ownerRestrictions
	e.deliberationMu.Unlock()
	if !enabled || store == nil {
		return nil
	}
	return store.History()
}

// LastOwnerRestrictionNote reports load warnings (for example, a quarantined
// corrupt file). It is not an error: the mechanism keeps working with an empty
// list rather than inventing a restriction.
func (e *Engine) LastOwnerRestrictionNote() string {
	if e == nil {
		return ""
	}
	e.deliberationMu.Lock()
	defer e.deliberationMu.Unlock()
	return e.lastOwnerRestrictionNote
}
