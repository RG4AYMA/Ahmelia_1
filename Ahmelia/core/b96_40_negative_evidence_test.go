package core

import (
	"strings"
	"testing"
)

func TestB9640NegativeUserModelEvidenceIsLowConfidence(t *testing.T) {
	e := newB963334AuditEngine(t)
	var responses []string
	e.SetResponseCallback(func(msg string) { responses = append(responses, msg) })
	queries := []string{
		"¿Cuál es mi objetivo de prueba?",
		"¿Qué nivel tengo en una capacidad inexistente?",
	}
	for _, q := range queries {
		responses = nil
		e.Process(q)
		if len(responses) == 0 {
			continue
		}
		// The exact text is provider/domain-dependent. The observable invariant is
		// that an unavailable User Model fact must not become high-confidence
		// deterministic evidence. This is checked by the response contract trace.
		for _, ev := range e.lastDecisionEvidence.Evaluation.RelevantFindings {
			if strings.Contains(strings.ToLower(ev.Target+" "+ev.Subject), "user") && ev.Confidence >= 0.99 {
				t.Fatalf("negative user-model evidence leaked as high confidence: %+v", ev)
			}
		}
	}
}
