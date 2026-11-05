package solving

import (
	"os"
	"path/filepath"
	"testing"
)

// TestDiagnose_SinFallaNoInventaCausa confirma que sobre un
// ExecutionResult que no falló, no se fabrica un diagnóstico.
func TestDiagnose_SinFallaNoInventaCausa(t *testing.T) {
	ok := &ExecutionResult{Action: "crear_carpeta", Outcome: OutcomeExecuted}
	d := Diagnose(ok, nil)
	if d.Category != "" {
		t.Errorf("esperaba categoría vacía (no hubo falla), dio %q", d.Category)
	}
}

func TestDiagnose_NilExecResult(t *testing.T) {
	d := Diagnose(nil, nil)
	if d.Category != "" {
		t.Errorf("esperaba categoría vacía con execResult nil, dio %q", d.Category)
	}
}

// TestDiagnose_MoverArchivo_YaExiste usa una falla REAL (no
// fabricada): mover a un destino que ya tiene un archivo con ese
// nombre, tal como lo rechaza files.MoveFile.
func TestDiagnose_MoverArchivo_YaExiste(t *testing.T) {
	base := t.TempDir()
	src := filepath.Join(base, "informe.pdf")
	os.WriteFile(src, []byte("x"), 0644)
	dest := filepath.Join(base, "ya-esta.pdf")
	os.WriteFile(dest, []byte("y"), 0644)

	res := Execute(ExecutionRequest{Action: "mover_archivo", Params: map[string]string{"origen": src, "destino": dest}})
	if res.Outcome != OutcomeFailed {
		t.Fatalf("setup: esperaba que la ejecución fallara, dio %q", res.Outcome)
	}

	d := Diagnose(res, nil)
	if d.Category != FailureAlreadyExists {
		t.Errorf("esperaba FailureAlreadyExists, dio %q (raw: %s)", d.Category, d.RawError)
	}
	if d.AutoRetryable {
		t.Error("ya_existe nunca debería marcarse como reintentable automáticamente")
	}
}

// TestDiagnose_MoverArchivo_OrigenNoExiste cubre la otra categoría
// real.
func TestDiagnose_MoverArchivo_OrigenNoExiste(t *testing.T) {
	base := t.TempDir()
	res := Execute(ExecutionRequest{
		Action: "mover_archivo",
		Params: map[string]string{"origen": filepath.Join(base, "fantasma.pdf"), "destino": base},
	})
	if res.Outcome != OutcomeFailed {
		t.Fatalf("setup: esperaba fallo, dio %q", res.Outcome)
	}

	d := Diagnose(res, nil)
	if d.Category != FailureNotFound {
		t.Errorf("esperaba FailureNotFound, dio %q (raw: %s)", d.Category, d.RawError)
	}
}

// TestDiagnose_CrearCarpeta_YaExiste confirma la misma categoría
// desde una Action distinta — la clasificación es por MENSAJE, no por
// Action.
func TestDiagnose_CrearCarpeta_YaExiste(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "Proyectos")
	os.MkdirAll(target, 0755)

	res := Execute(ExecutionRequest{Action: "crear_carpeta", Params: map[string]string{"ruta": target}})
	if res.Outcome != OutcomeFailed {
		t.Fatalf("setup: esperaba fallo, dio %q", res.Outcome)
	}

	d := Diagnose(res, nil)
	if d.Category != FailureAlreadyExists {
		t.Errorf("esperaba FailureAlreadyExists, dio %q (raw: %s)", d.Category, d.RawError)
	}
}

// TestDiagnose_SinFailureRecoveryEnLaSkill confirma que sin Skill (o
// con Skill sin FailureRecovery), HasSkillRecovery queda false — no
// se inventa una recuperación que la fuente nunca documentó.
func TestDiagnose_SinFailureRecoveryEnLaSkill(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "Proyectos")
	os.MkdirAll(target, 0755)
	res := Execute(ExecutionRequest{Action: "crear_carpeta", Params: map[string]string{"ruta": target}})

	d := Diagnose(res, &Skill{Label: "Crear proyecto"}) // sin FailureRecovery
	if d.HasSkillRecovery {
		t.Error("no debería haber recuperación conocida: la Skill no trae FailureRecovery")
	}
}

// TestDiagnose_ConFailureRecoveryEnLaSkill confirma que, cuando SÍ
// existe, se adjunta tal cual — y que nunca se marca AutoRetryable
// solo porque hay texto de recuperación disponible (texto libre no
// estructurado no habilita auto-ejecución).
func TestDiagnose_ConFailureRecoveryEnLaSkill(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "Proyectos")
	os.MkdirAll(target, 0755)
	res := Execute(ExecutionRequest{Action: "crear_carpeta", Params: map[string]string{"ruta": target}})

	skill := &Skill{
		Label:           "Crear proyecto",
		FailureRecovery: []string{"si la carpeta ya existe, usá un nombre con sufijo numérico"},
	}
	d := Diagnose(res, skill)
	if !d.HasSkillRecovery {
		t.Fatal("esperaba HasSkillRecovery true")
	}
	if len(d.SkillRecovery) != 1 || d.SkillRecovery[0] != "si la carpeta ya existe, usá un nombre con sufijo numérico" {
		t.Errorf("SkillRecovery no coincide con lo que trae la Skill: %v", d.SkillRecovery)
	}
	if d.AutoRetryable {
		t.Error("tener FailureRecovery en texto no debería habilitar AutoRetryable — sigue sin ser una acción estructurada")
	}

	desc := d.Describe()
	if desc == "" {
		t.Error("Describe() no debería devolver vacío para una falla real diagnosticada")
	}
}
