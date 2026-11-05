package dialogue

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strings"
	"time"
)

// ============================================================================
// CONVERSATIONAL_MEMORY.GO — Bloque 62, Fase 5 (Conversational Mind), Capa 5.2
// ============================================================================
//
// ConversationFlow (Bloque 18) ya trackea temas — pero vive y muere con
// el proceso: se instancia nueva en cada NewEngine() (language.NewConversationFlow(),
// sin Load ni Save), y topicsDiscussed es una lista plana sin estado
// (no hay "activo" vs. "pendiente" vs. "cerrado"). Eso alcanza para
// "de qué hablamos EN ESTA sesión", pero no para el ejemplo central del
// roadmap: "lunes hablamos de Unity, martes de música, miércoles
// 'continuemos con Unity donde lo dejamos'" — eso exige sobrevivir al
// cierre del proceso.
//
// Conversational Memory es esa capa: persiste a disco (mismo criterio
// que learning.json/knowledge_memory.json — Load/Save, arranca vacía
// sin archivo previo), y agrega lo que ConversationFlow no tenía:
// ESTADO por tema (activo/pendiente/cerrado) y hace la distinción
// reciente vs. remoto que pide el roadmap.
//
// No reemplaza a ConversationFlow (18) ni a WorkingMemory (cognition,
// 13, TTL 5 min) — son tres capas de memoria conversacional con
// alcances distintos y complementarios:
//   WorkingMemory (13)      → últimos 5 minutos, se borra solo
//   ConversationFlow (18)   → toda la sesión actual, se pierde al cerrar
//   ConversationalMemory(62)→ persiste entre sesiones/días

// TopicStatus es el estado de un tema — lo que ConversationFlow (18)
// nunca modeló.
type TopicStatus string

const (
	TopicActive  TopicStatus = "activo"    // el tema de AHORA MISMO — solo puede haber uno
	TopicPending TopicStatus = "pendiente" // se tocó y se dejó sin cerrar — candidato a "retomar"
	TopicClosed  TopicStatus = "cerrado"   // el usuario lo dio por terminado explícitamente
)

// TopicRecord es lo que se sabe de un tema a través del tiempo —
// across sesiones, no solo la actual.
type TopicRecord struct {
	Name         string
	Status       TopicStatus
	FirstTouched string // RFC3339
	LastTouched  string // RFC3339
	TurnCount    int    // cuántas veces se tocó, sumando TODAS las sesiones
	LastNote     string // opcional: en qué quedó — vacío si nadie lo especificó
}

// ConversationalMemory persiste el estado de los temas por Name.
type ConversationalMemory struct {
	Topics map[string]*TopicRecord `json:"topics"`
}

// ConversationalMemoryPath — mismo criterio que KnowledgeMemoryPath
// (solving/knowledge_memory.go, Bloque 49): persiste de inmediato en
// cada actualización.
const ConversationalMemoryPath = "memory/conversational_memory.json"

// RecentThreshold / RemoteThreshold definen el corte entre "contexto
// reciente" y "contexto remoto" que pide el roadmap. 24h separa "hoy"
// de "otro día" — no es una ciencia exacta, es un punto de partida
// razonable, ajustable sin tocar el resto del archivo.
const RecentThreshold = 24 * time.Hour

// NewConversationalMemory arma una memoria vacía.
func NewConversationalMemory() *ConversationalMemory {
	return &ConversationalMemory{Topics: make(map[string]*TopicRecord)}
}

// LoadConversationalMemory carga el archivo si existe; si no, arranca
// vacía — mismo patrón que LoadLearningState/LoadKnowledgeMemory.
func LoadConversationalMemory(path string) *ConversationalMemory {
	if data, err := ioutil.ReadFile(path); err == nil {
		var cm ConversationalMemory
		if err := json.Unmarshal(data, &cm); err == nil && cm.Topics != nil {
			return &cm
		}
	}
	return NewConversationalMemory()
}

// Save persiste a disco.
func (cm *ConversationalMemory) Save(path string) error {
	data, err := json.MarshalIndent(cm, "", "  ")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(path, data, 0644)
}

// Touch registra actividad sobre un tema: lo marca Active, actualiza
// LastTouched/TurnCount, y — esto es lo que ConversationFlow no
// hacía — degrada a Pending cualquier OTRO tema que estuviera Active
// (solo puede haber un tema activo genuino a la vez; el anterior no se
// cierra solo, queda pendiente, candidato natural a "retomar").
func (cm *ConversationalMemory) Touch(topic string) {
	topic = strings.TrimSpace(topic)
	if topic == "" {
		return
	}
	now := time.Now().Format(time.RFC3339)

	for name, rec := range cm.Topics {
		if name != topic && rec.Status == TopicActive {
			rec.Status = TopicPending
		}
	}

	rec, exists := cm.Topics[topic]
	if !exists {
		rec = &TopicRecord{Name: topic, FirstTouched: now}
		cm.Topics[topic] = rec
	}
	rec.Status = TopicActive
	rec.LastTouched = now
	rec.TurnCount++
}

// Close marca un tema como cerrado explícitamente — false si el tema
// no existe en la memoria.
func (cm *ConversationalMemory) Close(topic string) bool {
	rec, ok := cm.find(topic)
	if !ok {
		return false
	}
	rec.Status = TopicClosed
	return true
}

// SetNote deja constancia de en qué quedó un tema — opcional, no todos
// los Touch() van a tener una nota asociada, y eso está bien (mismo
// criterio que MemoryEntry.FirstValidatedAt en knowledge_memory.go: un
// campo vacío es un dato real, "nadie lo anotó", no un error).
func (cm *ConversationalMemory) SetNote(topic, note string) bool {
	rec, ok := cm.find(topic)
	if !ok {
		return false
	}
	rec.LastNote = note
	return true
}

