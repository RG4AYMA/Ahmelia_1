package core

// model_consultation.go — EL MODELO INTERPRETA, EL ARNÉS VALIDA.
//
// Esta es la primera participación del modelo en el motor real fuera del turno.
// No se llama en la respuesta: se agenda al final y corre en segundo plano con
// el presupuesto más bajo del gate. El modelo solo puede elegir entre:
//
//   - admit_work_restriction: el arnés admite una restricción PROVISIONAL de
//     iniciativa de trabajo hasta fin del día, con fuente confirmed_inference;
//   - no_change: no se toca nada;
//   - open_work: candidato no autorizado, solo para probar la puerta.
//
// El arnés revalida la frase, la confianza, la evidencia y la ausencia de una
// restricción previa. Si el modelo falla, tarda o alucina, no se admite nada.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"ahmelia/consult"
	"ahmelia/restriction"
)

const (
	consultCandidateAdmitRest = "admit_work_restriction"
	consultCandidateNoChange  = "no_change"
	consultCandidateOpenWork  = "open_work"

	modelConsultationBudget        = 180 * time.Second
	modelConsultationMinConfidence = 0.80
)

// EnableModelConsultation turns on the background interpretation of rest
// phrases that the deterministic parser did not consume. It reuses the
// linguistic client, so it does not create a second model boundary.
func (e *Engine) EnableModelConsultation() {
	if e == nil {
		return
	}
	e.deliberationMu.Lock()
	defer e.deliberationMu.Unlock()
	if e.consultClient == nil {
		if e.deliberationClient != nil {
			e.consultClient = e.deliberationClient
		} else if e.linguisticClient != nil {
			e.consultClient = e.linguisticClient
		}
	}
	if e.consultClient != nil && e.consultWorker == nil {
		e.consultWorker = consult.NewWorker(e.consultClient, 2*time.Minute)
	}
	if e.linguisticClient != nil {
		e.planMu.Lock()
		if e.planClient == nil {
			e.planClient = e.linguisticClient
		}
		e.planMu.Unlock()
	}
	e.modelConsultationOn = e.consultClient != nil
}

func (e *Engine) LastModelConsultation() (consult.Decision, string) {
	if e == nil {
		return consult.Decision{}, ""
	}
	e.deliberationMu.Lock()
	defer e.deliberationMu.Unlock()
	return e.lastModelConsultation, e.lastModelConsultationErr
}

// scheduleModelConsultation captures the turn and launches at most one
// background consultation. The turn has already been answered; this cannot
// delay or change it.
func (e *Engine) scheduleModelConsultation(t *processTurn) {
	if e == nil || t == nil {
		return
	}
	// Every turn invalidates an in-flight proposal: a reading of turn N must
	// not be applied after the owner has moved to turn N+1. The sequence is
	// owned by this worker, so it does not read the conversation turn ID from
	// another goroutine.
	e.deliberationMu.Lock()
	e.consultSeq++
	turnID := fmt.Sprintf("consult-%d", e.consultSeq)
	e.consultTurnID = turnID
	running := e.modelConsultationRunning
	on := e.modelConsultationOn
	client := e.consultClient
	e.deliberationMu.Unlock()

	if t.ownerRestrictionHandled || !on || client == nil || running {
		return
	}
	if !looksLikeRestDeclaration(t.input) {
		return
	}
	now := e.nowTime()
	if e.OwnerWorkInitiativeRestricted(now) {
		return
	}

	e.deliberationMu.Lock()
	if e.modelConsultationRunning {
		e.deliberationMu.Unlock()
		return
	}
	e.modelConsultationRunning = true
	e.deliberationMu.Unlock()

	input := t.input
	go func() {
		defer func() {
			e.deliberationMu.Lock()
			e.modelConsultationRunning = false
			e.deliberationMu.Unlock()
		}()
		ctx, cancel := context.WithTimeout(context.Background(), modelConsultationBudget)
		defer cancel()

		decision, err := e.consultRestTurn(ctx, input, turnID)
		e.deliberationMu.Lock()
		e.lastModelConsultation = decision
		if err != nil {
			e.lastModelConsultationErr = err.Error()
		} else {
			e.lastModelConsultationErr = ""
		}
		e.deliberationMu.Unlock()

		if !e.modelConsultationCurrent(turnID) || !decision.Accepted {
			return
		}
		if _, applied, applyErr := e.applyModelConsultationDecision(input, turnID, decision, e.nowTime()); applyErr != nil {
			e.deliberationMu.Lock()
			e.lastModelConsultationErr = applyErr.Error()
			e.deliberationMu.Unlock()
		} else if applied {
			e.deliberationMu.Lock()
			e.lastOwnerRestrictionNote = "restricción provisional admitida por lectura del modelo"
			e.deliberationMu.Unlock()
		}
	}()
}

// modelConsultationCurrent reports whether the proposal still belongs to the
// latest turn. A newer turn invalidates it even if the model returns later.
func (e *Engine) modelConsultationCurrent(turnID string) bool {
	if e == nil {
		return false
	}
	e.deliberationMu.Lock()
	defer e.deliberationMu.Unlock()
	return e.consultTurnID == turnID
}

