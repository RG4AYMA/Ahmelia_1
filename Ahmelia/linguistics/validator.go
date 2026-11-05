package linguistics

import (
	"strings"
)

// ValidationResult contiene la interpretación después de aplicar
// validaciones estructurales y las advertencias encontradas.
//
// Warnings no bloquea la interpretación: informa a Ahmelia de que
// alguna parte de la salida del LLM fue descartada o considerada
// inconsistente.
type ValidationResult struct {
	Interpretation Interpretation
	Warnings       []string
}

// Validator protege el resto de Ahmelia frente a salidas lingüísticas
// inconsistentes del LLM.
//
// IMPORTANTE:
// No intenta comprender el lenguaje por sí mismo.
// No clasifica intents.
// No inventa entidades.
// No resuelve referencias.
//
// Solo aplica reglas de consistencia estructural.
type Validator struct{}

// NewValidator crea un nuevo validador lingüístico.
func NewValidator() *Validator {
	return &Validator{}
}

// Validate revisa una interpretación producida por el LLM y devuelve
// una versión estructuralmente más segura.
func (v *Validator) Validate(
	input string,
	interpretation Interpretation,
) ValidationResult {
	result := ValidationResult{
		Interpretation: interpretation,
		Warnings:       []string{},
	}

	input = strings.TrimSpace(input)

	// Asegurar slices no nil.
	if result.Interpretation.Entities == nil {
		result.Interpretation.Entities = []string{}
	}

	if result.Interpretation.References == nil {
		result.Interpretation.References = []string{}
	}

	if result.Interpretation.Constraints == nil {
		result.Interpretation.Constraints = []string{}
	}

	if result.Interpretation.Relations == nil {
		result.Interpretation.Relations = []Relation{}
	}
	validRelations := result.Interpretation.Relations[:0]
	for _, rel := range result.Interpretation.Relations {
		rel.Source = strings.TrimSpace(rel.Source)
		rel.Predicate = strings.TrimSpace(rel.Predicate)
		rel.Target = strings.TrimSpace(rel.Target)
		if rel.Source == "" || rel.Predicate == "" || rel.Target == "" {
			result.Warnings = append(result.Warnings, "relación descartada por estructura incompleta")
			continue
		}
		if rel.Confidence < 0 {
			rel.Confidence = 0
		}
		if rel.Confidence > 1 {
			rel.Confidence = 1
		}
		duplicate := false
		for _, existing := range validRelations {
			if strings.EqualFold(existing.Source, rel.Source) && strings.EqualFold(existing.Predicate, rel.Predicate) && strings.EqualFold(existing.Target, rel.Target) {
				duplicate = true
				break
			}
		}
		if !duplicate {
			validRelations = append(validRelations, rel)
		}
	}
	result.Interpretation.Relations = validRelations

	// Normalizar duplicados dentro de cada campo.
	result.Interpretation.Entities =
		uniqueStrings(result.Interpretation.Entities)

	result.Interpretation.References =
		uniqueStrings(result.Interpretation.References)

	result.Interpretation.Constraints =
		uniqueStrings(result.Interpretation.Constraints)

	// Una misma expresión no debería aparecer simultáneamente como
	// entidad y referencia. En caso de conflicto conservamos la entidad,
	// porque una referencia contextual por definición no debería estar
	// completamente identificada dentro de la entrada.
	result.Interpretation.References, result.Warnings =
		removeOverlaps(
			result.Interpretation.References,
			result.Interpretation.Entities,
			result.Warnings,
			"expresión eliminada de references por coincidir con entities",
		)

	// Una restricción no debería ser idéntica a una referencia.
	result.Interpretation.Constraints, result.Warnings =
		removeOverlaps(
			result.Interpretation.Constraints,
			result.Interpretation.References,
			result.Warnings,
			"expresión eliminada de constraints por coincidir con references",
		)

	// Si una referencia aparece literalmente completa dentro de la
	// entrada pero no tiene ninguna señal contextual, la marcamos como
	// sospechosa. No inventamos una entidad; simplemente la eliminamos.
	result.Interpretation.References =
		removeNonContextualReferences(
			input,
			result.Interpretation.References,
			&result.Warnings,
		)

	// Mantener confidence dentro del rango permitido incluso si el
	// validador se usa independientemente de normalizeInterpretation().
	if result.Interpretation.Confidence < 0 {
		result.Interpretation.Confidence = 0
		result.Warnings = append(
			result.Warnings,
			"confidence ajustada al mínimo permitido",
		)
	}

	if result.Interpretation.Confidence > 1 {
		result.Interpretation.Confidence = 1
		result.Warnings = append(
			result.Warnings,
			"confidence ajustada al máximo permitido",
		)
	}

	return result
}

// uniqueStrings elimina duplicados y valores vacíos conservando
// el orden original.
func uniqueStrings(values []string) []string {
	seen := make(map[string]bool)
	result := make([]string, 0, len(values))

	for _, value := range values {
		value = strings.TrimSpace(value)

		if value == "" {
			continue
		}

		key := strings.ToLower(value)

		if seen[key] {
			continue
		}

		seen[key] = true
		result = append(result, value)
	}

	return result
}

// removeOverlaps elimina valores presentes simultáneamente en dos
// categorías.
func removeOverlaps(
	values []string,
	conflicts []string,
	warnings []string,
	message string,
) ([]string, []string) {
	conflictSet := make(map[string]bool)

	for _, conflict := range conflicts {
		conflictSet[strings.ToLower(strings.TrimSpace(conflict))] = true
	}

	result := make([]string, 0, len(values))

	for _, value := range values {
		key := strings.ToLower(strings.TrimSpace(value))

		if conflictSet[key] {
			warnings = append(
				warnings,
				message+": "+value,
			)
			continue
		}

		result = append(result, value)
	}

	return result, warnings
}

// removeNonContextualReferences elimina referencias claramente
// sospechosas.
//
// Esta función NO intenta identificar entidades. Solo busca señales
// mínimas de que una expresión sea realmente contextual.
func removeNonContextualReferences(
	input string,
	references []string,
	warnings *[]string,
) []string {
	result := make([]string, 0, len(references))

	for _, reference := range references {
		if isContextualReference(reference) {
			result = append(result, reference)
			continue
		}

		*warnings = append(
			*warnings,
			"referencia descartada por no mostrar una señal contextual: "+reference,
		)
	}

	return result
}

// isContextualReference comprueba si una expresión contiene señales
// mínimas típicas de dependencia contextual.
//
// No determina qué significa la referencia.
func isContextualReference(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))

	indicators := []string{
		"eso",
		"esto",
		"aquello",
		"lo de ",
		"lo que ",
		"el anterior",
		"la anterior",
		"haz lo mismo",
		"como antes",
		"igual que",
		"ayer",
		"antes",
	}

	for _, indicator := range indicators {
		if strings.Contains(value, indicator) {
			return true
		}
	}

	return false
}
