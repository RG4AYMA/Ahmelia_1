package core

// deliberation_shadow.go — ENGANCHE DE LA DELIBERACIÓN EN SOMBRA.
//
// Este archivo conecta el espacio de trabajo y el contrato de propuesta con el
// turno real. La deliberación corre cuando el turno ya terminó:

//   - no retrasa la respuesta;
//   - no modifica la decisión del arnés;
//   - no autoriza ni ejecuta nada;
//   - usa la ranura del modelo con prioridad propia (la más baja).
//
// Lo único que produce es evidencia: un registro tipado de en qué coincide el
// modelo con el arnés, en qué lo refina y en qué discrepa.

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"ahmelia/deliberation"
	"ahmelia/interaction"
	"ahmelia/situation"
)

const (
	// shadowDeliberationBudget acota el trabajo de fondo. Es independiente del
	// presupuesto del turno: si el modelo no responde, el turno ya habló.
	//
	// Medido en este equipo (2026-11-03, qwen3:4b-instruct-2507-q4_K_M):
	// ~47 s por propuesta en caliente cuando la máquina está libre, pero una
	// propuesta completa de 400 tokens tardó 144 s con el modelo recién
	// cargado a ~2 tok/s. El propietario fijó 300 s (2026-11-03) para que la
	// sombra complete en este hardware. Sigue sin tocar el turno: si llega
	// una petición más prioritaria, el gobernador cancela este lease tras
	// 500 ms de gracia.
	shadowDeliberationBudget = 300 * time.Second
	// shadowDeliberationMaxTokens acota la salida del carril de ideas. 260
	// truncaba el JSON de modelos de 4B cuando incluían self_goal; 400 deja
	// espacio para la lectura, la crítica y la meta sin abrir la puerta a una
	// respuesta ilimitada.
	shadowDeliberationMaxTokens = 400
	// shadowDeliberationMaxEvidence acota la evidencia de situación que entra
	// al espacio de trabajo.
	shadowDeliberationMaxEvidence = 6
)

// EnableShadowDeliberation turns on post-turn deliberation in the live process.
// It is opt-in so test suites that construct an Engine do not start background
// model work or write runtime state.
func (e *Engine) EnableShadowDeliberation() {
	if e == nil {
		return
	}
	if e.deliberationClient == nil && e.linguisticClient != nil {
		e.deliberationClient = e.linguisticClient
	}
	if e.deliberationShadow == nil {
		e.deliberationShadow = deliberation.NewShadowLog(deliberation.ShadowLogPath)
	} else {
		e.deliberationShadow.SetPath(deliberation.ShadowLogPath)
	}
	e.deliberationMu.Lock()
	e.deliberationEnabled = true
	e.deliberationMu.Unlock()
}

// LastShadowDeliberation returns a copy of the most recent record. Safe for
// diagnostics and tests.
func (e *Engine) LastShadowDeliberation() deliberation.Record {
	if e == nil {
		return deliberation.Record{}
	}
	e.deliberationMu.Lock()
	defer e.deliberationMu.Unlock()
	return e.lastDeliberation
}

// ShadowDeliberationCounts summarizes outcomes since process start.
func (e *Engine) ShadowDeliberationCounts() map[deliberation.Outcome]int {
	if e == nil || e.deliberationShadow == nil {
		return nil
	}
	return e.deliberationShadow.Counts()
}

// scheduleShadowDeliberation captures the turn's final state and launches one
// background job. At most one shadow job runs at a time: a slow model must not
// pile up work behind the user's conversation.
func (e *Engine) scheduleShadowDeliberation(t *processTurn) {
	if e == nil || t == nil {
		return
	}
	e.deliberationMu.Lock()
	enabled := e.deliberationEnabled
	running := e.deliberationRunning
	if enabled && !running && e.deliberationClient != nil && t.cognitionRan && e.lastBehavioralDecision.Version != "" {
		e.deliberationRunning = true
	} else {
		enabled = false
	}
	e.deliberationMu.Unlock()
	if !enabled {
		return
	}

	ws := e.buildDeliberationWorkspace(t)
	if ws.Empty() {
		e.deliberationMu.Lock()
		e.deliberationRunning = false
		e.deliberationMu.Unlock()
		return
	}
	harness := e.currentHarnessDecision()
	input := t.input
	turnID := fmt.Sprintf("%d", e.activeConversationTurnID)

	go func() {
		defer func() {
			e.deliberationMu.Lock()
			e.deliberationRunning = false
			e.deliberationMu.Unlock()
		}()
		ctx, cancel := context.WithTimeout(context.Background(), shadowDeliberationBudget)
		defer cancel()
		rec := e.deliberateShadow(ctx, ws, harness, input, turnID)
		if rec.Outcome != deliberation.OutcomeUnavailable {
			fmt.Printf("🧠 [deliberación sombra] %s\n", rec.Describe())
		}
	}()
}

