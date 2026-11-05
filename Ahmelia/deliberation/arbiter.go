package deliberation

// arbiter.go — ÁRBITRO EN MODO SOMBRA.
//
// En modo sombra el árbitro NO decide. Compara la propuesta del modelo contra
// la decisión que el arnés ya tomó y registra la relación entre ambas:
//
//   agree       misma estrategia, sin objeción
//   refine      misma estrategia, con crítica explícita
//   disagree    estrategia distinta y fundada
//   observation contribución fundada sin postura estratégica
//   abstain     el modelo declara que no puede juzgar
//   ungrounded  propuesta sin evidencia válida del workspace
//   invalid     salida ilegible o contrato roto
//
// La decisión del arnés permanece intacta. El registro es evidencia para
// evaluar al modelo, no una orden para cambiar la conducta.

import (
	"fmt"
	"strings"
	"time"
)

type Outcome string

const (
	OutcomeAgree       Outcome = "agree"
	OutcomeRefine      Outcome = "refine"
	OutcomeDisagree    Outcome = "disagree"
	OutcomeObservation Outcome = "observation"
	OutcomeAbstain     Outcome = "abstain"
	OutcomeUngrounded  Outcome = "ungrounded"
	OutcomeInvalid     Outcome = "invalid"
	// OutcomeUnavailable means the model could not be reached or did not
	// answer within the shadow budget. It is absence of evidence, not a
	// disagreement.
	OutcomeUnavailable Outcome = "unavailable"
)

// Record is one shadow deliberation. It is append-only evidence.
type Record struct {
	At       time.Time       `json:"at"`
	TurnID   string          `json:"turn_id,omitempty"`
	Input    string          `json:"input,omitempty"`
	Contract string          `json:"contract"`
	Harness  HarnessDecision `json:"harness"`
	Proposal Proposal        `json:"proposal"`
	Outcome  Outcome         `json:"outcome"`
	Reasons  []string        `json:"reasons,omitempty"`
	Error    string          `json:"error,omitempty"`
}

// InvalidRecord builds a record for output the harness could not parse. It
// keeps the failure visible instead of turning it into silence.
func InvalidRecord(harness HarnessDecision, input string, turnID string, err error) Record {
	r := Record{
		At:       time.Now(),
		TurnID:   turnID,
		Input:    input,
		Contract: ContractVersion,
		Harness:  harness,
		Outcome:  OutcomeInvalid,
		Error:    err.Error(),
	}
	r.Reasons = append(r.Reasons, "model output ignored: contract not satisfied")
	return r
}

// UnavailableRecord builds a record for a model that could not answer. It is
// not a failed proposal: there was no proposal at all.
func UnavailableRecord(harness HarnessDecision, input string, turnID string, err error) Record {
	r := Record{
		At:       time.Now(),
		TurnID:   turnID,
		Input:    input,
		Contract: ContractVersion,
		Harness:  harness,
		Outcome:  OutcomeUnavailable,
		Error:    err.Error(),
	}
	r.Reasons = append(r.Reasons, "shadow deliberation did not run")
	return r
}

// Evaluate compares a bound proposal with the harness decision. The harness
// decision is authoritative and is returned unchanged in the record.
func Evaluate(harness HarnessDecision, p Proposal, input, turnID string) Record {
	r := Record{
		At:       time.Now(),
		TurnID:   turnID,
		Input:    input,
		Contract: ContractVersion,
		Harness:  harness,
		Proposal: p,
	}
	switch {
	case p.Abstain:
		r.Outcome = OutcomeAbstain
		r.Reasons = append(r.Reasons, "model declared abstention")
	case !p.Grounded:
		r.Outcome = OutcomeUngrounded
		r.Reasons = append(r.Reasons, "no cited evidence belongs to the workspace")
	case p.Strategy == StrategyUnknown:
		r.Outcome = OutcomeObservation
		r.Reasons = append(r.Reasons, "grounded contribution without strategic posture")
	case p.Strategy == harness.Strategy:
		if strings.TrimSpace(p.Critique) != "" {
			r.Outcome = OutcomeRefine
			r.Reasons = append(r.Reasons, "same posture with an explicit critique")
		} else {
			r.Outcome = OutcomeAgree
			r.Reasons = append(r.Reasons, "same posture, no objection")
		}
	default:
		r.Outcome = OutcomeDisagree
		r.Reasons = append(r.Reasons, fmt.Sprintf("model proposed %q over harness %q", p.Strategy, harness.Strategy))
	}
	if len(p.RejectedEvidenceIDs) > 0 {
		r.Reasons = append(r.Reasons, "rejected citations: "+strings.Join(p.RejectedEvidenceIDs, ","))
	}
	if p.ModeSuggestion != "" {
		r.Reasons = append(r.Reasons, "mode suggestion recorded (owner decides): "+string(p.ModeSuggestion))
	}
	if p.ModeSuggestionRejected != "" {
		r.Reasons = append(r.Reasons, p.ModeSuggestionRejected)
	}
	return r
}
