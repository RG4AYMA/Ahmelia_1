package dialogue

import (
	"strings"
	"testing"
	"time"
)

func testHistoryNow() time.Time {
	return time.Date(2026, 9, 5, 11, 0, 0, -5*60*60, time.FixedZone("-05", -5*60*60))
}

func TestB9635_HistoricalGateBehaviorMatrix(t *testing.T) {
	now := testHistoryNow()
	h := NewConversationHistory()
	id := h.BeginSession("old", now.Add(-48*time.Hour))
	h.RecordTurn(id, HistoricalTurn{ID: 1, At: now.Add(-48 * time.Hour), UserText: "Decidimos usar Aurora principalmente para supervisar el estado del sistema.", Assistant: "Sí, esa fue la decisión.", Topic: "supervisión del sistema", Objective: "supervisar el estado del sistema"})
	h.RecordTurn(id, HistoricalTurn{ID: 2, At: now.Add(-47 * time.Hour), UserText: "Tuvimos un problema porque el estado no se actualizaba.", Assistant: "El estado quedó desactualizado.", Topic: "supervisión del sistema", Relation: "problem"})
	h.RecordTurn(id, HistoricalTurn{ID: 3, At: now.Add(-46 * time.Hour), UserText: "Lo solucionamos cambiando el mecanismo de actualización.", Assistant: "Quedó resuelto.", Topic: "supervisión del sistema", Relation: "solution"})
	h.RecordTurn(id, HistoricalTurn{ID: 4, At: now.Add(-45 * time.Hour), UserText: "Después acordamos mantener ese mecanismo.", Assistant: "Correcto.", Topic: "supervisión del sistema", Relation: "decision"})
	h.RecordTurn(id, HistoricalTurn{ID: 5, At: now.Add(-44 * time.Hour), UserText: "Todavía falta implementar la selección semántica de evidencia.", Assistant: "Ese trabajo sigue pendiente.", Topic: "memoria histórica", Relation: "pending"})
	h.RecordTurn(id, HistoricalTurn{ID: 6, At: now.Add(-43 * time.Hour), UserText: "Prefiero respuestas históricas resumidas por defecto y detalle solo cuando lo pida.", Assistant: "Entendido.", Topic: "respuesta histórica", Relation: "preference"})
	h.RecordTurn(id, HistoricalTurn{ID: 7, At: now.Add(-42 * time.Hour), UserText: "Nuestro objetivo era separar el almacenamiento histórico del estado actual.", Assistant: "Queda como objetivo.", Topic: "memoria histórica", Relation: "objective"})
	h.CompleteSession(id, "historial de memoria", now.Add(-41*time.Hour))

	cases := []struct{ name, query, want string }{
		{"decision", "¿Qué habíamos decidido hacer con Aurora?", "Decidimos usar Aurora principalmente"},
		{"problem", "¿Qué problema tuvimos con la supervisión?", "Tuvimos un problema"},
		{"solution", "¿Cómo lo solucionamos?", "Lo solucionamos"},
		{"pending", "¿Qué nos había quedado pendiente de la memoria histórica?", "falta implementar"},
		{"preference", "¿Cómo habíamos decidido que debía responder Ahmelia?", "Prefiero respuestas históricas resumidas"},
		{"objective", "¿Cuál era el objetivo de la memoria?", "Nuestro objetivo era separar"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := ResolveHistoricalRecall(h, tc.query, now)
			if !r.Found {
				t.Fatalf("no evidence for %q", tc.query)
			}
			if !strings.Contains(r.Response, tc.want) {
				t.Fatalf("missing %q in %q", tc.want, r.Response)
			}
			if strings.Contains(r.Response, "Recuerdo conversaciones anteriores") {
				t.Fatalf("fallback contaminated result: %q", r.Response)
			}
		})
	}
}

