package core

import (
	"strings"
	"testing"

	"ahmelia/interaction"
)

// Batería de admisión de respuestas personales — Eje Compañera, Paso 1.
//
// Frases exactas del propietario (R11): viven únicamente aquí y en la
// evidencia. La cadena completa es:
//
//	Nivel 0: el usuario pregunta por su dato; no hay dato → respuesta honesta
//	y reconfirmación que abre una pregunta.
//	Ruido conversacional entre medio: no debe ingresar jamás al modelo.
//	Respuesta real del usuario: ingresa con procedencia.
//	Nivel 1: la consulta repetida responde con el dato real, nunca con ruido.
func TestPersonalAnswerAdmission_SceneChain(t *testing.T) {
	e := newInteractionE2EEngine(t)
	var responses []string
	e.SetResponseCallback(func(msg string) { responses = append(responses, msg) })

	// Nivel 0 — el usuario pregunta por su dato.
	e.Process("sabes mi bebida favorita?")
	if len(responses) == 0 {
		t.Fatal("sin respuesta a la consulta del usuario")
	}
	first := responses[len(responses)-1]
	if !strings.Contains(first, "Todavía no tengo ese dato guardado") {
		t.Errorf("respuesta no honesta en Nivel 0: %q", first)
	}
	if !strings.Contains(first, "¿Me lo dices?") {
		t.Errorf("no hay reconfirmación que abra la pregunta: %q", first)
	}
	if prefs := scenePrefs(t); len(prefs) != 0 {
		t.Errorf("la pregunta del usuario no debe ingresar datos: %+v", prefs)
	}

	// Ruido conversacional («que onda»): JAMÁS ingresa al modelo de usuario.
	e.Process("que onda")
	if prefs := scenePrefs(t); len(prefs) != 0 {
		t.Errorf("ruido conversacional fabricó un dato: %+v", prefs)
	}

	// Respuesta real del usuario a la pregunta abierta: sí ingresa.
	e.Process("me gusta la gaseosa de piña")
	prefs := scenePrefs(t)
	if len(prefs) != 1 {
		t.Errorf("la respuesta verificada no quedó registrada (prefs=%+v)", prefs)
	} else {
		if prefs[0].Key != "bebida favorita" || !strings.Contains(prefs[0].Value, "gaseosa de piña") {
			t.Errorf("valor registrado inesperado: key=%q value=%q", prefs[0].Key, prefs[0].Value)
		}
		if strings.Contains(prefs[0].Value, "que onda") {
			t.Errorf("se registró ruido como respuesta: %q", prefs[0].Value)
		}
	}

	// Nivel 1 — la consulta repetida responde con el dato real, no con ruido.
	e.Process("sabes mi bebida favorita?")
	last := responses[len(responses)-1]
	if !strings.Contains(last, "gaseosa de piña") {
		t.Errorf("la consulta no devolvió el dato real: %q", last)
	}
	if strings.Contains(last, "que onda") {
		t.Errorf("la consulta devolvió ruido: %q", last)
	}
}

// TestPersonalAnswerAdmission_UserQuestionsNeverIngest — una pregunta del
// propio usuario jamás ingresa datos al modelo, ni siquiera cuando existe
// una pregunta abierta de Ahmelia.
func TestPersonalAnswerAdmission_UserQuestionsNeverIngest(t *testing.T) {
	e := newInteractionE2EEngine(t)
	var responses []string
	e.SetResponseCallback(func(msg string) { responses = append(responses, msg) })

	e.Process("sabes mi bebida favorita?")
	e.Process("por qué no?")
	if prefs := scenePrefs(t); len(prefs) != 0 {
		t.Errorf("una pregunta del usuario ingresó datos: %+v", prefs)
	}
	if len(responses) == 0 {
		t.Error("sin respuesta al segundo turno")
	}
}

// TestPersonalAnswerAdmission_ProvenanceAndRegistry — la pregunta abierta deja
// rastro auditable (abierta → respondida) y la respuesta guarda procedencia
// real: source = turno del enunciado, evidence = turno donde Ahmelia preguntó.
func TestPersonalAnswerAdmission_ProvenanceAndRegistry(t *testing.T) {
	e := newInteractionE2EEngine(t)
	e.SetResponseCallback(func(string) {})

	e.Process("sabes mi bebida favorita?")
	snap := e.conversationState.Snapshot()
	if len(snap.OpenQuestions) != 1 {
		t.Fatalf("preguntas abiertas=%d, esperaba 1 (%+v)", len(snap.OpenQuestions), snap.OpenQuestions)
	}
	open := snap.OpenQuestions[0]
	if open.Key != "bebida favorita" || open.Status != interaction.OpenQuestionOpen || open.AskedTurn != 1 {
		t.Fatalf("pregunta abierta inesperada: %+v", open)
	}

	e.Process("que onda")
	if prefs := scenePrefs(t); len(prefs) != 0 {
		t.Errorf("ruido conversacional fabricó un dato: %+v", prefs)
	}
	snap = e.conversationState.Snapshot()
	if len(snap.OpenQuestions) != 1 || snap.OpenQuestions[0].Status != interaction.OpenQuestionOpen {
		t.Fatalf("la pregunta no debe cerrarse con ruido: %+v", snap.OpenQuestions)
	}

	e.Process("me gusta la gaseosa de piña")
	snap = e.conversationState.Snapshot()
	if snap.OpenQuestions[0].Status != interaction.OpenQuestionAnswered || snap.OpenQuestions[0].AnsweredTurn != 3 {
		t.Fatalf("la pregunta debió quedar respondida en el turno 3: %+v", snap.OpenQuestions[0])
	}
	prefs := scenePrefs(t)
	if len(prefs) != 1 {
		t.Fatalf("preferencias=%d, esperaba 1", len(prefs))
	}
	if prefs[0].Provenance.SourceID != "turn-3" || prefs[0].Provenance.EvidenceID != "turn-1" {
		t.Fatalf("procedencia inesperada: %+v", prefs[0].Provenance)
	}
}

func scenePrefs(t *testing.T) []interaction.UserPreference {
	t.Helper()
	store, err := interaction.LoadUserModelStore(interaction.UserModelPath, "ayma240")
	if err != nil {
		t.Fatalf("no se pudo cargar el UserModel: %v", err)
	}
	prefs, err := interaction.GetRelevantPreferences(store, interaction.UserModelActorCognition, interaction.UserModelQueryContext{Topics: []string{"bebida favorita"}})
	if err != nil {
		t.Fatalf("consulta de preferencias: %v", err)
	}
	return prefs
}
