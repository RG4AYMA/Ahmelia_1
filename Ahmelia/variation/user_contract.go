package variation

// Contratos G5 del núcleo conversacional (eje compañera): P1 (declaración)
// y P2 (no inventa) sobre el modelo de usuario. Los chequeadores son puros,
// operan sobre hechos estructurados tipados (nunca sniffing de frases) y
// devuelven violaciones con prefijo del contrato. Vacío = contrato cumplido.
//
// Integración prevista (una llamada, cuando el fix del Paso 1 pase): el
// camino de ingestión propone el registro + hechos del turno de origen;
// la compuerta llama CheckPreferenceIngestion antes de persistir; el camino
// de consulta llama CheckPreferenceAnswer con la key y el valor afirmado.

import (
	"fmt"
	"strings"

	"ahmelia/interaction"
)

// IngestionFacts describe una propuesta de ingesta al modelo del usuario.
type IngestionFacts struct {
	Record interaction.UserPreference
	// ValueUserDeclared: el VALUE del registro proviene VERBATIM de alguna
	// declaración del usuario (dentro de la ventana que el fix vigile), no
	// de texto de contexto repescado.
	ValueUserDeclared bool
	// SourceTurnQuestion: el turno de origen fue una PREGUNTA.
	SourceTurnQuestion bool
}

// CheckPreferenceIngestion es P1: nada entra al modelo del usuario sin
// declaración explícita; un turno-pregunta jamás ingresa datos.
func CheckPreferenceIngestion(f IngestionFacts) []string {
	var out []string
	if f.SourceTurnQuestion {
		out = append(out, "P1: un turno-pregunta jamás ingresa datos al modelo del usuario")
	}
	if f.Record.Provenance.Kind != interaction.UserSourceExplicitUser {
		out = append(out, fmt.Sprintf("P1: la entrada exige procedencia explicit_user (llegó kind=%q)", f.Record.Provenance.Kind))
	}
	if strings.TrimSpace(f.Record.Key) == "" {
		out = append(out, "P1: ingesta con key vacía")
	}
	if strings.TrimSpace(f.Record.Value) == "" {
		out = append(out, "P1: ingesta con value vacío")
	}
	if !f.ValueUserDeclared {
		out = append(out, "P1: el valor no proviene verbatim de una declaración del usuario (contexto no entra)")
	}
	return out
}

// AnswerFacts describe una respuesta del camino del modelo del usuario.
type AnswerFacts struct {
	// Key es la consulta (dominio preferencia) por la que preguntó el usuario.
	Key string
	// Stored son los registros relevantes del store declarado.
	Stored []interaction.UserPreference
	// ClaimedValue es el valor que la respuesta AFIRMA; vacío significa que
	// la respuesta no afirma ningún valor (el camino honesto "no lo sé").
	ClaimedValue string
}

// CheckPreferenceAnswer es P2: ante consulta sin dato declarado la respuesta
// es "no lo sé" — nunca un valor derivado del contexto.
func CheckPreferenceAnswer(f AnswerFacts) []string {
	claim := strings.TrimSpace(f.ClaimedValue)
	if claim == "" {
		return nil
	}
	var out []string
	match := -1 // coincidencia activa con el valor verbatim
	for i, r := range f.Stored {
		if strings.TrimSpace(r.Key) != strings.TrimSpace(f.Key) {
			continue
		}
		switch {
		case r.Status == interaction.UserModelStatusActive && strings.TrimSpace(r.Value) == claim:
			match = i
		case r.Status != interaction.UserModelStatusActive:
			out = append(out, fmt.Sprintf("P2: la respuesta afirmó desde un registro %q retirado", r.Status))
		case strings.TrimSpace(r.Value) != claim:
			out = append(out, "P2: el valor afirmado no coincide verbatim con lo declarado")
		}
	}
	if match < 0 && len(out) == 0 {
		out = append(out, "P2: se afirmó un valor de preferencia sin registro declarado (invención)")
	}
	return out
}
