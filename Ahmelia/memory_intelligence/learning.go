package memory_intelligence

import (
	"ahmelia/interaction"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"time"
)

// ============================================================================
// LEARNING.GO — Bloque 7 (Learning Adaptation)
// ============================================================================
//
// Usa Reflection (Bloque 6) para adaptar ESTRATEGIAS en tiempo real:
// si Reflection detecta que Ahmelia falla mucho en crear_carpeta, la
// próxima vez intenta una estrategia alternativa en vez de la que viene
// fallando.
//
// Dos capas:
// 1. LearningState: persiste qué se aprendió entre sesiones (learning.json)
// 2. AdaptStrategy: cambia la estrategia ANTES de intentar ejecutar la acción

// StrategySet agrupa estrategias por prioridad (primaria, secundaria, fallback)
type StrategySet struct {
	Primary   string // "directo" — intent normal
	Secondary string // "con reintentos" — si falló, reintentar
	Fallback  string // "alternativa" — si falló mucho, cambiar enfoque
	LastUsed  string // cuál fue la última usada
	SuccessBy string // cuál fue la más exitosa (del registro del EventLog)
}

// LearningState persiste qué aprendió Ahmelia entre sesiones.
// Se guarda en learning.json (paralelo a events.json/habits.json)
type LearningState struct {
	// Por cada Action, sus estrategias disponibles
	Strategies map[string]*StrategySet `json:"strategies"`

	// Confianza en cada Action (0-100, derivada de Reflection)
	ActionConfidence map[string]float64 `json:"action_confidence"`

	// Patrones de fallo: qué category de error es más común por Action
	FailurePatternsByAction map[string]map[string]int `json:"failure_patterns_by_action"`

	// Cuántas veces se cambió de estrategia exitosamente
	SuccessfulAdaptations int `json:"successful_adaptations"`

	// Timestamp de última actualización
	LastUpdate string `json:"last_update"`
}

// LearningStatePath — mismo criterio que EventLogPath (eventlog.go):
// persiste de inmediato en cada actualización, para no perder adaptaciones
// si el proceso se cierra abrupto.
const LearningStatePath = "memory/learning.json"

// DefaultStrategies retorna el mapa de estrategias estándar por Action.
func DefaultStrategies() map[string]*StrategySet {
	return map[string]*StrategySet{
		"abrir_programa": {
			Primary:   "directo",
			Secondary: "con cmd",
			Fallback:  "buscar en registro",
		},
		"buscar_archivo": {
			Primary:   "busqueda_rapida",
			Secondary: "busqueda_profunda",
			Fallback:  "manual_browse",
		},
		"crear_carpeta": {
			Primary:   "directo",
			Secondary: "con powershell",
			Fallback:  "validar permisos",
		},
		"abrir_archivo": {
			Primary:   "default_app",
			Secondary: "app_alternativa",
			Fallback:  "cmd_directo",
		},
		"reproducir_musica": {
			Primary:   "player_default",
			Secondary: "player_alternativo",
			Fallback:  "browser",
		},
		"cambiar_volumen": {
			Primary:   "sistem_settings",
			Secondary: "via_programas",
			Fallback:  "via_keyboard",
		},
	}
}

// NewLearningState crea un LearningState vacío con estrategias por defecto.
func NewLearningState() *LearningState {
	return &LearningState{
		Strategies:              DefaultStrategies(),
		ActionConfidence:        make(map[string]float64),
		FailurePatternsByAction: make(map[string]map[string]int),
		SuccessfulAdaptations:   0,
	}
}

// LoadLearningState carga learning.json si existe; sino retorna nuevo.
func LoadLearningState(filePath string) *LearningState {
	if data, err := ioutil.ReadFile(filePath); err == nil {
		var ls LearningState
		if err := json.Unmarshal(data, &ls); err == nil {
			return &ls
		}
	}
	return NewLearningState()
}

// Save persiste el LearningState a learning.json.
func (ls *LearningState) Save(filePath string) error {
	ls.LastUpdate = time.Now().Format(time.RFC3339)
	data, err := json.MarshalIndent(ls, "", "  ")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(filePath, data, 0644)
}

// UpdateFromReflection toma los resultados de Reflection y actualiza:
// - ActionConfidence (tasa de éxito)
// - FailurePatterns (qué falla en cada Action)
func (ls *LearningState) UpdateFromReflection(log *EventLog) {
	// La confianza se reconstruye solo desde evidencia interpretable. Los
	// eventos históricos sin estado epistémico no demuestran el efecto.
	ls.ActionConfidence = make(map[string]float64)
	ls.FailurePatternsByAction = make(map[string]map[string]int)
	// Recalcular confianza por Action basado en EventLog actual
	actionStats := make(map[string]struct {
		successes int
		total     int
	})

	for _, ev := range log.Events {
		if ev.Action == "" || ev.Action == "desconocido" {
			continue
		}
		if !ev.Outcome.EvidenceStatus.SupportsLearning() {
			continue
		}

		stat := actionStats[ev.Action]
		if ev.Outcome.Success {
			stat.successes++
		}
		stat.total++
		actionStats[ev.Action] = stat

		// Registrar categoría de fallo por Action
		if ev.Outcome.EvidenceStatus == EvidenceContradicted && ev.Outcome.Category != "" {
			if ls.FailurePatternsByAction[ev.Action] == nil {
				ls.FailurePatternsByAction[ev.Action] = make(map[string]int)
			}
			ls.FailurePatternsByAction[ev.Action][ev.Outcome.Category]++
		}
	}

	// Actualizar confianzas
	for action, stat := range actionStats {
		if stat.total > 0 {
			ls.ActionConfidence[action] = float64(stat.successes) / float64(stat.total) * 100
		}
	}
}

