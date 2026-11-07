package core

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOwnerRestrictionOneQuestionAndReceptiveRecreation(t *testing.T) {
	loc := time.FixedZone("COT", -5*3600)
	now := time.Date(2026, 11, 5, 8, 0, 0, 0, loc)
	dir := t.TempDir()

	e := buildTestEngine(t)
	e.sceneNow = func() time.Time { return now }
	e.enableOwnerRestrictionsAt(filepath.Join(dir, "owner_restrictions.json"))
	e.enableOwnerStateAt(filepath.Join(dir, "owner_state.json"))
	if err := e.SetOwnerReceptivity("high", 2*time.Hour, []string{"turn:open"}); err != nil {
		t.Fatalf("receptividad: %v", err)
	}
	var responses []string
	e.SetResponseCallback(func(response string) { responses = append(responses, response) })

	e.Process("hoy no trabajamos")
	first := strings.Join(responses, " ")
	if !strings.Contains(first, "motivo") {
		t.Fatalf("la primera respuesta debía preguntar el motivo: %q", first)
	}
	if !strings.Contains(first, "pausa") {
		t.Fatalf("con receptividad alta debía proponer una pausa: %q", first)
	}
	if got := e.OwnerQuestionsAsked(); got != 1 {
		t.Fatalf("preguntas = %d, quería 1", got)
	}

	e.Process("hoy sí trabajamos")
	responses = nil
	e.Process("hoy no trabajamos")
	second := strings.Join(responses, " ")
	if strings.Contains(second, "motivo") {
		t.Fatalf("no debía repetir la pregunta: %q", second)
	}
	if strings.Contains(second, "pausa") {
		t.Fatalf("no debía repetir la propuesta en la ventana: %q", second)
	}
	if got := e.OwnerQuestionsAsked(); got != 1 {
		t.Fatalf("preguntas = %d, quería 1", got)
	}

	e2 := buildTestEngine(t)
	e2.sceneNow = func() time.Time { return now }
	e2.enableOwnerRestrictionsAt(filepath.Join(dir, "owner_restrictions_low.json"))
	e2.enableOwnerStateAt(filepath.Join(dir, "owner_state_low.json"))
	if err := e2.SetOwnerReceptivity("low", 2*time.Hour, []string{"turn:quiet"}); err != nil {
		t.Fatalf("receptividad: %v", err)
	}
	var lowResponses []string
	e2.SetResponseCallback(func(response string) { lowResponses = append(lowResponses, response) })
	e2.Process("hoy no trabajamos")
	low := strings.Join(lowResponses, " ")
	if strings.Contains(low, "pausa") {
		t.Fatalf("con receptividad baja no debía proponer recreación: %q", low)
	}
	if !strings.Contains(low, "motivo") {
		t.Fatalf("la primera pregunta seguía permitida: %q", low)
	}
}
