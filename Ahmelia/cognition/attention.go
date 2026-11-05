package cognition

import (
	"fmt"
	"sort"
	"strings"

	"ahmelia/shared"
)

// ============================================================================
// ATTENTION.GO — Bloque 12, Fase 2.6 (Integración Cognitiva)
// ============================================================================
//
// El roadmap (sección "3. Attention Manager") es corto a propósito:
//
//   No todo es importante. Debe decidir: Esto importa. Esto no. Como un
//   humano.
//
// Hasta este bloque, cada vez que la rama IntentQuestion de Process()
// llamaba a e.mentalAgenda.Observe("Contexto: "+uc.Narrative(), ...), le
// pasaba el UnifiedContext ENTERO — las nueve fuentes juntas, sin
// filtrar, cada vez, aunque solo una o dos tuvieran algo que de verdad
// importara para la pregunta actual. MentalAgendaV2 (Capa 5.5) fue
// pensada para acumular observaciones relevantes, no para tragarse un
// volcado completo de contexto en cada pregunta — eso es exactamente el
// tipo de ruido que el roadmap dice que Attention Manager debería cortar
// ANTES de que llegue a memoria de trabajo.
//
// Attention Manager no decide qué CONTENIDO existe (eso sigue siendo
// 100% Context Builder) — decide, de lo que Context Builder ya armó, qué
// vale la pena atender ahora y qué se descarta. Mismo principio de
// capas que el resto de Fase 2.6: cada módulo nuevo orquesta lo que ya
// existe, no reimplementa nada.

// Signal es un candidato a "esto importa ahora": una sección del
// UnifiedContext, con un puntaje de relevancia (Score) y el motivo del
// puntaje, para que Attend() no sea una caja negra ni para el código ni
// para quien lea el trace.
type Signal struct {
	Label   string // mismo label que usa UnifiedContext.Narrative() ("Ahora mismo", "Conversación", etc.)
	Content string
	Score   float64
	Reason  string
}

// baseSalience es cuánto le importa a Ahmelia cada tipo de fuente EN
// PROMEDIO, antes de ajustar por el tema activo de la conversación. No
// es arbitrario: Conversación y Ahora mismo (World) son lo más cercano
// a "qué me está pasando ya mismo"; Insights/Aprendizajes son
// retrospectivos — útiles, pero no urgentes salvo que el tema activo
// los toque directamente (ahí entra el bonus de relevancia).
var baseSalience = map[string]float64{
	"Ahora mismo":         0.60,
	"Conversación":        0.70,
	"Objetivos":           0.65,
	"Estado":              0.50,
	"Cronología reciente": 0.45,
	"Hábitos":             0.40,
	"Aprendizajes":        0.35,
	"Reflexión":           0.55,
	"Insights":            0.50,
}

// DefaultAttentionThreshold — una señal por debajo de esto se descarta:
// existe, pero no es lo bastante relevante ahora como para ocupar
// memoria de trabajo. 0.45 deja pasar señales con salience base media
// (Cronología, Insights) SOLO si además tocan el tema activo; deja
// pasar siempre a Conversación/Objetivos/Ahora mismo.
const DefaultAttentionThreshold = 0.45

// DefaultMaxSignals — incluso entre lo que pasa el umbral, atender más
// de 3-4 cosas a la vez ya no es "atención selectiva", es la misma
// sobrecarga que Attention Manager existe para evitar.
const DefaultMaxSignals = 4

// topicBonus se suma al salience base cuando el contenido de la señal
// comparte vocabulario con el tema activo de la conversación — la señal
// "es sobre lo que estamos hablando ahora", no genérica.
const topicBonus = 0.20

// Manager aplica los pesos de baseSalience + el bonus de tema activo
// sobre un UnifiedContext ya construido, y devuelve qué atender.
type Manager struct {
	Threshold  float64
	MaxSignals int
}

// NewManager arma el Attention Manager con los defaults del roadmap.
// Los campos son públicos para que pruebas (o un futuro Bloque de
// auto-ajuste) puedan afinar Threshold/MaxSignals sin tocar este
// archivo.
func NewManager() *Manager {
	return &Manager{Threshold: DefaultAttentionThreshold, MaxSignals: DefaultMaxSignals}
}

