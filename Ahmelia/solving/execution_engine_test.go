package solving

import (
	"os"
	"path/filepath"
	"testing"
)

// TestExecute_ActionNoEjecutable confirma el fail-safe central: una
// Action que no está en executableActions (ej. abrir_programa, que
// vive solo en core/engine.go atada a estado de sesión) nunca se
// ejecuta acá, sin importar Confirmed.
func TestExecute_ActionNoEjecutable(t *testing.T) {
	res := Execute(ExecutionRequest{Action: "abrir_programa", Confirmed: true})
	if res.Outcome != OutcomeNotExecutable {
		t.Errorf("esperaba OutcomeNotExecutable, dio %q", res.Outcome)
	}
	if res.Executed() {
		t.Error("abrir_programa nunca debería ejecutarse desde este motor")
	}
}

// TestExecute_ActionSinClasificar confirma que una Action que ni
// siquiera está en shared.ActionSafety cae al default cauto (fail-safe
// de Bloque 65) — acá además ni siquiera está en executableActions, así
// que el resultado real es NotExecutable, pero el punto es que jamás
// se ejecuta.
func TestExecute_ActionSinClasificar(t *testing.T) {
	res := Execute(ExecutionRequest{Action: "instalar_paquete", Confirmed: false})
	if res.Executed() {
		t.Error("una Action sin clasificar jamás debería ejecutarse")
	}
}

// TestExecute_CrearCarpeta_FaltaParametro confirma que falta el
// parámetro requerido se reporta como tal, no como fallo de sistema.
func TestExecute_CrearCarpeta_FaltaParametro(t *testing.T) {
	res := Execute(ExecutionRequest{Action: "crear_carpeta", Params: map[string]string{}})
	if res.Outcome != OutcomeMissingParams {
		t.Errorf("esperaba OutcomeMissingParams, dio %q", res.Outcome)
	}
}

// TestExecute_CrearCarpeta_Exito confirma el camino feliz completo:
// crear_carpeta es Safe (Bloque 65) → RequiresConfirmation/
// RequiresAuthorization dan false → se ejecuta sin necesitar Confirmed.
func TestExecute_CrearCarpeta_Exito(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "PDF")

	res := Execute(ExecutionRequest{
		Action: "crear_carpeta",
		Params: map[string]string{"ruta": target},
	})
	if !res.Executed() {
		t.Fatalf("esperaba ejecución exitosa, dio %q (%s)", res.Outcome, res.Detail)
	}
	if info, err := os.Stat(target); err != nil || !info.IsDir() {
		t.Errorf("la carpeta no se creó de verdad en %s", target)
	}
}

// TestExecute_MoverArchivo_Exito confirma mover_archivo (Reversible,
// tampoco necesita Confirmed según la política declarada en 65).
func TestExecute_MoverArchivo_Exito(t *testing.T) {
	base := t.TempDir()
	src := filepath.Join(base, "informe.pdf")
	if err := os.WriteFile(src, []byte("contenido"), 0644); err != nil {
		t.Fatal(err)
	}
	destDir := filepath.Join(base, "PDF")
	if err := os.MkdirAll(destDir, 0755); err != nil {
		t.Fatal(err)
	}

	res := Execute(ExecutionRequest{
		Action: "mover_archivo",
		Params: map[string]string{"origen": src, "destino": destDir},
	})
	if !res.Executed() {
		t.Fatalf("esperaba ejecución exitosa, dio %q (%s)", res.Outcome, res.Detail)
	}
	if _, err := os.Stat(filepath.Join(destDir, "informe.pdf")); err != nil {
		t.Errorf("el archivo no terminó en el destino esperado: %v", err)
	}
}

// TestExecute_RenombrarArchivo_Exito confirma que renombrar_archivo comparte
// el mismo contrato canónico y tiene un binding standalone real.
func TestExecute_RenombrarArchivo_Exito(t *testing.T) {
	base := t.TempDir()
	src := filepath.Join(base, "viejo.txt")
	if err := os.WriteFile(src, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	res := Execute(ExecutionRequest{
		Action: "renombrar_archivo",
		Params: map[string]string{"ruta": src, "nombre_nuevo": "nuevo.txt"},
	})
	if !res.Executed() {
		t.Fatalf("esperaba ejecución exitosa, dio %q (%s)", res.Outcome, res.Detail)
	}
	if _, err := os.Stat(filepath.Join(base, "nuevo.txt")); err != nil {
		t.Errorf("el archivo renombrado no existe: %v", err)
	}
}

// TestExecute_MoverArchivo_ErrorDeFiles confirma que un error real de
// files/ (origen inexistente) se reporta como OutcomeFailed, no como
// un panic ni un OutcomeExecuted falso.
func TestExecute_MoverArchivo_ErrorDeFiles(t *testing.T) {
	base := t.TempDir()
	res := Execute(ExecutionRequest{
		Action: "mover_archivo",
		Params: map[string]string{
			"origen":  filepath.Join(base, "no-existe.pdf"),
			"destino": base,
		},
	})
	if res.Outcome != OutcomeFailed {
		t.Errorf("esperaba OutcomeFailed, dio %q", res.Outcome)
	}
	if res.Executed() {
		t.Error("un error de files/ nunca debería reportarse como ejecutado")
	}
}

// TestExecutableActions_TodasTienenTierClasificado — auditoría cruzada
// con Bloque 65: cada Action que este motor sabe ejecutar debe tener
// un SafetyTier asignado a mano en shared.ActionSafety, para que la
// política de confirmación/autorización sea una decisión consciente y
// no el default cauto por descuido.
func TestExecutableActions_TodasTienenTierClasificado(t *testing.T) {
	for action := range executableActions {
		res := Execute(ExecutionRequest{Action: action, Params: map[string]string{}})
		if res.Tier == "" {
			t.Errorf("Action ejecutable %q no tiene SafetyTier clasificado en shared.ActionSafety", action)
		}
	}
}
