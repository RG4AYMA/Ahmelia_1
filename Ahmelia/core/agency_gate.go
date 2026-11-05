package core

// agency_gate.go — PUERTA CONVERSACIONAL DEL PROPIETARIO (PASO 3.2).
//
// El libro de metas propias ya existe (paso 3.1), pero hasta ahora solo se
// podía leer y decidir por API. Esta puerta permite al propietario, por
// conversación:
//
//   - listar las propuestas que esperan su decisión;
//   - autorizar una propuesta con su ID exacto;
//   - rechazarla con su ID exacto.
//
// Reglas de autoridad:
//
//   - El reconocimiento es determinista, sobre el texto crudo del turno. El
//     modelo no interpreta la orden ni elige el ID.
//   - Sin un ID exacto no hay decisión: nunca se infiere por título, orden o
//     parecido.
//   - Más de un ID, verbos contradictorios o negación explícita fallan
//     cerrados: no se cambia nada.
//   - Autorizar compone dos actos del arnés: marca la propuesta como
//     autorizada y la registra en el planificador con cupo 1. No la ejecuta:
//     la autonomía lee AutonomousState, no el planificador.

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"ahmelia/agency"
	"ahmelia/interaction"
	"ahmelia/shared"
)

type agentGoalCommandKind string

const (
	agentGoalNone      agentGoalCommandKind = ""
	agentGoalList      agentGoalCommandKind = "list"
	agentGoalAuthorize agentGoalCommandKind = "authorize"
	agentGoalReject    agentGoalCommandKind = "reject"
)

type agentGoalCommand struct {
	Kind    agentGoalCommandKind
	ID      string
	Reason  string
	Problem string
}

var (
	agentGoalIDRe        = regexp.MustCompile(`\bag-[0-9]+(?:-[0-9]+)?\b`)
	agentGoalAuthorizeRe = regexp.MustCompile(`\b(autoriza|autorizar|aprueba|aprobar|acepta|aceptar)\b`)
	agentGoalRejectRe    = regexp.MustCompile(`\b(rechaza|rechazar|deniega|denegar|descarta|descartar)\b`)
	agentGoalSoftVerbRe  = regexp.MustCompile(`\b(autorices|apruebes|aceptes|rechaces|deniegues|descartes)\b`)
	agentGoalMetaRe      = regexp.MustCompile(`\bmetas?\b`)
	agentGoalOwnRe       = regexp.MustCompile(`\b(propias?|propusiste|propuso|propuestas?)\b`)
	agentGoalNegationRe  = regexp.MustCompile(`\b(no|nunca|jamas|tampoco)\b`)
)

// parseAgentGoalCommand reads only explicit owner commands. Anything ambiguous
// is either ignored (normal conversation) or rejected with an explanation; it
// is never guessed.
func parseAgentGoalCommand(raw string) agentGoalCommand {
	norm := shared.Normalize(raw)
	if strings.TrimSpace(norm) == "" {
		return agentGoalCommand{}
	}
	ids := uniqueStrings(agentGoalIDRe.FindAllString(norm, -1))
	authorize := agentGoalAuthorizeRe.MatchString(norm)
	reject := agentGoalRejectRe.MatchString(norm)
	softVerb := agentGoalSoftVerbRe.MatchString(norm)
	hasMeta := agentGoalMetaRe.MatchString(norm)

	if !authorize && !reject && !softVerb {
		// Read-only listing. It requires both "meta(s)" and a marker that the
		// proposal belongs to Ahmelia; a generic "¿qué metas tienes?" keeps
		// its normal route.
		if hasMeta && agentGoalOwnRe.MatchString(norm) {
			return agentGoalCommand{Kind: agentGoalList}
		}
		return agentGoalCommand{}
	}
	if !hasMeta && len(ids) == 0 {
		return agentGoalCommand{}
	}
	// Negation is checked only in the command head, never in the optional
	// explanation ("... porque no es el momento" is still a valid rejection).
	if agentGoalNegationRe.MatchString(agentGoalCommandHead(norm, ids)) {
		return agentGoalCommand{
			Kind:    agentGoalAuthorize,
			Problem: "No cambié ninguna meta: tu mensaje contiene una negación. Si quieres decidir, dilo en positivo y con el ID exacto (por ejemplo: autoriza la meta ag-...).",
		}
	}
	if authorize && reject {
		return agentGoalCommand{
			Kind:    agentGoalAuthorize,
			Problem: "Veo una autorización y un rechazo en el mismo mensaje. No cambié ninguna meta. Dime una sola decisión con su ID exacto.",
		}
	}
	if !authorize && !reject {
		return agentGoalCommand{
			Kind:    agentGoalAuthorize,
			Problem: "No cambié nada: no reconozco esa forma como una orden directa. Dilo así: autoriza la meta <id> o rechaza la meta <id>.",
		}
	}
	if len(ids) > 1 {
		return agentGoalCommand{
			Kind:    agentGoalAuthorize,
			Problem: "Veo más de un identificador en tu mensaje. No cambié nada. Dime exactamente una meta, con su ID.",
		}
	}
	if len(ids) == 0 {
		return agentGoalCommand{
			Kind:    agentGoalAuthorize,
			Problem: "Para autorizar o rechazar una meta propia necesito su ID exacto (por ejemplo: autoriza la meta ag-...). No cambié nada.",
		}
	}

	kind := agentGoalAuthorize
	if reject {
		kind = agentGoalReject
	}
	return agentGoalCommand{
		Kind:   kind,
		ID:     ids[0],
		Reason: agentGoalDecisionReason(raw, ids[0]),
	}
}