// candidates arma la lista completa de señales posibles a partir de un
// UnifiedContext — un candidato por cada sección no vacía. Separado de
// Attend() para poder probarlo sin depender del umbral/tope.
func candidates(uc *UnifiedContext) []Signal {
	if uc == nil {
		return nil
	}
	var out []Signal
	add := func(label, content string) {
		if content == "" {
			return
		}
		out = append(out, Signal{Label: label, Content: content, Score: baseSalience[label]})
	}
	add("Ahora mismo", uc.World)
	add("Conversación", uc.Conversation)
	add("Objetivos", uc.Goals)
	add("Estado", uc.MentalState)
	add("Cronología reciente", uc.Timeline)
	add("Hábitos", uc.Habits)
	add("Aprendizajes", uc.Learning)
	add("Reflexión", uc.Reflection)
	add("Insights", uc.Insights)
	return out
}

// sharesTopic es una comparación deliberadamente simple (¿alguna
// palabra del tema activo aparece en el contenido?) — no un match
// semántico. Mismo criterio de "specific enough, no over-engineering"
// que ParseContextQuery/ParsePipelineQuery: un falso negativo acá solo
// significa que la señal se juzga por su salience base, no que se
// pierda información.
func sharesTopic(content, normTopic string) bool {
	if normTopic == "" {
		return false
	}
	normContent := shared.Normalize(content)
	for _, word := range strings.Fields(normTopic) {
		if len(word) < 4 {
			continue // palabras cortas ("de", "que", "la") no cuentan como señal de tema
		}
		if strings.Contains(normContent, word) {
			return true
		}
	}
	return false
}

// Attend puntúa todas las señales candidatas de un UnifiedContext,
// aplica el bonus de tema activo, y devuelve dos listas: lo que pasó el
// umbral (ordenado de mayor a menor score, recortado a MaxSignals) y lo
// que se descartó — este segundo grupo no es un error ni se pierde
// información real (sigue estando en uc si hace falta consultarlo
// puntualmente vía Timeline/Habits/etc.), es simplemente lo que
// Attention Manager decidió que no ocupa memoria de trabajo AHORA.
func (m *Manager) Attend(uc *UnifiedContext, focusTopic string) (attended []Signal, discarded []Signal) {
	normTopic := shared.Normalize(focusTopic)
	all := candidates(uc)

	for _, s := range all {
		s.Reason = fmt.Sprintf("salience base %.2f", s.Score)
		if sharesTopic(s.Content, normTopic) {
			s.Score += topicBonus
			s.Reason = fmt.Sprintf("salience base %.2f + tema activo (%.2f)", s.Score-topicBonus, topicBonus)
		}
		if s.Score >= m.Threshold {
			attended = append(attended, s)
		} else {
			discarded = append(discarded, s)
		}
	}

	sort.SliceStable(attended, func(i, j int) bool { return attended[i].Score > attended[j].Score })
	if len(attended) > m.MaxSignals {
		// Lo que no entra en el tope no se pierde — pasa a discarded,
		// para que Narrative()/tests puedan seguir explicando por qué
		// algo relevante igual no se mostró (sobrecupo, no irrelevancia).
		discarded = append(discarded, attended[m.MaxSignals:]...)
		attended = attended[:m.MaxSignals]
	}

	return attended, discarded
}

// Narrative arma el texto de "esto es lo que me importa ahora" a partir
// de señales ya atendidas (el resultado de Attend, no de candidates crudo).
func Narrative(attended []Signal) string {
	if len(attended) == 0 {
		return "Ahora mismo no tengo nada puntual que me llame la atención."
	}
	parts := make([]string, len(attended))
	for i, s := range attended {
		parts[i] = s.Label + ": " + s.Content
	}
	return strings.Join(parts, ". ")
}

// attentionTriggers detecta preguntas sobre qué le importa a Ahmelia
// AHORA — distinto de "contexto completo" (Bloque 9, pide TODO sin
// filtrar) y de "pipeline cognitivo" (Bloque 11, pregunta por el
// PROCESO). Esto pregunta por el resultado del filtro de atención.
var attentionTriggers = []string{
	"que es importante ahora",
	"que te importa ahora",
	"en que te estas enfocando",
	"en que estas concentrada",
	"que estas atendiendo",
	"que es lo mas relevante ahora",
	"a que le prestas atencion",
}

// ParseAttentionQuery detecta si la pregunta pide el resultado del
// filtro de atención (no el contexto completo sin filtrar).
func ParseAttentionQuery(normalizedText string) bool {
	for _, trigger := range attentionTriggers {
		if strings.Contains(normalizedText, trigger) {
			return true
		}
	}
	return false
}
