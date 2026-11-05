package cognition

import (
	"testing"

	"ahmelia/interaction"
)

func TestB96331_ConversationModelUsesExistingContracts(t *testing.T) {
	req := interaction.SemanticRequest{
		Raw:        "creo que deberíamos cambiar el sistema de enemigos",
		Normalized: "creo que deberiamos cambiar el sistema de enemigos",
		Kind:       interaction.KindProposal,
		SpeechAct:  interaction.SpeechPropose,
		Topic:      "sistema de enemigos",
		Confidence: 0.86,
		Objective:  interaction.Objective{Text: "evaluar un cambio del sistema de enemigos"},
	}
	ctx := interaction.ConversationSnapshot{
		SessionID:       "session-test",
		ActiveTopic:     "sistema de enemigos",
		ActiveObjective: "mejorar el combate",
		ActiveThreadID:  "thread-1",
		Turns: []interaction.TurnRecord{
			{ID: 1, UserText: "estamos trabajando en el combate", Topic: "sistema de enemigos"},
		},
	}

	m := NewConversationalCognitiveModel(req, ctx)

	if m.Version != "B96.33.1" {
		t.Fatalf("version inesperada: %q", m.Version)
	}
	if m.Situation.Kind != SituationProposal {
		t.Fatalf("situación = %q, esperaba proposal", m.Situation.Kind)
	}
	if m.Situation.Topic != "sistema de enemigos" {
		t.Fatalf("topic = %q", m.Situation.Topic)
	}
	if !m.Situation.HasHistory || !m.Situation.HasActiveWork == false {
		// HasActiveWork is false here intentionally; the expression above
		// only guards the history contract without requiring fake work.
	}
	if !m.Signals.HasSemanticEvidence || !m.Signals.GoalDirected {
		t.Fatalf("señales inesperadas: %+v", m.Signals)
	}
	if !m.IsCognitivelyReady() {
		t.Fatal("una propuesta semánticamente estructurada debería estar lista para la siguiente capa")
	}
	if m.Provenance.SemanticSource != "interaction.SemanticRequest" ||
		m.Provenance.StateSource != "interaction.ConversationSnapshot" ||
		!m.Provenance.DerivedOnly {
		t.Fatalf("provenance incorrecta: %+v", m.Provenance)
	}
}

func TestB96331_ActiveContextFillsMissingTopicAndGoal(t *testing.T) {
	req := interaction.SemanticRequest{
		Raw:        "sigamos con eso",
		Normalized: "sigamos con eso",
		Kind:       interaction.KindContinuation,
		Continues:  true,
		Confidence: 0.91,
	}
	ctx := interaction.ConversationSnapshot{
		ActiveTopic:     "inventario",
		ActiveObjective: "terminar el sistema de inventario",
		ActiveThreadID:  "thread-7",
	}

	m := NewConversationalCognitiveModel(req, ctx)

	if m.Situation.Topic != "inventario" {
		t.Fatalf("topic heredado = %q", m.Situation.Topic)
	}
	if m.Situation.ActiveGoal != "terminar el sistema de inventario" {
		t.Fatalf("goal heredado = %q", m.Situation.ActiveGoal)
	}
	if !m.Situation.IsContinuation {
		t.Fatal("debería conservar la continuidad de B96.32")
	}
	if !m.Signals.HasActiveContext {
		t.Fatal("debería detectar contexto activo")
	}
}

func TestB96331_UnresolvedReferenceDoesNotAuthorizeAction(t *testing.T) {
	req := interaction.SemanticRequest{
		Raw:        "cambia eso",
		Normalized: "cambia eso",
		Kind:       interaction.KindAction,
		Confidence: 0.72,
		References: []interaction.Reference{
			{
				Kind:       interaction.ReferenceCurrentTopic,
				Expression: "eso",
				Resolved:   false,
			},
		},
	}

	m := NewConversationalCognitiveModel(req, interaction.ConversationSnapshot{})

	if !m.Signals.HasUnresolvedRefs {
		t.Fatal("debería detectar referencia no resuelta")
	}
	if !m.Signals.NeedsClarification {
		t.Fatal("una referencia no resuelta debería marcar necesidad de aclaración")
	}
	if !m.Signals.NeedsAction {
		t.Fatal("la semántica sigue siendo una acción; el modelo no debe ocultarla")
	}
	// B96.33.1 never turns NeedsAction into authorization.
	if !m.IsCognitivelyReady() {
		t.Fatal("la estructura semántica existe aunque requiera aclaración")
	}
}

func TestB96331_EmptyRequestIsNotReady(t *testing.T) {
	m := NewConversationalCognitiveModel(interaction.SemanticRequest{}, interaction.ConversationSnapshot{})

	if m.IsCognitivelyReady() {
		t.Fatal("un request vacío no debería estar cognitivamente listo")
	}
	if m.Situation.Kind != SituationUnknown {
		t.Fatalf("situación vacía = %q", m.Situation.Kind)
	}
}

func TestB96331_DoesNotAliasMutableInputSlices(t *testing.T) {
	req := interaction.SemanticRequest{
		Normalized: "abre unity",
		Kind:       interaction.KindAction,
		Confidence: 0.95,
		References: []interaction.Reference{{Expression: "unity", Resolved: true}},
		Entities:   []string{"Unity"},
		Objective:  interaction.Objective{Constraints: []string{"solo pruebas"}},
		Action:     interaction.ActionIntent{Parameters: map[string]string{"raw": "abre unity"}},
	}
	m := NewConversationalCognitiveModel(req, interaction.ConversationSnapshot{})

	req.References[0].Expression = "mutated"
	req.Entities[0] = "mutated"
	req.Objective.Constraints[0] = "mutated"
	req.Action.Parameters["raw"] = "mutated"

	if m.Request.References[0].Expression != "unity" ||
		m.Request.Entities[0] != "Unity" ||
		m.Request.Objective.Constraints[0] != "solo pruebas" ||
		m.Request.Action.Parameters["raw"] != "abre unity" {
		t.Fatal("el modelo no debe compartir slices/maps mutables con el request de entrada")
	}
}