// agentGoalCommandHead returns the command portion before the first goal ID or
// the optional explanation. Negation is only meaningful there.
func agentGoalCommandHead(norm string, ids []string) string {
	head := norm
	if len(ids) > 0 {
		if at := strings.Index(norm, ids[0]); at >= 0 {
			head = norm[:at]
		}
	}
	lower := strings.ToLower(head)
	for _, marker := range []string{"porque", "ya que", "puesto que"} {
		if at := strings.Index(lower, marker); at >= 0 {
			head = head[:at]
			break
		}
	}
	return head
}

// agentGoalDecisionReason keeps the owner's optional explanation, bounded.
func agentGoalDecisionReason(raw, id string) string {
	lower := strings.ToLower(raw)
	idx := strings.Index(lower, id)
	if idx < 0 {
		return ""
	}
	tail := strings.TrimSpace(raw[idx+len(id):])
	tail = strings.TrimRight(tail, " .,;:¡!¿?—-")
	if tail == "" {
		return ""
	}
	lowerTail := strings.ToLower(tail)
	for _, marker := range []string{"porque ", "ya que ", "puesto que ", "debido a "} {
		if at := strings.Index(lowerTail, marker); at >= 0 {
			tail = strings.TrimSpace(tail[at+len(marker):])
			break
		}
	}
	if runes := []rune(tail); len(runes) > 180 {
		tail = string(runes[:180]) + "…"
	}
	return tail
}

