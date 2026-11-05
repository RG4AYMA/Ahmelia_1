package integration

import (
	"encoding/json"
	"strings"
	"testing"

	"ahmelia/programming"
	"ahmelia/tools"
)

func TestCodeSynthesisPromptCarriesPriorAttemptCausalContext(t *testing.T) {
	ctx := &tools.RecoveryContext{
		AttemptNumber: 2,
		Objective:     "crear un programa Go llamado f10_repair",
		FailureReason: "package f10_repair is not in std",
		PriorAttempt: &tools.RecoveryAttempt{
			Number:              1,
			EffectSpecification: []byte(`{"files":[{"path":"f10_repair.go","content":"package main"}]}`),
			EffectEvaluations:   []json.RawMessage{json.RawMessage(`{"judgment":"incompatible","path":"f10_repair_test.go"}`)},
		},
	}
	objective := programming.CodeObjective{
		Raw:             "crear un programa Go llamado f10_repair",
		RecoveryContext: ctx,
		Language:        "Go",
		ProjectID:       "f10_repair",
		ProjectRoot:     t.TempDir(),
	}

	prompt := buildCodeSynthesisPrompt(objective)
	for _, want := range []string{
		`"prior_attempt"`,
		`"effect_specification"`,
		`"effect_evaluations"`,
		`package f10_repair is not in std`,
		"Do not reason as if this were a fresh objective.",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt lost causal recovery context %q", want)
		}
	}
}

func TestCloneRecoveryContextPreservesPriorAttempt(t *testing.T) {
	original := &tools.RecoveryContext{
		PriorAttempt: &tools.RecoveryAttempt{
			Number:              1,
			EffectSpecification: []byte(`{"change":"A"}`),
			EffectEvaluations:   []json.RawMessage{json.RawMessage(`{"judgment":"compatible"}`)},
		},
	}
	clone := original.Clone()
	if clone == nil || clone.PriorAttempt == nil {
		t.Fatal("clone lost prior attempt")
	}
	clone.PriorAttempt.EffectSpecification[0] = '{'
	clone.PriorAttempt.EffectEvaluations[0][0] = '{'
	if string(original.PriorAttempt.EffectSpecification) != `{"change":"A"}` {
		t.Fatal("effect specification was not deep-cloned")
	}
	if string(original.PriorAttempt.EffectEvaluations[0]) != `{"judgment":"compatible"}` {
		t.Fatal("effect evaluation was not deep-cloned")
	}
}
