package solving

import (
	"os"
	"path/filepath"
	"testing"
)

// entornoProyectoSkill — fixture con 3 Steps que SÍ matchean Actions
// reales (crea carpeta / mueve archivo / renombra archivo), para
// poder ejercitar la secuencia completa Execute→Verify sin datos
// fabricados a mano en Operations (se calcula con detectOperations,
// igual que goResolverGoSkill en goal_skill_resolver_test.go).
func entornoProyectoSkill() *Skill {
	steps := []string{
		"crea carpeta Proyectos",
		"mueve el archivo a la carpeta",
		"renombra el archivo",
	}
	return &Skill{
		SkillID:         "prep-entorno",
		Label:           "Configurar el entorno de proyecto",
		ProcedureTitle:  "Configurar el entorno de proyecto",
		Steps:           steps,
		Operations:      detectOperations(steps),
		Sources:         []string{"nota interna"},
		State:           StateProcedural,
		FailureRecovery: []string{"si la carpeta ya existe, seguí con el siguiente paso igual"},
	}
}

// TestDecompose_MetaAbstractaSinSkill_NoInventaSubgoals es el límite
// central del bloque: un pedido abstracto nunca antes enseñado no
// produce una lista de subobjetivos fabricada — Decompose lo admite
// honestamente.
func TestDecompose_MetaAbstractaSinSkill_NoInventaSubgoals(t *testing.T) {
	skills := NewSkillLibrary()
	d := Decompose("prepara el entorno para desarrollar mi juego", skills)
	if len(d.Subgoals) != 0 {
		t.Fatalf("sin ninguna Skill guardada sobre esto, no debería haber Subgoals inventados — hubo %d", len(d.Subgoals))
	}
	if d.Resolution.Kind != KindUnknown {
		t.Errorf("esperaba KindUnknown, dio %s", d.Resolution.Kind)
	}
}

// TestDecompose_AccionLocal_EsUnSoloSubgoalTrivial confirma la
// descomposición trivial de un pedido que ya es una sola acción.
func TestDecompose_AccionLocal_EsUnSoloSubgoalTrivial(t *testing.T) {
	skills := NewSkillLibrary()
	d := Decompose("crea carpeta nueva", skills)
	if d.Resolution.Kind != KindLocalAction {
		t.Fatalf("esperaba KindLocalAction, dio %s", d.Resolution.Kind)
	}
	if len(d.Subgoals) != 1 {
		t.Fatalf("esperaba exactamente 1 Subgoal trivial, hubo %d", len(d.Subgoals))
	}
	if d.Subgoals[0].Action != "crear_carpeta" || !d.Subgoals[0].Executable {
		t.Errorf("Subgoal trivial mal formado: %+v", d.Subgoals[0])
	}
}

// TestDecompose_ConSkillGuardada_ExponeLosPasosReales confirma que,
// cuando SÍ hay una Skill con procedimiento, los Subgoals vienen de
// sus Operations reales (54) — no se recalculan con otra lógica.
func TestDecompose_ConSkillGuardada_ExponeLosPasosReales(t *testing.T) {
	skills := NewSkillLibrary()
	skills.Install(entornoProyectoSkill())

	d := Decompose("configura el entorno de proyecto", skills)
	if len(d.Subgoals) != 3 {
		t.Fatalf("esperaba 3 Subgoals (uno por Step), hubo %d: %+v", len(d.Subgoals), d.Subgoals)
	}
	if d.Subgoals[0].Action != "crear_carpeta" {
		t.Errorf("Subgoal[0].Action = %q, esperaba crear_carpeta", d.Subgoals[0].Action)
	}
	if d.Subgoals[1].Action != "mover_archivo" {
		t.Errorf("Subgoal[1].Action = %q, esperaba mover_archivo", d.Subgoals[1].Action)
	}
	// renombrar_archivo ahora converge con la autoridad de ejecución:
	// tiene binding real en Engine y en el executor standalone.
	if d.Subgoals[2].Action != "renombrar_archivo" {
		t.Errorf("Subgoal[2].Action = %q, esperaba renombrar_archivo", d.Subgoals[2].Action)
	}
	if d.Subgoals[2].OperationStatus != OpWired {
		t.Errorf("esperaba OperationStatus Wired para renombrar_archivo, dio %s", d.Subgoals[2].OperationStatus)
	}
	if !d.Subgoals[2].Executable {
		t.Error("renombrar_archivo debería ser Executable=true")
	}
	for i, sg := range d.Subgoals {
		if sg.Status != SubgoalPending {
			t.Errorf("Subgoal[%d] debería empezar Pending antes de ExecuteDecomposition, dio %s", i, sg.Status)
		}
	}
}

