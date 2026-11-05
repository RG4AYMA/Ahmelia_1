package autonomy

import (
	"sync"
	"time"

	"ahmelia/memory"
	"ahmelia/planning"
)

// ============================================================================
// STATE.GO — Bloque 72, Fase 6 (Autonomía Cognitiva e Iniciativa), Capa 6.1
// ============================================================================
//
// Alcance honesto de este bloque (mismo criterio que shared/execution_safety.go
// al arrancar Fase 5): esto NO ejecuta nada, NO decide nada, NO corre en
// background. Es la respuesta consultable a una sola pregunta — "¿en qué
// estado cognitivo está Ahmelia ahora mismo?" — sobre la cual 73 (Goal
// Monitor), 74 (Context Observer), 75 (Initiative Engine), 76 (Continuous
// Conversation) y 77 (Autonomous Task Loop) van a apoyarse. Ninguno de esos
// bloques existe todavía; AutonomousState no los anticipa con código
// especulativo, solo deja el lugar correcto para que enganchen.
//
// Auditoría previa a este archivo (evita segunda fuente de verdad):
//   - planning.Goal ya modela objetivo/progreso/bloqueos — CurrentGoal y
//     PendingGoals son punteros a esa struct, no una copia.
//   - memory.PendingClarification ya modela la pregunta pendiente que
//     B96.39 clarification movement deja armada — PendingQuestions/
//     PendingAssumptions se llenan a partir de ahí vía SyncPendingClarification,
//     nunca se escriben a mano por otro camino.
//   - language.ConversationFlow ya modela el tema activo — ActiveTopic se
//     fija vía SetActiveTopic(flow.CurrentTopic()), no se recalcula acá.
//   - cognition.StateManager (Bloque 15) YA existe y responde "¿está
//     ocupada, y con qué?" a nivel de turno conversacional (Escuchando/
//     Pensando/Ejecutando/Aprendiendo). ExecutionState de este bloque es
//     DELIBERADAMENTE otra cosa: no es "qué hace Ahmelia en este turno",
//     es "en qué paso del ciclo de vida de una tarea autónoma está" —
//     el ciclo Executing→Verifying→Recovering que conecta con 66→67→68
//     (Execution/Verification/Failure Recovery), que cognition.State no
//     representa porque esos bloques no existían cuando se escribió.
//     No se fusionan ambos: StateManager sigue siendo la fuente de "estoy
//     ocupada ahora"; AutonomousState es la fuente de "en qué fase de la
//     tarea estoy". Esta nota documenta la distinción para que un bloque
//     futuro no los mezcle sin querer.
//
// Deliberadamente AUSENTE de este archivo (pertenece a bloques futuros,
// ver roadmap): ninguna goroutine, ningún timer, ninguna decisión de
// iniciativa, ninguna ejecución. Solo lectura/escritura de estado.

// ExecutionState es el paso del ciclo de vida de una tarea autónoma.
// Ocho valores, tal como los define el roadmap de Fase 6 — ni más ni
// menos: agregar un estado nuevo acá implica que 77 (Autonomous Task
// Loop) ya lo necesita de verdad, no que "podría hacer falta".
type ExecutionState string

const (
	ExecIdle           ExecutionState = "Idle"
	ExecThinking       ExecutionState = "Thinking"
	ExecWaitingForUser ExecutionState = "WaitingForUser"
	ExecExecuting      ExecutionState = "Executing"
	ExecVerifying      ExecutionState = "Verifying"
	ExecRecovering     ExecutionState = "Recovering"
	ExecCompleted      ExecutionState = "Completed"
	ExecBlocked        ExecutionState = "Blocked"
)

// executionTransitions es la tabla de transiciones válidas, tomada
// directamente de los dos caminos que describe el roadmap de B72:
//
//	Idle → Thinking → Executing → Verifying → Completed
//	Executing → Recovering → (Executing | WaitingForUser | Blocked)
//
// más las salidas de sentido común que esos dos caminos implican sin
// nombrarlas explícitamente (volver a Idle después de Completed/Blocked
// para poder arrancar la próxima tarea; WaitingForUser puede retomar en
// Thinking una vez que el usuario responde). No se agrega ninguna
// transición que el roadmap no sugiera — una tabla más permisiva "por
// si acaso" es exactamente el tipo de invención que este bloque evita.
var executionTransitions = map[ExecutionState]map[ExecutionState]bool{
	ExecIdle: {
		ExecThinking: true,
	},
	ExecThinking: {
		ExecExecuting:      true,
		ExecWaitingForUser: true,
		ExecIdle:           true, // se descartó el pensamiento, nada que ejecutar
	},
	ExecWaitingForUser: {
		ExecThinking: true, // el usuario respondió, se retoma
		ExecIdle:     true, // se abandonó la espera
	},
	ExecExecuting: {
		ExecVerifying:  true,
		ExecRecovering: true, // falló durante la ejecución
		ExecBlocked:    true, // riesgo/autorización detenida en seco
	},
	ExecVerifying: {
		ExecCompleted:  true,
		ExecRecovering: true, // la verificación encontró que algo salió mal
	},
	ExecRecovering: {
		ExecExecuting:      true, // retry
		ExecWaitingForUser: true, // necesita que el usuario decida cómo seguir
		ExecBlocked:        true, // no se pudo recuperar
	},
	ExecCompleted: {
		ExecIdle: true,
	},
	ExecBlocked: {
		ExecIdle: true,
	},
}