// SuggestStrategyChange examina ActionConfidence y recomienda si cambiar
// de estrategia. Retorna (recomendación, confianza_actual)
func (ls *LearningState) SuggestStrategyChange(action string) (string, float64) {
	conf := ls.ActionConfidence[action]

	// Si confianza es baja (<40%), sugerir cambio a secundaria
	// Si es muy baja (<20%), sugerir fallback
	if conf < 20 && ls.Strategies[action] != nil {
		return ls.Strategies[action].Fallback, conf
	}
	if conf < 40 && ls.Strategies[action] != nil {
		return ls.Strategies[action].Secondary, conf
	}

	// Si está bien, mantener primaria
	if ls.Strategies[action] != nil {
		return ls.Strategies[action].Primary, conf
	}
	return "desconocida", conf
}

// AdaptStrategy retorna cuál estrategia usar para una Action dado el
// estado actual de Learning. Se llama ANTES de intentar ejecutar la acción.
func (ls *LearningState) AdaptStrategy(action string) string {
	if strategy, ok := ls.Strategies[action]; ok {
		recommended, _ := ls.SuggestStrategyChange(action)
		strategy.LastUsed = recommended
		return recommended
	}
	return "desconocida"
}

// NoteOutcome cierra el ciclo de adaptación: se llama DESPUÉS de intentar
// ejecutar la Action con la estrategia que AdaptStrategy recomendó.
//   - Si la estrategia usada no era la Primary y el intento salió bien,
//     cuenta como una adaptación exitosa (Ahmelia cambió de enfoque y le
//     funcionó, no fue casualidad).
//   - Siempre deja registro de cuál fue la última estrategia usada y, si
//     funcionó, cuál es la más exitosa hasta ahora — para que
//     DescribeLearning() pueda contar algo más que porcentajes sueltos.
func (ls *LearningState) NoteOutcome(action, strategyUsed string, success bool) {
	strat, ok := ls.Strategies[action]
	if !ok {
		return
	}
	strat.LastUsed = strategyUsed
	if success {
		if strategyUsed != strat.Primary {
			ls.SuccessfulAdaptations++
		}
		strat.SuccessBy = strategyUsed
	}
}

// DescribeLearning arma una narrativa sobre lo que Ahmelia aprendió.
func (ls *LearningState) DescribeLearning() string {
	if len(ls.ActionConfidence) == 0 {
		return "Todavía no tengo datos suficientes para describir mis aprendizajes."
	}

	// Encontrar la Action con mayor confianza vs. menor
	var bestAction string
	var bestConf float64
	var worstAction string
	var worstConf float64 = 101

	for action, conf := range ls.ActionConfidence {
		if conf > bestConf {
			bestConf = conf
			bestAction = action
		}
		if conf < worstConf && conf > 0 {
			worstConf = conf
			worstAction = action
		}
	}

	if bestAction == "" {
		return "Aún sin datos de aprendizaje."
	}

	msg := fmt.Sprintf(
		"He acumulado experiencia. Soy más confiable en %s (%.0f%%), ",
		bestAction, bestConf,
	)
	if worstAction != "" {
		msg += fmt.Sprintf("pero sigo teniendo dificultad con %s (%.0f%%).", worstAction, worstConf)
	} else {
		msg += "y ahora soy más consistente en general."
	}

	if ls.SuccessfulAdaptations > 0 {
		msg += fmt.Sprintf(" Cambié de estrategia %d vez(es) y me funcionó.", ls.SuccessfulAdaptations)
	}

	return msg
}

// learningTriggers: patrones para detectar si el usuario pregunta
// sobre lo que aprendió Ahmelia.
var learningTriggers = []string{
	"que aprendiste",
	"como evolucionaste",
	"que cambio",
	"que adaptaste",
	"cambio de estrategia",
	"adaptacion",
	"adaptación",
	"estrategia nueva",
	"como mejoaste",
	"que mejoro",
	"experiencia",
}

// ParseLearningQuery detecta si la pregunta pide reflexión sobre LO QUE
// APRENDIÓ (Learning), no sobre CÓMO ESTÁ FUNCIONANDO (Reflection).
func ParseLearningQuery(req interaction.SemanticRequest) bool {
	return interaction.HasSemanticQuery(req, interaction.QueryHistoryLearning)
}
