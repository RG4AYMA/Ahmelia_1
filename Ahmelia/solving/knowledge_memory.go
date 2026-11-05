package solving

import (
	"ahmelia/interaction"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strings"
	"time"
)

// ============================================================================
// KNOWLEDGE_MEMORY.GO — Bloque 49, Fase 4 (Knowledge Acquisition Mind)
// ============================================================================
//
// Learning Validator (Bloque 48) recalcula la fiabilidad de cada Action
// DESDE CERO cada vez que se consulta — recorre el EventLog completo
// otra vez, cada vez. Funciona, pero es trabajo repetido: si nada
// cambió desde la última consulta, la respuesta es exactamente la
// misma. Knowledge Memory persiste el último Validation conocido por
// Action (memory/knowledge_memory.json, mismo criterio que
// learning.json/events.json — LoadX/Save con el mismo formato) para
// poder responder sin recalcular.
//
// Además guarda algo que ni el EventLog ni LearningState conservan:
// CUÁNDO una Action cruzó el umbral de confiabilidad por primera vez
// (FirstValidatedAt). Bloque 48 puede decir "esto es confiable HOY",
// pero no puede decir desde cuándo — ese dato se pierde si nadie lo
// anota en el momento exacto en que pasa. Knowledge Memory es ese
// registro. "Integrar, no crear": no valida nada distinto de Bloque 48,
// solo le da persistencia y una dimensión temporal que antes no existía.

// MemoryEntry es el último Validation conocido para una Action, más
// cuándo se registró por primera y por última vez.
type MemoryEntry struct {
	Validation
	FirstValidatedAt string // RFC3339 — "" si nunca llegó a StatusValidated
	LastCheckedAt    string // RFC3339 — última vez que se actualizó esta entrada
}

// KnowledgeMemory persiste el estado de validación conocido, por Action.
type KnowledgeMemory struct {
	Entries map[string]*MemoryEntry `json:"entries"`
}

// KnowledgeMemoryPath — mismo criterio que LearningStatePath
// (memory_intelligence/learning.go): persiste de inmediato en cada
// actualización.
const KnowledgeMemoryPath = "memory/knowledge_memory.json"

// NewKnowledgeMemory arma una memoria vacía.
func NewKnowledgeMemory() *KnowledgeMemory {
	return &KnowledgeMemory{Entries: make(map[string]*MemoryEntry)}
}

// LoadKnowledgeMemory carga knowledge_memory.json si existe; si no,
// arranca vacía — mismo patrón que LoadLearningState.
func LoadKnowledgeMemory(path string) *KnowledgeMemory {
	if data, err := ioutil.ReadFile(path); err == nil {
		var km KnowledgeMemory
		if err := json.Unmarshal(data, &km); err == nil && km.Entries != nil {
			return &km
		}
	}
	return NewKnowledgeMemory()
}

// Save persiste la memoria a disco.
func (km *KnowledgeMemory) Save(path string) error {
	data, err := json.MarshalIndent(km, "", "  ")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(path, data, 0644)
}

// Remember actualiza (o crea) la entrada para la Action de v, a partir
// de un Validation fresco (típicamente recién salido de
// Validator.Validate, Bloque 48). FirstValidatedAt se anota SOLO la
// primera vez que Status llega a StatusValidated — nunca se pisa
// después, aunque la Action se vuelva a consultar mil veces más.
func (km *KnowledgeMemory) Remember(v *Validation) {
	if v == nil {
		return
	}
	now := time.Now().Format(time.RFC3339)

	entry, exists := km.Entries[v.Action]
	if !exists {
		entry = &MemoryEntry{}
		km.Entries[v.Action] = entry
	}
	entry.Validation = *v
	entry.LastCheckedAt = now
	if v.Status == StatusValidated && entry.FirstValidatedAt == "" {
		entry.FirstValidatedAt = now
	}
}

// RememberAll actualiza la memoria para un lote entero — mismo patrón
// de encadenar sobre la salida de otro bloque que ValidateAll (48)
// sobre RecommendAll (45).
func (km *KnowledgeMemory) RememberAll(vals []*Validation) {
	for _, v := range vals {
		km.Remember(v)
	}
}

// Recall devuelve la última entrada conocida para una Action SIN
// recalcular nada — ok=false si nunca se registró ninguna validación
// para esa Action.
func (km *KnowledgeMemory) Recall(action string) (*MemoryEntry, bool) {
	entry, ok := km.Entries[action]
	return entry, ok
}

// Describe narra qué Actions ya son confiables Y desde cuándo — el dato
// que Learning Validator (48) no puede dar por sí solo, porque no
// persiste entre consultas. No recorre EventLog ni recalcula nada: es
// pura lectura de lo que ya se registró.
func (km *KnowledgeMemory) Describe() string {
	if len(km.Entries) == 0 {
		return "Todavía no registré ninguna validación en memoria — preguntame primero cuánto sabés de algo."
	}

	var validated []string
	for _, e := range km.Entries {
		if e.Status != StatusValidated || e.FirstValidatedAt == "" {
			continue
		}
		since := e.FirstValidatedAt
		if t, err := time.Parse(time.RFC3339, e.FirstValidatedAt); err == nil {
			since = t.Format("2006-01-02")
		}
		validated = append(validated, fmt.Sprintf("%s (confiable desde %s)", e.Label, since))
	}

	if len(validated) == 0 {
		return "Tengo Actions registradas, pero ninguna cruzó todavía el umbral de confiabilidad."
	}
	return "Ya son confiables de forma consolidada: " + strings.Join(validated, "; ") + "."
}

// ── Trigger de consulta ──────────────────────────────────────────────

var knowledgeMemoryTriggers = []string{
	"que ya es confiable de forma permanente",
	"desde cuando confias en",
	"que quedo confirmado",
	"que aprendizajes ya se consolidaron",
}

// ParseKnowledgeMemoryQuery detecta si se pregunta por lo que YA quedó
// registrado como confiable — distinta de Learning Validator (48, que
// SIEMPRE recalcula sobre el plan actual): esta responde desde memoria
// persistida, sin volver a analizar nada.
func ParseKnowledgeMemoryQuery(req interaction.SemanticRequest) bool {
	return interaction.HasSemanticQuery(req, interaction.QuerySolvingKnowledgeMemory)
}
