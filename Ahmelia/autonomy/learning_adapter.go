package autonomy

import (
	"context"
	"fmt"
	"sync"

	"ahmelia/memory_intelligence"
)

// LearningMemoryAdapter conecta B78 con el historial de experiencias y el
// estado de aprendizaje que ya existen en memory_intelligence.
//
// No crea una memoria paralela. EventStore conserva el hecho bruto de la
// ejecución y LearningStore deriva de ese historial la confianza por acción.
// El adaptador solo traduce VerificationResult al contrato real de esas dos
// capas.
//
// B93.3: antes de este bloque, el adaptador guardaba *EventLog/*LearningState
// directo y protegía sus propias llamadas con un mutex local — pero ese
// mutex no protegía nada contra core/engine.go, que tocaba los mismos punteros
// desde la goroutine de conversación sin ningún lock. Ahora ambos lados
// hablan con el mismo EventStore/LearningStore (dueño único, memory_intelligence
// package), y el mutex de acá solo protege el estado propio del adaptador
// (el mapa `recorded` de deduplicación).
type eventRecorder interface {
	Append(action, target string, outcome memory_intelligence.Outcome, sessionID string)
	Snapshot() []memory_intelligence.RawEvent
}

type idempotentEventRecorder interface {
	AppendOnce(action, target string, outcome memory_intelligence.Outcome, sessionID, experienceID string) bool
}

type LearningMemoryAdapter struct {
	mu sync.Mutex

	events        eventRecorder
	learningStore *memory_intelligence.LearningStore
	sessionID     string
	learningPath  string

	// Evita registrar dos veces el mismo resultado cuando una integración
	// externa llama RecordOutcome y luego UpdateState sobre la misma acción.
	recorded map[string]struct{}
}

// NewLearningMemoryAdapter crea el adaptador B82 sobre los Store B93.3.
func NewLearningMemoryAdapter(
	eventStore *memory_intelligence.EventStore,
	learningStore *memory_intelligence.LearningStore,
	sessionID string,
) *LearningMemoryAdapter {
	return newLearningMemoryAdapter(
		eventStore,
		learningStore,
		sessionID,
		memory_intelligence.LearningStatePath,
	)
}

func newLearningMemoryAdapter(
	events eventRecorder,
	learningStore *memory_intelligence.LearningStore,
	sessionID string,
	learningPath string,
) *LearningMemoryAdapter {
	return &LearningMemoryAdapter{
		events:        events,
		learningStore: learningStore,
		sessionID:     sessionID,
		learningPath:  learningPath,
		recorded:      make(map[string]struct{}),
	}
}

// RecordOutcome implementa CycleLearner.
// El Cycle llama esta fase únicamente después de una verificación exitosa.
func (a *LearningMemoryAdapter) RecordOutcome(
	ctx context.Context,
	req ActionRequest,
	verif VerificationResult,
) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("B82: contexto cancelado: %w", err)
	}
	if !verif.Success {
		return fmt.Errorf("B82: RecordOutcome recibió un resultado no exitoso")
	}
	return a.record(req, true, verif.ErrorReason, evidenceStatus(verif), experienceID(req, verif))
}

// UpdateState implementa CycleUpdater.
// Registra fallos que no pasan por LEARN y, en ambos caminos, recalcula
// ActionConfidence/FailurePatterns a partir del EventStore completo.
func (a *LearningMemoryAdapter) UpdateState(
	ctx context.Context,
	req ActionRequest,
	verif VerificationResult,
) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("B82: contexto cancelado: %w", err)
	}

	if !verif.Success && !verificationOutcomeUnknown(verif) {
		if err := a.record(req, false, verif.ErrorReason, evidenceStatus(verif), experienceID(req, verif)); err != nil {
			return err
		}
	}

	if a.learningStore == nil || a.events == nil {
		return fmt.Errorf("B82: EventStore o LearningStore no configurado")
	}

	// La derivación se hace sobre un snapshot del historial completo — el
	// propio LearningStore toma su lock por separado, a propósito (ver
	// nota en LearningStore.UpdateFromReflection sobre orden de locks).
	events := a.events.Snapshot()
	a.learningStore.UpdateFromReflection(events)

	if err := a.learningStore.Save(a.learningPath); err != nil {
		return fmt.Errorf("B82: guardar LearningState: %w", err)
	}

	return nil
}

func (a *LearningMemoryAdapter) record(
	req ActionRequest,
	success bool,
	reason string,
	evidence memory_intelligence.EvidenceStatus,
	identity string,
) error {
	if a.events == nil {
		return fmt.Errorf("B82: EventStore no configurado")
	}

	key := req.ID
	if key == "" {
		key = req.Action + "|" + req.Target
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	if _, exists := a.recorded[key]; exists {
		return nil
	}

	outcome := memory_intelligence.Outcome{
		Success:        success,
		Reason:         reason,
		EvidenceStatus: evidence,
	}
	if once, ok := a.events.(idempotentEventRecorder); ok {
		once.AppendOnce(req.Action, req.Target, outcome, a.sessionID, identity)
	} else {
		a.events.Append(req.Action, req.Target, outcome, a.sessionID)
	}
	a.recorded[key] = struct{}{}
	return nil
}

func experienceID(req ActionRequest, verif VerificationResult) string {
	if authority := verif.AuthorityResult; authority != nil && authority.AuthorityWorkID() != "" && authority.AuthorityAttemptNumber() > 0 {
		return fmt.Sprintf("work:%s:attempt:%d", authority.AuthorityWorkID(), authority.AuthorityAttemptNumber())
	}
	if req.ID == "" {
		return ""
	}
	return "task:" + req.ID
}

func evidenceStatus(verif VerificationResult) memory_intelligence.EvidenceStatus {
	switch verif.ObservationStatus {
	case ObservationConfirmed:
		return memory_intelligence.EvidenceConfirmed
	case ObservationContradicted:
		return memory_intelligence.EvidenceContradicted
	case ObservationInsufficient:
		return memory_intelligence.EvidenceInsufficient
	default:
		return memory_intelligence.EvidenceUnknown
	}
}
