package interaction

import (
	"strings"
	"time"
)

// SemanticQueryType is the canonical query-domain contract. Providers consume it
// instead of inspecting raw human text. It extends SemanticMeaning; it does not
// create a second cognitive system.
type SemanticQueryType string

type TemporalReference struct {
	From  time.Time
	To    time.Time
	Label string
	Known bool
}

const (
	QueryNone                       SemanticQueryType = ""
	QueryHistoryActivity            SemanticQueryType = "historyactivity"
	QuerySolvingAdaptiveConfidence  SemanticQueryType = "solvingadaptiveconfidence"
	QuerySolvingAnalogy             SemanticQueryType = "solvinganalogy"
	QuerySolvingCausalExplain       SemanticQueryType = "solvingcausalexplain"
	QuerySolvingCausalProject       SemanticQueryType = "solvingcausalproject"
	QuerySolvingComparison          SemanticQueryType = "solvingcomparison"
	QuerySolvingConflict            SemanticQueryType = "solvingconflict"
	QuerySolvingConstraint          SemanticQueryType = "solvingconstraint"
	QuerySolvingCounterfactual      SemanticQueryType = "solvingcounterfactual"
	QuerySolvingCurriculum          SemanticQueryType = "solvingcurriculum"
	QuerySolvingDecision            SemanticQueryType = "solvingdecision"
	QuerySolvingDecompose           SemanticQueryType = "solvingdecompose"
	QuerySolvingDependency          SemanticQueryType = "solvingdependency"
	QuerySolvingDiscovery           SemanticQueryType = "solvingdiscovery"
	QuerySolvingFailure             SemanticQueryType = "solvingfailure"
	QuerySolvingGoalAnalysis        SemanticQueryType = "solvinggoalanalysis"
	QuerySolvingHypothesis          SemanticQueryType = "solvinghypothesis"
	QuerySolvingKnowledgeMap        SemanticQueryType = "solvingknowledgemap"
	QuerySolvingKnowledgeMemory     SemanticQueryType = "solvingknowledgememory"
	QuerySolvingLearningIntegration SemanticQueryType = "solvinglearningintegration"
	QuerySolvingPlanner             SemanticQueryType = "solvingplanner"
	QuerySolvingReflection          SemanticQueryType = "solvingreflection"
	QuerySolvingSelfEval            SemanticQueryType = "solvingselfeval"
	QuerySolvingSemanticSkillSearch SemanticQueryType = "solvingsemanticskillsearch"
	QuerySolvingSkillBuild          SemanticQueryType = "solvingskillbuild"
	QuerySolvingSkillBuilder        SemanticQueryType = "solvingskillbuilder"
	QuerySolvingSkillContext        SemanticQueryType = "solvingskillcontext"
	QuerySolvingSkillProcedure      SemanticQueryType = "solvingskillprocedure"
	QuerySolvingSkillUpdateCheck    SemanticQueryType = "solvingskillupdatecheck"
	QuerySolvingSkillVersionHistory SemanticQueryType = "solvingskillversionhistory"
	QuerySolvingSkillVersionUpdate  SemanticQueryType = "solvingskillversionupdate"
	QuerySolvingSourceValidation    SemanticQueryType = "solvingsourcevalidation"
	QuerySolvingStrategy            SemanticQueryType = "solvingstrategy"
	QuerySolvingValidator           SemanticQueryType = "solvingvalidator"
	QueryHistoryTimeline            SemanticQueryType = "history_timeline"
	QueryHistoryRelations           SemanticQueryType = "history_relations"
	QueryHistoryPatterns            SemanticQueryType = "history_patterns"
	QueryHistoryHabits              SemanticQueryType = "history_habits"
	QueryHistoryExperience          SemanticQueryType = "history_experience"
	QueryHistoryInsights            SemanticQueryType = "history_insights"
	QueryHistoryReflection          SemanticQueryType = "history_reflection"
	QueryHistoryLearning            SemanticQueryType = "history_learning"
	QueryMentalEvolution            SemanticQueryType = "mental_evolution"
	QueryMentalDetailed             SemanticQueryType = "mental_detailed"
	QueryMentalState                SemanticQueryType = "mental_state"
	QueryEnvironmentResources       SemanticQueryType = "environment_resources"
	QueryEnvironmentTabs            SemanticQueryType = "environment_tabs"
	QueryEnvironmentContext         SemanticQueryType = "environment_context"
	QueryEnvironmentActiveWork      SemanticQueryType = "environment_active_work"
	QueryEnvironmentOpenApps        SemanticQueryType = "environment_open_apps"
	QuerySelfKnowledge              SemanticQueryType = "self_knowledge"
)

