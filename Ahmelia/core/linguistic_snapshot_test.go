package core

import (
	"testing"
	"time"

	"ahmelia/dialogue"
	"ahmelia/interaction"
)

func TestBuildLinguisticSituationSnapshotProjectsRecentTurnsAndPendingQuestion(t *testing.T) {
	now := time.Date(2026, 10, 31, 9, 0, 0, 0, time.UTC)
	history := dialogue.NewConversationHistory()
	history.Episodes = []*dialogue.ConversationEpisode{{
		ID:        "ep-1",
		SessionID: "s-1",
		Start:     now.Add(-2 * time.Minute),
		Status:    "active",
		Turns: []dialogue.HistoricalTurn{
			{At: now.Add(-2 * time.Minute), UserText: "abre el proyecto de Unity", Assistant: "Abriendo Unity."},
			{At: now.Add(-90 * time.Second), UserText: "abre eso", Assistant: "¿A qué te refieres?"},
		},
	}}

	queue := interaction.NewClarificationQueue()
	queue.Push("abrir el proyecto", "¿cuál de los dos proyectos?", nil)

	engine := &Engine{conversationHistory: history, clarificationQueue: queue}
	snapshot := engine.buildLinguisticSituationSnapshot(now, "abre eso")
	if snapshot == nil || len(snapshot.Facts) == 0 {
		t.Fatal("el snapshot debe contener los turnos recientes y la pregunta pendiente")
	}

	var sawTurn, sawAnswer, sawPending bool
	for _, f := range snapshot.Facts {
		if f.ID == "" {
			t.Fatal("todo hecho debe tener ID estable")
		}
		switch f.Kind {
		case "turno_usuario":
			if f.Text == "abre el proyecto de Unity" {
				sawTurn = true
			}
			if f.Text == "abre eso" {
				t.Fatal("la entrada actual no debe reinyectarse como turno previo")
			}
		case "respuesta_previa":
			sawAnswer = true
		case "pregunta_pendiente":
			sawPending = true
		}
	}
	if !sawTurn || !sawAnswer || !sawPending {
		t.Fatalf("proyección incompleta: turn=%v answer=%v pending=%v facts=%+v", sawTurn, sawAnswer, sawPending, snapshot.Facts)
	}
	if !snapshot.HasID("f1") {
		t.Fatal("los IDs deben ser citables por el modelo")
	}
}

func TestBuildLinguisticSituationSnapshotEmptyEngine(t *testing.T) {
	var engine *Engine
	if got := engine.buildLinguisticSituationSnapshot(time.Now(), "hola"); got != nil {
		t.Fatalf("un engine nil no debe producir contexto: %+v", got)
	}
	var empty Engine
	if got := empty.buildLinguisticSituationSnapshot(time.Now(), "hola"); got != nil {
		t.Fatalf("un engine sin estado no debe producir contexto: %+v", got)
	}
}
