package core

// persistent_mode_gate.go — PUERTA CONVERSACIONAL DEL MODO PERSISTENTE.
//
// El modo persistente pertenece al propietario. Esta puerta es determinista y
// corre antes de cualquier interpretación del turno:
//
//   - «¿cuál es el modo persistente?» / «modo actual» / «modo por defecto»
//     informa el estado y recuerda quién puede cambiarlo;
//   - «cambia el modo a focus» (o «modo: focus») cambia un único modo;
//   - negación, varios modos o una asignación desconocida fallan cerrados.
//
// El modelo no participa en el reconocimiento. Su sugerencia se registra en la
// deliberación, pero nunca se aplica aquí.

import (
	"fmt"
	"regexp"
	"strings"

	"ahmelia/agency"
	"ahmelia/interaction"
	"ahmelia/persistentmode"
	"ahmelia/shared"
)

type persistentModeCommandKind string

const (
	persistentModeNone  persistentModeCommandKind = ""
	persistentModeQuery persistentModeCommandKind = "query"
	persistentModeSet   persistentModeCommandKind = "set"
)

type persistentModeCommand struct {
	Kind    persistentModeCommandKind
	Mode    agency.Mode
	Problem string
}

var (
	persistentModeWordRe  = regexp.MustCompile(`\bmodos?\b`)
	persistentModeQueryRe = regexp.MustCompile(`\b(modo persistente|modo actual|modo por defecto)\b`)
	persistentModeNegRe   = regexp.MustCompile(`\b(no|nunca|jamas|tampoco)\b`)
	persistentModeVerbRe  = regexp.MustCompile(`\b(cambia|cambiar|cambies|cambie|pasa|pasar|pases|pase|pon|poner|pongas|ponga|ponte|quedate|quedar|quedes|quede|activa|activar|actives|active|usa|usar|uses|use|elige|elegir|elijas|elija|selecciona|seleccionar|selecciones|seleccione|vuelve|volver|vuelvas|vuelva|entra|entrar|entres|entre|establece|establecer|establezcas|establezca|fija|fijar|fijes|fije|deja|dejar|dejes|deje)\b`)
	// Assignment is read from the raw text because shared.Normalize removes
	// the colon. Only letters are captured so surrounding punctuation is not
	// mistaken for a mode name.
	persistentModeAssignmentRe = regexp.MustCompile(`(?i)\bmodo\s*[:=]\s*(\p{L}+)`)
)

// parsePersistentModeCommand reads only explicit owner commands. A sentence
// that merely mentions a mode is not an order.
func parsePersistentModeCommand(raw string) persistentModeCommand {
	norm := shared.Normalize(raw)
	if strings.TrimSpace(norm) == "" {
		return persistentModeCommand{}
	}
	modes := persistentmode.ModesInText(norm)
	assignment, hasAssignment, token := persistentModeAssignment(raw)
	hasModeWord := persistentModeWordRe.MatchString(norm)
	hasVerb := persistentModeVerbRe.MatchString(norm)
	query := persistentModeQueryRe.MatchString(norm)

	intent := hasModeWord && (hasVerb || hasAssignment)
	if !intent {
		if query {
			return persistentModeCommand{Kind: persistentModeQuery}
		}
		return persistentModeCommand{}
	}
	if persistentModeNegRe.MatchString(persistentModeHead(norm)) {
		return persistentModeCommand{
			Kind:    persistentModeSet,
			Problem: "No cambié el modo persistente: tu mensaje contiene una negación. Si quieres cambiarlo, dilo en positivo y con un solo modo (por ejemplo: cambia el modo a focus).",
		}
	}
	if hasAssignment && !assignment.Valid() {
		return persistentModeCommand{
			Kind:    persistentModeSet,
			Problem: fmt.Sprintf("No reconozco «%s» como modo persistente. No cambié nada. Los modos válidos son focus, explore, rest y companion.", token),
		}
	}
	if len(modes) > 1 {
		return persistentModeCommand{
			Kind:    persistentModeSet,
			Problem: "Veo más de un modo en tu mensaje. No cambié nada. Dime exactamente un modo: focus, explore, rest o companion.",
		}
	}
	if len(modes) == 0 {
		// Do not hijack a normal sentence that merely contains the word
		// "modo". Only an explicit persistent-mode phrase or an assignment
		// asks for a missing mode.
		if hasAssignment || query {
			return persistentModeCommand{
				Kind:    persistentModeSet,
				Problem: "No cambié el modo persistente: necesito un modo exacto (focus, explore, rest o companion). Por ejemplo: cambia el modo a focus.",
			}
		}
		return persistentModeCommand{}
	}
	return persistentModeCommand{Kind: persistentModeSet, Mode: modes[0]}
}