var semanticQuerySpecs = map[SemanticQueryType][]string{
	QueryEnvironmentActiveWork:      {"en que archivo", "en que proyecto", "en que estoy trabajando", "que estas viendo", "que estoy viendo"},
	QueryEnvironmentContext:         {"que estas haciendo", "que hago", "cual es tu contexto", "en que contexto", "contexto de trabajo", "que estamos haciendo"},
	QueryEnvironmentOpenApps:        {"que aplicaciones tienes abiertas", "que aplicaciones hay abiertas", "que ventanas tienes abiertas", "que ventanas hay abiertas", "que programas tienes abiertos", "que programas hay abiertos", "que tengo abierto", "que tengo abiertos", "que tengo abierta", "que tengo abiertas"},
	QueryEnvironmentResources:       {"como esta el sistema", "como va el sistema", "cuanta cpu", "cuanta ram", "cuanta memoria", "como esta la cpu", "como esta la ram", "como esta la memoria", "cuanta bateria", "como esta la bateria", "cuanta gpu", "como esta la gpu"},
	QueryEnvironmentTabs:            {"pestana", "que tienes abierto en chrome", "que tienes abierto en el navegador"},
	QueryHistoryActivity:            {"que hicimos", "que hice", "en que estabamos"},
	QueryHistoryRelations:           {"que esta relacionado con ", "que mas hace lo mismo que ", "de que otra forma puedo decir ", "de que otra forma digo ", "con que mas se conecta "},
	QueryHistoryExperience:          {"por que falla ", "por que te falla ", "por que no funciona ", "que tan bien te va con ", "cual es tu experiencia con "},
	QueryHistoryHabits:              {"que habitos tienes", "que habitos has detectado", "que costumbres tienes"},
	QueryHistoryInsights:            {"que insights tienes", "que conclusiones sacaste", "que conclusiones tienes"},
	QueryHistoryLearning:            {"que aprendiste", "como evolucionaste", "que cambio"},
	QueryHistoryPatterns:            {"que patrones has detectado", "que patrones detectaste", "que sueles hacer", "que patron sigues"},
	QueryHistoryReflection:          {"como te va", "como funcionas", "mejorando"},
	QueryMentalDetailed:             {"reporte mental", "estado mental completo", "detalle de tu estado"},
	QueryMentalEvolution:            {"como has evolucionado", "como ha cambiado tu", "tu evolucion"},
	QueryMentalState:                {"como te sientes", "que piensas", "en que estas pensando", "en que estas enfocada", "que te preocupa", "cuales son tus dudas", "tienes dudas", "estado mental", "como vas"},
	QuerySelfKnowledge:              {"presentate", "quien eres", "quien te creo", "cual es tu proposito", "que puedes hacer"},
	QuerySolvingAdaptiveConfidence:  {"que tan seguro estas de esto", "deberia confiar en esta respuesta", "que harias con esta confianza", "conviene explorar otra opcion antes de"},
	QuerySolvingAnalogy:             {"ya resolviste algo parecido a", "hiciste algo parecido a", "tienes experiencia parecida a", "esto se parece a algo que ya hiciste en", "que tan parecido es esto a algo que ya hiciste en"},
	QuerySolvingCausalExplain:       {"por que esta bloqueada", "que causa el bloqueo", "cual es la causa de esto"},
	QuerySolvingCausalProject:       {"que se desbloquearia si resuelvo", "cual gap conviene resolver primero", "que pasaria si resuelvo"},
	QuerySolvingComparison:          {"compara las hipotesis", "que tan cerca estuvo la otra opcion", "pudiste haber elegido otra cosa", "que otras opciones considerarte"},
	QuerySolvingConflict:            {"hay conflicto entre las fuentes sobre", "las fuentes se contradicen sobre", "necesita revision manual la informacion sobre"},
	QuerySolvingConstraint:          {"que restricciones tiene", "esto cumple con", "se puede cumplir con", "hay riesgo de no cumplir", "que requisitos estan en riesgo en"},
	QuerySolvingCounterfactual:      {"que hubiera pasado si elegias otra cosa", "que pasaria si eligieras diferente", "y si hubieras elegido la otra opcion"},
	QuerySolvingCurriculum:          {"que necesito aprender para", "que deberia estudiar para", "arma un plan de aprendizaje para", "que me falta aprender para"},
	QuerySolvingDecision:            {"que deberia hacer con", "conviene avanzar con", "que decision tomarias sobre", "deberiamos seguir con", "me conviene arrancar con"},
	QuerySolvingDecompose:           {"descompone en tareas", "descompón en tareas", "dame la jerarquia de tareas", "que tareas necesito para", "desglosa esto en tareas", "divide esto en tareas", "plan de tareas para"},
	QuerySolvingDependency:          {"que orden debo seguir para", "en que orden hago", "que me esta bloqueando en", "que bloqueos hay en", "detecta bloqueos en"},
	QuerySolvingDiscovery:           {"que fuentes usarias para aprender", "de donde aprenderias sobre", "que fuentes tienes para", "donde buscarias informacion sobre"},
	QuerySolvingFailure:             {"que riesgo de fallo tiene esto", "que suele salir mal con esto", "hay antecedentes de fallo", "esto fallo antes"},
	QuerySolvingGoalAnalysis:        {"analiza este objetivo", "analiza esta peticion", "analiza este pedido", "descompone este objetivo", "descompon este objetivo", "cuales son los subobjetivos", "que necesitaria para", "como abordarias esto", "analiza esta idea"},
	QuerySolvingHypothesis:          {"que hipotesis tienes para", "como podrias resolver", "por donde empezarias con", "que probarias primero en", "que se te ocurre para"},
	QuerySolvingKnowledgeMap:        {"que sabes hacer de", "que ya sabes de", "cuanto sabes de", "que tan preparada estas para", "que conoces sobre como hacer"},
	QuerySolvingKnowledgeMemory:     {"que ya es confiable de forma permanente", "desde cuando confias en", "que quedo confirmado", "que aprendizajes ya se consolidaron"},
	QuerySolvingLearningIntegration: {"con que estrategia lo intentarias", "deberia cambiar de estrategia para esto", "que tan aprendido esta esto"},
	QuerySolvingPlanner:             {"que deberias reforzar", "cual es tu plan de aprendizaje", "en que deberias enfocarte para mejorar", "que te conviene practicar"},
	QuerySolvingReflection:          {"deberias reconsiderar algo", "hiciste bien en elegir eso", "te arrepientes de alguna eleccion", "reflexiona sobre tu decision"},
	QuerySolvingSelfEval:            {"que tan bien razonaste", "como te fue pensando esto", "evalua tu propio razonamiento", "que tan solido fue tu analisis", "como te fue con ese razonamiento"},
	QuerySolvingSemanticSkillSearch: {"que sabes sobre", "que notas mencionan", "en que apuntes aparece", "donde mencione", "sabes usar X", "sabes hacer X", "¿sabes usar Blender?", "que sabes sobre X", "sabes usar", "sabes hacer"},
	QuerySolvingSkillBuild:          {"guarda una nota sobre", "guarda lo que aprendiste sobre", "construye una nota de estudio sobre", "instala una nota sobre", "aprende sobre", "aprende ", "aprender sobre", "aprender "},
	QuerySolvingSkillBuilder:        {"que notas de estudio tienes", "tienes apuntes de", "tu biblioteca de conocimiento", "que hay guardado sobre"},
	QuerySolvingSkillContext:        {"que te falta aprender realmente para", "actualiza el curriculum para", "revisa mi progreso de aprendizaje para"},
	QuerySolvingSkillProcedure:      {"como instalo", "como hago", "como uso", "como usar", "como se instala", "como se hace", "pasos para", "cuales son los pasos de", "cuales son los pasos para", "muestrame los pasos de", "dame el procedimiento de", "el procedimiento de", "explicame como", "dame los pasos de"},
	QuerySolvingSkillUpdateCheck:    {"sigue actualizada la nota sobre", "cambio la fuente de informacion sobre", "detecta si cambio algo sobre", "la nota de estudio sigue vigente sobre"},
	QuerySolvingSkillVersionHistory: {"cuantas versiones tiene la nota sobre", "cuando aprendiste por primera vez sobre", "el historial de versiones de"},
	QuerySolvingSkillVersionUpdate:  {"actualiza la nota sobre", "renueva la nota de estudio sobre", "versiona la nota sobre"},
	QuerySolvingSourceValidation:    {"esa fuente es confiable", "puedo confiar en esa documentacion", "esa informacion de internet es confiable", "valida esas fuentes sobre", "corrobora la informacion sobre"},
	QuerySolvingStrategy:            {"que estrategia elegirias", "con cual te quedarias", "cual conviene mas de verdad"},
	QuerySolvingValidator:           {"que tan confiable es eso que aprendiste", "puedo confiar en esa confianza", "esos datos son suficientes", "estas segura de esa confianza", "esa confianza esta bien fundada"},
}

