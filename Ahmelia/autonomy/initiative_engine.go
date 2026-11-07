package autonomy

import (
	"sync"
	"time"
)

// DecisionAction define la acción proactiva propuesta por el motor de iniciativa.
type DecisionAction string

const (
	ActionNoAction   DecisionAction = "NO_ACTION"
	ActionWait       DecisionAction = "WAIT"
	ActionAskUser    DecisionAction = "ASK_USER"
	ActionInformUser DecisionAction = "INFORM_USER"
	ActionSuggest    DecisionAction = "SUGGEST"
	ActionExecute    DecisionAction = "EXECUTE"
	ActionResumeGoal DecisionAction = "RESUME_GOAL"
)

// RiskLevel define la evaluación preliminar de riesgo de la decisión.
type RiskLevel string

const (
	RiskLow    RiskLevel = "LOW"
	RiskMedium RiskLevel = "MEDIUM"
	RiskHigh   RiskLevel = "HIGH"
)

// InitiativeDecision representa el veredicto puro del B75 (sin ejecutar).
type InitiativeDecision struct {
	ID           string         `json:"id"`
	Action       DecisionAction `json:"action"`
	Reason       string         `json:"reason"`
	Risk         RiskLevel      `json:"risk"`
	TargetGoalID string         `json:"target_goal_id,omitempty"`
	Timestamp    time.Time      `json:"timestamp"`
}

// NewInitiativeDecisionFromRecovery creates a pure initiative decision for a
// recovery event. It does not execute anything; B91.4 uses it to hand the
// recovery result to the existing Initiative/Conversation pipeline.
func NewInitiativeDecisionFromRecovery(action DecisionAction, reason, goalID string, risk RiskLevel) InitiativeDecision {
	return InitiativeDecision{
		ID:           generateUUID(),
		Action:       action,
		Reason:       reason,
		Risk:         risk,
		TargetGoalID: goalID,
		Timestamp:    time.Now(),
	}
}

// InitiativeConfig contiene únicamente parámetros operativos con uso real en B75.
type InitiativeConfig struct {
	MinInactivityForResume time.Duration
}

// InitiativeEngine analiza las entradas de B72, B73 y B74 para decidir si iniciar acción.
type InitiativeEngine struct {
	mu     sync.RWMutex
	config InitiativeConfig
}

// NewInitiativeEngine instancia el motor con configuración limpia.
func NewInitiativeEngine(cfg InitiativeConfig) *InitiativeEngine {
	if cfg.MinInactivityForResume <= 0 {
		cfg.MinInactivityForResume = 15 * time.Minute
	}

	return &InitiativeEngine{
		config: cfg,
	}
}

// MinInactivityForResume reports the configured threshold. It is read-only and
// lets the caller anticipate a consultation before the threshold is crossed.
func (ie *InitiativeEngine) MinInactivityForResume() time.Duration {
	if ie == nil {
		return 0
	}
	ie.mu.RLock()
	defer ie.mu.RUnlock()
	return ie.config.MinInactivityForResume
}

// Evaluate evalúa las observaciones y estados aplicando la jerarquía estricta:
// CRITICAL > RESUME_GOAL > GOAL_STATE_CHANGED > WAIT > NO_ACTION
func (ie *InitiativeEngine) Evaluate(
	observations []ContextObservation,
	hasPendingGoal bool,
	pendingGoalID string,
	lastUserActivity time.Time,
) InitiativeDecision {
	ie.mu.RLock()
	defer ie.mu.RUnlock()

	now := time.Now()

	// Prioridad 1: Inspeccionar si existe alguna señal CRÍTICA del sistema (CRITICAL > TODO)
	for _, obs := range observations {
		if obs.Processed {
			continue
		}

		if obs.Type == TypeSystemSignal {
			if level, ok := obs.Payload["severity"].(string); ok && level == "CRITICAL" {
				return InitiativeDecision{
					ID:        generateUUID(),
					Action:    ActionAskUser,
					Reason:    "Señal crítica del sistema detectada; requiere confirmación del usuario",
					Risk:      RiskHigh,
					Timestamp: now,
				}
			}
		}
	}

	// Prioridad 2: Reanudación de objetivo por inactividad prolongada del usuario
	if hasPendingGoal && now.Sub(lastUserActivity) >= ie.config.MinInactivityForResume {
		return InitiativeDecision{
			ID:           generateUUID(),
			Action:       ActionResumeGoal,
			Reason:       "Inactividad del usuario superó el umbral configurado con un objetivo pendiente",
			Risk:         RiskLow,
			TargetGoalID: pendingGoalID,
			Timestamp:    now,
		}
	}

	// Prioridad 3: Notificación por cambio de estado en un objetivo
	for _, obs := range observations {
		if obs.Processed {
			continue
		}

		if obs.Type == TypeGoalStateChanged {
			targetID := pendingGoalID
			if idFromPayload, ok := obs.Payload["goal_id"].(string); ok && idFromPayload != "" {
				targetID = idFromPayload
			}

			if status, ok := obs.Payload["status"].(string); ok && status == "active" && targetID != "" {
				return InitiativeDecision{
					ID:           generateUUID(),
					Action:       ActionResumeGoal,
					Reason:       "Activación de un objetivo; el cambio de estado habilita su primera evaluación autónoma",
					Risk:         RiskLow,
					TargetGoalID: targetID,
					Timestamp:    now,
				}
			}

			return InitiativeDecision{
				ID:           generateUUID(),
				Action:       ActionInformUser,
				Reason:       "Cambio detectado en el estado de un objetivo",
				Risk:         RiskLow,
				TargetGoalID: targetID,
				Timestamp:    now,
			}
		}
	}

	// Prioridad 4: Hay observaciones no procesadas pero son eventos normales de trabajo -> WAIT
	unprocessedCount := 0
	for _, obs := range observations {
		if !obs.Processed {
			unprocessedCount++
		}
	}

	if unprocessedCount > 0 {
		return InitiativeDecision{
			ID:        generateUUID(),
			Action:    ActionWait,
			Reason:    "Observaciones normales presentes; no justifican interrupción inmediata",
			Risk:      RiskLow,
			Timestamp: now,
		}
	}

	// Prioridad 5: Sin observaciones ni objetivos pendientes -> NO_ACTION
	return InitiativeDecision{
		ID:        generateUUID(),
		Action:    ActionNoAction,
		Reason:    "Sin observaciones pendientes ni objetivos activos",
		Risk:      RiskLow,
		Timestamp: now,
	}
}
