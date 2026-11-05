package core

import (
	"strings"
	"testing"
)

// TestExecutiveMind_CicloCompleto confirma el ciclo Planner↔Progress
// completo (Bloque 46): crear un proyecto real, consultar su progreso,
// registrar que se completó una tarea, y confirmar que el progreso
// reportado cambia — todo vía knowledgeEngine.Query(), el mismo camino
// real que usa Process() en producción, sin invocar TTS directamente.
func TestExecutiveMind_CicloCompleto(t *testing.T) {
	e := buildTestEngine(t)

	create := e.knowledgeEngine.Query("quiero crear un proyecto para las gafas xr")
	if create.Content == "" {
		t.Fatal("esperaba una respuesta al crear el proyecto")
	}
	if !strings.Contains(create.Content, "Proyecto") {
		t.Errorf("esperaba confirmación de creación de proyecto, obtuve: %q", create.Content)
	}

	statusBefore := e.knowledgeEngine.Query("como vamos con las gafas xr")
	if !strings.Contains(statusBefore.Content, "0/") {
		t.Errorf("esperaba progreso inicial en 0 tareas completadas, obtuve: %q", statusBefore.Content)
	}

	complete := e.knowledgeEngine.Query("termine una tarea de las gafas xr")
	if !strings.Contains(complete.Content, "completada") {
		t.Errorf("esperaba confirmación de tarea completada, obtuve: %q", complete.Content)
	}

	statusAfter := e.knowledgeEngine.Query("como vamos con las gafas xr")
	if !strings.Contains(statusAfter.Content, "1/") {
		t.Errorf("esperaba progreso actualizado a 1 tarea completada, obtuve: %q", statusAfter.Content)
	}
}

// TestExecutiveMind_ProyectoInexistenteEsHonesto confirma que preguntar
// por un proyecto que nunca se creó no rompe ni finge datos — dice
// explícitamente que no existe.
func TestExecutiveMind_ProyectoInexistenteEsHonesto(t *testing.T) {
	e := buildTestEngine(t)

	result := e.knowledgeEngine.Query("como vamos con un proyecto que no existe")

	if !strings.Contains(result.Content, "No tengo ningún proyecto") {
		t.Errorf("esperaba el mensaje honesto de proyecto no encontrado, obtuve: %q", result.Content)
	}
}

// TestExecutiveMind_DosProyectosNoSeMezclan confirma que dos proyectos
// distintos, creados en la misma sesión, mantienen su progreso por
// separado.
func TestExecutiveMind_DosProyectosNoSeMezclan(t *testing.T) {
	e := buildTestEngine(t)

	e.knowledgeEngine.Query("quiero crear un proyecto para el proyecto xr")
	e.knowledgeEngine.Query("quiero crear un proyecto para el proyecto hardware")

	e.knowledgeEngine.Query("termine una tarea de el proyecto xr")

	statusXR := e.knowledgeEngine.Query("como vamos con el proyecto xr")
	statusHW := e.knowledgeEngine.Query("como vamos con el proyecto hardware")

	if !strings.Contains(statusXR.Content, "1/") {
		t.Errorf("esperaba que XR tuviera 1 tarea completada, obtuve: %q", statusXR.Content)
	}
	if !strings.Contains(statusHW.Content, "0/") {
		t.Errorf("hardware no debería haberse visto afectado por el avance de XR, obtuve: %q", statusHW.Content)
	}
}
