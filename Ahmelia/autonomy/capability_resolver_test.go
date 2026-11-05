package autonomy

import (
	"context"
	"strings"
	"testing"

	"ahmelia/planning"
	"ahmelia/solving"
)

// ----------------------------------------------------------------------------
// Camino feliz: KindLocalAction ejecutable, para cada una de las 3 Action.
// ----------------------------------------------------------------------------

func TestB89CapabilityResolver_CreaCarpeta(t *testing.T) {
	goal := planning.NewGoal("Organizar", "", planning.PriorityMedium, "ahmelia")
	task := planning.NewTask(goal.ID, "crear", "crea carpeta llamada proyectos_2026", 1, 0)

	r := NewCapabilityResolver(nil)
	req, err := r.MapTask(context.Background(), goal, task)
	if err != nil {
		t.Fatalf("MapTask() error: %v", err)
	}
	if req.Action != "crear_carpeta" || req.Target != "proyectos_2026" || req.Parameters["ruta"] != "proyectos_2026" {
		t.Fatalf("request inesperado: %+v", req)
	}
}

func TestB89CapabilityResolver_MoverArchivo(t *testing.T) {
	goal := planning.NewGoal("Organizar", "", planning.PriorityMedium, "ahmelia")
	task := planning.NewTask(goal.ID, "mover", "mueve informe.pdf a documentos/informe.pdf", 1, 0)

	r := NewCapabilityResolver(nil)
	req, err := r.MapTask(context.Background(), goal, task)
	if err != nil {
		t.Fatalf("MapTask() error: %v", err)
	}
	if req.Action != "mover_archivo" || req.Parameters["origen"] != "informe.pdf" || req.Parameters["destino"] != "documentos/informe.pdf" {
		t.Fatalf("request inesperado: %+v", req)
	}
	if req.Target != "documentos/informe.pdf" {
		t.Fatalf("target esperado destino, got=%q", req.Target)
	}
}

func TestB89CapabilityResolver_PuntoEnNombreDeArchivoSeConserva(t *testing.T) {
	// Motivo real documentado en el header: el remainder se corta sobre el
	// texto ORIGINAL, no sobre shared.Normalize(), para no perder el "."
	// de un nombre de archivo real.
	goal := planning.NewGoal("Organizar", "", planning.PriorityMedium, "ahmelia")
	task := planning.NewTask(goal.ID, "mover", "mueve Informe.PDF a Backups/Informe.PDF", 1, 0)

	r := NewCapabilityResolver(nil)
	req, err := r.MapTask(context.Background(), goal, task)
	if err != nil {
		t.Fatalf("MapTask() error: %v", err)
	}
	if req.Parameters["origen"] != "Informe.PDF" || req.Parameters["destino"] != "Backups/Informe.PDF" {
		t.Fatalf("mayúsculas/puntuación perdidas: %+v", req)
	}
}

// ----------------------------------------------------------------------------
// skills == nil es seguro (Search lo tolera, ver goal_skill_resolver.go).
// ----------------------------------------------------------------------------

func TestB89CapabilityResolver_SkillLibraryNilNoRompe(t *testing.T) {
	goal := planning.NewGoal("x", "", planning.PriorityMedium, "ahmelia")
	task := planning.NewTask(goal.ID, "crear", "crea carpeta backups", 1, 0)

	r := NewCapabilityResolver(nil)
	if _, err := r.MapTask(context.Background(), goal, task); err != nil {
		t.Fatalf("MapTask() con skills nil no debería fallar: %v", err)
	}
}

// ----------------------------------------------------------------------------
// Rechazos honestos.
// ----------------------------------------------------------------------------

func TestB89CapabilityResolver_RechazaGoalOTaskNil(t *testing.T) {
	r := NewCapabilityResolver(nil)
	goal := planning.NewGoal("x", "", planning.PriorityMedium, "ahmelia")
	task := planning.NewTask(goal.ID, "x", "crea carpeta x", 1, 0)

	if _, err := r.MapTask(context.Background(), nil, task); err == nil {
		t.Fatal("esperaba rechazo con goal nil")
	}
	if _, err := r.MapTask(context.Background(), goal, nil); err == nil {
		t.Fatal("esperaba rechazo con task nil")
	}
}

func TestB89CapabilityResolver_RechazaTaskSinTexto(t *testing.T) {
	goal := planning.NewGoal("x", "", planning.PriorityMedium, "ahmelia")
	task := planning.NewTask(goal.ID, "", "", 1, 0)

	r := NewCapabilityResolver(nil)
	if _, err := r.MapTask(context.Background(), goal, task); err == nil {
		t.Fatal("esperaba rechazo: no hay texto del que resolver ninguna capacidad")
	}
}

func TestB89CapabilityResolver_RechazaSinCapacidadResuelta(t *testing.T) {
	// KindUnknown: ni acción local ni Skill guardada — 0 Subgoals.
	goal := planning.NewGoal("x", "", planning.PriorityMedium, "ahmelia")
	task := planning.NewTask(goal.ID, "x", "cuéntame algo sobre el clima de Marte", 1, 0)

	r := NewCapabilityResolver(nil)
	if _, err := r.MapTask(context.Background(), goal, task); err == nil {
		t.Fatal("esperaba rechazo: la tarea no resuelve a ninguna capacidad ejecutable")
	}
}

// TestB89RenombrarArchivoConvergeComoKindLocalAction documenta el cierre
// del contrato: la misma capability canónica que habilita ejecución también
// debe poder atravesar el resolver de lenguaje natural.
func TestB89RenombrarArchivoConvergeComoKindLocalAction(t *testing.T) {
	goal := planning.NewGoal("x", "", planning.PriorityMedium, "ahmelia")
	task := planning.NewTask(goal.ID, "renombrar", "renombra viejo.txt a nuevo.txt", 1, 0)

	r := NewCapabilityResolver(nil)
	_, err := r.MapTask(context.Background(), goal, task)
	if err != nil {
		t.Fatalf("renombrar_archivo debería atravesar el resolver ahora que capability, parser y ejecución convergen: %v", err)
	}
}

