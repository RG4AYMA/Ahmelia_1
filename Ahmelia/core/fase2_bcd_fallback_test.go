package core

// fase2_bcd_fallback_test.go — B/C/D del cuadro binario de Fase 2:
//
//	B: la lectura abierta resuelve el sujeto cuando el SEMANTIC de reglas
//	   queda vacío (owner_of_data) y el respaldo raíz+persona vive en
//	   ownership (probado en su paquete). Aquí se fija la integración.
//	C: el acto tipado de la lectura abierta (evaluación → feedback) enruta la
//	   conducta cuando las reglas locales no la clasificaron — nunca pisa
//	   actos ejecutables, preguntas o correcciones.
//	D: una sola habla por turno — el mismo contenido normalizado no se emite
//	   dos veces dentro del turno activo.

import (
	"testing"

	"ahmelia/interaction"
	"ahmelia/linguistics"
)

func TestMergeOpenMeaningFillsEmptySubjectFromOwnerOfData(t *testing.T) {
	base := interaction.Analyze("que tal si ordenamos la biblioteca", interaction.Context{})
	base.Subject = "" // el SEMANTIC de reglas no resolvió

	filled := mergeOpenMeaning(base, linguistics.OpenMeaning{OwnerOfData: linguistics.OpenOwnerAhmelia})
	if filled.Subject != interaction.UserObservationSubjectSelf {
		t.Fatalf("owner_of_data ahmelia no llenó el sujeto: %q", filled.Subject)
	}
	filledUser := mergeOpenMeaning(base, linguistics.OpenMeaning{OwnerOfData: linguistics.OpenOwnerUser})
	if filledUser.Subject != interaction.UserObservationSubjectUser {
		t.Fatalf("owner_of_data user no llenó el sujeto: %q", filledUser.Subject)
	}

	// Un sujeto ya resuelto por reglas no se pisa.
	resolved := interaction.Analyze("mi color favorito es el negro", interaction.Context{})
	if resolved.Subject != interaction.UserObservationSubjectUser {
		t.Fatalf("precondición: reglas debían resolver user: %q", resolved.Subject)
	}
	kept := mergeOpenMeaning(resolved, linguistics.OpenMeaning{OwnerOfData: linguistics.OpenOwnerAhmelia})
	if kept.Subject != interaction.UserObservationSubjectUser {
		t.Fatalf("la lectura abierta pisó el sujeto de reglas: %q", kept.Subject)
	}
}

func TestMergeOpenMeaningTipsFeedbackActFromEvaluacionReading(t *testing.T) {
	base := interaction.Analyze("tengo ganas de pintar el garage este sabado", interaction.Context{})
	if base.CommunicativeActs.Has(interaction.ActFeedback) {
		t.Fatalf("precondición: reglas no debían clasificar feedback: %+v", base.CommunicativeActs)
	}
	tipped := mergeOpenMeaning(base, linguistics.OpenMeaning{Purpose: linguistics.OpenPurposeFeedback, Confidence: .9, Meaning: "elogio"})
	if !tipped.CommunicativeActs.Has(interaction.ActFeedback) {
		t.Fatalf("evaluación abierta no tipó feedback: %+v", tipped.CommunicativeActs)
	}
	found := false
	for _, ev := range tipped.CommunicativeActs.Evidence {
		if ev.Act == interaction.ActFeedback && ev.Source == "open_reading:evaluacion" {
			found = true
		}
	}
	if !found {
		t.Fatalf("falta evidencia del acto tipado: %+v", tipped.CommunicativeActs.Evidence)
	}

	// Un acto ejecutable nunca se pisa con la lectura abierta.
	command := interaction.Analyze("abre la puerta", interaction.Context{})
	kept := mergeOpenMeaning(command, linguistics.OpenMeaning{Purpose: linguistics.OpenPurposeFeedback, Confidence: .9})
	if command.CommunicativeActs.Has(interaction.ActCommand) {
		if !kept.CommunicativeActs.Has(interaction.ActCommand) || kept.CommunicativeActs.Has(interaction.ActFeedback) {
			t.Fatalf("la lectura abierta pisó un acto ejecutable: %+v", kept.CommunicativeActs)
		}
	}
}

func TestSaySuppressesIdenticalSpeechWithinTurn(t *testing.T) {
	e := newInteractionE2EEngine(t)
	var responses []string
	e.SetResponseCallback(func(msg string) { responses = append(responses, msg) })

	e.conversationTurnActive = true
	e.activeConversationTurnID = 42
	e.say("Misma frase de prueba.")
	e.say("misma FRASE de prueba.") // mismo contenido normalizado: duplicado
	e.say("Otra frase distinta.")
	if len(responses) != 2 {
		t.Fatalf("la invariante de una sola habla por turno no suprimió el duplicado: %+v", responses)
	}
	e.activeConversationTurnID = 43
	e.say("Misma frase de prueba.")
	if len(responses) != 3 {
		t.Fatalf("un turno nuevo debe poder repetir contenido: %+v", responses)
	}
}
