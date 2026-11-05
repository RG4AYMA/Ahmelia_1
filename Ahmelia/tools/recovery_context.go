package tools

import "encoding/json"

// RecoveryEvidence is domain-neutral causal evidence preserved across a
// recovery boundary. Core/Production does not interpret Payload; the provider
// that owns the domain defines its schema. This keeps the common contract
// generic without flattening the evidence to a diagnostic string.
type RecoveryEvidence struct {
	Kind    string          `json:"kind"`
	Source  string          `json:"source,omitempty"`
	Stage   string          `json:"stage,omitempty"`
	Summary string          `json:"summary,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// RecoveryContext is the structured causal context handed to the next
// strategy/synthesis attempt. It is evidence, not a recovery decision.
// Retry/Replan/Block/AskUser remain owned by the existing recovery authority.
type RecoveryAttempt struct {
	Number              int               `json:"number"`
	EffectSpecification json.RawMessage   `json:"effect_specification,omitempty"`
	EffectEvaluations   []json.RawMessage `json:"effect_evaluations,omitempty"`
}

type RecoveryContext struct {
	AttemptNumber int                `json:"attempt_number"`
	Objective     string             `json:"objective,omitempty"`
	ExpectedState []string           `json:"expected_state,omitempty"`
	ObservedState []string           `json:"observed_state,omitempty"`
	FailureReason string             `json:"failure_reason,omitempty"`
	Evidence      []RecoveryEvidence `json:"evidence,omitempty"`
	PriorAttempt  *RecoveryAttempt   `json:"prior_attempt,omitempty"`
}

func (c *RecoveryContext) Clone() *RecoveryContext {
	if c == nil {
		return nil
	}
	out := *c
	out.ExpectedState = append([]string(nil), c.ExpectedState...)
	out.ObservedState = append([]string(nil), c.ObservedState...)
	if c.PriorAttempt != nil {
		out.PriorAttempt = &RecoveryAttempt{
			Number:              c.PriorAttempt.Number,
			EffectSpecification: append(json.RawMessage(nil), c.PriorAttempt.EffectSpecification...),
		}
		if len(c.PriorAttempt.EffectEvaluations) > 0 {
			out.PriorAttempt.EffectEvaluations = make([]json.RawMessage, len(c.PriorAttempt.EffectEvaluations))
			for i, evaluation := range c.PriorAttempt.EffectEvaluations {
				out.PriorAttempt.EffectEvaluations[i] = append(json.RawMessage(nil), evaluation...)
			}
		}
	}
	if len(c.Evidence) > 0 {
		out.Evidence = make([]RecoveryEvidence, len(c.Evidence))
		for i, e := range c.Evidence {
			out.Evidence[i] = e
			if e.Payload != nil {
				out.Evidence[i].Payload = append(json.RawMessage(nil), e.Payload...)
			}
		}
	}
	return &out
}