func semanticPhrasePresent(text, phrase string) bool {
	text = strings.ToLower(strings.TrimSpace(text))
	phrase = strings.TrimSpace(strings.ToLower(phrase))
	if phrase == "" {
		return false
	}
	for _, left := range []string{" ", ":", "¿", "¡"} {
		for _, right := range []string{" ", ".", ",", ":", "?", "!", "¿", "¡"} {
			if strings.Contains(left+text+right, left+phrase+right) {
				return true
			}
		}
	}
	return strings.Contains(text, phrase) && (strings.HasPrefix(text, phrase) || strings.Contains(text, " "+phrase))
}

// classifySemanticQuery is the only place where the live human wording is
// normalized into a typed query. Downstream sources never inspect Raw/Normalized.
func classifySemanticQuery(n string) SemanticQueryType {
	best := QueryNone
	bestLen := -1
	for q, triggers := range semanticQuerySpecs {
		for _, trigger := range triggers {
			if semanticPhrasePresent(n, trigger) {
				l := len(trigger)
				if l > bestLen {
					best, bestLen = q, l
				}
			}
		}
	}
	// Temporal history is structural: it requires both an activity query and
	// an explicit temporal reference.
	if best == QuerySolvingCausalExplain && strings.Contains(n, "que pasaria") {
		return QuerySolvingCausalProject
	}
	if containsHistoryActivity(n) && hasTemporalReference(n) {
		return QueryHistoryTimeline
	}
	return best
}

