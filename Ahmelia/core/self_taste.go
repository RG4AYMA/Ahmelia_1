package core

// self_taste.go — GUSTOS PROPIOS CON EVIDENCIA (PASO 3.3).
//
// El arnés registra experiencias verificadas y el propietario decide si
// cuentan como gusto. El modelo no declara un gusto por su cuenta: solo lo ve
// como contexto en la deliberación cuando el posterior alcanzó MinN=5.
//
// Puerta conversacional determinista, misma disciplina que la puerta de metas:
//
//   - «¿qué gustos propios tienes?» lista gustos formados y experiencias
//     pendientes con su ID y su referencia.
//   - «confirma el gusto tg-…» convierte una experiencia verificada en
//     evidencia positiva.
//   - «rechaza el gusto tg-…» la descarta.
//   - «corrige tu gusto por <actividad>» borra la evidencia anterior de esa
//     actividad y abre un posterior negativo. La corrección pesa más.
//
// Sin ID exacto no se confirma ni se rechaza. Sin actividad explícita no hay
// corrección. El modelo no participa en el reconocimiento.

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"ahmelia/agency"
	"ahmelia/interaction"
	"ahmelia/personality"
	"ahmelia/planning"
	"ahmelia/shared"
	"ahmelia/taste"
)

type selfTasteCommandKind string

const (
	selfTasteNone    selfTasteCommandKind = ""
	selfTasteList    selfTasteCommandKind = "list"
	selfTasteConfirm selfTasteCommandKind = "confirm"
	selfTasteReject  selfTasteCommandKind = "reject"
	selfTasteCorrect selfTasteCommandKind = "correct"
)

type selfTasteCommand struct {
	Kind    selfTasteCommandKind
	ID      string
	Key     string
	Reason  string
	Problem string
}

var (
	selfTasteIDRe       = regexp.MustCompile(`\btg-[0-9]+(?:-[0-9]+)?\b`)
	selfTasteConfirmRe  = regexp.MustCompile(`\b(confirma|confirmar|acepta|aceptar|aprueba|aprobar)\b`)
	selfTasteRejectRe   = regexp.MustCompile(`\b(rechaza|rechazar|descarta|descartar|deniega|denegar)\b`)
	selfTasteCorrectRe  = regexp.MustCompile(`\b(corrige|corregir|rectifica|rectificar)\b`)
	selfTasteNounRe     = regexp.MustCompile(`\b(gustos?|preferencias?)\b`)
	selfTasteOwnRe      = regexp.MustCompile(`\b(propios?|tuyos?|tus|te gusta|te gustan)\b`)
	selfTasteNegationRe = regexp.MustCompile(`\b(no|nunca|jamas|tampoco)\b`)
	// Negated commands ("no confirmes", "nunca rechaces") keep the positive
	// verb from matching on its own. They must fail closed instead of being
	// read as their opposite.
	selfTasteNegatedVerbRe = regexp.MustCompile(`\b(no|nunca|jamas|tampoco)\s+(confirm|aprueb|acept|rechac|descart|denieg|corrig|corrij|rectific)\w*\b`)
)

