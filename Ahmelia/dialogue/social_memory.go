package dialogue

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strings"
	"time"
)

// ============================================================================
// SOCIAL_MEMORY.GO — Bloque 63, Fase 5 (Conversational Mind), Capa 5.3
// ============================================================================
//
// Conversational Memory (62) recuerda TEMAS. Social Memory recuerda
// PERSONAS — no solo un nombre suelto, sino hechos concretos sobre esa
// persona (dónde trabaja, qué le gusta, en qué participó) y cuántas
// veces se la mencionó. Mismo criterio de persistencia que 62: un
// archivo JSON aparte (social_memory.json), Load/Save, sobrevive al
// cierre del proceso.
//
// Alcance honesto de este bloque: la extracción de hechos NO usa NLP
// libre ("Sebas trabaja en AK Game Studios" parseado automáticamente).
// Ahmelia no tiene un reconocedor de entidades — inventar un parser que
// adivine dónde termina el nombre y empieza el hecho sería el mismo
// tipo de fabricación sin fundamento que el proyecto viene evitando
// desde Curriculum Builder (47). En su lugar, se pide un comando
// explícito con separador claro: "recordá que Sebas: trabaja en AK
// Game Studios" — sin ambigüedad sobre qué es el nombre y qué es el
// hecho. La categoría del hecho (trabajo/gusto/proyecto/relación) SÍ se
// infiere por keywords, mismo criterio que domainTemplates
// (goal_analyzer.go, 21) — heurística declarada, no oculta.

// Fact es un dato puntual sobre una persona.
type Fact struct {
	Category string // "trabajo", "gusto", "proyecto", "relacion", "general" — abierto, no un enum estricto
	Value    string
	NotedAt  string // RFC3339
}

// Person es lo que Ahmelia sabe de alguien, acumulado a través de
// múltiples conversaciones.
type Person struct {
	Name           string
	Facts          []Fact
	FirstMentioned string // RFC3339
	LastMentioned  string // RFC3339
	MentionCount   int
}

// SocialMemory persiste personas por Name.
type SocialMemory struct {
	People map[string]*Person `json:"people"`
}

// SocialMemoryPath — mismo criterio que ConversationalMemoryPath (62):
// persiste de inmediato en cada actualización.
const SocialMemoryPath = "memory/social_memory.json"

// NewSocialMemory arma una memoria vacía.
func NewSocialMemory() *SocialMemory {
	return &SocialMemory{People: make(map[string]*Person)}
}

// LoadSocialMemory carga el archivo si existe; si no, arranca vacía —
// mismo patrón que LoadConversationalMemory/LoadKnowledgeMemory.
func LoadSocialMemory(path string) *SocialMemory {
	if data, err := ioutil.ReadFile(path); err == nil {
		var sm SocialMemory
		if err := json.Unmarshal(data, &sm); err == nil && sm.People != nil {
			return &sm
		}
	}
	return NewSocialMemory()
}

// Save persiste a disco.
func (sm *SocialMemory) Save(path string) error {
	data, err := json.MarshalIndent(sm, "", "  ")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(path, data, 0644)
}

// Remember hace get-or-create de una persona y actualiza sus
// estadísticas de mención — mismo espíritu que Touch() en
// ConversationalMemory (62), pero para personas en vez de temas.
func (sm *SocialMemory) Remember(name string) *Person {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	now := time.Now().Format(time.RFC3339)
	p, ok := sm.People[name]
	if !ok {
		p = &Person{Name: name, FirstMentioned: now}
		sm.People[name] = p
	}
	p.LastMentioned = now
	p.MentionCount++
	return p
}

// AddFact agrega un hecho a una persona (get-or-create vía Remember).
// No duplica si ya existe exactamente el mismo Category+Value — evita
// que "recordá que Sebas: le gusta Unreal" repetido cinco veces deje
// cinco copias del mismo dato.
func (sm *SocialMemory) AddFact(name, category, value string) *Person {
	p := sm.Remember(name)
	if p == nil {
		return nil
	}
	for _, f := range p.Facts {
		if f.Category == category && strings.EqualFold(f.Value, value) {
			return p
		}
	}
	p.Facts = append(p.Facts, Fact{Category: category, Value: value, NotedAt: time.Now().Format(time.RFC3339)})
	return p
}

// Recall busca una persona por nombre — exacto primero, laxo después
// (substring en cualquier dirección), mismo criterio que Recall() en
// ConversationalMemory (62).
func (sm *SocialMemory) Recall(name string) (*Person, bool) {
	target := strings.ToLower(strings.TrimSpace(name))
	if target == "" {
		return nil, false
	}
	if p, ok := sm.People[name]; ok {
		return p, true
	}
	for pname, p := range sm.People {
		n := strings.ToLower(pname)
		if strings.Contains(n, target) || strings.Contains(target, n) {
			return p, true
		}
	}
	return nil, false
}

