package autonomy

import (
	"context"
	"errors"
	"testing"
	"time"
)

type mockEnvironmentInspector struct {
	observation EnvironmentObservation
	err         error
	called      bool
}

func (m *mockEnvironmentInspector) InspectAction(
	ctx context.Context,
	req ActionRequest,
) (EnvironmentObservation, error) {
	m.called = true
	return m.observation, m.err
}

func TestEnvironmentVerifier_Confirmed(t *testing.T) {
	inspector := &mockEnvironmentInspector{
		observation: EnvironmentObservation{
			Confirmed: true,
			Details:   "la carpeta existe",
			Status:    ObservationConfirmed,
			ActionID:  "b80-1", Operation: "crear_carpeta", Target: "test", Source: "test.filesystem", ObservedAt: time.Now().UTC(),
			Facts: []ObservationFact{{Subject: "test", Predicate: "filesystem.object_type", Expected: "directory", Observed: "directory"}},
		},
	}

	verifier := NewEnvironmentVerifier(inspector)

	result := verifier.Verify(
		context.Background(),
		ActionRequest{
			ID:     "b80-1",
			Action: "crear_carpeta",
			Target: "test",
		},
		VerificationResult{
			Success: true,
			Output:  "ejecución completada",
		},
	)

	if !result.Success {
		t.Fatalf("se esperaba éxito, error: %s", result.ErrorReason)
	}

	if !result.Observed {
		t.Fatal("se esperaba evidencia del entorno")
	}

	if !inspector.called {
		t.Fatal("se esperaba consultar el inspector")
	}
	if result.ObservationStatus != ObservationConfirmed {
		t.Fatalf("observation status=%q, want %q", result.ObservationStatus, ObservationConfirmed)
	}
}

func TestEnvironmentVerifier_EnvironmentNotConfirmed(t *testing.T) {
	inspector := &mockEnvironmentInspector{
		observation: EnvironmentObservation{
			Confirmed: false,
			Details:   "la carpeta no existe",
			Status:    ObservationContradicted,
		},
	}

	verifier := NewEnvironmentVerifier(inspector)

	result := verifier.Verify(
		context.Background(),
		ActionRequest{
			ID:     "b80-2",
			Action: "crear_carpeta",
			Target: "test",
		},
		VerificationResult{
			Success: true,
			Output:  "ejecución completada",
		},
	)

	if result.Success {
		t.Fatal("no se esperaba éxito cuando el entorno no confirma la acción")
	}

	if !result.Observed {
		t.Fatal("se esperaba que el entorno hubiera sido observado")
	}
	if result.ObservationStatus != ObservationContradicted {
		t.Fatalf("observation status=%q, want %q", result.ObservationStatus, ObservationContradicted)
	}
}

func TestEnvironmentVerifier_TechnicalFailure(t *testing.T) {
	inspector := &mockEnvironmentInspector{}

	verifier := NewEnvironmentVerifier(inspector)

	result := verifier.Verify(
		context.Background(),
		ActionRequest{
			ID:     "b80-3",
			Action: "crear_carpeta",
		},
		VerificationResult{
			Success:     false,
			Output:      "salida",
			ErrorReason: "fallo de ejecución",
		},
	)

	if result.Success {
		t.Fatal("una ejecución técnica fallida no puede ser éxito")
	}

	if inspector.called {
		t.Fatal("no debería inspeccionarse el entorno después de un fallo técnico")
	}
	if result.ObservationStatus != ObservationNotObserved {
		t.Fatalf("observation status=%q, want %q", result.ObservationStatus, ObservationNotObserved)
	}
}

func TestEnvironmentVerifier_InspectorError(t *testing.T) {
	inspector := &mockEnvironmentInspector{
		err: errors.New("fallo de inspección"),
	}

	verifier := NewEnvironmentVerifier(inspector)

	result := verifier.Verify(
		context.Background(),
		ActionRequest{
			ID:     "b80-4",
			Action: "crear_carpeta",
		},
		VerificationResult{
			Success: true,
			Output:  "ejecutado",
		},
	)

	if result.Success {
		t.Fatal("un error del inspector no puede producir éxito")
	}

	if result.ErrorReason == "" {
		t.Fatal("se esperaba una razón del error")
	}
	if result.ObservationStatus != ObservationInsufficient {
		t.Fatalf("observation status=%q, want %q", result.ObservationStatus, ObservationInsufficient)
	}
}

func TestEnvironmentVerifier_NoInspector(t *testing.T) {
	verifier := NewEnvironmentVerifier(nil)

	result := verifier.Verify(
		context.Background(),
		ActionRequest{
			ID:     "b80-5",
			Action: "crear_carpeta",
		},
		VerificationResult{
			Success: true,
			Output:  "ejecutado",
		},
	)

	if result.Success {
		t.Fatal("sin inspector no se debe declarar éxito ambiental")
	}
	if result.ObservationStatus != ObservationInsufficient {
		t.Fatalf("observation status=%q, want %q", result.ObservationStatus, ObservationInsufficient)
	}
}

func TestEnvironmentVerifierPreservesContradictionWithoutDetailsHeuristic(t *testing.T) {
	inspector := &mockEnvironmentInspector{observation: EnvironmentObservation{
		Confirmed: false,
		Status:    ObservationContradicted,
	}}
	result := NewEnvironmentVerifier(inspector).Verify(context.Background(), ActionRequest{ID: "b80-state", Action: "x"}, VerificationResult{Success: true})
	if result.ObservationStatus != ObservationContradicted {
		t.Fatalf("observation status=%q, want explicit contradicted state", result.ObservationStatus)
	}
}

