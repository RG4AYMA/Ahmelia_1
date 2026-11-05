package shared

import "testing"

// TestActionSafety_Auditoria_LasTreceWiredEstanClasificadas confirma
// que ninguna de las 13 Action realmente conectadas (WiredActions)
// quedó sin clasificar por descuido — mismo criterio de auditoría que
// TestWiredActions_* ya aplica en este paquete.
func TestActionSafety_Auditoria_LasTreceWiredEstanClasificadas(t *testing.T) {
	for action := range WiredActions {
		if _, ok := ActionSafety[action]; !ok {
			t.Errorf("Action Wired %q no tiene tier de seguridad asignado — cae al default cauto por descuido, no por decisión", action)
		}
	}
}

// TestActionSafety_NingunaWiredEsDestructiveOSensitive documenta el
// hallazgo real de la auditoría: hoy no hay ninguna acción destructiva
// ni sensible conectada. Si este test se rompe algún día, es una señal
// real de que se conectó una Action más seria y merece revisión
// consciente, no un ajuste automático.
func TestActionSafety_NingunaWiredEsDestructiveOSensitive(t *testing.T) {
	for action := range WiredActions {
		tier := ActionSafety[action]
		if tier == SafetyTierDestructive || tier == SafetyTierSensitive {
			t.Errorf("Action Wired %q está clasificada como %q — si esto es intencional, está bien, pero confirmalo revisando este test, no lo dejes pasar en silencio", action, tier)
		}
	}
}

// TestRequiresConfirmation_SafeYReversibleNoPreguntan confirma la
// política declarada: estos dos tiers ejecutan sin pedir confirmación.
func TestRequiresConfirmation_SafeYReversibleNoPreguntan(t *testing.T) {
	if RequiresConfirmation("abrir_programa") { // Safe
		t.Error("una Action Safe no debería requerir confirmación")
	}
	if RequiresConfirmation("mover_archivo") { // Reversible
		t.Error("una Action Reversible no debería requerir confirmación")
	}
}

// TestRequiresConfirmation_DestructiveYSensitiveSiPreguntan confirma
// que ambos tiers de riesgo alto exigen confirmación.
func TestRequiresConfirmation_DestructiveYSensitiveSiPreguntan(t *testing.T) {
	ActionSafety["__test_destructive__"] = SafetyTierDestructive
	ActionSafety["__test_sensitive__"] = SafetyTierSensitive
	defer delete(ActionSafety, "__test_destructive__")
	defer delete(ActionSafety, "__test_sensitive__")

	if !RequiresConfirmation("__test_destructive__") {
		t.Error("una Action Destructive debería requerir confirmación")
	}
	if !RequiresConfirmation("__test_sensitive__") {
		t.Error("una Action Sensitive debería requerir confirmación")
	}
}

// TestRequiresAuthorization_SoloSensitiveLaExige confirma el nivel más
// estricto: ni Safe, ni Reversible, ni Destructive la disparan — solo
// Sensitive.
func TestRequiresAuthorization_SoloSensitiveLaExige(t *testing.T) {
	ActionSafety["__test_destructive2__"] = SafetyTierDestructive
	ActionSafety["__test_sensitive2__"] = SafetyTierSensitive
	defer delete(ActionSafety, "__test_destructive2__")
	defer delete(ActionSafety, "__test_sensitive2__")

	if RequiresAuthorization("abrir_programa") {
		t.Error("Safe no debería requerir autorización")
	}
	if RequiresAuthorization("mover_archivo") {
		t.Error("Reversible no debería requerir autorización")
	}
	if RequiresAuthorization("__test_destructive2__") {
		t.Error("Destructive por sí sola no debería requerir autorización (solo confirmación)")
	}
	if !RequiresAuthorization("__test_sensitive2__") {
		t.Error("Sensitive debería requerir autorización")
	}
}

// TestRequiresConfirmation_ActionSinClasificarEsCautaPorDefecto
// (fail-safe, no fail-open) confirma que una Action que no aparece en
// ActionSafety se trata como si necesitara confirmación Y autorización
// — nunca se asume "segura" por default.
func TestRequiresConfirmation_ActionSinClasificarEsCautaPorDefecto(t *testing.T) {
	if !RequiresConfirmation("__accion_jamas_clasificada__") {
		t.Error("una Action no clasificada debería requerir confirmación por defecto (fail-safe)")
	}
	if !RequiresAuthorization("__accion_jamas_clasificada__") {
		t.Error("una Action no clasificada debería requerir autorización por defecto (fail-safe)")
	}
	if SafetyTierOf("__accion_jamas_clasificada__") != "" {
		t.Error("SafetyTierOf de una Action no clasificada debería devolver \"\" — nunca inventar un tier")
	}
}
