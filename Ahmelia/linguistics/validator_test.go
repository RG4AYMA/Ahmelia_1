package linguistics

import (
	"testing"

	"ahmelia/comprehension"
)

func TestValidatorValidate(t *testing.T) {
	validator := NewValidator()

	tests := []struct {
		name            string
		input           string
		interpretation  Interpretation
		wantEntities    []string
		wantReferences  []string
		wantConstraints []string
	}{
		{
			name:  "referencia no contextual sospechosa",
			input: "abre Blender",
			interpretation: Interpretation{
				Intent:     comprehension.IntentCommand,
				References: []string{"Blender"},
				Confidence: 1,
			},
			wantEntities:    []string{},
			wantReferences:  []string{},
			wantConstraints: []string{},
		},
		{
			name:  "pregunta no contextual confundida como referencia",
			input: "¿qué hora es?",
			interpretation: Interpretation{
				References: []string{"qué hora es"},
				Confidence: 0.9,
			},
			wantEntities:    []string{},
			wantReferences:  []string{},
			wantConstraints: []string{},
		},
		{
			name:  "referencia contextual valida",
			input: "continúa con lo que hacíamos ayer",
			interpretation: Interpretation{
				Intent:     comprehension.IntentCommand,
				References: []string{"lo que hacíamos ayer"},
				Confidence: 0.9,
			},
			wantEntities:    []string{},
			wantReferences:  []string{"lo que hacíamos ayer"},
			wantConstraints: []string{},
		},
		{
			name:  "solapamiento entidad referencia",
			input: "abre Blender",
			interpretation: Interpretation{
				Intent:     comprehension.IntentCommand,
				Entities:   []string{"Blender"},
				References: []string{"Blender"},
				Confidence: 1,
			},
			wantEntities:    []string{"Blender"},
			wantReferences:  []string{},
			wantConstraints: []string{},
		},
		{
			name:  "duplicados",
			input: "abre Blender",
			interpretation: Interpretation{
				Intent:      comprehension.IntentCommand,
				Entities:    []string{"Blender", "blender", " Blender "},
				References:  []string{},
				Constraints: []string{"no cierres Blender", "no cierres Blender"},
				Confidence:  0.9,
			},
			wantEntities:    []string{"Blender"},
			wantReferences:  []string{},
			wantConstraints: []string{"no cierres Blender"},
		},
		{
			name:  "referencia y restriccion iguales",
			input: "haz lo mismo",
			interpretation: Interpretation{
				Intent:      comprehension.IntentCommand,
				References:  []string{"haz lo mismo"},
				Constraints: []string{"haz lo mismo"},
				Confidence:  0.9,
			},
			wantEntities:    []string{},
			wantReferences:  []string{"haz lo mismo"},
			wantConstraints: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validator.Validate(tt.input, tt.interpretation)

			got := result.Interpretation

			if !equalStrings(got.Entities, tt.wantEntities) {
				t.Errorf(
					"Entities = %v, esperado %v",
					got.Entities,
					tt.wantEntities,
				)
			}

			if !equalStrings(got.References, tt.wantReferences) {
				t.Errorf(
					"References = %v, esperado %v",
					got.References,
					tt.wantReferences,
				)
			}

			if !equalStrings(got.Constraints, tt.wantConstraints) {
				t.Errorf(
					"Constraints = %v, esperado %v",
					got.Constraints,
					tt.wantConstraints,
				)
			}

			t.Logf(
				"Input: %q | Entities: %v | References: %v | Constraints: %v | Warnings: %v",
				tt.input,
				got.Entities,
				got.References,
				got.Constraints,
				result.Warnings,
			)
		})
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}

func TestValidatorPreservesStructuredRelations(t *testing.T) {
	v := NewValidator()
	result := v.Validate("pronto podrás ver el mundo", Interpretation{Relations: []Relation{{Source: "user", Predicate: "anticipates", Target: "future_state", Confidence: .94}}})
	if len(result.Interpretation.Relations) != 1 {
		t.Fatalf("relations=%d", len(result.Interpretation.Relations))
	}
	if result.Interpretation.Relations[0].Predicate != "anticipates" {
		t.Fatalf("predicate=%q", result.Interpretation.Relations[0].Predicate)
	}
}
