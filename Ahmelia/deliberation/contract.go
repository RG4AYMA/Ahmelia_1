package deliberation

// contract.go — CONTRATO DE PROPUESTA TIPADA.
//
// El modelo no entrega prosa libre: entrega una propuesta con campos
// acotados. El arnés la valida antes de que exista como evidencia:
//
//   - la estrategia debe pertenecer al vocabulario cerrado;
//   - cada evidencia citada debe existir en el espacio de trabajo;
//   - la confianza se acota a [0,1];
//   - una propuesta sin fundamento no se "repara": se marca como no fundada.
//
// Una propuesta válida sigue sin tener autoridad. Es una contribución al
// razonamiento, no una orden de ejecución.

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"ahmelia/agency"
)

const ContractVersion = "B96.41.1-deliberation+agency"

// Strategy is the closed vocabulary of conversational postures the model may
// propose. It mirrors cognition.ConversationalResponseMode but is deliberately
// a separate type: deliberation cannot import cognition and claim authority
// over it.
type Strategy string

const (
	StrategyUnknown          Strategy = ""
	StrategyAnswer           Strategy = "answer"
	StrategyClarify          Strategy = "clarify"
	StrategyDiscuss          Strategy = "discuss"
	StrategyPropose          Strategy = "propose"
	StrategyCorrect          Strategy = "correct"
	StrategyDisagree         Strategy = "disagree"
	StrategyConfirm          Strategy = "confirm"
	StrategyRequestExecution Strategy = "request_execution"
	StrategyWait             Strategy = "wait"
	StrategyContinue         Strategy = "continue"
	StrategyChangeTopic      Strategy = "change_topic"
)

var validStrategies = map[Strategy]bool{
	StrategyAnswer:           true,
	StrategyClarify:          true,
	StrategyDiscuss:          true,
	StrategyPropose:          true,
	StrategyCorrect:          true,
	StrategyDisagree:         true,
	StrategyConfirm:          true,
	StrategyRequestExecution: true,
	StrategyWait:             true,
	StrategyContinue:         true,
	StrategyChangeTopic:      true,
}

// Proposal is the model's bounded contribution to the current turn.
type Proposal struct {
	Version string `json:"version"`

	// Reading is the model's one-sentence hypothesis of what is happening.
	Reading string `json:"reading"`
	// Strategy is the posture the model would take.
	Strategy Strategy `json:"strategy"`
	// Critique names what the harness decision may be missing. It is not an
	// instruction; it is a disagreement or refinement candidate.
	Critique string `json:"critique"`
	// Expectation is what should be observably true after the turn if the
	// reading is correct. It is the seed of step 4 (verification).
	Expectation string `json:"expectation"`

	EvidenceIDs []string `json:"evidence_ids,omitempty"`
	// RejectedEvidenceIDs records citations the harness refused. Kept for
	// diagnostics; they never count as grounding.
	RejectedEvidenceIDs []string `json:"rejected_evidence_ids,omitempty"`
	Grounded            bool     `json:"grounded"`

	// SelfGoal is an OPTIONAL proposal of a goal of Ahmelia's own. It is
	// allowed only when the harness placed an impulse in the workspace. The
	// harness validates it, records it in the agency ledger and waits for the
	// owner's decision; it never registers or executes the goal by itself.
	SelfGoal *agency.GoalProposal `json:"self_goal,omitempty"`
	// SelfGoalRejected explains why an offered self-goal was discarded. The
	// critique part of the proposal survives.
	SelfGoalRejected string `json:"self_goal_rejected,omitempty"`

	// ModeSuggestion is an OPTIONAL suggestion for the owner's persistent
	// mode. It is recorded as evidence and never applied: only the owner
	// changes the mode.
	ModeSuggestion agency.Mode `json:"mode_suggestion,omitempty"`
	// ModeSuggestionRejected explains why an offered suggestion was ignored.
	ModeSuggestionRejected string `json:"mode_suggestion_rejected,omitempty"`

	Confidence float64 `json:"confidence"`
	Abstain    bool    `json:"abstain"`
}

// HarnessDecision is the decision the harness has ALREADY made for this turn.
// Deliberation reads it; it cannot overwrite it.
type HarnessDecision struct {
	Action      string   `json:"action"`
	Strategy    Strategy `json:"strategy"`
	Intent      string   `json:"intent"`
	Speak       bool     `json:"speak"`
	Uncertainty bool     `json:"uncertainty"`
	Reason      string   `json:"reason,omitempty"`
	Evidence    []string `json:"evidence,omitempty"`
}

