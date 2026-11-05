package integration

import (
	"testing"

	"ahmelia/situation"
)

// Aceptación del consumo P4 (directiva Paso 2): la perspectiva de preferencias
// formadas convierte cada stance en UNA DecisionFinding legible por las
// propuestas, semánticamente DERIVADA.
func TestPreferencePerspectiveConvierteStancesEnFindings(t *testing.T) {
	sit := situation.Situation{ID: "sit-test-pref"}
	c := PreferencePerspective(sit, []PreferenceStancePort{
		{Key: "gaseosa", Stance: "likes", Mean: .8},
	})
	if len(c.Relations) != 1 {
		t.Fatalf("expected 1 relation por stance, got %d", len(c.Relations))
	}
	if c.Confidence <= 0 {
		t.Fatalf("la lectura debe fijar su propia confianza (min01 la aplica): %.2f", c.Confidence)
	}
	ev := situation.Evaluate(sit, []situation.Contribution{c})
	if len(ev.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d: %+v", len(ev.Findings), ev.Findings)
	}
	f := ev.Findings[0]
	if f.RelationType != "reflects_user_preference" || f.Target != "gaseosa" || f.Subject != "user" {
		t.Fatalf("finding con relación incorrecta: %+v", f)
	}
	if f.Epistemic != situation.EpistemicDerived {
		t.Fatalf("el stance posterior es DERIVADO, nunca observado canónico: %+v", f)
	}
	if f.Confidence <= .5 {
		t.Fatalf("la confianza del stance debe sobrevivir min01: %.2f", f.Confidence)
	}
	if f.Perspective != "preference_stance" {
		t.Fatalf("proveniencia de la lectura faltante: %q", f.Perspective)
	}
}

// Sin stances formados la perspectiva no informa cero — la ausencia no se
// disimula: cero contribution-level relations.
func TestPreferencePerspectiveSinDatosNoInventa(t *testing.T) {
	sit := situation.Situation{ID: "sit-test-pref"}
	c := PreferencePerspective(sit, nil)
	if len(c.Relations) != 0 {
		t.Fatalf("sin datos no debe haber relaciones inventadas: %+v", c.Relations)
	}
}
