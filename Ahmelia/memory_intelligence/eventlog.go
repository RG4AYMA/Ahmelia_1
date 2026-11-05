package memory_intelligence

import (
	"encoding/json"
	"os"
	"time"
)

// ============================================================================
// EVENTLOG.GO — Fase 1 (Observación) de Pattern Learning
// ============================================================================
//
// Por qué esto no podía ser Episode (episode.go): Episode acumula la sesión
// activa y al cerrar Ahmelia la COLAPSA a un resumen de una línea
// (Summarize()) — perfecto para "¿qué hiciste hoy?", inútil para contar
// "este flujo ocurrió 18 veces a lo largo de tres semanas", porque el detalle
// evento por evento se pierde justo ahí. EventLog es la capa de abajo: nunca
// resume, nunca interpreta — solo acumula el hecho crudo (qué pasó, cuándo,
// en qué sesión, si funcionó) para que Fase 2 (secuencias) y Fase 3
// (patrones) puedan recalcular sobre el historial completo en cualquier
// momento. Tampoco es Knowledge (memory/long.go): Knowledge ya interpreta
// (Confidence, Importance, Tags, Uses agregado); esto es el material crudo
// ANTES de esa interpretación.

const EventLogPath = "memory/events.json"

// Outcome describe cómo terminó una acción — no solo si funcionó, sino por
// qué. Va en su propio tipo (no como campos sueltos en RawEvent) porque
// Category/Reason son datos de un nivel distinto: RawEvent es "qué se
// ejecutó, cuándo, en qué sesión"; Outcome es "cómo salió".
//
// Nivel 1 (esto): Success, Category, Reason — alcanza para que Fase 5
// (Experiencia) pueda explicar POR QUÉ algo falla seguido, no solo QUE
// falla. Nivel 2 (Duration, Method, Confidence) queda para cuando haya un
// caso real que lo necesite — un campo que nadie llena todavía es peor que
// no tener el campo, invita a leerlo como si tuviera datos.
type Outcome struct {
	Success        bool
	EvidenceStatus EvidenceStatus `json:"evidence_status,omitempty"`
	Category       string         // vacío si Success==true
	Reason         string         // texto legible de por qué — el mismo que ya se le dice al usuario, no uno inventado aparte
}

// EvidenceStatus expresa qué sabe Ahmelia sobre el efecto en el entorno.
// El éxito de una llamada al proveedor no demuestra que el objetivo se cumplió.
type EvidenceStatus string

const (
	EvidenceUnknown      EvidenceStatus = "unknown"
	EvidenceConfirmed    EvidenceStatus = "confirmed"
	EvidenceContradicted EvidenceStatus = "contradicted"
	EvidenceInsufficient EvidenceStatus = "insufficient"
)

func (s EvidenceStatus) SupportsLearning() bool {
	return s == EvidenceConfirmed || s == EvidenceContradicted
}

// Categorías fijas de fallo — strings fijos (mismo criterio que las Action
// de classifier.go), no un enum con iota, para que sean triviales de leer
// en memory/events.json sin traducir números. Se asignan en engine.go, en
// cada uno de los puntos donde ya se sabe por qué falló un comando.
const (
	CategoryProgramNotFound     = "program_not_found"
	CategoryVolumeError         = "volume_error"
	CategoryWindowNotFound      = "window_not_found"
	CategoryFileNotFound        = "file_not_found"
	CategoryClarificationNeeded = "clarification_needed" // no es un fallo real: Ahmelia preguntó algo y todavía no hay respuesta
)

// RawEvent es un hecho sin interpretar: algo que Ahmelia ejecutó, cuándo,
// en qué sesión y cómo salió. Nada más — ni Confidence ni Tags; eso se
// calcula después, a partir de listas de RawEvent, nunca al momento de
// grabar uno solo.
type RawEvent struct {
	Action       string             `json:"action"`
	Target       string             `json:"target"`
	Outcome      Outcome            `json:"outcome"`
	Metadata     ExperienceMetadata `json:"metadata,omitempty"`
	Timestamp    time.Time          `json:"timestamp"`
	SessionID    string             `json:"session_id"`
	ExperienceID string             `json:"experience_id,omitempty"`
}

// ExperienceMetadata links an outcome to the work and instrument that
// produced it. It preserves causal context without treating a tool as a skill.
type ExperienceMetadata struct {
	Objective           string               `json:"objective,omitempty"`
	SkillID             string               `json:"skill_id,omitempty"`
	CapabilityID        string               `json:"capability_id,omitempty"`
	InstrumentID        string               `json:"instrument_id,omitempty"`
	GenerationSource    string               `json:"generation_source,omitempty"`
	TargetEnvironment   string               `json:"target_environment,omitempty"`
	VerificationProfile string               `json:"verification_profile,omitempty"`
	WorkID              string               `json:"work_id,omitempty"`
	AttemptNumber       int                  `json:"attempt_number,omitempty"`
	ObservationSource   string               `json:"observation_source,omitempty"`
	VerificationSource  string               `json:"verification_source,omitempty"`
	ObservationDetails  string               `json:"observation_details,omitempty"`
	ObservedFacts       []ObservedEffectFact `json:"observed_facts,omitempty"`
}

type ObservedEffectFact struct {
	Subject   string `json:"subject"`
	Predicate string `json:"predicate"`
	Expected  string `json:"expected"`
	Observed  string `json:"observed"`
}

// EventLog es el log persistente completo, entre sesiones.
type EventLog struct {
	Events []RawEvent `json:"events"`
}

// LoadEventLog carga el log desde disco. Si no existe todavía (primera vez
// que corre esta fase en esta instalación), arranca vacío sin error — mismo
// criterio que memory.LoadLongMemory.
func LoadEventLog() *EventLog {
	log := &EventLog{Events: []RawEvent{}}
	file, err := os.ReadFile(EventLogPath)
	if err == nil {
		_ = json.Unmarshal(file, log)
	}
	return log
}

// Record agrega un evento y persiste de inmediato — mismo criterio que
// memory.RecordUsage: si el proceso se cierra abrupto (Ctrl+C, corte de luz),
// no se pierde el evento esperando a un Shutdown que nunca llega. El caller
// (core/engine.go) es responsable de filtrar action vacío/"desconocido"
// antes de llamar, igual que ya hace con Episode.Record — este archivo no
// decide eso, solo registra lo que le pasan.
func (el *EventLog) Record(action, target string, outcome Outcome, sessionID string) {
	el.Events = append(el.Events, RawEvent{
		Action:    action,
		Target:    target,
		Outcome:   outcome,
		Timestamp: time.Now(),
		SessionID: sessionID,
	})
	el.Save()
}

func (el *EventLog) Save() {
	data, _ := json.MarshalIndent(el, "", "  ")
	_ = os.WriteFile(EventLogPath, data, 0644)
}

// NOTA DE ALCANCE: este archivo crece sin poda — cada comando exitoso o
// fallido agrega una línea para siempre. Para el volumen esperado en esta
// etapa (cientos o miles de eventos) no es un problema real, pero es una
// decisión pendiente, no un descuido: cuando el archivo empiece a pesar,
// hay que decidir una política de expiración (¿cuántos días? ¿comprimir en
// vez de borrar?) antes de implementarla — no se inventa un número acá.
