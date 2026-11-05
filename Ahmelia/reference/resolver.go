package reference

import (
	"ahmelia/environment"
	"ahmelia/shared"
)

// ============================================================================
// REFERENCE/ — el "Context Resolver" que responde "¿a qué se refiere eso?"
// ============================================================================
//
// Por qué existe este paquete: la resolución de referencias implícitas
// (ábrelo, vuelve a eso) empezó como un par de `if shared.IsPronoun(target)`
// dentro de core/engine.go, duplicados en el caso de abrir_programa y en
// el de cambiar_ventana. No escala: cada tipo nuevo de referencia (a un
// archivo, a algo dicho hace un rato, a "mi editor"...) habría sido otro
// bloque de ifs más adentro de engine.go.
//
// La forma de la API es genérica a propósito: engine.go solo conoce
// Reference y Resolve() — nunca pregunta "¿era una app o un archivo?",
// eso ya viene resuelto en el campo Type. Así, cuando se agregue un tipo
// nuevo, engine.go no cambia — solo Resolve() aprende a reconocerlo.
//
// Lo que NO se hizo, a propósito: no hay un archivo por tipo de
// referencia (content.go, conversation.go, memory.go, goal.go,
// personal.go...) aunque la forma del código ya lo sugiere. Ahora mismo
// eso serían archivos vacíos — ninguno tiene una fuente de datos real
// detrás todavía:
//
//   - Content (archivo/proyecto activo): el DATO ya existe
//     (AppState.File/Project vía title_parser.go) pero falta decidir
//     qué significa "resolver" un archivo sin una ruta real en disco.
//   - Conversation (eso, lo anterior): necesita memoria de turno a
//     turno, que hoy no existe.
//   - Memory (ayer, hace un rato): necesita memoria con timestamps
//     consultable, no solo memory/long.json como bitácora plana.
//   - Goal (continúa, termina): necesita un módulo de planificación
//     real, no existe ninguno todavía.
//   - Personal (mi proyecto, mi editor): necesita un perfil de usuario
//     con preferencias, no existe.
//
// Se agregan cuando esa fuente de datos exista de verdad — mismo
// criterio que ya se sigue en environment/state.go para no inventar
// sensores (CPU/GPU/batería) que nadie alimenta todavía.

// ReferenceType identifica a qué clase de cosa apunta una Reference ya
// resuelta. El Engine puede hacer type-switch sobre esto sin necesitar
// saber cómo se llegó a la respuesta.
type ReferenceType string

const (
	TypeApplication ReferenceType = "application"
	// Reservados para cuando tengan una fuente real detrás (ver arriba):
	// TypeContent, TypeConversation, TypeMemory, TypeGoal, TypePersonal.
)

// Reference es el ÚNICO tipo que engine.go necesita entender, sin
// importar si el usuario dijo "ábrelo", "ese proyecto" o "continúa".
//
// Los tres desenlaces posibles, distinguidos explícitamente en vez de
// colapsarlos en un booleano:
//
//  1. target NO era ninguna referencia implícita -> WasReference=false.
//     El caller debe seguir tratando target como texto literal.
//  2. SÍ era una referencia, pero no hay nada a lo que apuntar (ej.
//     WorldState vacío) -> WasReference=true, Found=false.
//  3. SÍ era una referencia y se resolvió -> WasReference=true,
//     Found=true, y el campo correspondiente al Type queda listo.
type Reference struct {
	Type         ReferenceType
	WasReference bool
	Found        bool
	Confidence   float64

	// App solo es válido cuando Type == TypeApplication. Cuando existan
	// más tipos, cada uno tendrá su propio campo (o, si crecen mucho,
	// se pasa a una interfaz — no hace falta decidir eso todavía con
	// un solo tipo real implementado).
	App *environment.AppState
}

// Resolver envuelve environment.Manager (la única fuente de verdad hoy)
// y traduce pronombres/referencias a algo concreto.
type Resolver struct {
	world *environment.Manager
}

// New crea un Resolver sobre el WorldState existente. No es dueño del
// Manager — solo lo consulta, igual que ya hacía engine.go directamente.
func New(world *environment.Manager) *Resolver {
	return &Resolver{world: world}
}

// Resolve es el único método que engine.go debería llamar. Decide
// internamente qué tipo de referencia es target y a cuál resolutor
// especializado delegar. Hoy solo hay uno real (aplicaciones); cuando se
// agregue Content/Conversation/etc., se agregan ACÁ adentro — engine.go
// no vuelve a cambiar.
func (r *Resolver) Resolve(target string) Reference {
	if ref, ok := r.resolveApp(target); ok {
		return ref
	}
	// Ningún resolutor reconoció target como una referencia implícita.
	return Reference{WasReference: false}
}

// resolveApp es el Nivel 1 (referencia inmediata): "ábrelo", "ciérralo",
// "vuelve a eso" -> la app que WorldState recuerda como activa.
func (r *Resolver) resolveApp(target string) (Reference, bool) {
	if !shared.IsPronoun(target) {
		return Reference{}, false
	}
	app, found := r.world.ActiveApp()
	return Reference{
		Type:         TypeApplication,
		WasReference: true,
		Found:        found,
		App:          app,
		// Confianza fija por ahora: solo hay un candidato posible (la
		// única app activa). Un score.go real solo tiene sentido cuando
		// haya que ELEGIR entre varios candidatos — hoy no los hay.
		Confidence: 0.85,
	}, true
}
