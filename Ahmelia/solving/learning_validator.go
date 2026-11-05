package solving

import (
	"ahmelia/interaction"
	"fmt"
	"strings"

	memoryintelligence "ahmelia/memory_intelligence"
)

// ============================================================================
// LEARNING_VALIDATOR.GO — Bloque 48, Fase 4 (Knowledge Acquisition Mind)
// ============================================================================
//
// UpdateFromReflection() (Bloque 7) calcula ActionConfidence como
// successes/total*100 — pero descarta el total apenas termina de
// calcular el porcentaje. Una Action con 1 intento exitoso queda con
// 100% de confianza, exactamente igual que una con 50 intentos y 50
// éxitos. Y SuggestStrategyChange() (Bloque 8) las trata IGUAL: si el
// primer intento de una Action nueva falla, ActionConfidence cae a 0%
// y escala directo a Fallback — con una sola muestra de por medio.
//
// Learning Validator no recalcula nada de Learning (Bloque 7/8) ni
// reimplementa su criterio — vuelve a recorrer el EventLog (mismo
// espíritu que ComputeExperience, Bloque 3) para reconstruir CUÁNTOS
// intentos hay detrás de cada ActionConfidence, y clasifica cada una
// según un umbral mínimo de muestras. "Integrar, no crear": no inventa
// un concepto de confianza nuevo, valida la fiabilidad ESTADÍSTICA de
// la que ya existe. Es el primer bloque de la "Opción A.5" — Learning
// Validation antes de Knowledge Memory y Learning Planner, sin ninguna
// dependencia de Internet Gateway (4.1, todavía pendiente).

// MinSampleSize es el mínimo de intentos para considerar una
// ActionConfidence estadísticamente confiable. Por debajo de esto, el
// número existe pero puede cambiar mucho con el próximo intento —
// mismo motivo por el que una encuesta de 2 personas no se reporta
// igual que una de 200.
const MinSampleSize = 5

// ValidationStatus distingue tres niveles de fiabilidad, no un simple
// válido/inválido — "sin datos" es un caso real y distinto de "pocos
// datos": la Action nunca se intentó vs. se intentó poco.
type ValidationStatus string

const (
	StatusNoData    ValidationStatus = "sin_datos" // Action nunca ejecutada — ni un evento en el EventLog
	StatusTentative ValidationStatus = "tentativa" // menos de MinSampleSize intentos
	StatusValidated ValidationStatus = "validada"  // MinSampleSize intentos o más
)

// Validation es el resultado de validar la ActionConfidence de UNA
// Action contra el EventLog real.
type Validation struct {
	Action     string
	Label      string
	Confidence float64
	Samples    int
	Status     ValidationStatus
}

// IsReliable es true solo si hay datos suficientes como para confiar en
// Confidence tal como está — mismo espíritu que HasLead() (25) /
// HasAnalogy() (28): un booleano claro para que quien llame no tenga
// que interpretar Status a mano.
func (v *Validation) IsReliable() bool {
	return v.Status == StatusValidated
}

// Describe arma la narrativa de una validación puntual.
func (v *Validation) Describe() string {
	switch v.Status {
	case StatusNoData:
		return fmt.Sprintf("%s: nunca lo intenté — no tengo ninguna confianza calculada todavía.", v.Label)
	case StatusTentative:
		return fmt.Sprintf("%s: %.0f%% de confianza, pero con solo %d intento(s) — todavía es pronto para confiar del todo en ese número.", v.Label, v.Confidence, v.Samples)
	default:
		return fmt.Sprintf("%s: %.0f%% de confianza, respaldado por %d intentos — dato confiable.", v.Label, v.Confidence, v.Samples)
	}
}