// deliberateShadow performs one model call, validates the contribution against
// the workspace, compares it with the harness decision and records the result.
// It is synchronous so tests can exercise the whole path without goroutines.
func (e *Engine) deliberateShadow(ctx context.Context, ws *deliberation.Workspace, harness deliberation.HarnessDecision, input, turnID string) deliberation.Record {
	if e == nil || e.deliberationClient == nil {
		return deliberation.UnavailableRecord(harness, input, turnID, fmt.Errorf("deliberation client unavailable"))
	}
	raw, err := e.deliberationClient.GenerateShadowContext(ctx, deliberation.BuildPrompt(ws, harness), deliberation.Schema(), shadowDeliberationMaxTokens)
	if err != nil {
		return e.recordShadow(deliberation.UnavailableRecord(harness, input, turnID, err))
	}
	proposal, err := deliberation.ParseProposal(raw)
	if err != nil {
		return e.recordShadow(deliberation.InvalidRecord(harness, input, turnID, err))
	}
	proposal.Bind(ws)
	// A self-goal is only an articulation of a harness impulse. Without a
	// cited impulse, it is the model inventing a motive: drop it, keep the
	// critique and record why.
	if proposal.SelfGoal != nil && !citesImpulse(ws, proposal.SelfGoal.EvidenceIDs) {
		proposal.SelfGoalRejected = "la meta propia no cita un impulso del espacio de trabajo"
		proposal.SelfGoal = nil
	}
	rec := e.recordShadow(deliberation.Evaluate(harness, proposal, input, turnID))
	e.recordAgentGoal(rec, ws, turnID)
	// Paso 4.1: la expectativa del modelo queda como hipótesis separada,
	// source=model y checkable=false. No se compara con la observación ni
	// altera el veredicto del arnés.
	e.recordModelExpectation(rec, turnID)
	return rec
}

func (e *Engine) recordShadow(rec deliberation.Record) deliberation.Record {
	if e == nil {
		return rec
	}
	if e.deliberationShadow != nil {
		_ = e.deliberationShadow.Record(rec)
	}
	e.deliberationMu.Lock()
	e.lastDeliberation = rec
	e.deliberationMu.Unlock()
	return rec
}

// currentHarnessDecision projects the already-made decision into the
// deliberation contract. It is a read-only projection.
func (e *Engine) currentHarnessDecision() deliberation.HarnessDecision {
	if e == nil {
		return deliberation.HarnessDecision{}
	}
	d := e.lastBehavioralDecision
	return deliberation.HarnessDecision{
		Action:      string(d.Action),
		Strategy:    deliberation.Strategy(d.Strategy.Mode),
		Intent:      string(e.lastResponseIntent.Kind),
		Speak:       d.SpeakToCurrentTurn,
		Uncertainty: d.Uncertainty,
		Reason:      d.Reason,
		Evidence:    append([]string(nil), d.Evidence...),
	}
}