// CanTransition verifica si el cambio from→to está en la tabla —
// expuesto para que quien llame a TransitionTo pueda chequear antes
// sin provocar el error, si lo prefiere (ej. para decidir si conviene
// intentarlo antes de loggear un fallo).
func CanTransition(from, to ExecutionState) bool {
	return executionTransitions[from][to]
}

// InvalidTransitionError se devuelve cuando TransitionTo pide un salto
// que la tabla no contempla — nunca se ejecuta en silencio ni se
// "corrige" a la transición más parecida: eso sería inventar una
// decisión que no le corresponde a este bloque.
type InvalidTransitionError struct {
	From ExecutionState
	To   ExecutionState
}

func (e *InvalidTransitionError) Error() string {
	return "autonomy: transición inválida de " + string(e.From) + " a " + string(e.To)
}

// AutonomousState es la vista consultable del estado cognitivo actual.
// Es una orquestación sobre planning.Goal / memory.PendingClarification /
// el tema de ConversationFlow — no duplica ninguno de los tres, solo los
// referencia y agrega lo que ninguno modela por sí solo: en qué fase de
// tarea autónoma está Ahmelia y desde cuándo.
type AutonomousState struct {
	mu sync.Mutex

	currentGoal  *planning.Goal
	pendingGoals []*planning.Goal
	activeTask   string
	activeTopic  string

	lastActivity    time.Time
	lastInteraction time.Time

	execution      ExecutionState
	executionSince time.Time

	pendingQuestions   []string
	pendingAssumptions []string
}

// New crea el estado autónomo inicial: sin objetivo, sin tarea, en
// ExecIdle — criterio de aceptación #1 del bloque (estado inicial).
func New() *AutonomousState {
	now := time.Now()
	return &AutonomousState{
		currentGoal:     nil,
		pendingGoals:    []*planning.Goal{},
		activeTask:      "",
		activeTopic:     "",
		lastActivity:    now,
		lastInteraction: now,
		execution:       ExecIdle,
		executionSince:  now,
	}
}

/* =======================
   OBJETIVOS
   Punteros directos a planning.Goal — nunca una copia de sus campos.
======================= */

// CurrentGoal devuelve el objetivo activo, o nil si no hay ninguno.
func (s *AutonomousState) CurrentGoal() *planning.Goal {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.currentGoal
}

// SetCurrentGoal reemplaza el objetivo activo (Goal A → Goal B, criterio
// de aceptación #2). Pasar nil es válido: significa "sin objetivo activo
// ahora", no un error.
func (s *AutonomousState) SetCurrentGoal(g *planning.Goal) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.currentGoal = g
	s.lastActivity = time.Now()
}

// PendingGoals devuelve una copia del slice (no del contenido: cada
// *planning.Goal sigue siendo el mismo puntero) para que el caller no
// pueda mutar la lista interna sin pasar por AddPendingGoal/RemovePendingGoal.
func (s *AutonomousState) PendingGoals() []*planning.Goal {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*planning.Goal, len(s.pendingGoals))
	copy(out, s.pendingGoals)
	return out
}

// AddPendingGoal agrega un objetivo a la cola de pendientes — este es el
// dato que 73 (Goal Monitor) va a recorrer para decidir si vale la pena
// recordarlo ("tenemos pendiente el prototipo, ¿retomamos?"). Este
// bloque solo guarda la cola; decidir cuándo recordarla no es tarea de
// B72.
func (s *AutonomousState) AddPendingGoal(g *planning.Goal) {
	if g == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pendingGoals = append(s.pendingGoals, g)
	s.lastActivity = time.Now()
}

// RemovePendingGoal saca un objetivo de la cola por ID (ej. cuando pasa
// a ser CurrentGoal, o se completa/cancela directamente desde la cola).
func (s *AutonomousState) RemovePendingGoal(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	kept := s.pendingGoals[:0]
	for _, g := range s.pendingGoals {
		if g.ID != id {
			kept = append(kept, g)
		}
	}
	s.pendingGoals = kept
	s.lastActivity = time.Now()
}

/* =======================
   TAREA / TEMA ACTIVOS
======================= */

// ActiveTask devuelve la descripción corta de la tarea en curso (vacía
// si no hay ninguna).
func (s *AutonomousState) ActiveTask() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.activeTask
}