// FactsByCategory filtra los Facts de una persona por categoría — ""
// devuelve todos.
func (p *Person) FactsByCategory(category string) []Fact {
	if p == nil {
		return nil
	}
	if category == "" {
		return p.Facts
	}
	var out []Fact
	for _, f := range p.Facts {
		if f.Category == category {
			out = append(out, f)
		}
	}
	return out
}

// Describe narra lo que se sabe de una persona — el ejemplo del
// roadmap ("Sebas → trabaja en AK Game Studios → le gusta Unreal →
// participó en el proyecto X"), como una frase encadenada.
func (p *Person) Describe() string {
	if p == nil {
		return "No tengo ningún registro de esa persona."
	}
	if len(p.Facts) == 0 {
		return fmt.Sprintf("Solo sé que existe %s — todavía no tengo ningún dato más sobre ella/él.", p.Name)
	}
	var values []string
	for _, f := range p.Facts {
		values = append(values, f.Value)
	}
	return fmt.Sprintf("%s: %s.", p.Name, strings.Join(values, ". "))
}

// factCategoryHints infiere la categoría de un hecho por keywords —
// mismo criterio declarado que domainTemplates (solving/goal_analyzer.go,
// Bloque 21): heurística de punto de partida, no exhaustiva, ampliable
// sin tocar el resto del archivo.
var factCategoryHints = []struct {
	Keywords []string
	Category string
}{
	{[]string{"trabaja en", "trabaja para", "empresa", "estudia en"}, "trabajo"},
	{[]string{"le gusta", "prefiere", "fan de", "le encanta"}, "gusto"},
	{[]string{"participo en", "participó en", "proyecto"}, "proyecto"},
	{[]string{"es mi", "es amigo", "es colega", "es hermano", "es socio", "es su"}, "relacion"},
}

// InferFactCategory detecta la categoría de un hecho a partir de sus
// palabras — "general" si ninguna plantilla matchea (no es un error,
// es un hecho sin categoría reconocida todavía).
func InferFactCategory(fact string) string {
	norm := strings.ToLower(fact)
	for _, h := range factCategoryHints {
		for _, kw := range h.Keywords {
			if strings.Contains(norm, kw) {
				return h.Category
			}
		}
	}
	return "general"
}

// ── Comando para agregar un hecho ────────────────────────────────────

var addFactTriggers = []string{"recorda que ", "recuerda que ", "acordate que ", "anota que "}

// ParseAddFactCommand detecta "recordá que Nombre: hecho" y separa
// nombre de hecho por el separador ":" EXPLÍCITO — sin eso, no hay
// forma confiable de saber dónde termina el nombre y empieza el hecho
// sin un reconocedor de entidades que Ahmelia no tiene (ver nota de
// alcance arriba del archivo).
func ParseAddFactCommand(rawInput string) (name, fact string, ok bool) {
	lower := strings.ToLower(rawInput)
	for _, trigger := range addFactTriggers {
		idx := strings.Index(lower, trigger)
		if idx == -1 {
			continue
		}
		rest := rawInput[idx+len(trigger):]
		parts := strings.SplitN(rest, ":", 2)
		if len(parts) != 2 {
			continue
		}
		n := strings.TrimSpace(parts[0])
		f := strings.TrimSpace(parts[1])
		if n == "" || f == "" {
			continue
		}
		return n, f, true
	}
	return "", "", false
}

// ── Trigger de consulta ──────────────────────────────────────────────

var socialMemoryQueryTriggers = []string{"que sabes de", "quien es", "que recordas de", "que sabes sobre"}

// ParseSocialMemoryQuery detecta si se pregunta por una persona —
// distinto de ParseConversationalMemoryQuery (62, temas) y de
// ParseAddFactCommand (arriba, agrega en vez de consultar).
func ParseSocialMemoryQuery(normalizedText string) bool {
	for _, trigger := range socialMemoryQueryTriggers {
		if strings.Contains(normalizedText, trigger) {
			return true
		}
	}
	return false
}

// ExtractPersonName saca el nombre después del trigger de consulta.
func ExtractPersonName(normalizedText string) string {
	for _, trigger := range socialMemoryQueryTriggers {
		if idx := strings.Index(normalizedText, trigger); idx != -1 {
			return strings.TrimSpace(normalizedText[idx+len(trigger):])
		}
	}
	return ""
}
