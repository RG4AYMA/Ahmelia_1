package resource

import (
	"sync"
	"time"
)

// ProgressSignal es la señal tipada de progreso que se emite cuando una tarea
// de modelo supera un umbral de espera. No contiene texto: la redacción pertenece
// al contrato de respuesta (R5).
type ProgressSignal struct {
	Class     TaskClass     // clase de tarea: interpretation | generation | proposal
	StartedAt time.Time     // cuándo empezó la tarea
	Estimate  *Estimate     // estimación del perfil si existe (nil si no hay perfil)
	Threshold time.Duration // umbral que se superó para emitir esta señal
}

// Estimate contiene la estimación de tiempo basada en el perfil de hardware.
type Estimate struct {
	Model              string        // modelo que se está usando
	FirstResponseP50   time.Duration // p50 medido para primera respuesta
	FirstResponseP95   time.Duration // p95 medido para primera respuesta
	TokensPerSecondP50 float64       // p50 tokens/seg
}

// ProgressSink es la interfaz que recibe señales de progreso.
// Implementaciones pueden loguear, enviar a canal, actualizar UI, etc.
type ProgressSink interface {
	// OnProgress recibe una señal de progreso. No debe bloquear.
	OnProgress(sig ProgressSignal)
}

// NoOpSink es un sink que no hace nada (default).
type NoOpSink struct{}

func (NoOpSink) OnProgress(sig ProgressSignal) {}

// ThresholdConfig define los umbrales para emitir señales.
type ThresholdConfig struct {
	// WarnAfter es el tiempo tras el cual se emite la primera señal.
	WarnAfter time.Duration
	// RepeatEvery es el intervalo para repetir la señal mientras la tarea continúe.
	RepeatEvery time.Duration
	// MaxSignals es el máximo de señales por tarea (0 = ilimitado).
	MaxSignals int
}

// DefaultThresholdConfig retorna configuración por defecto.
func DefaultThresholdConfig() ThresholdConfig {
	return ThresholdConfig{
		WarnAfter:   5 * time.Second,
		RepeatEvery: 10 * time.Second,
		MaxSignals:  3,
	}
}

// ProgressEmitter emite señales de progreso basadas en tiempo transcurrido.
// No conoce el modelo ni el governor; solo mide tiempo y emite al sink.
type ProgressEmitter struct {
	sink   ProgressSink
	config ThresholdConfig
	mu     sync.Mutex
	state  map[TaskClass]*emitterState
}

type emitterState struct {
	startedAt    time.Time
	signalsSent  int
	lastSignalAt time.Time
	ticker       *time.Ticker
	stopCh       chan struct{}
}

func NewProgressEmitter(sink ProgressSink, config ThresholdConfig) *ProgressEmitter {
	if sink == nil {
		sink = NoOpSink{}
	}
	if config.WarnAfter <= 0 {
		config = DefaultThresholdConfig()
	}
	return &ProgressEmitter{
		sink:   sink,
		config: config,
		state:  make(map[TaskClass]*emitterState),
	}
}

// Start inicia el seguimiento para una clase de tarea.
// Debe llamarse cuando la tarea entra en el gate (tras Enter exitoso).
func (e *ProgressEmitter) Start(class TaskClass) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if _, ok := e.state[class]; ok {
		return // ya iniciada
	}

	st := &emitterState{
		startedAt:    time.Now(),
		lastSignalAt: time.Now(),
		stopCh:       make(chan struct{}),
	}
	e.state[class] = st

	// Primera señal tras WarnAfter
	go e.run(class, st)
}

// Stop detiene el seguimiento para una clase.
// Debe llamarse cuando la tarea sale del gate (Exit).
func (e *ProgressEmitter) Stop(class TaskClass) {
	e.mu.Lock()
	st, ok := e.state[class]
	if !ok {
		e.mu.Unlock()
		return
	}
	close(st.stopCh)
	delete(e.state, class)
	e.mu.Unlock()
}

func (e *ProgressEmitter) run(class TaskClass, st *emitterState) {
	// Primera espera
	select {
	case <-time.After(e.config.WarnAfter):
	case <-st.stopCh:
		return
	}

	for {
		e.mu.Lock()
		// Verificar límites
		if e.config.MaxSignals > 0 && st.signalsSent >= e.config.MaxSignals {
			e.mu.Unlock()
			return
		}
		now := time.Now()
		st.signalsSent++
		st.lastSignalAt = now
		// Emitir sin lock para no bloquear
		sink := e.sink
		e.mu.Unlock()

		sig := ProgressSignal{
			Class:     class,
			StartedAt: st.startedAt,
			Threshold: now.Sub(st.startedAt),
			Estimate:  nil, // se rellena externamente si hay perfil
		}
		sink.OnProgress(sig)

		// Esperar siguiente intervalo
		st.ticker = time.NewTicker(e.config.RepeatEvery)
		select {
		case <-st.ticker.C:
			st.ticker.Stop()
			continue
		case <-st.stopCh:
			st.ticker.Stop()
			return
		}
	}
}

// GetState devuelve el estado actual para una clase (para debug/observabilidad).
func (e *ProgressEmitter) GetState(class TaskClass) (startedAt time.Time, signalsSent int, running bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	st, ok := e.state[class]
	if !ok {
		return time.Time{}, 0, false
	}
	return st.startedAt, st.signalsSent, true
}