// SetActiveTask fija la tarea en curso.
func (s *AutonomousState) SetActiveTask(task string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.activeTask = task
	s.lastActivity = time.Now()
}

// ActiveTopic devuelve el tema conversacional activo.
func (s *AutonomousState) ActiveTopic() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.activeTopic
}

// SetActiveTopic se alimenta de language.ConversationFlow.CurrentTopic()
// desde el caller — este bloque no importa language ni recalcula el
// tema por su cuenta, solo lo refleja (ver nota de cabecera).
func (s *AutonomousState) SetActiveTopic(topic string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.activeTopic = topic
}

/* =======================
   ACTIVIDAD / INTERACCIÓN
   Insumo directo para 73 (Goal Monitor): "hace cuánto que no pasa nada
   con este objetivo" se calcula a partir de LastActivity, no se
   reinventa en cada bloque que lo necesite.
======================= */

// RecordInteraction marca que el usuario acaba de interactuar (habló o
// escribió) — distinto de RecordActivity: interacción es siempre del
// usuario, actividad puede venir de Ahmelia ejecutando algo sola en
// 77.
func (s *AutonomousState) RecordInteraction() {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	s.lastInteraction = now
	s.lastActivity = now
}

// RecordActivity marca actividad genérica (de Ahmelia o del usuario)
// sin implicar que hubo un turno de conversación.
func (s *AutonomousState) RecordActivity() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastActivity = time.Now()
}

// LastActivity devuelve cuándo fue la última actividad registrada.
func (s *AutonomousState) LastActivity() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastActivity
}

// LastInteraction devuelve cuándo fue el último turno real de usuario.
func (s *AutonomousState) LastInteraction() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastInteraction
}

/* =======================
   ESTADO DE EJECUCIÓN
======================= */

// Execution devuelve el ExecutionState actual.
func (s *AutonomousState) Execution() ExecutionState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.execution
}

// ExecutionSince devuelve desde cuándo está en el ExecutionState actual.
func (s *AutonomousState) ExecutionSince() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.executionSince
}

// WaitingForUser es un atajo booleano — true si y solo si el estado
// actual es ExecWaitingForUser. Mismo espíritu que
// cognition.StateManager.Busy(): un booleano simple para el caso de uso
// más común, sin obligar al caller a comparar contra el enum.
func (s *AutonomousState) WaitingForUser() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.execution == ExecWaitingForUser
}

// TransitionTo intenta pasar al nuevo ExecutionState. Devuelve
// InvalidTransitionError sin cambiar nada si el salto no está en
// executionTransitions — fail-safe, mismo criterio que
// shared.RequiresConfirmation: mejor rechazar una transición rara que
// dejar que el estado quede en algo inconsistente.
func (s *AutonomousState) TransitionTo(to ExecutionState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !executionTransitions[s.execution][to] {
		return &InvalidTransitionError{From: s.execution, To: to}
	}
	s.execution = to
	s.executionSince = time.Now()
	s.lastActivity = s.executionSince
	return nil
}

/* =======================
   PREGUNTAS Y SUPUESTOS PENDIENTES
   Espejo de memory.PendingClarification — nunca se escriben a mano por
   otro camino que no sea SyncPendingClarification/ClearPendingClarification.
======================= */

// PendingQuestions devuelve una copia de las preguntas sin resolver.
func (s *AutonomousState) PendingQuestions() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.pendingQuestions))
	copy(out, s.pendingQuestions)
	return out
}

// PendingAssumptions devuelve una copia de los supuestos sin confirmar.
func (s *AutonomousState) PendingAssumptions() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.pendingAssumptions))
	copy(out, s.pendingAssumptions)
	return out
}

// SyncPendingClarification refleja un memory.PendingClarification real
// (el que dejó la frontera de clarificación B96.22) en este estado, y mueve la
// ejecución a ExecWaitingForUser — la única forma correcta de llegar a
// ese estado, en vez de que algún caller futuro lo fuerce a mano sin
// tener una aclaración real pendiente. pc == nil limpia el pendiente
// (ClearPendingClarification en session.go es el disparador natural de
// esto en core/engine.go).
func (s *AutonomousState) SyncPendingClarification(pc *memory.PendingClarification) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if pc == nil {
		s.pendingQuestions = nil
		s.pendingAssumptions = nil
		if s.execution == ExecWaitingForUser {
			s.execution = ExecThinking
			s.executionSince = time.Now()
		}
		return nil
	}

	s.pendingAssumptions = append([]string{}, pc.PendingAssumptions...)
	s.pendingQuestions = []string{pc.OriginalGoal}

	if s.execution == ExecWaitingForUser {
		return nil
	}
	if !executionTransitions[s.execution][ExecWaitingForUser] {
		return &InvalidTransitionError{From: s.execution, To: ExecWaitingForUser}
	}
	s.execution = ExecWaitingForUser
	s.executionSince = time.Now()
	return nil
}
