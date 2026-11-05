package solving

import (
	"os"
	"path/filepath"
	"testing"
)

// TestVerify_NoAplicaSiNuncaSeEjecuto confirma que sin ejecución real
// no se inventa una verificación.
func TestVerify_NoAplicaSiNuncaSeEjecuto(t *testing.T) {
	blocked := &ExecutionResult{Action: "crear_carpeta", Outcome: OutcomeNeedsConfirmation}
	res := Verify("crear_carpeta", map[string]string{"ruta": "/no/importa"}, blocked, nil)
	if res.Outcome != VerificationNotApplicable {
		t.Errorf("esperaba VerificationNotApplicable, dio %q", res.Outcome)
	}
}

// TestVerify_SinVerificador confirma el fail-safe: una Action
// ejecutada pero sin case de postcondición nunca se reporta como
// confirmada por default.
func TestVerify_SinVerificador(t *testing.T) {
	fake := &ExecutionResult{Action: "algo_nuevo", Outcome: OutcomeExecuted}
	res := Verify("algo_nuevo", map[string]string{}, fake, nil)
	if res.Outcome != VerificationUnchecked {
		t.Errorf("esperaba VerificationUnchecked, dio %q", res.Outcome)
	}
	if res.Confirmed() {
		t.Error("nunca debería reportarse Confirmed sin un check real")
	}
}

// TestVerify_CrearCarpeta_Confirmado es el ejemplo exacto que
// motivó el bloque: no basta con "no hubo error", hay que confirmar
// que la carpeta existe de verdad.
func TestVerify_CrearCarpeta_Confirmado(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "Proyectos")
	execResult := Execute(ExecutionRequest{Action: "crear_carpeta", Params: map[string]string{"ruta": target}})

	res := Verify("crear_carpeta", map[string]string{"ruta": target}, execResult, nil)
	if !res.Confirmed() {
		t.Fatalf("esperaba confirmado, dio %q (%s)", res.Outcome, res.Detail)
	}
	if len(res.Facts) != 1 || res.Facts[0].Subject != target || res.Facts[0].Predicate != "filesystem.object_type" || res.Facts[0].Expected != "directory" || res.Facts[0].Observed != "directory" {
		t.Fatalf("structured postcondition fact incomplete: %+v", res.Facts)
	}
}

// TestVerify_CrearCarpeta_NoConfirmadoSiAlguienLaBorroDespues es el
// caso central que justifica este bloque entero: Execute() dijo
// OutcomeExecuted (no hubo error de Go), pero el mundo real ya no
// coincide — acá se simula borrando la carpeta después de ejecutar,
// como stand-in de cualquier condición de carrera externa.
func TestVerify_CrearCarpeta_NoConfirmadoSiAlguienLaBorroDespues(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "Proyectos")
	execResult := Execute(ExecutionRequest{Action: "crear_carpeta", Params: map[string]string{"ruta": target}})
	if !execResult.Executed() {
		t.Fatal("setup: esperaba que la creación inicial funcionara")
	}

	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}

	res := Verify("crear_carpeta", map[string]string{"ruta": target}, execResult, nil)
	if res.Confirmed() {
		t.Error("no debería confirmarse: la carpeta ya no existe en el mundo real, aunque Execute haya dicho que sí")
	}
	if res.Outcome != VerificationNotConfirmed {
		t.Errorf("esperaba VerificationNotConfirmed, dio %q", res.Outcome)
	}
}