// Validator es el Learning Validator en sí. Depende de EventStore (para
// contar muestras reales, no confiar en un número ya agregado) y de
// LearningStore (para la confianza que Bloque 7/8 ya calculó) — mismo
// criterio de dependencia real que Knowledge Mapper (23) y Learning
// Integration (45).
//
// B93.3: antes tenía *EventLog/*LearningState directo y rangeaba
// EventLog.Events sin lock; ahora toma un Snapshot() por validación —
// nunca guarda una referencia mutable compartida.
type Validator struct {
	EventStore *memoryintelligence.EventStore
	Learning   *memoryintelligence.LearningStore
}

// NewValidator arma el Learning Validator. Ambos pueden ser nil — sin
// EventStore no hay forma de contar muestras (todo sale StatusNoData);
// sin Learning, Confidence queda en 0 pero Samples/Status igual se
// calculan bien.
func NewValidator(eventStore *memoryintelligence.EventStore, learning *memoryintelligence.LearningStore) *Validator {
	return &Validator{EventStore: eventStore, Learning: learning}
}

// Validate valida la ActionConfidence de UNA Action puntual.
func (v *Validator) Validate(action, label string) *Validation {
	val := &Validation{Action: action, Label: label, Status: StatusNoData}
	if v.EventStore == nil {
		return val
	}

	samples := 0
	for _, ev := range v.EventStore.Snapshot() {
		if ev.Action == action {
			samples++
		}
	}
	val.Samples = samples
	if samples == 0 {
		return val
	}

	if v.Learning != nil {
		val.Confidence = v.Learning.Confidence(action)
	}

	if samples < MinSampleSize {
		val.Status = StatusTentative
	} else {
		val.Status = StatusValidated
	}
	return val
}

// ValidateAll valida las recomendaciones de estrategia que Learning
// Integration (Bloque 45) ya armó — mismo patrón de encadenar sobre la
// salida de otro bloque que CurriculumBuilder.Build (47).
func (v *Validator) ValidateAll(recs []*StrategyRecommendation) []*Validation {
	var out []*Validation
	for _, r := range recs {
		out = append(out, v.Validate(r.Action, r.Label))
	}
	return out
}

// DescribeValidations narra solo lo que aporta: Actions validadas se
// resumen en un conteo (no aportan nada distinto una por una), las
// tentativas y sin datos se listan explícitas — mismo criterio de "no
// llenar de líneas sin valor" que DescribeRecommendations (45) y
// DescribeRisks (44).
func DescribeValidations(vals []*Validation) string {
	if len(vals) == 0 {
		return "No hay nada para validar todavía."
	}

	var tentative, noData []string
	validatedCount := 0
	for _, v := range vals {
		switch v.Status {
		case StatusValidated:
			validatedCount++
		case StatusTentative:
			tentative = append(tentative, v.Describe())
		case StatusNoData:
			noData = append(noData, v.Label)
		}
	}

	var parts []string
	if validatedCount > 0 {
		parts = append(parts, fmt.Sprintf("%d Action(es) con confianza bien respaldada.", validatedCount))
	}
	if len(tentative) > 0 {
		parts = append(parts, strings.Join(tentative, " "))
	}
	if len(noData) > 0 {
		parts = append(parts, fmt.Sprintf("Sin ningún intento registrado todavía: %s.", strings.Join(noData, ", ")))
	}
	return strings.Join(parts, " ")
}

// ── Trigger de consulta ──────────────────────────────────────────────

var validatorTriggers = []string{
	"que tan confiable es eso que aprendiste",
	"puedo confiar en esa confianza",
	"esos datos son suficientes",
	"estas segura de esa confianza",
	"esa confianza esta bien fundada",
}

// ParseValidatorQuery detecta si el usuario pregunta específicamente
// por la FIABILIDAD de lo que Ahmelia aprendió — distinta de Learning
// Integration (Bloque 45, "con qué estrategia lo intentarías"): esta
// pregunta si ese número de confianza tiene suficiente respaldo detrás
// como para tomarlo en serio.
func ParseValidatorQuery(req interaction.SemanticRequest) bool {
	return interaction.HasSemanticQuery(req, interaction.QuerySolvingValidator)
}
