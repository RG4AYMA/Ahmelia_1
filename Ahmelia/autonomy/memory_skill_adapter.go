package autonomy

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"ahmelia/memory"
	"ahmelia/memory_intelligence"
	"ahmelia/solving"
)

// MemorySkillContext es la fotografía cognitiva que B84 entrega al
// orquestador/planner: memoria relevante, skill conocida y aprendizaje
// disponible para la acción solicitada.
//
// B84 no crea ni duplica ninguno de esos almacenes. Solo los consulta y
// los presenta juntos en un contrato pequeño y determinista.
type MemorySkillContext struct {
	Query string

	Memories []*memory.Knowledge
	Skill    *solving.Skill

	Strategy   string
	Confidence float64

	HasMemory   bool
	HasSkill    bool
	HasLearning bool
}

// MemorySkillAdapter integra las fuentes ya existentes:
//   - memory.LongMemory          -> recuerdos/experiencias
//   - solving.SkillLibrary       -> procedimientos aprendidos
//   - memory_intelligence.LearningState -> confianza y estrategia.
//
// Es deliberadamente de solo lectura. B82 sigue siendo responsable de
// registrar resultados; B84 decide qué conocimiento previo está disponible
// antes de planificar una acción.
type MemorySkillAdapter struct {
	mu sync.RWMutex

	longMemory    *memory.LongMemory
	skillLibrary  *solving.SkillLibrary
	learningStore *memory_intelligence.LearningStore

	// Límite defensivo para que una consulta autónoma no arrastre toda la
	// memoria histórica al planner.
	maxMemories int
}

// NewMemorySkillAdapter crea B84 sobre los almacenes existentes.
func NewMemorySkillAdapter(
	longMemory *memory.LongMemory,
	skillLibrary *solving.SkillLibrary,
	learningStore *memory_intelligence.LearningStore,
) *MemorySkillAdapter {
	return &MemorySkillAdapter{
		longMemory:    longMemory,
		skillLibrary:  skillLibrary,
		learningStore: learningStore,
		maxMemories:   5,
	}
}

// Resolve consulta las tres fuentes sin escribir en ninguna de ellas.
//
// La consulta se intenta como:
//  1. acción + target en LongMemory;
//  2. target/acción en SkillLibrary;
//  3. confianza/estrategia de LearningState.
//
// La ausencia de datos es válida: B84 devuelve un contexto parcial, nunca
// inventa una memoria, skill o confianza.
func (a *MemorySkillAdapter) Resolve(
	ctx context.Context,
	req ActionRequest,
) (*MemorySkillContext, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("B84: contexto cancelado: %w", err)
	}

	a.mu.RLock()
	defer a.mu.RUnlock()

	result := &MemorySkillContext{
		Query: req.Action + "|" + req.Target,
	}

	if a.longMemory != nil {
		result.Memories = recallRelevant(a.longMemory, req.Action, req.Target, a.maxMemories)
		result.HasMemory = len(result.Memories) > 0
	}

	if a.skillLibrary != nil {
		result.Skill = resolveSkill(a.skillLibrary, req.Action, req.Target)
		result.HasSkill = result.Skill != nil
	}

	if a.learningStore != nil && req.Action != "" {
		result.Strategy, result.Confidence = a.learningStore.SuggestStrategyChange(req.Action)
		result.HasLearning = a.learningStore.HasConfidence(req.Action)
	}

	return result, nil
}

// SetMaxMemories limita cuántos recuerdos devuelve Resolve. Un valor <= 0
// restaura el límite seguro por defecto.
func (a *MemorySkillAdapter) SetMaxMemories(n int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if n <= 0 {
		a.maxMemories = 5
		return
	}
	a.maxMemories = n
}

// Snapshot devuelve una consulta genérica útil para componentes que aún no
// tienen un ActionRequest completo. No incrementa Uses ni modifica memoria.
func (a *MemorySkillAdapter) Snapshot(
	ctx context.Context,
	query string,
) ([]*memory.Knowledge, *solving.Skill, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, fmt.Errorf("B84: contexto cancelado: %w", err)
	}

	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil, nil
	}

	a.mu.RLock()
	defer a.mu.RUnlock()

	var memories []*memory.Knowledge
	if a.longMemory != nil {
		memories = recallByQuery(a.longMemory, query, a.maxMemories)
	}

	var skill *solving.Skill
	if a.skillLibrary != nil {
		skill = resolveSkill(a.skillLibrary, query, query)
	}

	return memories, skill, nil
}

func recallRelevant(
	long *memory.LongMemory,
	action string,
	target string,
	max int,
) []*memory.Knowledge {
	if long == nil || action == "" {
		return nil
	}

	results := long.RecallBySameCommand(action, target)

	// Si no hay coincidencia exacta, una coincidencia por target todavía
	// puede aportar experiencia útil al planner.
	if len(results) == 0 && target != "" {
		results = long.RecallByTarget(target)
	}

	return sortAndLimitMemories(results, max)
}

func recallByQuery(
	long *memory.LongMemory,
	query string,
	max int,
) []*memory.Knowledge {
	if long == nil {
		return nil
	}

	normalized := strings.ToLower(strings.TrimSpace(query))
	var results []*memory.Knowledge
	for _, k := range long.Data {
		if k == nil {
			continue
		}
		if strings.Contains(strings.ToLower(k.Input), normalized) ||
			strings.Contains(strings.ToLower(k.Target), normalized) ||
			strings.Contains(strings.ToLower(k.Action), normalized) {
			results = append(results, k)
		}
	}
	return sortAndLimitMemories(results, max)
}

func sortAndLimitMemories(
	results []*memory.Knowledge,
	max int,
) []*memory.Knowledge {
	sort.SliceStable(results, func(i, j int) bool {
		if results[i] == nil {
			return false
		}
		if results[j] == nil {
			return true
		}
		return results[i].LastUsed.After(results[j].LastUsed)
	})

	if max > 0 && len(results) > max {
		results = results[:max]
	}
	return results
}

func resolveSkill(
	library *solving.SkillLibrary,
	action string,
	target string,
) *solving.Skill {
	if library == nil || len(library.Skills) == 0 {
		return nil
	}

	// Primero intentamos coincidencias exactas por ID/dominio.
	for key, skill := range library.Skills {
		if skill == nil {
			continue
		}
		if strings.EqualFold(key, target) ||
			strings.EqualFold(skill.SkillID, target) ||
			strings.EqualFold(skill.Label, target) ||
			strings.EqualFold(skill.EffectiveLabel(), target) {
			return skill
		}
	}

	// Después reutilizamos el buscador semántico existente, sin crear otro
	// índice ni otra representación de Skills.
	query := strings.TrimSpace(target)
	if query == "" {
		query = strings.TrimSpace(action)
	}
	if query == "" {
		return nil
	}

	search := solving.NewSemanticSkillSearcher().Search(query, library)
	if search == nil || len(search.Matches) == 0 {
		return nil
	}

	// Solo aceptamos una coincidencia. La ambigüedad no debe convertirse
	// silenciosamente en una Skill elegida al azar.
	if len(search.Matches) != 1 {
		return nil
	}
	return search.Matches[0].Note
}
