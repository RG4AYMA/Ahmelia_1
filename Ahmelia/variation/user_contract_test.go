package variation

import (
	"strings"
	"testing"

	"ahmelia/interaction"
)

// --- Propiedad P1 universal: nada entra sin declaración explícita ---

func TestP1QuestionTurnsNeverIngestProperties(t *testing.T) {
	dims := map[string]Dimension{
		"source_kind": {Name: "source_kind", Values: []string{"explicit_user", "observed", "inferred", "system", "imported"}},
		"turn_kind":   {Name: "turn_kind", Values: []string{"pregunta", "declaracion", "charla"}},
		"value_origin": {Name: "value_origin", Values: []string{"verbatim", "contexto", "vacio"}},
	}

	ForAll(t, dims, 45, func(s GeneratedSituation) bool {
		facts := IngestionFacts{
			Record: interaction.UserPreference{
				Key:        "duracion_ducha",
				Value:      valueFor(s.Values["value_origin"]),
				Confidence: 0.8,
				Provenance: interaction.UserModelSourceRef{
					Kind:     interaction.UserModelSourceKind(s.Values["source_kind"]),
					SourceID: "turn-42",
					ActorID:  "user",
				},
			},
			ValueUserDeclared:  s.Values["value_origin"] == "verbatim",
			SourceTurnQuestion: s.Values["turn_kind"] == "pregunta",
		}

		violations := CheckPreferenceIngestion(facts)
		joined := strings.Join(violations, " | ")

		// Reglas incondicionales del contrato: cada condición falsa debe
		// producir su violación, pase lo que pase con el resto.
		checks := []struct{ cond bool; prefix string }{
			{s.Values["turn_kind"] == "pregunta", "P1: un turno-pregunta"},
			{s.Values["source_kind"] != "explicit_user", "P1: la entrada exige procedencia"},
			{s.Values["value_origin"] == "vacio", "P1: ingesta con value vacío"},
			{s.Values["value_origin"] != "verbatim", "P1: el valor no proviene verbatim"},
		}
		for _, c := range checks {
			if c.cond && !strings.Contains(joined, c.prefix) {
				return false
			}
			if !c.cond && strings.Contains(joined, c.prefix) {
				return false
			}
		}

		// El caso totalmente legal jamás es rechazado: declaración explícita,
		// turno declarante, valor verbatim de la declaración.
		legal := InterpretCandidate(s.Values)
		if legal && len(violations) != 0 {
			return false
		}
		return true
	})
}

func valueFor(origin string) string {
	switch origin {
	case "verbatim":
		return "quince minutos"
	case "contexto":
		return "que onda" // réplica del valor repescado del contexto
	default:
		return ""
	}
}

// InterpretCandidate resume si el caso generado es una declaración válida.
func InterpretCandidate(values map[string]string) bool {
	return values["source_kind"] == "explicit_user" &&
		values["turn_kind"] == "declaracion" &&
		values["value_origin"] == "verbatim"
}

// TestP1WitnessFabricationOfRunB fija la fabricación del 2026-10-16 14:05:34
// como regresión permanente: turno-pregunta + valor repescado del contexto +
// etiqueta explicit_user + confianza 0.8 debe quedar RECHAZADA por contrato.
func TestP1WitnessFabricationOfRunB(t *testing.T) {
	facts := IngestionFacts{
		Record: interaction.UserPreference{
			Key:        "bebida favorita",
			Value:      "que onda",
			Confidence: 0.8,
			Status:     interaction.UserModelStatusActive,
			Provenance: interaction.UserModelSourceRef{
				Kind:     interaction.UserSourceExplicitUser,
				SourceID: "turn-9",
				ActorID:  "user",
			},
		},
		ValueUserDeclared:  false,
		SourceTurnQuestion: true,
	}

	violations := CheckPreferenceIngestion(facts)
	joined := strings.Join(violations, " | ")

	if !strings.Contains(joined, "turno-pregunta") {
		t.Fatalf("la pregunta de la corrida 2 debe quedar rechazada por la regla de turno: %v", violations)
	}
	if !strings.Contains(joined, "verbatim") {
		t.Fatalf("el valor repescado 'que onda' debe quedar rechazado por la regla de verbatim: %v", violations)
	}
	// El resto de reglas NO aplica a este testigo (la etiqueta explicit_user
	// y los campos no vacíos quedaron intactos en el disco): el chequeador
	// penaliza SOLO lo roto.
	if strings.Contains(joined, "explicit_user") {
		t.Fatalf("la regla de procedencia no corresponde a este testigo: %v", violations)
	}
}

// Declaración legítima de primera persona: la puerta queda abierta.
func TestP1AcceptsTrueFirstPersonDeclaration(t *testing.T) {
	facts := IngestionFacts{
		Record: interaction.UserPreference{
			Key:        "temperatura_cafe",
			Value:      "tibio",
			Confidence: 0.8,
			Provenance: interaction.UserModelSourceRef{
				Kind:     interaction.UserSourceExplicitUser,
				SourceID: "turn-31",
				ActorID:  "user",
			},
		},
		ValueUserDeclared:  true,
		SourceTurnQuestion: false,
	}
	if violations := CheckPreferenceIngestion(facts); len(violations) != 0 {
		t.Fatalf("declaración legítima no debe generar violaciones: %v", violations)
	}
}

