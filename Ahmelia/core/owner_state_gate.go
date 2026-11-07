package core

// owner_state_gate.go — ESTADO EFÍMERO DECLARADO POR EL PROPIETARIO.
//
// Reconoce solo formas explícitas de dolor, cansancio y recuperación. No
// interpreta lenguaje general ni crea un hecho médico: guarda una lectura con
// TTL y responde sin interrogar. El modelo no escribe este estado.

import (
	"fmt"
	"strings"
	"time"

	"ahmelia/interaction"
	"ahmelia/ownerstate"
)

type ownerStateCommandKind int

const (
	ownerStateNone ownerStateCommandKind = iota
	ownerStatePain
	ownerStateFatigue
	ownerStateClear
)

const ownerStateTTL = 2 * time.Hour

func parseOwnerStateCommand(raw string) ownerStateCommandKind {
	s := normalizeOwnerRestrictionPhrase(raw)
	if s == "" {
		return ownerStateNone
	}
	if strings.Contains(s, "no me duele") || strings.HasPrefix(s, "no estoy cansad") || strings.HasPrefix(s, "no estoy agotad") {
		return ownerStateNone
	}
	if strings.HasPrefix(s, "ya se me paso") || strings.HasPrefix(s, "ya me siento bien") ||
		strings.HasPrefix(s, "estoy mejor") || strings.HasPrefix(s, "se me quito") {
		return ownerStateClear
	}
	if strings.Contains(s, "dolor de cabeza") || strings.Contains(s, "migrana") ||
		(strings.Contains(s, "me duele") && (strings.Contains(s, "cabeza") || strings.Contains(s, "espalda") || strings.Contains(s, "estomago"))) {
		return ownerStatePain
	}
	if strings.HasPrefix(s, "estoy cansad") || strings.HasPrefix(s, "estoy agotad") ||
		strings.Contains(s, "necesito descansar") || strings.Contains(s, "sin energia") {
		return ownerStateFatigue
	}
	return ownerStateNone
}

func (e *Engine) ownerStatePhase(t *processTurn) bool {
	if e == nil || t == nil || !e.ownerStateEnabled() {
		return false
	}
	kind := parseOwnerStateCommand(t.input)
	if kind == ownerStateNone {
		return false
	}
	now := e.nowTime()
	var text string
	switch kind {
	case ownerStatePain, ownerStateFatigue:
		stateKind := "headache"
		intensity := 0.8
		if kind == ownerStateFatigue {
			stateKind = "fatigue"
			intensity = 0.6
		}
		err := e.SetOwnerEphemeralState(ownerstate.State{
			Kind:       stateKind,
			Intensity:  intensity,
			Source:     "explicit",
			ObservedAt: now,
			ExpiresAt:  now.Add(ownerStateTTL),
			Privacy:    ownerstate.PrivacySensitive,
			Persist:    ownerstate.PersistEphemeral,
			Evidence:   []string{"turn:" + strings.TrimSpace(t.input)},
		})
		if err != nil {
			text = fmt.Sprintf("No pude guardar tu estado: %v. No cambié nada.", err)
			break
		}
		if kind == ownerStatePain {
			text = "Lo siento. Mientras te sientas así no voy a proponerte trabajo ni actividades por mi cuenta. Tus órdenes concretas siguen en pie."
		} else {
			text = "Entendido. Bajo las propuestas no urgentes mientras descansas; tus órdenes concretas siguen en pie."
		}
	case ownerStateClear:
		if err := e.ClearOwnerState(); err != nil {
			text = fmt.Sprintf("No pude limpiar tu estado: %v. No cambié nada.", err)
			break
		}
		text = "Me alegra. Vuelvo a proponer actividades solo si tiene sentido."
	}
	if strings.TrimSpace(text) == "" {
		return false
	}

	req := t.semanticReq
	if strings.TrimSpace(req.Raw) == "" {
		req.Raw = t.input
		req.Objective.Text = t.input
	}
	e.renderVerbatimSystemResponse(req, "owner_state_gate", text, interaction.MoveAcknowledge, interaction.ResponseIntentAcknowledge, false)
	if e.flowTrace != nil {
		e.flowTrace.Mark("OWNER STATE GATE", fmt.Sprintf("kind=%d", kind))
	}
	t.ownerStateHandled = true
	return true
}

func (e *Engine) ownerStateEnabled() bool {
	if e == nil {
		return false
	}
	e.deliberationMu.Lock()
	defer e.deliberationMu.Unlock()
	return e.ownerStateOn && e.ownerState != nil
}
