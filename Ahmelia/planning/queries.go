package planning

import "strings"

// ============================================================================
// QUERIES.GO — Bloque 46, Executive Project Manager (Planner + Progress)
// ============================================================================
//
// Mismo criterio de trigger que ya usa solving/ desde Bloque 21: cada
// pregunta tiene su propia lista de frases disparadoras y su propia
// función Parse*Query, testeable en aislamiento sin tocar core/engine.go.
// La diferencia es de PAQUETE, no de estilo — esto vive en planning
// porque es el que sabe qué significa "crear un proyecto" o "consultar
// progreso"; solving no tiene por qué saberlo.
//
// Las tres preguntas que Bloque 46 conecta, tal como se acordó al
// definir el recorte (Planner + Progress como núcleo, Scheduler/
// Reviewer/Goal DependencyGraph quedan para bloques futuros):
//   - Crear un proyecto real (Planner.GeneratePlan)
//   - Consultar cómo va (Progress.GetStatus/GetRecommendations)
//   - Registrar que se completó una tarea (Task.Complete +
//     Progress.UpdateTaskCompleted) — el cierre del ciclo Planner↔Progress

var createProjectTriggers = []string{
	"quiero crear un proyecto para",
	"necesito un proyecto para",
	"planifica el proyecto de",
	"arma un proyecto para",
	"quiero planificar",
}

// ParseCreateProjectQuery detecta un pedido de crear un proyecto y
// devuelve el nombre/descripción que sigue al trigger. ok=false si no
// hay trigger, o si el trigger está pero no queda nada después (pedido
// vacío — "quiero planificar" a secas no alcanza para nombrar un
// proyecto).
func ParseCreateProjectQuery(normalizedText string) (name string, ok bool) {
	return stripPlanningTrigger(normalizedText, createProjectTriggers)
}

var projectStatusTriggers = []string{
	"como vamos con",
	"como va el proyecto",
	"cual es el progreso de",
	"que tan avanzado esta",
	"como esta el proyecto",
}

// ParseProjectStatusQuery detecta una consulta de progreso y devuelve
// el nombre del proyecto sobre el que se pregunta.
func ParseProjectStatusQuery(normalizedText string) (name string, ok bool) {
	return stripPlanningTrigger(normalizedText, projectStatusTriggers)
}

var completeTaskTriggers = []string{
	"termine una tarea de",
	"complete una tarea de",
	"marca como hecha la siguiente tarea de",
	"avance en el proyecto",
}

// ParseCompleteTaskQuery detecta que el usuario reporta haber
// terminado la tarea pendiente de un proyecto, y devuelve el nombre
// del proyecto.
func ParseCompleteTaskQuery(normalizedText string) (name string, ok bool) {
	return stripPlanningTrigger(normalizedText, completeTaskTriggers)
}

// stripPlanningTrigger busca el primer trigger de la lista que aparece
// en el texto y devuelve lo que queda después, recortado. Mismo
// criterio que stripTrigger en solving/goal_analyzer.go, pero acotado a
// este paquete — no hace falta compartir código entre dos sistemas que
// hoy no se llaman entre sí.
func stripPlanningTrigger(normalizedText string, triggers []string) (rest string, ok bool) {
	for _, trigger := range triggers {
		idx := strings.Index(normalizedText, trigger)
		if idx == -1 {
			continue
		}
		tail := strings.TrimSpace(normalizedText[idx+len(trigger):])
		tail = strings.TrimPrefix(tail, ":")
		tail = strings.TrimSpace(tail)
		if strings.HasPrefix(tail, "de ") {
			tail = tail[3:]
		}
		if tail == "" {
			return "", false
		}
		return tail, true
	}
	return "", false
}
