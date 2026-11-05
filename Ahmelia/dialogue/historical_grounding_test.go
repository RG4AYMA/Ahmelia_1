package dialogue

import (
	"strings"
	"testing"
	"time"
)

func TestB9635ConcreteSubjectCannotRetrieveUnrelatedEpisode(t *testing.T) {
	h := NewConversationHistory()
	now := time.Now()
	unrelated := h.BeginSession("unrelated", now.Add(-2*time.Hour))
	h.RecordTurn(unrelated, HistoricalTurn{
		ID: 1, At: now.Add(-2 * time.Hour),
		UserText: "Nuestro objetivo era separar el almacenamiento histórico del estado actual.",
		Topic:    "memoria histórica", Relation: "objective",
	})
	related := h.BeginSession("aurora", now.Add(-time.Hour))
	h.RecordTurn(related, HistoricalTurn{
		ID: 1, At: now.Add(-time.Hour),
		UserText: "Decidimos usar Aurora principalmente para supervisar el estado del sistema.",
		Topic:    "Aurora", Relation: "decision",
	})

	r := ResolveHistoricalRecall(h, "¿Qué habíamos decidido sobre Aurora?", now)
	if !r.Found || !strings.Contains(r.Response, "Aurora") {
		t.Fatalf("expected Aurora evidence, got found=%v response=%q", r.Found, r.Response)
	}
	if strings.Contains(r.Response, "separar el almacenamiento histórico") {
		t.Fatalf("unrelated objective leaked into Aurora answer: %q", r.Response)
	}
}

func TestB9635RetrievalMetaIsNotHistoricalEvidence(t *testing.T) {
	h := NewConversationHistory()
	now := time.Now()
	id := h.BeginSession("aurora", now.Add(-time.Hour))
	h.RecordTurn(id, HistoricalTurn{
		ID: 1, At: now.Add(-time.Hour),
		UserText:  "Decidimos usar Aurora para supervisar el sistema.",
		Assistant: "Sí. Lo relevante es: Decidimos usar Aurora para supervisar el sistema.",
		Topic:     "Aurora", Relation: "decision",
	})

	r := ResolveHistoricalRecall(h, "¿Por qué cambiamos esa decisión sobre Aurora?", now)
	for _, e := range r.Evidence {
		for _, turn := range e.Episode.Turns {
			if strings.Contains(turn.Assistant, "Lo relevante es") && len(r.candidates) > 0 {
				for _, c := range r.candidates {
					if strings.Contains(c.Text, "Lo relevante es") && c.Provenance == EvidenceMeta {
						t.Fatalf("retrieval meta must not be selected as evidence: %#v", c)
					}
				}
			}
		}
	}
}