// Recall busca un tema por nombre, con la misma comparación laxa
// (substring en cualquier dirección) que topicWasDiscussed en
// el antiguo selector de reanudación — el usuario rara vez nombra un tema con las
// mismas palabras exactas con que se registró.
func (cm *ConversationalMemory) Recall(topic string) (*TopicRecord, bool) {
	return cm.find(topic)
}

func (cm *ConversationalMemory) find(topic string) (*TopicRecord, bool) {
	target := strings.ToLower(strings.TrimSpace(topic))
	if target == "" {
		return nil, false
	}
	if rec, ok := cm.Topics[topic]; ok {
		return rec, true
	}
	for name, rec := range cm.Topics {
		n := strings.ToLower(name)
		if strings.Contains(n, target) || strings.Contains(target, n) {
			return rec, true
		}
	}
	return nil, false
}

// PendingTopics devuelve los temas que se tocaron y quedaron sin
// cerrar — los candidatos naturales para "qué nos quedó pendiente".
func (cm *ConversationalMemory) PendingTopics() []*TopicRecord {
	var out []*TopicRecord
	for _, rec := range cm.Topics {
		if rec.Status == TopicPending {
			out = append(out, rec)
		}
	}
	return out
}

// RecentTopics devuelve los temas tocados dentro de RecentThreshold —
// "contexto reciente" que pide el roadmap.
func (cm *ConversationalMemory) RecentTopics() []*TopicRecord {
	return cm.filterByAge(true)
}

// RemoteTopics devuelve los temas tocados hace más de RecentThreshold
// — "contexto remoto".
func (cm *ConversationalMemory) RemoteTopics() []*TopicRecord {
	return cm.filterByAge(false)
}

func (cm *ConversationalMemory) filterByAge(recent bool) []*TopicRecord {
	var out []*TopicRecord
	now := time.Now()
	for _, rec := range cm.Topics {
		t, err := time.Parse(time.RFC3339, rec.LastTouched)
		if err != nil {
			continue
		}
		isRecent := now.Sub(t) <= RecentThreshold
		if isRecent == recent {
			out = append(out, rec)
		}
	}
	return out
}

// Describe narra en qué quedó un tema — la respuesta a "continuemos
// con Unity donde lo dejamos", SIN repetir información que ya se dijo:
// solo cuándo fue la última vez y la nota, si hay una.
func (rec *TopicRecord) Describe() string {
	if rec == nil {
		return "No tengo ningún registro de ese tema."
	}
	when := rec.LastTouched
	if t, err := time.Parse(time.RFC3339, rec.LastTouched); err == nil {
		when = formatSpanishDay(t)
	}

	base := fmt.Sprintf("La última vez que hablamos de %s fue %s (%d vez/veces en total). Estado: %s.",
		rec.Name, when, rec.TurnCount, rec.Status)
	if rec.LastNote != "" {
		base += " Quedamos en: " + rec.LastNote
	}
	return base
}

var spanishWeekdays = map[time.Weekday]string{
	time.Monday:    "el lunes",
	time.Tuesday:   "el martes",
	time.Wednesday: "el miércoles",
	time.Thursday:  "el jueves",
	time.Friday:    "el viernes",
	time.Saturday:  "el sábado",
	time.Sunday:    "el domingo",
}

// formatSpanishDay dice "hoy"/"ayer" para lo inmediato, y el día de la
// semana ("el lunes") para lo demás dentro de la última semana — más
// natural que una fecha ISO en una conversación hablada.
func formatSpanishDay(t time.Time) string {
	now := time.Now()
	days := int(now.Sub(t).Hours() / 24)
	switch {
	case now.Format("2006-01-02") == t.Format("2006-01-02"):
		return "hoy"
	case days <= 1:
		return "ayer"
	case days < 7:
		return spanishWeekdays[t.Weekday()]
	default:
		return t.Format("el 02/01")
	}
}

// ExtractMemoryTarget busca los triggers que SÍ esperan un tema
// después ("donde lo dejamos con X", "en que quedamos con X") y
// devuelve X. Para triggers sin tema ("que temas tenemos pendientes")
// devuelve "" — quien llame debe interpretar eso como "listar
// pendientes", no como un tema vacío.
func ExtractMemoryTarget(normalizedText string) string {
	withTarget := []string{"donde lo dejamos con", "en que quedamos con", "de que hablamos hace tiempo de"}
	for _, trigger := range withTarget {
		if idx := strings.Index(normalizedText, trigger); idx != -1 {
			return strings.TrimSpace(normalizedText[idx+len(trigger):])
		}
	}
	return ""
}

// ── Trigger de consulta ──────────────────────────────────────────────

var conversationalMemoryTriggers = []string{
	"donde lo dejamos con",
	"en que quedamos con",
	"que temas tenemos pendientes",
	"que quedo pendiente",
	"de que hablamos hace tiempo",
}

// ParseConversationalMemoryQuery detecta si se pregunta por el estado
// persistente de un tema entre sesiones — distinto de
// ParseConversationSummaryQuery (Bloque 18, resumen de la sesión
// ACTUAL) y de Working Memory (cognition, 13, TTL 5 min).
func ParseConversationalMemoryQuery(normalizedText string) bool {
	for _, trigger := range conversationalMemoryTriggers {
		if strings.Contains(normalizedText, trigger) {
			return true
		}
	}
	return false
}
