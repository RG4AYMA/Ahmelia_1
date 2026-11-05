package core

// persistent_mode.go — MODO PERSISTENTE APLICADO POR EL ARNÉS (PASO 3.4).
//
// El propietario elige una postura durable; el modelo solo la ve y puede
// sugerir otra. Este archivo conecta esa decisión con los consumidores que ya
// existen:
//
//   - sesgo acotado del modo conversacional del turno (expresión y detalle);
//   - impulsos del arnés (rest no abre ninguno);
//   - iniciativa conversacional (rest no interviene);
//   - cupo del planificador (rest deja el cupo en 0).
//
// No crea un nuevo agente ni un nuevo bucle. Si el archivo durable está
// dañado, EnablePersistentMode deja el mecanismo apagado: el arnés no repara
// ni adivina una decisión del propietario.

import (
	"errors"
	"fmt"
	"strings"

	"ahmelia/agency"
	"ahmelia/cognition"
	"ahmelia/persistentmode"
)

// EnablePersistentMode turns on the owner's durable mode in the live process.
// Like the other paso-3 mechanisms it is opt-in so test suites do not read or
// write runtime state.
func (e *Engine) EnablePersistentMode() {
	e.enablePersistentModeAt(persistentmode.StorePath)
}

func (e *Engine) enablePersistentModeAt(path string) {
	if e == nil {
		return
	}
	if e.persistentMode == nil {
		e.persistentMode = persistentmode.NewStore(path)
	} else {
		e.persistentMode.SetPath(path)
	}
	err := e.persistentMode.Load()
	if err == nil {
		// The first start has no file; write the explicit default so the
		// owner's state is auditable and survives the next restart.
		err = e.persistentMode.Ensure()
	}
	e.deliberationMu.Lock()
	if err != nil {
		e.lastPersistentModeErr = "no se pudo cargar el modo persistente: " + err.Error()
		e.persistentModeOn = false
	} else {
		e.lastPersistentModeErr = ""
		e.persistentModeOn = true
	}
	e.deliberationMu.Unlock()
}

// SetPersistentMode records an owner decision. It never applies a model
// suggestion: the caller is the owner gate or the composition root.
func (e *Engine) SetPersistentMode(mode agency.Mode, by string) (persistentmode.State, error) {
	if e == nil {
		return persistentmode.State{}, errors.New("persistentmode: motor no configurado")
	}
	e.deliberationMu.Lock()
	enabled := e.persistentModeOn && e.persistentMode != nil
	store := e.persistentMode
	e.deliberationMu.Unlock()
	if !enabled || store == nil {
		return persistentmode.State{}, persistentmode.ErrDisabled
	}
	state, err := store.Set(mode, by)
	if err != nil {
		e.deliberationMu.Lock()
		e.lastPersistentModeErr = err.Error()
		e.deliberationMu.Unlock()
		return state, err
	}
	e.deliberationMu.Lock()
	e.lastPersistentModeErr = ""
	e.deliberationMu.Unlock()
	return state, nil
}

// PersistentModeState returns a read-only copy of the owner's decision.
func (e *Engine) PersistentModeState() (persistentmode.State, bool) {
	return e.persistentModeState()
}

// LastPersistentModeError reports the last load or persist error, if any.
func (e *Engine) LastPersistentModeError() string {
	if e == nil {
		return ""
	}
	e.deliberationMu.Lock()
	defer e.deliberationMu.Unlock()
	return e.lastPersistentModeErr
}

func (e *Engine) persistentModeState() (persistentmode.State, bool) {
	if e == nil {
		return persistentmode.State{}, false
	}
	e.deliberationMu.Lock()
	enabled := e.persistentModeOn && e.persistentMode != nil
	store := e.persistentMode
	e.deliberationMu.Unlock()
	if !enabled || store == nil {
		return persistentmode.State{}, false
	}
	return store.Current(), true
}

func (e *Engine) persistentModeProfile() (persistentmode.Profile, bool) {
	state, ok := e.persistentModeState()
	if !ok {
		return persistentmode.Profile{}, false
	}
	return persistentmode.ProfileFor(state.Mode), true
}

// persistentModeAllowsImpulses is true when the harness may open a self-goal
// window. Without the mechanism, behavior stays as it was.
func (e *Engine) persistentModeAllowsImpulses() bool {
	profile, ok := e.persistentModeProfile()
	return !ok || profile.Impulses
}

// persistentModeAllowsInitiative is true when proactive intervention is
// admissible. Rest is the only mode that closes it.
func (e *Engine) persistentModeAllowsInitiative() bool {
	profile, ok := e.persistentModeProfile()
	return !ok || profile.Proactive
}

// agentGoalPlannerQuota returns the owner's ceiling for Ahmelia-owned goals.
// Rest sets it to zero; the other modes keep the previous quota of one.
func (e *Engine) agentGoalPlannerQuota() int {
	profile, ok := e.persistentModeProfile()
	if !ok {
		return AgentGoalPlannerQuota
	}
	return profile.PlannerQuota
}

// applyPersistentModeToConversationMode returns a biased COPY of the per-turn
// mode. Copying keeps the cognition detector's state intact: the persistent
// mode is a posture for this turn, not a rewrite of the detected mode.
func (e *Engine) applyPersistentModeToConversationMode(mode *cognition.ConversationMode) *cognition.ConversationMode {
	if e == nil || mode == nil {
		return mode
	}
	profile, ok := e.persistentModeProfile()
	if !ok {
		return mode
	}
	biased := *mode
	biased.Warmth = blendPersistentMode(mode.Warmth, profile.Warmth, 0.6)
	biased.DetailLevel = blendPersistentMode(mode.DetailLevel, profile.DetailLevel, 0.7)
	if profile.Proactive {
		biased.Initiative = blendPersistentMode(mode.Initiative, profile.Initiative, 0.6)
	} else {
		biased.Initiative = 0
		biased.CanBringTopics = false
	}
	return &biased
}

func blendPersistentMode(base, bias, weight float64) float64 {
	if weight < 0 {
		weight = 0
	}
	if weight > 1 {
		weight = 1
	}
	value := base*(1-weight) + bias*weight
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

// persistentModeSummary is the bounded text shared by the model-facing
// projections. It always names the owner as the source.
func persistentModeSummary(state persistentmode.State) string {
	profile := persistentmode.ProfileFor(state.Mode)
	text := fmt.Sprintf("%s (%s): %s", profile.Label, state.Mode, profile.Description)
	if source := strings.TrimSpace(state.ChangedBy); source != "" && source != "default" {
		if source == "owner" {
			source = "propietario"
		}
		text += " Fuente: " + source + "."
	}
	return text
}
