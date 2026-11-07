package core

// owner_restriction_gate.go — ESCENA 1: LA FRASE DEL PROPIETARIO SE CONVIERTE
// EN UNA RESTRICCIÓN ACOTADA.
//
// El reconocimiento es determinista y explícito: o la frase es una orden de
// descanso/reapertura, o el turno pasa intacto. El modelo no crea, activa ni
// levanta restricciones. La restricción de trabajo solo resta iniciativa
// propia hasta fin del día; no bloquea una orden concreta del propietario.

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	"ahmelia/interaction"
	"ahmelia/restriction"
)

type ownerRestrictionCommandKind int

const (
	ownerRestrictionNone ownerRestrictionCommandKind = iota
	ownerRestrictionRestToday
	ownerRestrictionWorkToday
)

type ownerRestrictionCommand struct {
	Kind ownerRestrictionCommandKind
}

// parseOwnerRestrictionCommand reads only explicit, whole-phrase commands. A
// sentence such as "hoy trabajo en el jardín" is not a reapertura.
func parseOwnerRestrictionCommand(raw string) ownerRestrictionCommand {
	normalized := normalizeOwnerRestrictionPhrase(raw)
	switch normalized {
	case "hoy no trabajamos", "hoy no trabajo", "no trabajamos hoy", "no trabajo hoy",
		"hoy descansamos", "hoy descanso", "descansamos hoy", "descanso hoy":
		return ownerRestrictionCommand{Kind: ownerRestrictionRestToday}
	case "hoy si trabajamos", "hoy si trabajo", "hoy trabajamos", "hoy trabajo",
		"volvemos a trabajar", "vuelvo a trabajar":
		return ownerRestrictionCommand{Kind: ownerRestrictionWorkToday}
	}
	return ownerRestrictionCommand{}
}

func normalizeOwnerRestrictionPhrase(raw string) string {
	lower := strings.ToLower(strings.TrimSpace(raw))
	lower = strings.NewReplacer(
		"á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ü", "u",
	).Replace(lower)
	lower = strings.Map(func(r rune) rune {
		if unicode.IsPunct(r) || unicode.IsSymbol(r) {
			return ' '
		}
		return r
	}, lower)
	return strings.Join(strings.Fields(lower), " ")
}

// ownerRestrictionPhase consumes an explicit owner command before any other
// interpretation of the turn. It is part of ownerGatePhase.
func (e *Engine) ownerRestrictionPhase(t *processTurn) bool {
	if e == nil || t == nil || !e.ownerRestrictionsEnabled() {
		return false
	}
	cmd := parseOwnerRestrictionCommand(t.input)
	if cmd.Kind == ownerRestrictionNone {
		return false
	}
	text, handled := e.applyOwnerRestrictionCommand(cmd, e.nowTime())
	if !handled || strings.TrimSpace(text) == "" {
		return false
	}

	req := t.semanticReq
	if strings.TrimSpace(req.Raw) == "" {
		req.Raw = t.input
		req.Objective.Text = t.input
	}
	e.renderVerbatimSystemResponse(req, "owner_restriction_gate", text, interaction.MoveAcknowledge, interaction.ResponseIntentAcknowledge, false)
	if e.flowTrace != nil {
		e.flowTrace.Mark("OWNER RESTRICTION GATE", fmt.Sprintf("kind=%d", cmd.Kind))
	}
	t.ownerRestrictionHandled = true
	return true
}

func (e *Engine) applyOwnerRestrictionCommand(cmd ownerRestrictionCommand, now time.Time) (string, bool) {
	switch cmd.Kind {
	case ownerRestrictionRestToday:
		if e.OwnerWorkInitiativeRestricted(now) {
			return "Ya tengo anotado que hoy no hay trabajo. No tomo iniciativa de trabajo hasta que termine el día; tus órdenes concretas siguen en pie.", true
		}
		r, err := e.SetWorkInitiativeRestriction("hoy no trabajamos", "owner", now)
		if err != nil {
			return fmt.Sprintf("No pude guardar la restricción de trabajo: %v. No cambié nada.", err), true
		}
		until := "fin del día"
		if r.ExpiresAt != nil {
			until = r.ExpiresAt.Format("15:04")
		}
		text := fmt.Sprintf("Entendido: hoy no trabajamos. Cierro mi iniciativa de trabajo hasta las %s; no voy a proponer ni retomar trabajo por mi cuenta. Tus órdenes concretas siguen en pie. Puedes levantar la restricción diciendo «hoy sí trabajamos».", until)
		if e.ownerStateEnabled() && e.AskOwnerRestQuestion() {
			text += " Si quieres, cuéntame el motivo; no insistiré."
		}
		text += e.maybeProposeRecreation(now, 2*time.Hour)
		return text, true
	case ownerRestrictionWorkToday:
		lifted := 0
		for _, r := range e.ActiveOwnerRestrictions(now) {
			if r.Scope != restriction.ScopeWorkInitiative {
				continue
			}
			if _, err := e.LiftOwnerRestriction(r.ID, "owner", now); err == nil {
				lifted++
			}
		}
		if lifted == 0 {
			return "No tenía una restricción de trabajo activa para hoy.", true
		}
		return "Listo: levanto la restricción de trabajo. Vuelvo a poder tomar iniciativa propia.", true
	}
	return "", false
}