func TestB9635_PrimaryEvidenceOutranksRetrievalResponses(t *testing.T) {
	now := testHistoryNow()
	h := NewConversationHistory()
	id := h.BeginSession("old", now.Add(-time.Hour))
	h.RecordTurn(id, HistoricalTurn{ID: 1, At: now.Add(-time.Hour), UserText: "Decidimos usar Aurora para supervisar el sistema.", Assistant: "Te escucho. Dime qué tienes en mente y lo vemos juntos.", Topic: "Aurora"})
	h.RecordTurn(id, HistoricalTurn{ID: 2, At: now.Add(-30 * time.Minute), UserText: "¿Qué habíamos decidido hacer con Aurora?", Assistant: "Recuerdo conversaciones anteriores relacionadas con eso, pero no tengo suficiente detalle para resumirlas con fiabilidad.", Topic: "Aurora"})
	h.CompleteSession(id, "sesión", now.Add(-20*time.Minute))
	r := ResolveHistoricalRecall(h, "¿Qué habíamos decidido hacer con Aurora?", now)
	if !r.Found || !strings.Contains(r.Response, "Decidimos usar Aurora") {
		t.Fatalf("primary evidence lost: %q", r.Response)
	}
	if strings.Contains(r.Response, "no tengo suficiente detalle") {
		t.Fatalf("retrieval meta leaked into answer: %q", r.Response)
	}
}

func TestB9635_CurrentEpisodeExcluded(t *testing.T) {
	now := testHistoryNow()
	h := NewConversationHistory()
	current := h.BeginSession("current", now.Add(-time.Minute))
	old := h.BeginSession("old", now.Add(-time.Hour))
	h.RecordTurn(current, HistoricalTurn{ID: 1, At: now.Add(-time.Minute), UserText: "¿Qué habíamos decidido sobre Aurora?", Topic: "Aurora"})
	h.RecordTurn(old, HistoricalTurn{ID: 2, At: now.Add(-time.Hour), UserText: "Decidimos usar Aurora como supervisor.", Topic: "Aurora"})
	r := ResolveHistoricalRecallExcluding(h, "¿Qué habíamos decidido sobre Aurora?", now, current)
	if !r.Found || !strings.Contains(r.Response, "Decidimos usar Aurora") {
		t.Fatalf("old evidence missing: %q", r.Response)
	}
	if strings.Contains(r.Response, "¿Qué habíamos decidido") {
		t.Fatalf("current query leaked: %q", r.Response)
	}
}

func TestB9635_DetailedModeUsesSameEvidenceModelAndTimeline(t *testing.T) {
	now := testHistoryNow()
	h := NewConversationHistory()
	id := h.BeginSession("old", now.Add(-3*time.Hour))
	h.RecordTurn(id, HistoricalTurn{ID: 1, At: now.Add(-3 * time.Hour), UserText: "Tuvimos un problema con la supervisión.", Topic: "supervisión", Relation: "problem"})
	h.RecordTurn(id, HistoricalTurn{ID: 2, At: now.Add(-2 * time.Hour), UserText: "Lo solucionamos cambiando el mecanismo de actualización.", Topic: "supervisión", Relation: "solution"})
	h.RecordTurn(id, HistoricalTurn{ID: 3, At: now.Add(-time.Hour), UserText: "Después acordamos mantener ese mecanismo.", Topic: "supervisión", Relation: "decision"})
	h.CompleteSession(id, "historial", now.Add(-50*time.Minute))
	r := ResolveHistoricalRecall(h, "Muéstrame todo el historial relacionado con la supervisión", now)
	for _, want := range []string{"Tuvimos un problema", "Lo solucionamos", "Después acordamos", "UTC-05:00"} {
		if !strings.Contains(r.Response, want) {
			t.Fatalf("missing %q: %s", want, r.Response)
		}
	}
}

func TestB9635_DetailedModeDoesNotRequireTopicKeywordOnEveryTurn(t *testing.T) {
	now := testHistoryNow()
	h := NewConversationHistory()
	id := h.BeginSession("old", now.Add(-3*time.Hour))
	h.RecordTurn(id, HistoricalTurn{ID: 1, At: now.Add(-3 * time.Hour), UserText: "Tuvimos un problema con Aurora.", Topic: "Aurora", Relation: "problem"})
	h.RecordTurn(id, HistoricalTurn{ID: 2, At: now.Add(-2 * time.Hour), UserText: "Lo solucionamos cambiando el mecanismo.", Topic: "Aurora", Relation: "solution"})
	h.RecordTurn(id, HistoricalTurn{ID: 3, At: now.Add(-time.Hour), UserText: "Finalmente optamos por mantenerlo.", Topic: "Aurora", Relation: "decision"})
	h.CompleteSession(id, "historial", now.Add(-50*time.Minute))
	r := ResolveHistoricalRecall(h, "Muéstrame todo el historial relacionado con Aurora", now)
	for _, want := range []string{"Tuvimos un problema", "Lo solucionamos", "Finalmente optamos"} {
		if !strings.Contains(r.Response, want) {
			t.Fatalf("missing %q: %s", want, r.Response)
		}
	}
}

