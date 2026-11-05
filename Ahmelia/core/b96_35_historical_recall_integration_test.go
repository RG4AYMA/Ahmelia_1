package core

import (
	"strings"
	"testing"
	"time"

	"ahmelia/dialogue"
)

func TestB9635_ProcessHistoricalRecallBypassesGenericConversationGate(t *testing.T) {
	e := buildTestEngine(t)
	now := time.Date(2026, 9, 5, 10, 30, 0, -5*60*60, time.FixedZone("-05", -5*60*60))
	h := dialogue.NewConversationHistory()
	current := h.BeginSession("current", now.Add(-5*time.Minute))
	previous := h.BeginSession("previous", now.Add(-2*time.Hour))
	h.RecordTurn(previous, dialogue.HistoricalTurn{
		ID: 1, At: now.Add(-2 * time.Hour),
		UserText:  "Decidimos usar Aurora principalmente para supervisar el estado del sistema.",
		Assistant: "Sí, esa fue la decisión.",
		Topic:     "supervisión del sistema",
		Objective: "supervisar el estado del sistema",
	})
	h.CompleteSession(previous, "Decidimos usar Aurora principalmente para supervisar el estado del sistema.", now.Add(-119*time.Minute))
	e.conversationHistory = h
	e.conversationEpisodeID = current

	var response string
	e.onResponse = func(msg string) { response = msg }
	e.Process("¿Qué habíamos decidido hacer con Aurora?")

	if !strings.Contains(response, "Decidimos usar Aurora principalmente") {
		t.Fatalf("historical bridge did not answer from memory: %q", response)
	}
	if strings.Contains(response, "Te escucho") {
		t.Fatalf("generic conversation gate swallowed historical recall: %q", response)
	}
}

func TestB9635_ProcessHistoricalRecallDetailedMode(t *testing.T) {
	e := buildTestEngine(t)
	now := time.Date(2026, 9, 5, 10, 30, 0, -5*60*60, time.FixedZone("-05", -5*60*60))
	h := dialogue.NewConversationHistory()
	current := h.BeginSession("current", now.Add(-5*time.Minute))
	previous := h.BeginSession("previous", now.Add(-3*time.Hour))
	h.RecordTurn(previous, dialogue.HistoricalTurn{ID: 1, At: now.Add(-3 * time.Hour), UserText: "Tuvimos un problema con la supervisión.", Assistant: "El estado no se actualizaba.", Topic: "supervisión"})
	h.RecordTurn(previous, dialogue.HistoricalTurn{ID: 2, At: now.Add(-2*time.Hour - 50*time.Minute), UserText: "Lo solucionamos cambiando el mecanismo de actualización.", Assistant: "Quedó resuelto.", Relation: "solution", Topic: "supervisión"})
	h.RecordTurn(previous, dialogue.HistoricalTurn{ID: 3, At: now.Add(-2*time.Hour - 40*time.Minute), UserText: "Después acordamos mantener ese mecanismo.", Assistant: "Correcto.", Topic: "supervisión"})
	h.CompleteSession(previous, "Problema, solución y decisión sobre supervisión.", now.Add(-2*time.Hour-30*time.Minute))
	e.conversationHistory = h
	e.conversationEpisodeID = current

	var response string
	e.onResponse = func(msg string) { response = msg }
	e.Process("Muéstrame todo el historial relacionado con la supervisión")

	for _, want := range []string{"Tuvimos un problema", "Lo solucionamos", "Después acordamos"} {
		if !strings.Contains(response, want) {
			t.Fatalf("detailed historical recall missing %q: %q", want, response)
		}
	}
	if !strings.Contains(response, "UTC-05:00") {
		t.Fatalf("detailed historical recall missing timezone: %q", response)
	}
}
