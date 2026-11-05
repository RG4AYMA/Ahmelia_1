package core

// fase2_render.go — Integración de la superficie determinista de Fase 2
// (linguistics/render.go) en el camino real de respuesta
// (generateResponseContractWithStrategy). El contrato de conducta viaja con
// una escena cerrada; aquí se traduce UNA vez al spec del renderizador
// (MoveEnFase2 es la única condición). Si el renderizador rechaza —escena
// fuera del conjunto, sujeto no mapeable o slots incompletos— la función
// devuelve ok=false y el turno sigue por los caminos existentes: la
// integración nunca puede dejar un turno PEOR que antes.

import (
	"strings"

	"ahmelia/interaction"
	"ahmelia/linguistics"
)

// fase2SubjectSlot traduce el sujeto del contrato (vocabulario de ownership)
// al slot del renderizador ("user"|"ahmelia").
func fase2SubjectSlot(s interaction.UserObservationSubject) string {
	switch s {
	case interaction.UserObservationSubjectUser:
		return "user"
	case interaction.UserObservationSubjectSelf:
		return "ahmelia"
	default:
		return ""
	}
}

// fase2ValueScene — escena responder_valor para un dato propio del usuario.
func fase2ValueScene(attribute, value string) *interaction.RenderScene {
	return &interaction.RenderScene{
		Scene:     string(linguistics.MoveResponderValor),
		Subject:   interaction.UserObservationSubjectUser,
		Attribute: strings.TrimSpace(attribute),
		Value:     strings.TrimSpace(value),
	}
}

// fase2AskBackScene — escena dato_faltante: reconfirmación honesta de un dato
// propio del usuario que aún no está registrado.
func fase2AskBackScene(topic string) *interaction.RenderScene {
	return &interaction.RenderScene{
		Scene:     string(linguistics.MoveDatoFaltante),
		Subject:   interaction.UserObservationSubjectUser,
		Attribute: strings.TrimSpace(topic),
	}
}

// firstQueryTopic — primer tópico disponible de una consulta de modelo de
// usuario ("" si no hay), para poblar el slot de atributo de la escena.
func firstQueryTopic(topics []string) string {
	if len(topics) == 0 {
		return ""
	}
	return topics[0]
}

// applyFase2Scene intenta producir la superficie hablada desde la escena
// cerrada del contrato. ok=false significa "esta superficie no es una escena
// de Fase 2" (sin escena, escena fuera del conjunto o slots insuficientes).
func (e *Engine) applyFase2Scene(contract interaction.ResponseContract) (string, bool) {
	scene := contract.RenderScene
	if scene == nil {
		return "", false
	}
	move := linguistics.RenderMove(strings.TrimSpace(scene.Scene))
	if !linguistics.MoveEnFase2(move) {
		return "", false
	}
	subject := fase2SubjectSlot(scene.Subject)
	text, ok := linguistics.Render(linguistics.RenderSpec{
		Move:     move,
		Sujeto:   subject,
		Atributo: strings.TrimSpace(scene.Attribute),
		Valor:    strings.TrimSpace(scene.Value),
	})
	if !ok {
		if e.flowTrace != nil {
			e.flowTrace.Mark("LANGUAGE DEGRADATION", "renderer=code_fase2_refused scene="+string(move)+" subject="+subject)
		}
		return "", false
	}
	if e.flowTrace != nil {
		e.flowTrace.Mark("LANGUAGE ORIGIN", "renderer=code:fase2 scene="+string(move)+" subject="+subject)
	}
	return text, true
}