type proposalJSON struct {
	Reading     string               `json:"reading"`
	Strategy    string               `json:"strategy"`
	Critique    string               `json:"critique"`
	Expectation string               `json:"expectation"`
	EvidenceIDs []string             `json:"evidence_ids"`
	Confidence  float64              `json:"confidence"`
	Abstain     bool                 `json:"abstain"`
	SelfGoal    *agency.GoalProposal `json:"self_goal"`
	// ModeSuggestion stays a string on the wire so an unknown value is
	// recorded as rejected instead of silently becoming the zero mode.
	ModeSuggestion string `json:"mode_suggestion"`
}

// ParseProposal decodes the model's structured contribution. Malformed output
// is an error, not a silent empty proposal.
func ParseProposal(raw string) (Proposal, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Proposal{}, errors.New("deliberation: respuesta vacía")
	}
	// Tolerate fenced JSON from small models without inventing content.
	if strings.HasPrefix(raw, "```") {
		raw = strings.TrimPrefix(raw, "```json")
		raw = strings.TrimPrefix(raw, "```")
		raw = strings.TrimSuffix(raw, "```")
		raw = strings.TrimSpace(raw)
	}
	var wire proposalJSON
	if err := json.Unmarshal([]byte(raw), &wire); err != nil {
		return Proposal{}, fmt.Errorf("deliberation: JSON inválido: %w", err)
	}
	p := Proposal{
		Version:     ContractVersion,
		Reading:     strings.TrimSpace(wire.Reading),
		Strategy:    NormalizeStrategy(wire.Strategy),
		Critique:    strings.TrimSpace(wire.Critique),
		Expectation: strings.TrimSpace(wire.Expectation),
		EvidenceIDs: append([]string(nil), wire.EvidenceIDs...),
		Confidence:  clamp01(wire.Confidence),
		Abstain:     wire.Abstain,
	}
	if raw := strings.TrimSpace(wire.ModeSuggestion); raw != "" {
		p.ModeSuggestion = agency.NormalizeMode(raw)
		if p.ModeSuggestion == agency.ModeUnknown {
			p.ModeSuggestionRejected = fmt.Sprintf("deliberation: modo sugerido desconocido %q", raw)
		}
	}
	if wire.SelfGoal != nil {
		goal := *wire.SelfGoal
		if err := goal.Validate(); err != nil {
			// The critique is still usable; only the self-goal is discarded.
			p.SelfGoalRejected = err.Error()
		} else {
			p.SelfGoal = &goal
		}
	}
	if p.Reading == "" && p.Critique == "" && p.Expectation == "" && p.Strategy == StrategyUnknown && !p.Abstain {
		return Proposal{}, errors.New("deliberation: propuesta vacía")
	}
	return p, nil
}

// NormalizeStrategy accepts only the closed vocabulary. Anything else becomes
// Unknown; the arbiter then records a proposal without posture instead of
// guessing what the model meant.
func NormalizeStrategy(s string) Strategy {
	candidate := Strategy(strings.ToLower(strings.TrimSpace(s)))
	if validStrategies[candidate] {
		return candidate
	}
	return StrategyUnknown
}

// Bind validates the proposal against the workspace. It filters citations,
// records rejected ones, and sets Grounded. The proposal is never repaired:
// text stays as the model wrote it, confidence is only clamped, and unknown
// strategies stay unknown.
func (p *Proposal) Bind(ws *Workspace) {
	if p == nil {
		return
	}
	p.Version = ContractVersion
	p.Confidence = clamp01(p.Confidence)
	p.RejectedEvidenceIDs = nil
	seen := make(map[string]bool, len(p.EvidenceIDs))
	valid := make([]string, 0, len(p.EvidenceIDs))
	for _, id := range p.EvidenceIDs {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		if ws.HasID(id) {
			valid = append(valid, id)
		} else {
			p.RejectedEvidenceIDs = append(p.RejectedEvidenceIDs, id)
		}
	}
	p.EvidenceIDs = valid
	p.Grounded = len(valid) > 0

	if p.SelfGoal != nil {
		if !p.SelfGoal.Bind(ws.HasID) {
			p.SelfGoalRejected = "la meta propia no cita evidencia válida del espacio de trabajo"
			p.SelfGoal = nil
		}
	}
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