func TestEnvironmentVerifierDoesNotInferContradictionFromDetails(t *testing.T) {
	inspector := &mockEnvironmentInspector{observation: EnvironmentObservation{
		Confirmed: false,
		Details:   "No se pudo consultar el directorio",
		Status:    ObservationNotObserved,
	}}
	result := NewEnvironmentVerifier(inspector).Verify(
		context.Background(),
		ActionRequest{ID: "evidence-not-observed", Action: "check_file"},
		VerificationResult{Success: true, Output: "execution returned"},
	)
	if result.Success {
		t.Fatal("un resultado no observado no puede considerarse verificado")
	}
	if result.ObservationStatus != ObservationNotObserved {
		t.Fatalf("observation status=%q, want %q", result.ObservationStatus, ObservationNotObserved)
	}
}

func TestEnvironmentVerifierLegacyUnclassifiedDetailsRemainInsufficient(t *testing.T) {
	inspector := &mockEnvironmentInspector{observation: EnvironmentObservation{
		Confirmed: false,
		Details:   "No se pudo consultar el directorio",
	}}
	result := NewEnvironmentVerifier(inspector).Verify(
		context.Background(),
		ActionRequest{ID: "evidence-legacy-insufficient", Action: "check_file"},
		VerificationResult{Success: true, Output: "execution returned"},
	)
	if result.Success {
		t.Fatal("evidencia no clasificada no puede considerarse verificada")
	}
	if result.ObservationStatus != ObservationInsufficient {
		t.Fatalf("observation status=%q, want %q", result.ObservationStatus, ObservationInsufficient)
	}
}

func TestEnvironmentVerifierContradictedStatusCannotBeOverriddenByConfirmedFlag(t *testing.T) {
	inspector := &mockEnvironmentInspector{observation: EnvironmentObservation{
		Confirmed: true,
		Details:   "el estado observado contradice la condición esperada",
		Status:    ObservationContradicted,
	}}
	result := NewEnvironmentVerifier(inspector).Verify(
		context.Background(),
		ActionRequest{ID: "evidence-conflicting-flags", Action: "check_file"},
		VerificationResult{Success: true, Output: "execution returned"},
	)
	if result.Success {
		t.Fatal("un estado explícitamente contradicho no puede convertirse en éxito")
	}
	if result.ObservationStatus != ObservationContradicted {
		t.Fatalf("observation status=%q, want %q", result.ObservationStatus, ObservationContradicted)
	}
}

func TestEnvironmentVerifierInsufficientStatusCannotBeOverriddenByConfirmedFlag(t *testing.T) {
	inspector := &mockEnvironmentInspector{observation: EnvironmentObservation{
		Confirmed: true,
		Details:   "la inspección no tuvo cobertura suficiente",
		Status:    ObservationInsufficient,
	}}
	result := NewEnvironmentVerifier(inspector).Verify(
		context.Background(),
		ActionRequest{ID: "evidence-insufficient-conflict", Action: "check_file"},
		VerificationResult{Success: true, Output: "execution returned"},
	)
	if result.Success {
		t.Fatal("evidencia insuficiente no puede convertirse en éxito por un booleano")
	}
	if result.ObservationStatus != ObservationInsufficient {
		t.Fatalf("observation status=%q, want %q", result.ObservationStatus, ObservationInsufficient)
	}
}

func TestEnvironmentVerifierRejectsConfirmedFlagWithoutStructuredFacts(t *testing.T) {
	inspector := &mockEnvironmentInspector{observation: EnvironmentObservation{Confirmed: true, Status: ObservationConfirmed, Details: "text claims success"}}
	result := NewEnvironmentVerifier(inspector).Verify(context.Background(), ActionRequest{ID: "no-facts", Action: "crear_carpeta", Target: "test"}, VerificationResult{Success: true})
	if result.Success {
		t.Fatal("confirmed flag and free text must not substitute for structured postcondition facts")
	}
	if result.ObservationStatus != ObservationInsufficient {
		t.Fatalf("status=%q, want insufficient when structured evidence is absent", result.ObservationStatus)
	}
}

func TestEnvironmentVerifierRejectsStaleOrMismatchedEvidence(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*EnvironmentObservation)
	}{
		{name: "stale", mutate: func(o *EnvironmentObservation) { o.ObservedAt = time.Now().UTC().Add(-time.Minute) }},
		{name: "wrong action", mutate: func(o *EnvironmentObservation) { o.ActionID = "previous-action" }},
		{name: "wrong target", mutate: func(o *EnvironmentObservation) { o.Target = "another-resource" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := EnvironmentObservation{Confirmed: true, Status: ObservationConfirmed, ActionID: "current", Operation: "crear_carpeta", Target: "resource", Source: "filesystem", ObservedAt: time.Now().UTC(), Facts: []ObservationFact{{Subject: "resource", Predicate: "filesystem.object_type", Expected: "directory", Observed: "directory"}}}
			tc.mutate(&o)
			result := NewEnvironmentVerifier(&mockEnvironmentInspector{observation: o}).Verify(context.Background(), ActionRequest{ID: "current", Action: "crear_carpeta", Target: "resource"}, VerificationResult{Success: true})
			if result.Success || result.Observed || result.ObservationStatus != ObservationInsufficient {
				t.Fatalf("invalid provenance should not be current evidence: %+v", result)
			}
		})
	}
}