func containsHistoryActivity(n string) bool {
	for _, p := range semanticQuerySpecs[QueryHistoryActivity] {
		if semanticPhrasePresent(n, p) {
			return true
		}
	}
	return false
}

func hasTemporalReference(n string) bool {
	if _, _, _, ok := temporalReference(n); ok {
		return true
	}
	return false
}

func temporalReference(n string) (time.Time, time.Time, string, bool) {
	now := time.Now()
	today := startOfSemanticDay(now)
	switch {
	case phrasePresent(n, "hoy"):
		return today, now, "hoy", true
	case phrasePresent(n, "ayer"):
		y := today.AddDate(0, 0, -1)
		return y, today, "ayer", true
	case phrasePresent(n, "esta semana"):
		m := mostRecentSemanticWeekday(today, time.Monday)
		return m, now, "esta semana", true
	}
	for name, wd := range map[string]time.Weekday{"lunes": time.Monday, "martes": time.Tuesday, "miercoles": time.Wednesday, "jueves": time.Thursday, "viernes": time.Friday, "sabado": time.Saturday, "domingo": time.Sunday} {
		if semanticPhrasePresent(n, name) {
			day := mostRecentSemanticWeekday(today, wd)
			end := day.AddDate(0, 0, 1)
			if day.Equal(today) {
				end = now
			}
			return day, end, "el " + name, true
		}
	}
	return time.Time{}, time.Time{}, "", false
}

func startOfSemanticDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}
func mostRecentSemanticWeekday(today time.Time, target time.Weekday) time.Time {
	diff := int(today.Weekday() - target)
	if diff < 0 {
		diff += 7
	}
	return today.AddDate(0, 0, -diff)
}

// InferTemporalReference projects the canonical temporal evidence onto the request.
func InferTemporalReference(n string) TemporalReference {
	from, to, label, ok := temporalReference(n)
	return TemporalReference{From: from, To: to, Label: label, Known: ok}
}

// HasSemanticQuery lets sources and tests ask whether the canonical meaning
// selected this query family. It never reparses Raw/Normalized.
func HasSemanticQuery(req SemanticRequest, q SemanticQueryType) bool {
	return req.Meaning.Query == q
}