// --- Propiedad P2 universal: sin dato declarado, no lo sé — nunca invención ---

func TestP2AnswersOnlyClaimDeclaredActives(t *testing.T) {
	dims := map[string]Dimension{
		"record_status": {Name: "record_status", Values: []string{"active", "superseded", "rejected", "ninguno"}},
		"claim":         {Name: "claim", Values: []string{"verbatim", "otro", "nada"}},
	}

	ForAll(t, dims, 48, func(s GeneratedSituation) bool {
		var stored []interaction.UserPreference
		status := interaction.UserModelItemStatus(s.Values["record_status"])
		if s.Values["record_status"] != "ninguno" {
			stored = append(stored, interaction.UserPreference{
				Key:    "temperatura_bano",
				Value:  "tibio",
				Status: status,
			})
		}
		claimed := ""
		switch s.Values["claim"] {
		case "verbatim":
			claimed = "tibio"
		case "otro":
			claimed = "hielo"
		}
		violations := CheckPreferenceAnswer(AnswerFacts{Key: "temperatura_bano", Stored: stored, ClaimedValue: claimed})
		joined := strings.Join(violations, " | ")

		// No afirmar nada = camino honesto: jamás hay violación.
		if s.Values["claim"] == "nada" && len(violations) != 0 {
			return false
		}
		// Afirmar un valor con store vacío = invención, SIEMPRE.
		if s.Values["record_status"] == "ninguno" && s.Values["claim"] != "nada" && !strings.Contains(joined, "invención") {
			return false
		}
		// Afirmar desde un registro retirado = corrimiento, SIEMPRE.
		if (s.Values["record_status"] == "superseded" || s.Values["record_status"] == "rejected") && s.Values["claim"] != "nada" && !strings.Contains(joined, "retirado") {
			return false
		}
		// Ahí Banco activo con valor coincidente = único caso limpio.
		limpio := s.Values["record_status"] == "active" && s.Values["claim"] == "verbatim"
		if limpio && len(violations) != 0 {
			return false
		}
		return true
	})
}

// TestP2WitnessDefenseInDepth: si P1 falla y una invención persiste, el
// camino de consulta igual debe rechazar afirmar frente a store insuficiente.
func TestP2WitnessDefenseInDepth(t *testing.T) {
	// Store limpio (el estado deseado post-fix) + pregunta de bebida:
	violations := CheckPreferenceAnswer(AnswerFacts{Key: "bebida favorita", ClaimedValue: "que onda"})
	if len(violations) != 1 || !strings.Contains(violations[0], "invención") {
		t.Fatalf("afirmar un valor sin declaración debe marcar invención: %v", violations)
	}
	// El store sucio de la corrida 2 (evento discutido en tablero 14:09):
	// aquí la invención DEBE pasar la P2 estricta (coincidía con el registro
	// fabricado) — y ese es exactamente el caso que la P1 arriba condena.
	stored := []interaction.UserPreference{{
		Key:    "bebida favorita",
		Value:  "que onda",
		Status: interaction.UserModelStatusActive,
		Provenance: interaction.UserModelSourceRef{
			Kind:     interaction.UserSourceExplicitUser,
			SourceID: "turn-9",
			ActorID:  "user",
		},
	}}
	if violations := CheckPreferenceAnswer(AnswerFacts{Key: "bebida favorita", Stored: stored, ClaimedValue: "que onda"}); len(violations) != 0 {
		t.Fatalf("P2 no debe duplicar el veredicto de P1 en su dominio: %v", violations)
	}
}

// TestP2HonestUnknownNeverViolates: la respuesta honesta ("no lo sé") es
// computable y jamás genera violación, con cualquier estado del store.
func TestP2HonestUnknownNeverViolates(t *testing.T) {
	cases := []AnswerFacts{
		{Key: "k", ClaimedValue: ""},
		{Key: "k", Stored: []interaction.UserPreference{{Key: "k", Value: "v", Status: interaction.UserModelStatusActive}}, ClaimedValue: ""},
	}
	for i, c := range cases {
		if violations := CheckPreferenceAnswer(c); len(violations) != 0 {
			t.Fatalf("respuesta honesta %d no debe violar: %v", i, violations)
		}
	}
}

// TestKeyAndValueBlanksAreRejected: los vacíos degradados nunca pasan la
// compuerta de declaración (higiene de entrada).
func TestKeyAndValueBlanksAreRejected(t *testing.T) {
	blank := interaction.UserPreference{Key: " ", Value: "\t",
		Provenance: interaction.UserModelSourceRef{Kind: interaction.UserSourceExplicitUser}}
	violations := CheckPreferenceIngestion(IngestionFacts{Record: blank, ValueUserDeclared: true})
	joined := strings.Join(violations, " | ")
	if !strings.Contains(joined, "key vacía") || !strings.Contains(joined, "value vacío") {
		t.Fatalf("vacíos deben violar higiene, got: %v", violations)
	}
	if len(violations) != 2 {
		t.Fatalf("solo las dos reglas de vacío deben aplicar con lo demás legal: %v", violations)
	}
}
