package memory_intelligence

import "testing"

// TestAdaptStrategy_EscalaSegunConfianza simula el ciclo completo de
// Bloque 8: ActionConfidence baja → AdaptStrategy escala de Primary a
// Secondary a Fallback, sin pasar por EventLog/disco (eso ya lo prueba
// engine.go en runtime real).
func TestAdaptStrategy_EscalaSegunConfianza(t *testing.T) {
	ls := NewLearningState()

	// Confianza alta (>=40%) -> Primary
	ls.ActionConfidence["crear_carpeta"] = 75
	if got := ls.AdaptStrategy("crear_carpeta"); got != "directo" {
		t.Fatalf("con 75%% de confianza esperaba 'directo', obtuve %q", got)
	}

	// Confianza media (<40%) -> Secondary
	ls.ActionConfidence["crear_carpeta"] = 35
	if got := ls.AdaptStrategy("crear_carpeta"); got != "con powershell" {
		t.Fatalf("con 35%% de confianza esperaba 'con powershell', obtuve %q", got)
	}

	// Confianza muy baja (<20%) -> Fallback
	ls.ActionConfidence["crear_carpeta"] = 10
	if got := ls.AdaptStrategy("crear_carpeta"); got != "validar permisos" {
		t.Fatalf("con 10%% de confianza esperaba 'validar permisos', obtuve %q", got)
	}
}

// TestNoteOutcome_CuentaAdaptacionesExitosas verifica que cambiar de
// estrategia y que funcione (no Primary) sume a SuccessfulAdaptations,
// y que repetir la Primary con éxito NO cuente como adaptación.
func TestNoteOutcome_CuentaAdaptacionesExitosas(t *testing.T) {
	ls := NewLearningState()

	ls.NoteOutcome("crear_carpeta", "directo", true)
	if ls.SuccessfulAdaptations != 0 {
		t.Fatalf("usar la estrategia Primary con éxito no debería contar como adaptación, obtuve %d", ls.SuccessfulAdaptations)
	}

	ls.NoteOutcome("crear_carpeta", "con powershell", true)
	if ls.SuccessfulAdaptations != 1 {
		t.Fatalf("esperaba 1 adaptación exitosa, obtuve %d", ls.SuccessfulAdaptations)
	}

	// Estrategia alternativa que falla no debería sumar.
	ls.NoteOutcome("crear_carpeta", "validar permisos", false)
	if ls.SuccessfulAdaptations != 1 {
		t.Fatalf("una estrategia fallida no debería sumar, obtuve %d", ls.SuccessfulAdaptations)
	}

	if ls.Strategies["crear_carpeta"].LastUsed != "validar permisos" {
		t.Fatalf("LastUsed debería reflejar el último intento aunque haya fallado, obtuve %q", ls.Strategies["crear_carpeta"].LastUsed)
	}
	if ls.Strategies["crear_carpeta"].SuccessBy != "con powershell" {
		t.Fatalf("SuccessBy debería quedar en la última estrategia que SÍ funcionó, obtuve %q", ls.Strategies["crear_carpeta"].SuccessBy)
	}
}

// TestUpdateFromReflection_AlimentaAdaptStrategy simula el flujo real:
// varios fallos consecutivos en el EventLog bajan ActionConfidence lo
// suficiente como para que AdaptStrategy escale de estrategia sola, sin
// que nadie la fuerce a mano.
func TestUpdateFromReflection_AlimentaAdaptStrategy(t *testing.T) {
	ls := NewLearningState()
	log := &EventLog{}

	// 1 éxito, 9 fallos -> 10% de confianza -> debería escalar a Fallback
	// (SuggestStrategyChange usa "< 20", así que tiene que quedar
	// estrictamente por debajo).
	log.Events = append(log.Events, RawEvent{Action: "crear_carpeta", Outcome: Outcome{Success: true, EvidenceStatus: EvidenceConfirmed}})
	for i := 0; i < 9; i++ {
		log.Events = append(log.Events, RawEvent{Action: "crear_carpeta", Outcome: Outcome{Success: false, EvidenceStatus: EvidenceContradicted, Category: "file_not_found"}})
	}

	ls.UpdateFromReflection(log)

	if got := ls.ActionConfidence["crear_carpeta"]; got != 10 {
		t.Fatalf("esperaba 10%% de confianza, obtuve %.0f%%", got)
	}
	if got := ls.AdaptStrategy("crear_carpeta"); got != "validar permisos" {
		t.Fatalf("con 20%% de confianza esperaba escalar a Fallback ('validar permisos'), obtuve %q", got)
	}
}

func TestUpdateFromReflection_UnverifiedOutcomesDoNotChangeCompetence(t *testing.T) {
	ls := NewLearningState()
	ls.ActionConfidence["crear_carpeta"] = 92
	log := &EventLog{Events: []RawEvent{
		{Action: "crear_carpeta", Outcome: Outcome{Success: true, EvidenceStatus: EvidenceInsufficient}},
		{Action: "crear_carpeta", Outcome: Outcome{Success: true}}, // histórico sin evidencia
	}}

	ls.UpdateFromReflection(log)

	if _, exists := ls.ActionConfidence["crear_carpeta"]; exists {
		t.Fatalf("una ejecución sin observación conservó confianza: %.1f", ls.ActionConfidence["crear_carpeta"])
	}
}

func TestUpdateFromReflection_ContradictedEvidenceLowersConfidence(t *testing.T) {
	ls := NewLearningState()
	log := &EventLog{Events: []RawEvent{
		{Action: "crear_carpeta", Outcome: Outcome{Success: true, EvidenceStatus: EvidenceConfirmed}},
		{Action: "crear_carpeta", Outcome: Outcome{Success: false, EvidenceStatus: EvidenceContradicted, Category: "effect_missing"}},
	}}

	ls.UpdateFromReflection(log)

	if got := ls.ActionConfidence["crear_carpeta"]; got != 50 {
		t.Fatalf("confianza derivada de una evidencia confirmada y otra refutada = %.1f; se esperaba 50", got)
	}
	if got := ls.FailurePatternsByAction["crear_carpeta"]["effect_missing"]; got != 1 {
		t.Fatalf("patrón de fallo observado = %d; se esperaba 1", got)
	}
}