// applyAgentGoalCommand performs the book decision and returns the exact text
// the owner must hear. It never touches the planner or the execution path.
func (e *Engine) applyAgentGoalCommand(cmd agentGoalCommand) (string, bool) {
	if cmd.Kind == agentGoalNone {
		return "", false
	}
	if cmd.Problem != "" {
		return cmd.Problem, true
	}
	if e == nil || e.agentGoalLedger == nil {
		return "No tengo el libro de metas propias disponible ahora mismo. No cambié nada.", true
	}

	switch cmd.Kind {
	case agentGoalList:
		open := e.AgentGoalProposals()
		if len(open) == 0 {
			return "No tengo ninguna meta propia esperando tu decisión ahora mismo.", true
		}
		var b strings.Builder
		b.WriteString("Metas propias esperando tu decisión:\n")
		limit := len(open)
		if limit > 5 {
			limit = 5
		}
		for _, rec := range open[:limit] {
			fmt.Fprintf(&b, "- %s · %s (riesgo %s, %d min, modo %s)\n",
				rec.ID, rec.Goal.Title, rec.Goal.Risk, rec.Goal.ExpectedMinutes, rec.Goal.Mode)
		}
		if len(open) > limit {
			fmt.Fprintf(&b, "Y %d más.\n", len(open)-limit)
		}
		if e.agentGoalPlannerQuota() <= 0 {
			b.WriteString("Ahora el modo persistente es descanso: no hay cupo de metas. Cambia a focus o explore para autorizar y registrar.")
		} else {
			b.WriteString("Para decidir: «autoriza la meta <id>» o «rechaza la meta <id>». Autorizar la registra en el planificador con cupo 1; no la ejecuta.")
		}
		return b.String(), true

	case agentGoalAuthorize, agentGoalReject:
		// Rest closes the goal window, including authorization: authorizing
		// without registering would leave the record stuck in "authorized"
		// with no conversational path to register it later.
		if cmd.Kind == agentGoalAuthorize && e.agentGoalPlannerQuota() <= 0 {
			if rec, ok := e.AgentGoalRecord(cmd.ID); ok && rec.Status == agency.GoalProposed {
				return fmt.Sprintf("No autoricé la meta %s (%q): el modo persistente es descanso y no abre cupo de metas. Cambia a focus o explore y vuelve a autorizarla. No cambié nada.", rec.ID, rec.Goal.Title), true
			}
		}
		var (
			rec agency.GoalRecord
			err error
		)
		if cmd.Kind == agentGoalAuthorize {
			rec, err = e.AuthorizeAgentGoal(cmd.ID, cmd.Reason)
		} else {
			rec, err = e.RejectAgentGoal(cmd.ID, cmd.Reason)
		}
		if err != nil {
			switch {
			case errors.Is(err, agency.ErrNotFound):
				return fmt.Sprintf("No encuentro ninguna meta propia con el ID %s. No cambié nada.", cmd.ID), true
			case errors.Is(err, agency.ErrNotProposed):
				status := "desconocido"
				if got, ok := e.AgentGoalRecord(cmd.ID); ok {
					status = string(got.Status)
				}
				return fmt.Sprintf("La meta %s ya está en estado %s; no la cambié.", cmd.ID, status), true
			default:
				return fmt.Sprintf("No pude aplicar la decisión sobre %s: %v. No cambié nada.", cmd.ID, err), true
			}
		}
		if cmd.Kind == agentGoalAuthorize {
			// Owner decision (2026-11-01): an authorized goal enters the
			// planner with quota 1. Registration is still a second act, so a
			// quota or persistence failure can be reported without undoing
			// the authorization; nothing is executed either way.
			updated, goal, regErr := e.RegisterAgentGoal(rec.ID)
			if regErr != nil {
				switch {
				case errors.Is(regErr, ErrAgentGoalRestMode):
					return fmt.Sprintf("Marco la meta %s como autorizada: %q. No la registré en el planificador: el modo persistente es descanso y no abre cupo de metas. Cambia a focus o explore para registrarla. No ejecuté nada.", rec.ID, rec.Goal.Title), true
				case errors.Is(regErr, ErrAgentGoalPlannerQuota):
					return fmt.Sprintf("Marco la meta %s como autorizada: %q. No la registré en el planificador: %v. No ejecuté nada.", rec.ID, rec.Goal.Title, regErr), true
				case errors.Is(regErr, agency.ErrAlreadyRegistered):
					return fmt.Sprintf("La meta %s ya estaba registrada en el planificador como %s. No ejecuté nada.", rec.ID, updated.PlannerGoalID), true
				default:
					return fmt.Sprintf("Marco la meta %s como autorizada: %q, pero no pude registrarla en el planificador: %v. No ejecuté nada.", rec.ID, rec.Goal.Title, regErr), true
				}
			}
			return fmt.Sprintf("Confirmo la autorización de la meta %s: %q. Queda registrada en el planificador con cupo 1 (goal %s), sin ejecución automática.", updated.ID, updated.Goal.Title, goal.ID), true
		}
		return fmt.Sprintf("Rechacé la meta %s: %q. No se ejecutará.", rec.ID, rec.Goal.Title), true
	}
	return "", false
}

// agentGoalPhase consumes an explicit owner decision before any pending work
// authorization can reinterpret the turn. It runs only after the regular input
// and conversation-state phases, so the exchange stays in the conversation.
func (e *Engine) agentGoalPhase(t *processTurn) bool {
	if e == nil || t == nil {
		return false
	}
	e.deliberationMu.Lock()
	enabled := e.agentGoalOn && e.agentGoalLedger != nil
	e.deliberationMu.Unlock()
	if !enabled {
		return false
	}

	cmd := parseAgentGoalCommand(t.input)
	if cmd.Kind == agentGoalNone {
		return false
	}
	text, handled := e.applyAgentGoalCommand(cmd)
	if !handled || strings.TrimSpace(text) == "" {
		return false
	}

	move, kind, needsInput := interaction.MoveAnswer, interaction.ResponseIntentAnswer, false
	switch {
	case cmd.Problem != "":
		move, kind, needsInput = interaction.MoveClarify, interaction.ResponseIntentClarify, true
	case cmd.Kind == agentGoalAuthorize || cmd.Kind == agentGoalReject:
		move, kind = interaction.MoveAcknowledge, interaction.ResponseIntentAcknowledge
	}

	req := t.semanticReq
	if strings.TrimSpace(req.Raw) == "" {
		req.Raw = t.input
		req.Objective.Text = t.input
	}
	// Owner decisions must be spoken verbatim: the ID, the state and the "no
	// se ejecutó" clause are the content.
	e.renderVerbatimSystemResponse(req, "agent_goal_owner_gate", text, move, kind, needsInput)
	if e.flowTrace != nil {
		e.flowTrace.Mark("AGENT GOAL GATE", fmt.Sprintf("%s id=%s", cmd.Kind, cmd.ID))
	}
	return true
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
	for _, v := range values {
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}