// TestB89CapabilityResolver_RechazaProcedimientoAprendido cubre el segundo
// límite honesto del header: un Subgoal que viene de una Skill aprendida
// (KindProceduralFull aquí, un único paso wired) sí puede ser Executable,
// pero B89 lo rechaza igual porque Operation.Text es solo el trigger
// matcheado ("mueve"), no la oración completa del Step original — no hay
// origen/destino real que extraer sin inventarlo.
func TestB89CapabilityResolver_RechazaProcedimientoAprendido(t *testing.T) {
	steps := []string{"mueve el archivo descargado a la carpeta correcta"}
	skill := &solving.Skill{
		SkillID:    "OrdenarDescargas",
		Label:      "Ordenar descargas",
		Content:    "Ordenar descargas: mueve el archivo descargado a la carpeta correcta.",
		Steps:      steps,
		Operations: []solving.Operation{{Text: "mueve", Action: "mover_archivo", Status: solving.OpWired}},
	}
	skills := solving.NewSkillLibrary()
	skills.Install(skill)

	goal := planning.NewGoal("x", "", planning.PriorityMedium, "ahmelia")
	task := planning.NewTask(goal.ID, "ordenar", "ordenar descargas", 1, 0)

	r := NewCapabilityResolver(skills)
	_, err := r.MapTask(context.Background(), goal, task)
	if err == nil {
		t.Fatal("esperaba rechazo: B89 no extrae parámetros de un paso de Skill aprendida")
	}
	if !strings.Contains(err.Error(), "procedimiento aprendido") {
		t.Fatalf("mensaje inesperado: %v", err)
	}
}

// TestB89CapabilityResolver_RechazaMultiPaso cubre el primer límite
// honesto del header: una Skill con más de un Subgoal ejecutable se
// rechaza entera en vez de ejecutar el primero y descartar el resto.
func TestB89CapabilityResolver_RechazaMultiPaso(t *testing.T) {
	steps := []string{"mueve el archivo viejo", "crea carpeta nueva para el proyecto"}
	skill := &solving.Skill{
		SkillID: "PrepararProyecto",
		Label:   "Preparar proyecto",
		Content: "Preparar proyecto: mueve el archivo viejo y crea carpeta nueva para el proyecto.",
		Steps:   steps,
		Operations: []solving.Operation{
			{Text: "mueve", Action: "mover_archivo", Status: solving.OpWired},
			{Text: "crea carpeta", Action: "crear_carpeta", Status: solving.OpWired},
		},
	}
	skills := solving.NewSkillLibrary()
	skills.Install(skill)

	goal := planning.NewGoal("x", "", planning.PriorityMedium, "ahmelia")
	task := planning.NewTask(goal.ID, "preparar", "preparar proyecto", 1, 0)

	r := NewCapabilityResolver(skills)
	_, err := r.MapTask(context.Background(), goal, task)
	if err == nil {
		t.Fatal("esperaba rechazo: tarea multi-paso, B89 solo mapea un único subobjetivo ejecutable")
	}
	if !strings.Contains(err.Error(), "subobjetivos") {
		t.Fatalf("mensaje inesperado: %v", err)
	}
}

func TestB89CapabilityResolver_RechazaContextoCancelado(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	goal := planning.NewGoal("x", "", planning.PriorityMedium, "ahmelia")
	task := planning.NewTask(goal.ID, "crear", "crea carpeta x", 1, 0)

	r := NewCapabilityResolver(nil)
	if _, err := r.MapTask(ctx, goal, task); err == nil {
		t.Fatal("esperaba rechazo con contexto ya cancelado")
	}
}

// ----------------------------------------------------------------------------
// extractAutonomousParams / longestMatchingTrigger — unidad directa.
// ----------------------------------------------------------------------------

func TestB89ExtractAutonomousParams_CrearCarpetaSinPrefijoNombrado(t *testing.T) {
	params, target, err := extractAutonomousParams("crear_carpeta", "crea carpeta backups")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if params["ruta"] != "backups" || target != "backups" {
		t.Fatalf("params/target inesperados: %+v target=%q", params, target)
	}
}

func TestB89ExtractAutonomousParams_RenombrarArchivo(t *testing.T) {
	params, target, err := extractAutonomousParams("renombrar_archivo", "renombra viejo.txt a nuevo.txt")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if params["ruta"] != "viejo.txt" || params["nombre_nuevo"] != "nuevo.txt" || target != "nuevo.txt" {
		t.Fatalf("params/target inesperados: %+v target=%q", params, target)
	}
}

func TestB89ExtractAutonomousParams_SinTriggerReconocido(t *testing.T) {
	if _, _, err := extractAutonomousParams("mover_archivo", "esto no tiene ningún patrón reconocible"); err == nil {
		t.Fatal("esperaba error: ningún trigger de mover_archivo aparece en el texto")
	}
}

func TestB89ExtractAutonomousParams_MoverArchivoSinSeparador(t *testing.T) {
	if _, _, err := extractAutonomousParams("mover_archivo", "mueve informe.pdf"); err == nil {
		t.Fatal("esperaba error: falta \" a \" para separar origen/destino")
	}
}

func TestB89ExtractAutonomousParams_ActionNoSoportadaEsBug(t *testing.T) {
	if _, _, err := extractAutonomousParams("abrir_programa", "abre vscode"); err == nil {
		t.Fatal("esperaba error: abrir_programa no es una de las 3 Action que B89 extrae")
	}
}