// consultRestTurn performs the bounded model call and the harness verdict. It
// is synchronous so tests can exercise the whole path without goroutines.
func (e *Engine) consultRestTurn(ctx context.Context, input, turnID string) (consult.Decision, error) {
	e.deliberationMu.Lock()
	client := e.consultClient
	on := e.modelConsultationOn
	e.deliberationMu.Unlock()
	if !on || client == nil {
		return consult.Decision{Source: consult.SourceNone}, consult.ErrUnavailable
	}

	candidates := []consult.Candidate{
		{ID: consultCandidateAdmitRest, Kind: "restriction", Description: "Aplicar una restricción provisional de iniciativa de trabajo hasta fin del día", Authorized: true, Risk: "low", Reversible: true},
		{ID: consultCandidateNoChange, Kind: "silence", Description: "No cambiar nada", Authorized: true, Risk: "low", Reversible: true},
		{ID: consultCandidateOpenWork, Kind: "work", Description: "Reabrir la iniciativa de trabajo", Authorized: false, Risk: "high", Reversible: false},
	}
	evidence := []consult.Evidence{
		{ID: "turn", Text: strings.TrimSpace(input)},
		{ID: "parser", Text: "el parser explícito no reconoció una orden de descanso o reapertura"},
	}
	if state, ok := e.persistentModeState(); ok {
		evidence = append(evidence, consult.Evidence{ID: "mode", Text: persistentModeSummary(state)})
	}

	req := consult.Request{
		Scene:          "scene1_rest_interpretation",
		Input:          strings.TrimSpace(input),
		HarnessReading: "frase candidata a restricción de trabajo; el parser determinista no la consumió",
		OwnerState:     "unknown",
		Candidates:     candidates,
		Evidence:       evidence,
	}
	proposal, err := consult.Consult(ctx, client, req)
	decision := consult.Decide(
		proposal,
		candidates,
		func(c consult.Candidate) (bool, string) {
			switch c.ID {
			case consultCandidateAdmitRest:
				if !looksLikeRestDeclaration(input) {
					return false, "not_a_rest_declaration"
				}
				if proposal.Choice.Confidence < modelConsultationMinConfidence {
					return false, "confidence_below_restriction_threshold"
				}
				if e.OwnerWorkInitiativeRestricted(e.nowTime()) {
					return false, "already_restricted"
				}
				return true, ""
			case consultCandidateNoChange:
				return true, ""
			default:
				return false, "not_authorized:" + c.ID
			}
		},
		consultCandidateNoChange,
	)
	return decision, err
}

// applyModelConsultationDecision is the final gate. It only admits a
// provisional restriction when the model was accepted for that exact
// candidate, the phrase still looks like a rest declaration and no restriction
// is already active.
func (e *Engine) applyModelConsultationDecision(statement, turnID string, d consult.Decision, now time.Time) (restriction.OwnerRestriction, bool, error) {
	if !d.Accepted || d.Candidate.ID != consultCandidateAdmitRest {
		return restriction.OwnerRestriction{}, false, nil
	}
	if !looksLikeRestDeclaration(statement) {
		return restriction.OwnerRestriction{}, false, nil
	}
	if e.OwnerWorkInitiativeRestricted(now) {
		return restriction.OwnerRestriction{}, false, nil
	}
	r, err := e.SetProvisionalWorkInitiativeRestriction(statement, turnID, now, []string{
		"model:consult",
		"turn:" + turnID,
		"candidate:" + d.Candidate.ID,
		fmt.Sprintf("confidence:%.2f", d.Proposal.Choice.Confidence),
	})
	return r, err == nil, err
}

// looksLikeRestDeclaration is a deterministic candidate filter, not a semantic
// authority. It keeps the model away from phrases such as "hoy trabajo en el
// jardín", which contain "trabajo" but are not a rest declaration.
func looksLikeRestDeclaration(input string) bool {
	s := normalizeOwnerRestrictionPhrase(input)
	if !strings.Contains(s, "hoy") && !strings.Contains(s, "dia") && !strings.Contains(s, "tomamos") {
		return false
	}
	switch {
	case strings.Contains(s, "no trabaj"),
		strings.Contains(s, "sin trabaj"),
		strings.Contains(s, "no hay trabaj"):
		return true
	case strings.Contains(s, "descans"),
		strings.Contains(s, "dia libre"),
		strings.Contains(s, "nos tomamos"):
		return true
	}
	return false
}

// initiativeConsultantAdapter exposes the consult worker to the autonomous
// loop. Submit never blocks; the loop consumes only a fresh proposal.
type initiativeConsultantAdapter struct {
	worker *consult.Worker
}

func (a initiativeConsultantAdapter) ProposeInitiative(req consult.Request, fingerprint string) bool {
	if a.worker == nil {
		return false
	}
	return a.worker.Submit(context.Background(), req, fingerprint)
}

func (a initiativeConsultantAdapter) FreshInitiative(fingerprint string) (consult.Proposal, bool) {
	if a.worker == nil {
		return consult.Proposal{}, false
	}
	return a.worker.Fresh(fingerprint)
}