func persistentModeAssignment(raw string) (agency.Mode, bool, string) {
	match := persistentModeAssignmentRe.FindStringSubmatch(raw)
	if len(match) < 2 {
		return agency.ModeUnknown, false, ""
	}
	token := strings.TrimSpace(match[1])
	return persistentmode.ParseModeWord(token), true, token
}

// persistentModeHead returns the text before the word "modo". Negation only
// counts there: an explanation after the mode ("... porque no quiero ruido")
// is still a valid command.
func persistentModeHead(norm string) string {
	if at := persistentModeWordRe.FindStringIndex(norm); at != nil {
		return norm[:at[0]]
	}
	return norm
}

func (e *Engine) applyPersistentModeCommand(cmd persistentModeCommand) (string, bool) {
	if cmd.Kind == persistentModeNone {
		return "", false
	}
	if cmd.Problem != "" {
		return cmd.Problem, true
	}
	if e == nil {
		return "No tengo el modo persistente disponible ahora mismo. No cambié nada.", true
	}

	switch cmd.Kind {
	case persistentModeQuery:
		state, ok := e.persistentModeState()
		if !ok {
			return "El modo persistente no está habilitado en este proceso. No cambié nada.", true
		}
		profile := persistentmode.ProfileFor(state.Mode)
		return fmt.Sprintf("Modo persistente actual: %s (%s). %s El modo por turno se decide aparte. Solo tú puedes cambiarlo; yo puedo sugerirlo.", profile.Label, state.Mode, profile.Description), true
	case persistentModeSet:
		state, err := e.SetPersistentMode(cmd.Mode, "owner")
		if err != nil {
			return fmt.Sprintf("No pude guardar el modo persistente: %v. No cambié nada.", err), true
		}
		profile := persistentmode.ProfileFor(state.Mode)
		return fmt.Sprintf("Modo persistente cambiado a %s (%s). %s Queda guardado para los próximos turnos y arranques. El modelo puede verlo y sugerirlo, pero no cambiarlo.", profile.Label, state.Mode, profile.Description), true
	}
	return "", false
}

// persistentModePhase consumes an explicit owner command before any other
// interpretation of the turn. It is part of ownerGatePhase.
func (e *Engine) persistentModePhase(t *processTurn) bool {
	if e == nil || t == nil {
		return false
	}
	if _, ok := e.persistentModeState(); !ok {
		return false
	}
	cmd := parsePersistentModeCommand(t.input)
	if cmd.Kind == persistentModeNone {
		return false
	}
	text, handled := e.applyPersistentModeCommand(cmd)
	if !handled || strings.TrimSpace(text) == "" {
		return false
	}

	move, kind, needsInput := interaction.MoveAnswer, interaction.ResponseIntentAnswer, false
	switch {
	case cmd.Problem != "":
		move, kind, needsInput = interaction.MoveClarify, interaction.ResponseIntentClarify, true
	case cmd.Kind == persistentModeSet:
		move, kind = interaction.MoveAcknowledge, interaction.ResponseIntentAcknowledge
	}

	req := t.semanticReq
	if strings.TrimSpace(req.Raw) == "" {
		req.Raw = t.input
		req.Objective.Text = t.input
	}
	e.renderVerbatimSystemResponse(req, "persistent_mode_owner_gate", text, move, kind, needsInput)
	if e.flowTrace != nil {
		e.flowTrace.Mark("PERSISTENT MODE GATE", fmt.Sprintf("%s mode=%s", cmd.Kind, cmd.Mode))
	}
	return true
}