// TestVerify_MoverArchivo_ConfirmadoDestinoCarpeta cubre el caso donde
// destino es una carpeta (MoveFile conserva el nombre del archivo).
func TestVerify_MoverArchivo_ConfirmadoDestinoCarpeta(t *testing.T) {
	base := t.TempDir()
	src := filepath.Join(base, "informe.pdf")
	if err := os.WriteFile(src, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	destDir := filepath.Join(base, "PDF")
	if err := os.MkdirAll(destDir, 0755); err != nil {
		t.Fatal(err)
	}

	params := map[string]string{"origen": src, "destino": destDir}
	execResult := Execute(ExecutionRequest{Action: "mover_archivo", Params: params})

	res := Verify("mover_archivo", params, execResult, nil)
	if !res.Confirmed() {
		t.Fatalf("esperaba confirmado, dio %q (%s)", res.Outcome, res.Detail)
	}
	if len(res.Facts) != 2 || res.Facts[0].Subject != src || res.Facts[0].Expected != "false" || res.Facts[0].Observed != "false" || res.Facts[1].Subject != filepath.Join(destDir, filepath.Base(src)) || res.Facts[1].Expected != "true" || res.Facts[1].Observed != "true" {
		t.Fatalf("move evidence did not preserve both source/destination postconditions: %+v", res.Facts)
	}
}

// TestVerify_RenombrarArchivo_Confirmado cubre el camino feliz de
// renombrar_archivo.
func TestVerify_RenombrarArchivo_Confirmado(t *testing.T) {
	base := t.TempDir()
	src := filepath.Join(base, "viejo.txt")
	if err := os.WriteFile(src, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	params := map[string]string{"ruta": src, "nombre_nuevo": "nuevo.txt"}
	execResult := Execute(ExecutionRequest{Action: "renombrar_archivo", Params: params})

	res := Verify("renombrar_archivo", params, execResult, nil)
	if !res.Confirmed() {
		t.Fatalf("esperaba confirmado, dio %q (%s)", res.Outcome, res.Detail)
	}
	if len(res.Facts) != 2 || res.Facts[0].Subject != src || res.Facts[0].Expected != "false" || res.Facts[0].Observed != "false" || res.Facts[1].Subject != filepath.Join(base, "nuevo.txt") || res.Facts[1].Expected != "true" || res.Facts[1].Observed != "true" {
		t.Fatalf("rename evidence did not preserve both source/destination postconditions: %+v", res.Facts)
	}
}

// TestVerify_AdjuntaExpectedOutcomeComoNotaSinDecidir confirma que
// ExpectedOutcome de una Skill viaja como nota informativa, pero NO
// cambia el Outcome calculado a partir del filesystem real.
func TestVerify_AdjuntaExpectedOutcomeComoNotaSinDecidir(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "Proyectos")
	skill := &Skill{ExpectedOutcome: []string{"deberías ver la carpeta Proyectos en el explorador"}}

	// Caso confirmado: la nota viaja.
	execOK := Execute(ExecutionRequest{Action: "crear_carpeta", Params: map[string]string{"ruta": target}})
	resOK := Verify("crear_carpeta", map[string]string{"ruta": target}, execOK, skill)
	if !resOK.Confirmed() {
		t.Fatalf("esperaba confirmado, dio %q", resOK.Outcome)
	}
	if len(resOK.SkillExpectedOutcome) == 0 {
		t.Error("esperaba que SkillExpectedOutcome viajara en el resultado")
	}

	// Caso no confirmado (carpeta borrada después): la nota NO debe
	// convertir esto en confirmado.
	os.Remove(target)
	resFail := Verify("crear_carpeta", map[string]string{"ruta": target}, execOK, skill)
	if resFail.Confirmed() {
		t.Error("ExpectedOutcome de la Skill nunca debería forzar un Confirmed que el filesystem real contradice")
	}
}

// TestExecuteAndVerify_CamoinoCompleto cubre el atajo de conveniencia.
func TestExecuteAndVerify_CaminoCompleto(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "Proyectos")

	execResult, verResult := ExecuteAndVerify(ExecutionRequest{
		Action: "crear_carpeta",
		Params: map[string]string{"ruta": target},
	}, nil)

	if !execResult.Executed() {
		t.Fatalf("esperaba ejecución exitosa, dio %q", execResult.Outcome)
	}
	if !verResult.Confirmed() {
		t.Fatalf("esperaba confirmación, dio %q (%s)", verResult.Outcome, verResult.Detail)
	}
}

func TestVerify_FilesystemReadFailureIsUncheckedNotContradicted(t *testing.T) {
	base := t.TempDir()
	parentFile := filepath.Join(base, "not-a-directory")
	if err := os.WriteFile(parentFile, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	impossibleChild := filepath.Join(parentFile, "child") // ENOTDIR, not proof of absence.

	cases := []struct {
		name   string
		action string
		params map[string]string
	}{
		{name: "folder", action: "crear_carpeta", params: map[string]string{"ruta": impossibleChild}},
		{name: "move source", action: "mover_archivo", params: map[string]string{"origen": impossibleChild, "destino": filepath.Join(base, "dest")}},
		{name: "rename source", action: "renombrar_archivo", params: map[string]string{"ruta": impossibleChild, "nombre_nuevo": "new.txt"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := Verify(tc.action, tc.params, &ExecutionResult{Action: tc.action, Outcome: OutcomeExecuted}, nil)
			if res.Outcome != VerificationUnchecked {
				t.Fatalf("filesystem read error must remain unknown, got outcome=%q detail=%q", res.Outcome, res.Detail)
			}
		})
	}
}

func TestVerify_MissingParametersAreUnchecked(t *testing.T) {
	cases := []struct {
		action string
		params map[string]string
	}{
		{action: "crear_carpeta", params: map[string]string{}},
		{action: "mover_archivo", params: map[string]string{"destino": "/tmp/dest"}},
		{action: "renombrar_archivo", params: map[string]string{"ruta": "/tmp/source"}},
	}
	for _, tc := range cases {
		t.Run(tc.action, func(t *testing.T) {
			res := Verify(tc.action, tc.params, &ExecutionResult{Action: tc.action, Outcome: OutcomeExecuted}, nil)
			if res.Outcome != VerificationUnchecked {
				t.Fatalf("missing parameters must remain unknown, got outcome=%q detail=%q", res.Outcome, res.Detail)
			}
		})
	}
}
