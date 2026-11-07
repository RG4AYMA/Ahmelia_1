package autonomy

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

type ObservationType string

const (
	TypeFileChanged      ObservationType = "FILE_CHANGED"
	TypeAppFocused       ObservationType = "APP_FOCUSED"
	TypeSystemSignal     ObservationType = "SYSTEM_SIGNAL"
	TypeTimerElapsed     ObservationType = "TIMER_ELAPSED"
	TypeGoalStateChanged ObservationType = "GOAL_STATE_CHANGED"
)

type ContextObservation struct {
	ID        string                 `json:"id"`
	Type      ObservationType        `json:"type"`
	Source    string                 `json:"source"`
	Payload   map[string]interface{} `json:"payload"`
	Timestamp time.Time              `json:"timestamp"`
	Processed bool                   `json:"processed"`
	hash      string
}

type ObserverConfig struct {
	MaxBufferSize       int
	DeduplicateWindowMs time.Duration
}

type ListenerFunc func(obs ContextObservation)

type ContextObserver struct {
	mu           sync.RWMutex
	config       ObserverConfig
	buffer       []ContextObservation
	listeners    map[int]ListenerFunc
	nextListener int
	recentHashes map[string]time.Time
}

func NewContextObserver(cfg ObserverConfig) *ContextObserver {
	if cfg.MaxBufferSize <= 0 {
		cfg.MaxBufferSize = 50
	}
	if cfg.DeduplicateWindowMs <= 0 {
		cfg.DeduplicateWindowMs = 1000 * time.Millisecond
	}

	return &ContextObserver{
		config:       cfg,
		buffer:       make([]ContextObservation, 0, cfg.MaxBufferSize),
		listeners:    make(map[int]ListenerFunc),
		recentHashes: make(map[string]time.Time),
	}
}

func (co *ContextObserver) Observe(obsType ObservationType, source string, payload map[string]interface{}) *ContextObservation {
	co.mu.Lock()

	now := time.Now()
	hash := co.calculateHash(obsType, source, payload)

	if lastSeen, exists := co.recentHashes[hash]; exists {
		if now.Sub(lastSeen) < co.config.DeduplicateWindowMs {
			co.mu.Unlock()
			return nil
		}
	}

	co.recentHashes[hash] = now
	co.cleanupExpiredHashes(now)

	obs := ContextObservation{
		ID:        generateUUID(),
		Type:      obsType,
		Source:    source,
		Payload:   payload,
		Timestamp: now,
		Processed: false,
		hash:      hash,
	}

	co.pushToBufferLocked(obs)
	listenersCopy := make([]ListenerFunc, 0, len(co.listeners))
	for _, l := range co.listeners {
		listenersCopy = append(listenersCopy, l)
	}

	co.mu.Unlock()

	co.notifyListeners(listenersCopy, obs)

	return &obs
}

// Subscribe registra un listener y devuelve la función para
// desuscribirlo. Bug real encontrado al auditar este archivo: la
// versión anterior guardaba los listeners en un slice y devolvía un
// closure que iteraba sobre ellos sin sacar nada (`_ = i; _ = l`) —
// Subscribe() nunca desuscribía en realidad, cualquier listener
// quedaba escuchando para siempre. Se cambió a un map con ID propio
// (asignado una vez, nunca reciclado) precisamente porque un índice
// de slice se corrompe si dos suscripciones se dan de baja en
// cualquier orden que no sea el inverso al de alta — con el map, el
// ID de esta suscripción sigue siendo válido sin importar qué otras
// entren o salgan.
func (co *ContextObserver) Subscribe(listener ListenerFunc) func() {
	co.mu.Lock()
	defer co.mu.Unlock()

	id := co.nextListener
	co.nextListener++
	co.listeners[id] = listener

	return func() {
		co.mu.Lock()
		defer co.mu.Unlock()
		delete(co.listeners, id)
	}
}

func (co *ContextObserver) GetUnprocessedObservations() []ContextObservation {
	co.mu.RLock()
	defer co.mu.RUnlock()

	unprocessed := make([]ContextObservation, 0)
	for _, obs := range co.buffer {
		if !obs.Processed {
			unprocessed = append(unprocessed, obs)
		}
	}
	return unprocessed
}

func (co *ContextObserver) MarkAsProcessed(id string) {
	co.mu.Lock()
	defer co.mu.Unlock()

	for i := range co.buffer {
		if co.buffer[i].ID == id {
			co.buffer[i].Processed = true
			break
		}
	}
}

func (co *ContextObserver) pushToBufferLocked(obs ContextObservation) {
	if len(co.buffer) >= co.config.MaxBufferSize {
		co.buffer = co.buffer[1:]
	}
	co.buffer = append(co.buffer, obs)
}

func (co *ContextObserver) notifyListeners(listeners []ListenerFunc, obs ContextObservation) {
	for _, listener := range listeners {
		go listener(obs)
	}
}

func (co *ContextObserver) calculateHash(obsType ObservationType, source string, payload map[string]interface{}) string {
	bytes, _ := json.Marshal(payload)
	raw := fmt.Sprintf("%s:%s:%s", obsType, source, string(bytes))
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

func (co *ContextObserver) cleanupExpiredHashes(now time.Time) {
	for h, t := range co.recentHashes {
		if now.Sub(t) > co.config.DeduplicateWindowMs*2 {
			delete(co.recentHashes, h)
		}
	}
}

// Generación de UUID RFC4122 nativa usando la librería estándar crypto/rand
func generateUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