func TestB9635_NoEvidenceIsNotFabricated(t *testing.T) {
	now := testHistoryNow()
	h := NewConversationHistory()
	id := h.BeginSession("old", now.Add(-time.Hour))
	h.RecordTurn(id, HistoricalTurn{ID: 1, At: now.Add(-time.Hour), UserText: "Hablamos del clima.", Topic: "clima"})
	h.CompleteSession(id, "clima", now.Add(-50*time.Minute))
	r := ResolveHistoricalRecall(h, "¿Qué habíamos decidido sobre el proyecto que nunca mencionamos?", now)
	if r.Found || r.Response != "" {
		t.Fatalf("fabricated evidence: %+v", r)
	}
}

func TestB9635_HistoricalQuestionIsNotEvidence(t *testing.T) {
	now := testHistoryNow()
	h := NewConversationHistory()
	id := h.BeginSession("old", now.Add(-time.Hour))
	h.RecordTurn(id, HistoricalTurn{ID: 1, At: now.Add(-time.Hour), UserText: "¿Qué habíamos decidido hacer con Aurora?", Assistant: "Decidimos usar Aurora para supervisar el sistema.", Topic: "Aurora"})
	h.CompleteSession(id, "historial", now.Add(-50*time.Minute))
	r := ResolveHistoricalRecall(h, "¿Qué habíamos decidido hacer con Aurora?", now)
	if strings.Contains(r.Response, "¿Qué habíamos decidido") {
		t.Fatalf("question surfaced as evidence: %q", r.Response)
	}
	if !strings.Contains(r.Response, "Decidimos usar Aurora") {
		t.Fatalf("answer evidence missing: %q", r.Response)
	}
}

func TestB9635_ResponseGranularity(t *testing.T) {
	now := testHistoryNow()
	h := NewConversationHistory()
	id := h.BeginSession("old", now.Add(-time.Hour))
	h.RecordTurn(id, HistoricalTurn{ID: 1, At: now.Add(-time.Hour), UserText: "Decidimos usar Aurora para supervisar el sistema.", Topic: "Aurora", Relation: "decision"})
	h.CompleteSession(id, "historial", now.Add(-50*time.Minute))
	summary := ResolveHistoricalRecall(h, "¿Qué habíamos decidido sobre Aurora?", now)
	detail := ResolveHistoricalRecall(h, "Muéstrame todo el historial relacionado con Aurora", now)
	if strings.Contains(summary.Response, "Tú:") || strings.Contains(summary.Response, "Ahmelia:") {
		t.Fatalf("summary dumped timeline: %q", summary.Response)
	}
	if !strings.Contains(detail.Response, "Tú:") {
		t.Fatalf("detail did not expose timeline: %q", detail.Response)
	}
}

func TestB9635_IntentClassification(t *testing.T) {
	cases := map[string]HistoricalIntent{
		"¿Qué habíamos decidido sobre X?":         HistoricalIntentDecision,
		"¿Qué problema tuvimos con X?":            HistoricalIntentProblem,
		"¿Cómo lo solucionamos?":                  HistoricalIntentSolution,
		"¿Qué quedó pendiente?":                   HistoricalIntentPending,
		"¿Qué preferíamos?":                       HistoricalIntentPreference,
		"¿Cuál era el objetivo?":                  HistoricalIntentObjective,
		"¿Por qué cambiamos la decisión sobre X?": HistoricalIntentDecisionEvolution,
		"Muéstrame todo el historial de X":        HistoricalIntentTimeline,
	}
	for q, want := range cases {
		if got := InferHistoricalIntent(q); got != want {
			t.Fatalf("%q: got %s want %s", q, got, want)
		}
	}
}