// TestExecuteDecomposition_CaminoCompleto ejecuta los 3 subobjetivos
// reales en secuencia y confirma que Verify (67) corrió sobre cada
// uno — no solo que Execute no dio error.
func TestExecuteDecomposition_CaminoCompleto(t *testing.T) {
	base := t.TempDir()
	skills := NewSkillLibrary()
	skills.Install(entornoProyectoSkill())
	d := Decompose("configura el entorno de proyecto", skills)

	folder := filepath.Join(base, "Proyectos")
	src := filepath.Join(base, "config.txt")
	if err := os.WriteFile(src, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	params := []map[string]string{
		{"ruta": folder},                   // crear_carpeta
		{"origen": src, "destino": folder}, // mover_archivo
		{"ruta": filepath.Join(folder, "config.txt"), // renombrar_archivo (ya movido)
			"nombre_nuevo": "config_final.txt"},
	}

	d = ExecuteDecomposition(d, params, false)

	if !d.AllConfirmed {
		t.Fatalf("esperaba todos los subobjetivos confirmados: %s", d.Describe())
	}
	for i, sg := range d.Subgoals {
		if sg.Status != SubgoalConfirmed {
			t.Errorf("Subgoal[%d] (%s) esperaba Confirmed, dio %s (%s)", i, sg.Action, sg.Status, sg.Detail)
		}
		if sg.VerResult == nil || !sg.VerResult.Confirmed() {
			t.Errorf("Subgoal[%d] debería tener un VerificationResult confirmado real, no solo ExecutionResult", i)
		}
	}
	if _, err := os.Stat(filepath.Join(folder, "config_final.txt")); err != nil {
		t.Errorf("el archivo final no está donde debería: %v", err)
	}
}

// TestExecuteDecomposition_SeDetieneEnElPrimerFallo confirma que un
// fallo real a mitad de secuencia bloquea los subobjetivos
// siguientes — nunca se asume que habrían funcionado.
func TestExecuteDecomposition_SeDetieneEnElPrimerFallo(t *testing.T) {
	base := t.TempDir()
	skills := NewSkillLibrary()
	skills.Install(entornoProyectoSkill())
	d := Decompose("configura el entorno de proyecto", skills)

	// El primer paso (crear_carpeta) va a fallar: la carpeta ya existe.
	folder := filepath.Join(base, "Proyectos")
	os.MkdirAll(folder, 0755)

	params := []map[string]string{
		{"ruta": folder},
		{"origen": filepath.Join(base, "no-importa.txt"), "destino": folder},
		{"ruta": filepath.Join(base, "no-importa.txt"), "nombre_nuevo": "x.txt"},
	}
	d = ExecuteDecomposition(d, params, false)

	if d.AllConfirmed {
		t.Fatal("no debería estar todo confirmado: el primer paso falló")
	}
	if d.Subgoals[0].Status != SubgoalFailed {
		t.Errorf("Subgoal[0] esperaba Failed, dio %s", d.Subgoals[0].Status)
	}
	if d.Subgoals[0].Diagnosis == nil || d.Subgoals[0].Diagnosis.Category != FailureAlreadyExists {
		t.Errorf("Subgoal[0] debería tener un FailureDiagnosis real (68), dio: %+v", d.Subgoals[0].Diagnosis)
	}
	if d.Subgoals[1].Status != SubgoalSkipped || d.Subgoals[2].Status != SubgoalSkipped {
		t.Errorf("Subgoals 1 y 2 deberían quedar Skipped tras el fallo del 0, dieron %s / %s",
			d.Subgoals[1].Status, d.Subgoals[2].Status)
	}
}

// TestGoalDecomposition_Describe_NoQuedaVacio confirma que Describe()
// siempre produce texto, tanto para el caso sin Subgoals (límite
// honesto) como para uno con Subgoals.
func TestGoalDecomposition_Describe_NoQuedaVacio(t *testing.T) {
	skills := NewSkillLibrary()
	empty := Decompose("algo que nunca se enseñó", skills)
	if empty.Describe() == "" {
		t.Error("Describe() no debería quedar vacío ni cuando no hay Subgoals")
	}

	skills.Install(entornoProyectoSkill())
	full := Decompose("configura el entorno de proyecto", skills)
	if full.Describe() == "" {
		t.Error("Describe() no debería quedar vacío con Subgoals identificados")
	}
}