// parseSelfTasteCommand reads only explicit owner commands about Ahmelia's own
// tastes. A generic "no me gusta el café" is the user's preference and keeps
// its normal route.
func parseSelfTasteCommand(raw string) selfTasteCommand {
	norm := shared.Normalize(raw)
	if strings.TrimSpace(norm) == "" {
		return selfTasteCommand{}
	}
	ids := uniqueStrings(selfTasteIDRe.FindAllString(norm, -1))
	confirm := selfTasteConfirmRe.MatchString(norm)
	reject := selfTasteRejectRe.MatchString(norm)
	correct := selfTasteCorrectRe.MatchString(norm)
	hasTaste := selfTasteNounRe.MatchString(norm)
	hasSelf := selfTasteOwnRe.MatchString(norm)
	hasTeGusta := strings.Contains(norm, "te gusta")

	// A negated command never becomes its opposite. This runs before verb
	// matching because "no confirmes ..." does not match "confirma".
	if (hasTaste || hasTeGusta) && selfTasteNegatedVerbRe.MatchString(norm) {
		return selfTasteCommand{
			Kind:    selfTasteConfirm,
			Problem: "No cambié ningún gusto: tu mensaje contiene una negación. Si quieres decidir, dilo en positivo y con el ID exacto (por ejemplo: confirma el gusto tg-...).",
		}
	}

	if correct {
		if !hasTaste && !hasTeGusta {
			return selfTasteCommand{}
		}
		if len(ids) > 1 {
			return selfTasteCommand{
				Kind:    selfTasteCorrect,
				Problem: "Veo más de un identificador en tu mensaje. No corregí nada. Dime una sola actividad, por ejemplo: corrige tu gusto por explorar.",
			}
		}
		key := selfTasteCorrectionKey(norm)
		if key == "" {
			return selfTasteCommand{
				Kind:    selfTasteCorrect,
				Problem: "Para corregir un gusto necesito la actividad exacta, por ejemplo: corrige tu gusto por explorar. No cambié nada.",
			}
		}
		return selfTasteCommand{Kind: selfTasteCorrect, Key: key, Reason: raw}
	}

	if confirm || reject {
		if !hasTaste {
			// "aprueba el plan" is not a taste command.
			return selfTasteCommand{}
		}
		if confirm && reject {
			return selfTasteCommand{
				Kind:    selfTasteConfirm,
				Problem: "Veo una confirmación y un rechazo en el mismo mensaje. No cambié ningún gusto. Dime una sola decisión con su ID exacto.",
			}
		}
		if selfTasteNegationRe.MatchString(agentGoalCommandHead(norm, ids)) {
			return selfTasteCommand{
				Kind:    selfTasteConfirm,
				Problem: "No cambié ningún gusto: tu mensaje contiene una negación. Si quieres decidir, dilo en positivo y con el ID exacto (por ejemplo: confirma el gusto tg-...).",
			}
		}
		if len(ids) > 1 {
			return selfTasteCommand{
				Kind:    selfTasteConfirm,
				Problem: "Veo más de un identificador en tu mensaje. No cambié nada. Dime exactamente una experiencia, con su ID.",
			}
		}
		if len(ids) == 0 {
			return selfTasteCommand{
				Kind:    selfTasteConfirm,
				Problem: "Para confirmar o rechazar una experiencia necesito su ID exacto (por ejemplo: confirma el gusto tg-...). No cambié nada.",
			}
		}
		kind := selfTasteConfirm
		if reject {
			kind = selfTasteReject
		}
		return selfTasteCommand{Kind: kind, ID: ids[0], Reason: raw}
	}

	if hasTaste && hasSelf {
		return selfTasteCommand{Kind: selfTasteList}
	}
	return selfTasteCommand{}
}

// selfTasteCorrectionKey extracts the activity after the explicit correction
// marker. It accepts "corrige tu gusto por X", "corrige tu gusto de X" and
// "corrige: no te gusta X"; anything else is not a correction.
func selfTasteCorrectionKey(norm string) string {
	lower := strings.ToLower(norm)
	for _, marker := range []string{" por ", " de ", " no te gusta ", " no te gustan "} {
		if at := strings.LastIndex(lower, marker); at >= 0 {
			key := strings.TrimSpace(norm[at+len(marker):])
			if cut := explanationCut(key); cut != "" {
				key = cut
			}
			key = taste.NormalizeKey(key)
			if key == "" || len([]rune(key)) < 3 {
				return ""
			}
			return key
		}
	}
	return ""
}

func explanationCut(text string) string {
	lower := strings.ToLower(text)
	for _, marker := range []string{" porque ", " ya que ", " puesto que ", " debido a "} {
		if at := strings.Index(lower, marker); at >= 0 {
			return strings.TrimSpace(text[:at])
		}
	}
	return ""
}

// EnableSelfTastes turns on the evidence book and the derived posterior. Like
// the other paso-3 mechanisms it is opt-in so tests do not write runtime state.
func (e *Engine) EnableSelfTastes() {
	if e == nil {
		return
	}
	if e.selfTaste == nil {
		e.selfTaste = taste.NewModel(taste.NewLedger(taste.LedgerPath))
	} else if ledger := e.selfTaste.Ledger(); ledger != nil {
		ledger.SetPath(taste.LedgerPath)
	}
	if err := e.selfTaste.Ledger().Load(); err != nil {
		e.deliberationMu.Lock()
		e.lastSelfTasteErr = "no se pudo cargar el registro de gustos: " + err.Error()
		e.deliberationMu.Unlock()
	}
	e.selfTaste.Rebuild()
	e.deliberationMu.Lock()
	e.selfTasteOn = true
	e.deliberationMu.Unlock()
}

// SelfTasteStances returns only formed tastes. It is a read-only copy.
func (e *Engine) SelfTasteStances() []taste.Stance {
	e.deliberationMu.Lock()
	enabled := e != nil && e.selfTasteOn && e.selfTaste != nil
	e.deliberationMu.Unlock()
	if !enabled {
		return nil
	}
	return e.selfTaste.Stances()
}

// SelfTastePending returns verified experiences waiting for the owner.
func (e *Engine) SelfTastePending() []taste.Candidate {
	e.deliberationMu.Lock()
	enabled := e != nil && e.selfTasteOn && e.selfTaste != nil
	e.deliberationMu.Unlock()
	if !enabled {
		return nil
	}
	return e.selfTaste.Pending()
}

