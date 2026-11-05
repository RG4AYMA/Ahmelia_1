package autonomy

import (
	"sync"
	"time"
)

// TurnState representa la soberanía del turno en el diálogo.
type TurnState string

const (
	TurnUserSpeaking    TurnState = "USER_SPEAKING"
	TurnAhmeliaIdle     TurnState = "AHMELIA_IDLE"
	TurnAhmeliaSpeaking TurnState = "AHMELIA_SPEAKING"
)

// DeferredTopic representa un tema o decisión pospuesta para no interrumpir el flujo actual.
type DeferredTopic struct {
	ID        string    `json:"id"`
	Topic     string    `json:"topic"`
	GoalID    string    `json:"goal_id,omitempty"`
	Reason    string    `json:"reason"`
	CreatedAt time.Time `json:"created_at"`
}

// ConversationStrategy define la forma táctica en que la decisión entra al flujo conversacional.
type ConversationStrategy string

const (
	StrategyNone             ConversationStrategy = "NONE"
	StrategyInterruptLightly ConversationStrategy = "INTERRUPT_LIGHTLY"
	StrategyAppendContext    ConversationStrategy = "APPEND_CONTEXT"
	StrategyResumeTopic      ConversationStrategy = "RESUME_TOPIC"
	StrategyDefer            ConversationStrategy = "DEFER"
)

// ConversationDecision representa la salida estructurada de B76 para B77 o la capa de diálogo.
type ConversationDecision struct {
	Strategy ConversationStrategy `json:"strategy"`
	Topic    string               `json:"topic,omitempty"`
	Reason   string               `json:"reason"`
	Deferred bool                 `json:"deferred"`
}

// ConversationManager orquesta la estrategia de integración conversacional.
type ConversationManager struct {
	mu             sync.Mutex
	currentTopic   string
	turnState      TurnState
	deferredTopics []DeferredTopic
}

// NewConversationManager instancia el gestor conversacional autónomo.
func NewConversationManager() *ConversationManager {
	return &ConversationManager{
		turnState:      TurnAhmeliaIdle,
		deferredTopics: make([]DeferredTopic, 0),
	}
}

// SetTurnState actualiza el estado del turno actual (ej. USER_SPEAKING, AHMELIA_IDLE).
func (cm *ConversationManager) SetTurnState(state TurnState) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.turnState = state
}

// SetCurrentTopic sincroniza el tema actual proveniente de la capa de lenguaje.
func (cm *ConversationManager) SetCurrentTopic(topic string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.currentTopic = topic
}

// CurrentTopic obtiene el tema de conversación registrado.
func (cm *ConversationManager) CurrentTopic() string {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	return cm.currentTopic
}

// DeferTopic guarda un tema o meta en la cola de diferidos.
func (cm *ConversationManager) DeferTopic(topic string, goalID string, reason string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	dt := DeferredTopic{
		ID:        generateUUID(),
		Topic:     topic,
		GoalID:    goalID,
		Reason:    reason,
		CreatedAt: time.Now(),
	}
	cm.deferredTopics = append(cm.deferredTopics, dt)
}

// PopDeferredTopic extrae y remueve el tema diferido más antiguo (FIFO).
func (cm *ConversationManager) PopDeferredTopic() (DeferredTopic, bool) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if len(cm.deferredTopics) == 0 {
		return DeferredTopic{}, false
	}

	dt := cm.deferredTopics[0]
	cm.deferredTopics = cm.deferredTopics[1:]
	return dt, true
}

// DeferredCount retorna el número de temas diferidos acumulados.
func (cm *ConversationManager) DeferredCount() int {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	return len(cm.deferredTopics)
}

// EvaluateStrategy evalúa un InitiativeDecision de B75 y determina la ConversationStrategy adecuada.
func (cm *ConversationManager) EvaluateStrategy(decision InitiativeDecision) ConversationDecision {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Regla 1: Si el usuario está hablando (USER_SPEAKING)
	if cm.turnState == TurnUserSpeaking {
		if decision.Action == ActionAskUser && decision.Risk == RiskHigh {
			return ConversationDecision{
				Strategy: StrategyInterruptLightly,
				Topic:    cm.currentTopic,
				Reason:   "Interrupción de alta prioridad por señal crítica durante intervención del usuario",
				Deferred: false,
			}
		}

		// Cualquier otra decisión proactiva se difiere para no interrumpir al usuario
		if decision.Action != ActionNoAction && decision.Action != ActionWait {
			dt := DeferredTopic{
				ID:        generateUUID(),
				Topic:     cm.currentTopic,
				GoalID:    decision.TargetGoalID,
				Reason:    decision.Reason,
				CreatedAt: time.Now(),
			}
			cm.deferredTopics = append(cm.deferredTopics, dt)

			return ConversationDecision{
				Strategy: StrategyDefer,
				Topic:    cm.currentTopic,
				Reason:   "El usuario está hablando; la decisión ha sido postergada",
				Deferred: true,
			}
		}

		return ConversationDecision{
			Strategy: StrategyNone,
			Topic:    cm.currentTopic,
			Reason:   "El usuario está hablando y no hay acción crítica pendiente",
			Deferred: false,
		}
	}

	// Regla 2: Ahmelia libre o conversando + RESUME_GOAL
	if decision.Action == ActionResumeGoal {
		targetTopic := decision.TargetGoalID

		// Si existen temas diferidos en cola, se prioriza recuperar el más antiguo
		if len(cm.deferredTopics) > 0 {
			dt := cm.deferredTopics[0]
			cm.deferredTopics = cm.deferredTopics[1:]
			if dt.Topic != "" {
				targetTopic = dt.Topic
			}
		}

		return ConversationDecision{
			Strategy: StrategyResumeTopic,
			Topic:    targetTopic,
			Reason:   "Reanudación activa de objetivo o tema postergado",
			Deferred: false,
		}
	}

	// Regla 3: INFORM_USER -> Anexar contexto sin alterar el tema actual en curso
	if decision.Action == ActionInformUser {
		return ConversationDecision{
			Strategy: StrategyAppendContext,
			Topic:    cm.currentTopic,
			Reason:   "Notificación contextual agregada como nota sin romper el hilo principal",
			Deferred: false,
		}
	}

	// Regla 4: SUGGEST -> Anexar propuesta al hilo
	if decision.Action == ActionSuggest {
		return ConversationDecision{
			Strategy: StrategyAppendContext,
			Topic:    cm.currentTopic,
			Reason:   "Sugerencia proactiva incorporada al flujo conversacional",
			Deferred: false,
		}
	}

	// Regla 5: ASK_USER fuera de habla del usuario
	if decision.Action == ActionAskUser {
		return ConversationDecision{
			Strategy: StrategyInterruptLightly,
			Topic:    cm.currentTopic,
			Reason:   "Consulta proactiva iniciada en estado receptivo",
			Deferred: false,
		}
	}

	// Regla 6: WAIT o NO_ACTION
	return ConversationDecision{
		Strategy: StrategyNone,
		Topic:    cm.currentTopic,
		Reason:   "Sin intervención requerida en la interacción",
		Deferred: false,
	}
}

// TurnState returns a detached read of the current conversational turn owner.
// It is an observation API; it does not authorize or schedule initiative.
func (cm *ConversationManager) TurnState() TurnState {
	if cm == nil {
		return TurnAhmeliaIdle
	}
	cm.mu.Lock()
	defer cm.mu.Unlock()
	return cm.turnState
}
