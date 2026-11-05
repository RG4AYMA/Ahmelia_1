package core

// open_marks_guard.go — consumo de las marcas explícitas BF1/BF2 (firma
// B1/B2 en el tablero, 2026-10-17 18:38; artefactos de GLM en
// linguistics/open_meaning_explicit.go + open_reading.go, post 18:47).
//
// Regla firmada por ambos lados (no negociable): un candidato con
// negation / condition / modality≠affirmed / incompatibility NO se captura
// como hecho llano ni autoriza. Ninguna marca autoriza por sí sola: esta
// guarda solo puede PROHIBIR la captura llana; la autoridad de la admisión
// sigue siendo exactamente la de antes (respuesta verificada a pregunta
// abierta). Sin marcas (modelo legado o sin declaración del modelo) el piso
// es el contrato cerrado de siempre: cero regresión.

import (
	"strings"

	"ahmelia/linguistics"
)

// openMarksBlockPlainCapture decide, SOLO con marcas explícitas, si el
// candidato del turno puede seguir capturándose como hecho llano. Función
// pura: tabla de tests, sin engine. Devuelve la marca que inhibe, para el
// rastro.
//
// Criterio literal de la firma: negation, condition, incompatibility y
// modality declarada ≠ affirmed. modality vacía NO inhibe (no hubo
// declaración del modelo; tampoco se inventa una: el default honesto es el
// piso cerrado). exception y provenance se cargan pero NO inhiben la captura
// llana por esta guarda (fuera de la firma de consumo; punto de extensión
// visible en la tabla de tests).
func openMarksBlockPlainCapture(m linguistics.OpenMarks) (bool, string) {
	switch {
	case m.Negation != nil:
		return true, "negation"
	case m.Condition != nil:
		return true, "condition"
	case len(m.Incompatibility) > 0:
		return true, "incompatibility"
	case m.Modality != "" && m.Modality != linguistics.ModalityAffirmed:
		return true, "modality=" + string(m.Modality)
	}
	return false, ""
}

// plainCaptureBlockedByOpenMarks aplica la guarda al turno vivo: solo la
// comprensión abierta DE ESTE ENUNCIADO puede inhibir (match por Raw contra
// el cargador del turno; la evidencia de turnos anteriores jamás bloquea).
// Cambia la FUENTE de lectura (significado abierto), no la autoridad.
func (e *Engine) plainCaptureBlockedByOpenMarks(t *processTurn) (bool, string) {
	if e == nil || t == nil || e.lastOpenMeaning == nil {
		return false, ""
	}
	evidenceRaw := strings.TrimSpace(e.lastOpenMeaning.Raw)
	if evidenceRaw == "" {
		return false, ""
	}
	matches := false
	for _, candidate := range []string{t.semanticReq.Raw, t.input} {
		if c := strings.TrimSpace(candidate); c != "" && c == evidenceRaw {
			matches = true
			break
		}
	}
	if !matches {
		return false, ""
	}
	return openMarksBlockPlainCapture(linguistics.DeriveReading(e.lastOpenMeaning.Meaning).Marks)
}