// sweepSelfTasteCandidates records verified activity as a pending experience.
// It never turns success into liking: the owner still has to confirm.
func (e *Engine) sweepSelfTasteCandidates(t *processTurn) {
	if e == nil || e.selfTaste == nil {
		return
	}
	if t != nil && t.interactionHasWork {
		w := t.interactionWorkContext
		if w.LastActionVerified && strings.TrimSpace(w.ID) != "" {
			key := taste.NormalizeKey(w.Provider)
			if key != "" {
				ref := strings.TrimSpace(w.ID)
				if w.AttemptNumber > 0 {
					ref = fmt.Sprintf("%s#%d", ref, w.AttemptNumber)
				}
				// A full queue is reported through LastError, not fatal.
				_, _, _ = e.selfTaste.RecordVerified(key, ref)
			}
		}
	}
	for _, goal := range e.selfTasteAgentGoals() {
		_, _, _ = e.selfTaste.RecordVerified(selfTasteKeyForGoal(goal), goal.ID)
	}
}

func (e *Engine) selfTasteAgentGoals() []*planning.Goal {
	var goals []*planning.Goal
	switch {
	case e == nil:
		return nil
	case e.goalStore != nil:
		goals = e.goalStore.All()
	case e.planner != nil:
		goals = e.planner.Goals()
	}
	out := make([]*planning.Goal, 0, len(goals))
	for _, g := range goals {
		if g == nil || g.Owner != "ahmelia" || g.Status != planning.GoalCompleted {
			continue
		}
		out = append(out, g)
	}
	return out
}

// selfTasteKeyForGoal maps the mode chosen by the harness at registration to a
// stable activity key. It reads the note written by RegisterAgentGoal; it does
// not infer a taste from the title.
func selfTasteKeyForGoal(goal *planning.Goal) string {
	if goal == nil {
		return "trabajar"
	}
	for _, note := range goal.Notes {
		note = strings.TrimSpace(note)
		if !strings.HasPrefix(note, "modo ") {
			continue
		}
		switch strings.TrimSpace(strings.TrimPrefix(note, "modo ")) {
		case string(agency.ModeFocus):
			return "concentrarme"
		case string(agency.ModeExplore):
			return "explorar"
		case string(agency.ModeRest):
			return "descansar"
		case string(agency.ModeCompanion):
			return "acompañar"
		}
	}
	return "trabajar"
}

func (e *Engine) selfTasteOwnerRef() string {
	if e != nil && e.activeConversationTurnID != 0 {
		return fmt.Sprintf("owner-turn-%d", e.activeConversationTurnID)
	}
	return "owner-conversation"
}