// buildDeliberationWorkspace assembles the bounded projection the model is
// allowed to read. It copies values; it never hands the model live pointers or
// mutable engine state.
func (e *Engine) buildDeliberationWorkspace(t *processTurn) *deliberation.Workspace {
	if e == nil || t == nil {
		return nil
	}
	ws := &deliberation.Workspace{}
	ws.Add("turno_actual", t.input, "interaction", 1)

	// Paso 3.4: la postura durable del propietario. Es contexto, no una
	// autorización para cambiarla.
	if state, ok := e.persistentModeState(); ok {
		ws.Add("modo_persistente", persistentModeSummary(state), "persistent_mode", 1)
	}
	if state, ok := e.OwnerStateActive(e.nowTime()); ok {
		ws.Add("estado_propietario", state.Kind, "owner_state", 1)
	}

	if text := describeSemanticRequest(t.semanticReq); text != "" {
		ws.Add("interpretacion", text, "interaction", t.semanticReq.Confidence)
	}

	// Impulses go before long situational evidence so the workspace cap
	// cannot silently drop the opening the model is allowed to answer.
	addImpulsesToWorkspace(ws, e.buildImpulses(t))

	// Paso 3.3: solo los gustos formados (MinN=5) entran como contexto. No
	// son órdenes ni evidencia canónica; la ausencia no se disimula.
	if stances := e.SelfTasteStances(); len(stances) > 0 {
		for i, s := range stances {
			if i >= 2 {
				break
			}
			text := fmt.Sprintf("%s: %s (media %.2f, evidencia %.0f", s.Key, s.Stance, s.Mean, s.EffN)
			if len(s.Evidence) > 0 {
				text += "; refs: " + strings.Join(s.Evidence, ", ")
			}
			text += ")"
			ws.Add("gusto_propio", text, "self_taste", s.Mean)
		}
	}

	if e.lastOpenMeaning != nil && time.Since(e.lastOpenMeaning.ObservedAt) <= 2*time.Minute {
		om := e.lastOpenMeaning.Meaning
		parts := nonEmptyStrings("purpose="+string(om.Purpose), "topic="+om.Topic, "meaning="+om.Meaning)
		if text := strings.Join(parts, " "); text != "" {
			ws.Add("significado_abierto", text, "linguistics", om.Confidence)
		}
	}

	if len(e.lastSituation.Evidence) > 0 {
		evidence := append([]situation.Evidence(nil), e.lastSituation.Evidence...)
		sort.SliceStable(evidence, func(i, j int) bool { return evidence[i].Confidence > evidence[j].Confidence })
		for _, ev := range evidence {
			if len(ws.Items) >= shadowDeliberationMaxEvidence+3 {
				break
			}
			text := strings.Join(nonEmptyStrings(ev.Subject, ev.Predicate, ev.Object, ev.Value), " ")
			if strings.TrimSpace(text) == "" {
				continue
			}
			ws.Add("situacion", text, firstNonEmpty(ev.Source, ev.Authority, "situation"), ev.Confidence)
		}
	}

	if t.interactionHasWork {
		w := t.interactionWorkContext
		text := strings.Join(nonEmptyStrings(
			"status="+w.Status,
			"objective="+w.Objective,
			"step="+w.CurrentStep,
			"last_result="+w.LastResult,
			fmt.Sprintf("verified=%t", w.LastActionVerified),
		), " ")
		ws.Add("trabajo_activo", text, "work", 1)
	}

	if t.hasPreviousTurn && strings.TrimSpace(t.previousTurn.UserText) != "" && t.previousTurn.UserText != t.input {
		ws.Add("turno_anterior", t.previousTurn.UserText, "conversation", 1)
	}

	if e.clarificationQueue != nil {
		if q := e.clarificationQueue.Oldest(); q != nil {
			text := strings.TrimSpace(q.Question)
			if goal := strings.TrimSpace(q.OriginalGoal); goal != "" {
				text = "objetivo: " + goal + "; pregunta: " + text
			}
			ws.Add("pregunta_pendiente", text, "clarification_queue", 1)
		}
	}

	return ws
}

func describeSemanticRequest(r interaction.SemanticRequest) string {
	parts := nonEmptyStrings(
		"kind="+string(r.Kind),
		"topic="+r.Topic,
		"speech_act="+string(r.SpeechAct),
	)
	if len(r.Meaning.Evidence) > 0 {
		parts = append(parts, "meaning="+strings.Join(r.Meaning.Evidence, " | "))
	} else if r.Meaning.Primary != "" {
		parts = append(parts, "meaning="+string(r.Meaning.Primary))
	}
	return strings.Join(parts, " ")
}