// applySelfTasteCommand performs the decision and returns the exact text the
// owner must hear. It never touches the planner or the execution path.
func (e *Engine) applySelfTasteCommand(cmd selfTasteCommand) (string, bool) {
	if cmd.Kind == selfTasteNone {
		return "", false
	}
	if cmd.Problem != "" {
		return cmd.Problem, true
	}
	if e == nil || e.selfTaste == nil {
		return "No tengo el registro de gustos propios disponible ahora mismo. No cambié nada.", true
	}

	switch cmd.Kind {
	case selfTasteList:
		stances := e.selfTaste.Stances()
		pending := e.selfTaste.Pending()
		if len(stances) == 0 && len(pending) == 0 {
			return "Todavía no tengo gustos propios formados ni experiencias verificadas esperando confirmación.", true
		}
		var b strings.Builder
		if len(stances) > 0 {
			b.WriteString("Gustos propios formados:\n")
			limit := len(stances)
			if limit > 5 {
				limit = 5
			}
			for _, s := range stances[:limit] {
				fmt.Fprintf(&b, "- %s: %s (media %.2f, evidencia %.0f", s.Key, s.Stance, s.Mean, s.EffN)
				if len(s.Evidence) > 0 {
					fmt.Fprintf(&b, "; refs: %s", strings.Join(s.Evidence, ", "))
				}
				b.WriteString(")\n")
			}
			if len(stances) > limit {
				fmt.Fprintf(&b, "Y %d más.\n", len(stances)-limit)
			}
		}
		if len(pending) > 0 {
			b.WriteString("Experiencias verificadas esperando tu confirmación:\n")
			limit := len(pending)
			if limit > 5 {
				limit = 5
			}
			for _, c := range pending[:limit] {
				fmt.Fprintf(&b, "- %s · %s (ref %s)\n", c.ID, c.Key, c.Ref)
			}
			if len(pending) > limit {
				fmt.Fprintf(&b, "Y %d más.\n", len(pending)-limit)
			}
		}
		b.WriteString("Para decidir: «confirma el gusto <id>», «rechaza el gusto <id>» o «corrige tu gusto por <actividad>». Nada de esto ejecuta trabajo.")
		return b.String(), true

	case selfTasteConfirm:
		rec, err := e.selfTaste.Confirm(cmd.ID)
		if err != nil {
			return selfTasteErrorText(cmd.ID, err), true
		}
		view := e.selfTaste.View(rec.Key)
		if view.Stance == personality.StanceUnformed {
			return fmt.Sprintf("Confirmo el gusto %s: la experiencia %s (%s) cuenta como evidencia. Aún no alcanza el mínimo de 5; no lo declaro como gusto formado.", rec.ID, rec.Key, rec.Ref), true
		}
		return fmt.Sprintf("Confirmo el gusto %s: la experiencia %s (%s) cuenta como evidencia. Ahora tengo un gusto formado por %s (%s, evidencia %.0f).", rec.ID, rec.Key, rec.Ref, rec.Key, view.Stance, view.EffN), true

	case selfTasteReject:
		rec, err := e.selfTaste.Reject(cmd.ID)
		if err != nil {
			return selfTasteErrorText(cmd.ID, err), true
		}
		return fmt.Sprintf("Rechazo el gusto %s: la experiencia %s (%s) no cuenta como evidencia.", rec.ID, rec.Key, rec.Ref), true

	case selfTasteCorrect:
		rec, err := e.selfTaste.Correct(cmd.Key, e.selfTasteOwnerRef(), cmd.Reason)
		if err != nil {
			return selfTasteErrorText(cmd.Key, err), true
		}
		view := e.selfTaste.View(rec.Key)
		if view.Stance == personality.StanceUnformed {
			return fmt.Sprintf("Corrijo mi gusto por %s: la evidencia anterior deja de contar y abro una observación negativa. El gusto anterior ya no se declara.", rec.Key), true
		}
		return fmt.Sprintf("Corrijo mi gusto por %s: la evidencia anterior deja de contar. Ahora la postura es %s (evidencia %.0f).", rec.Key, view.Stance, view.EffN), true
	}
	return "", false
}

func selfTasteErrorText(id string, err error) string {
	switch {
	case errors.Is(err, taste.ErrNotFound):
		return fmt.Sprintf("No encuentro ninguna experiencia de gusto con el ID %s. No cambié nada.", id)
	case errors.Is(err, taste.ErrNotPending):
		return fmt.Sprintf("La experiencia %s ya fue decidida; no la cambié.", id)
	case errors.Is(err, taste.ErrNoEvidence):
		return "Falta la actividad o la referencia trazable. No cambié nada."
	case errors.Is(err, taste.ErrPendingFull):
		return "Hay demasiadas experiencias esperando confirmación; no registré otra. No cambié nada."
	default:
		return fmt.Sprintf("No pude aplicar la decisión sobre %s: %v. No cambié nada.", id, err)
	}
}

// selfTastePhase consumes an explicit owner taste command. It also sweeps
// verified activity so new candidates appear without a background goroutine.
func (e *Engine) selfTastePhase(t *processTurn) bool {
	if e == nil || t == nil {
		return false
	}
	e.deliberationMu.Lock()
	enabled := e.selfTasteOn && e.selfTaste != nil
	e.deliberationMu.Unlock()
	if !enabled {
		return false
	}

	e.sweepSelfTasteCandidates(t)

	cmd := parseSelfTasteCommand(t.input)
	if cmd.Kind == selfTasteNone {
		return false
	}
	text, handled := e.applySelfTasteCommand(cmd)
	if !handled || strings.TrimSpace(text) == "" {
		return false
	}

	move, kind, needsInput := interaction.MoveAnswer, interaction.ResponseIntentAnswer, false
	switch {
	case cmd.Problem != "":
		move, kind, needsInput = interaction.MoveClarify, interaction.ResponseIntentClarify, true
	case cmd.Kind == selfTasteConfirm || cmd.Kind == selfTasteReject || cmd.Kind == selfTasteCorrect:
		move, kind = interaction.MoveAcknowledge, interaction.ResponseIntentAcknowledge
	}

	req := t.semanticReq
	if strings.TrimSpace(req.Raw) == "" {
		req.Raw = t.input
		req.Objective.Text = t.input
	}
	e.renderVerbatimSystemResponse(req, "self_taste_owner_gate", text, move, kind, needsInput)
	if e.flowTrace != nil {
		e.flowTrace.Mark("SELF TASTE GATE", fmt.Sprintf("%s id=%s key=%s", cmd.Kind, cmd.ID, cmd.Key))
	}
	return true
}
