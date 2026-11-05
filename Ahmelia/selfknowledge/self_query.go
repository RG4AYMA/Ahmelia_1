package selfknowledge

import (
	"fmt"
	"strings"

	"ahmelia/competence"
	"ahmelia/interaction"
	"ahmelia/personality"
	"ahmelia/shared"
)

// SelfQuery maneja preguntas sobre Ahmelia a sí misma
type SelfQuery struct {
	history         *ActionHistory
	registry        *ModuleRegistry
	version         string
	identity        *personality.Identity
	attributes      *SelfAttributeStore
	startTime       int64               // Unix timestamp
	creatorAskCount int                 // alterna la respuesta de "quién te creó" entre nombres
	competence      *competence.Overlay // T4/M5: si está adjunto, "qué sabes hacer" se responde con Standing derivado
}

// NewSelfQuery crea un sistema de auto-consultas
func NewSelfQuery(history *ActionHistory, registry *ModuleRegistry, version string) *SelfQuery {
	identity := personality.NewIdentity()
	return &SelfQuery{
		history:    history,
		registry:   registry,
		version:    version,
		identity:   identity,
		attributes: NewSelfAttributeStore(identity),
		startTime:  0, // Se establece en engine
	}
}

// AttachCompetence adjunta el overlay de competencia (T4/M5). Mientras no se
// llame, el comportamiento es exactamente el anterior (lista estática), por
// lo que este cambio es INERTE hasta la integración en el engine.
func (sq *SelfQuery) AttachCompetence(o *competence.Overlay) {
	if sq == nil || o == nil {
		return
	}
	sq.competence = o
}

// QuerySemantic consumes the semantic request produced at the interaction
// boundary. Stable self attributes are resolved from the self-owned store first;
// operational self-knowledge remains below that generic layer.
func (sq *SelfQuery) QuerySemantic(req interaction.SemanticRequest) (string, bool) {
	if sq == nil {
		return "", false
	}
	if req.Subject == interaction.UserObservationSubjectSelf {
		if sq.attributes != nil {
			if attribute, ok := sq.attributes.QuerySemantic(req); ok {
				return attribute.Value, true
			}
		}
		if ref, ok := interaction.ResolveSemanticAttribute(req); ok && !ref.HasValue && ref.Subject == interaction.UserObservationSubjectSelf && strings.TrimSpace(ref.Key) != "" {
			// Concordancia por la última palabra de la clave ("color favorito"
			// / "bebida favorita"; las claves propias de atributo terminan en
			// -o/-a). Antes toda clave producía "un ... propio": la corrida
			// del 18/10 mostraba "un bebida favorita propio".
			key := strings.TrimSpace(strings.TrimPrefix(ref.Key, "tu "))
			if strings.HasSuffix(key, "a") {
				return "No tengo una " + key + " propia registrada todavía.", true
			}
			return "No tengo un " + key + " propio registrado todavía.", true
		}
	}
	return sq.queryOperational(req.Raw)
}

// queryOperational contains self-state/operational questions that are not
// arbitrary durable attributes. It is kept behind QuerySemantic so Engine does
// not re-interpret the same person-attribute question in this layer.
func (sq *SelfQuery) queryOperational(question string) (string, bool) {
	return sq.queryLegacy(question)
}

// Query responde preguntas sobre Ahmelia
// Retorna respuesta y si fue entendida
func (sq *SelfQuery) Query(question string) (string, bool) {
	// Compatibility/test adapter only. The Engine uses QuerySemantic so the
	// semantic interpretation is produced exactly once upstream.
	req := interaction.Analyze(question, interaction.Context{})
	req = interaction.InferSemanticRelations(req)
	return sq.QuerySemantic(req)
}

func (sq *SelfQuery) queryLegacy(question string) (string, bool) {
	// shared.Normalize quita acentos además de pasar a minúsculas — es
	// la MISMA normalización que ya usan classifier.go y el resto del
	// sistema. Antes esto solo hacía ToLower, así que una lista de
	// patrones tenía que cubrir cada combinación de acentos a mano
	// ("quién te creó" Y "quien te creo" por separado). Eso es fragil:
	// un STT que transcribe con acentos MIXTOS ("quién te creo", con
	// acento en una palabra y no en la otra — le pasó a Whisper con
	// "creo" vs "creó") no calzaba con NINGUNA variante y caía a
	// incertidumbre genérica aunque la pregunta se entendiera bien.
	// Con Normalize, una sola forma sin acentos cubre todas las
	// combinaciones — ya no hace falta duplicar cada patrón.
	q := shared.Normalize(question)

	// Identidad básica. sq.identity ya existía con todo este contenido
	// (GetIntroduction, CreatedBy) desde antes — nunca tenía ningún
	// trigger que lo llamara. "Preséntate" y "quién te creó" no eran un
	// bug, eran simplemente preguntas que nadie había conectado todavía.
	if contains(q, []string{"presentate", "quien eres", "quien sos"}) {
		return sq.identity.GetIntroduction(), true
	}

	// Identity.Purpose ya existía con contenido real ("Aprender de ti y
	// automatizar lo que pueda de tu día a día") desde antes, igual que
	// GetIntroduction/CreatedBy en su momento — pero nunca tenía ningún
	// trigger que lo conectara. "Cuál es tu propósito" no era un bug de
	// lógica, era una pregunta que nadie había cableado todavía.
	if contains(q, []string{"cual es tu proposito", "para que sirves", "cual es tu mision", "que proposito tienes"}) {
		return sq.identity.Purpose, true
	}

	if contains(q, []string{"quien te creo", "quien te hizo", "tu creador", "quien es tu creador"}) {
		// Alterna entre dos formas de nombrar al mismo creador (nombre
		// completo / usuario) en vez de repetir siempre la misma —
		// pedido explícito, no una decisión de diseño mía.
		names := []string{"Adrián Marín", "Ayma240"}
		name := names[sq.creatorAskCount%len(names)]
		sq.creatorAskCount++
		return fmt.Sprintf("Me creó %s.", name), true
	}

	// Preguntas sobre acciones recientes / estado conversacional.
	// "¿Qué haces?" es una pregunta sobre la presencia/actividad actual,
	// no necesariamente sobre una acción ya ejecutada. Debe recibir una
	// respuesta directa en vez de caer al razonador genérico con confianza
	// artificialmente baja.

	if contains(q, []string{"que haces ahora", "que haces", "que estas haciendo", "que estás haciendo"}) {
		if last := sq.history.GetLastAction(); last != nil {
			return fmt.Sprintf("Ahora mismo estoy aquí contigo, siguiendo nuestra conversación. Lo último que hice fue %s.", last.Action), true
		}
		return "Ahora mismo estoy aquí contigo, escuchándote y siguiendo lo que quieras hacer o conversar.", true
	}
	// Presencia/saludo: "¿cómo estás?" no pregunta por una capacidad ni por una
	// acción — es el estado de Ahmelia en primera persona. Sin este patrón caía
	// al especialista vacío y la superficie era el hedge genérico de
	// conocimiento (defecto vivo de la corrida 2026-10-18).
	if contains(q, []string{"como estas", "como andas", "como te sentis", "como te encontras"}) {
		if last := sq.history.GetLastAction(); last != nil {
			return fmt.Sprintf("Estoy bien y sigo aquí contigo. Lo último que hice fue %s.", last.Action), true
		}
		return "Estoy bien y sigo aquí contigo, escuchándote.", true
	}
	if isGreetingOnly(q) {
		return "Hola, estoy aquí contigo para lo que quieras hacer o conversar.", true
	}
	if contains(q, []string{"que hiciste", "que acabas", "ultima accion"}) {
		return sq.answerWhatYouDid(), true
	}

	if contains(q, []string{"como funciona", "explica"}) {
		return sq.answerHowItWorks(q), true
	}

	if contains(q, []string{"que sabes", "que puedes"}) {
		return sq.answerCapabilities(), true
	}

	if contains(q, []string{"version", "que version"}) {
		return fmt.Sprintf("Soy Ahmelia v3.1. Última actualización: Capa 3 SelfKnowledge"), true
	}

	if contains(q, []string{"que paso", "que fallo", "ultimo error", "que error", "cual fue el error", "hubo error", "algo fallo"}) {
		return sq.answerWhatFailed(), true
	}

	if contains(q, []string{"como te va", "estadisticas"}) {
		return sq.answerStats(), true
	}

	if contains(q, []string{"que modulos", "de que estas hecho"}) {
		return sq.answerModules(), true
	}

	if contains(q, []string{"que estabamos", "antes", "sesion anterior"}) {
		return sq.answerPreviousContext(), true
	}

	if contains(q, []string{"ultimo archivo", "archivo anterior"}) {
		return sq.answerLastFile(), true
	}

	if contains(q, []string{"por que", "causa", "motivo"}) {
		return sq.answerWhy(q), true
	}

	return "", false
}

// answerWhatYouDid — "¿Qué hiciste?"
func (sq *SelfQuery) answerWhatYouDid() string {
	last := sq.history.GetLastAction()
	if last == nil {
		return "Aún no he hecho nada en esta sesión"
	}

	return fmt.Sprintf(
		"Hace poco: %s. Me pediste que haga esto porque dijiste '%s'. El resultado fue: %s",
		last.Action,
		last.Input,
		last.Result,
	)
}

// answerHowItWorks — "¿Cómo funciona X?"
func (sq *SelfQuery) answerHowItWorks(question string) string {
	// Buscar qué módulo pregunta
	for _, module := range sq.registry.ListModules() {
		if strings.Contains(strings.ToLower(question), strings.ToLower(module.Name)) {
			return fmt.Sprintf(
				"El módulo %s: %s. Mis capacidades son: %s",
				module.Name,
				module.Description,
				strings.Join(module.Capabilities, ", "),
			)
		}
	}

	return "Explica conceptos como: cómo busco archivos, cómo aprendo, cómo clasifico comandos"
}

// answerCapabilities — "¿Qué sabes hacer?"
// T4 (plan unificado, M5/E2): con overlay adjunto, lo VERIFICADO se deriva de
// evidencia (Ledger + procedimientos + oráculo) vía Standing. Piso híbrido
// (directiva DeepSeek Paso 1.1, 19/10): la lista estática de
// personality.Identity NO desaparece en silencio — se conserva como piso
// declarado, distinguida explícitamente de la evidencia verificada. Sin
// evidencia, la respuesta sigue honesta — nunca inventa capacidades. Sin
// overlay adjunto, la lista estática continúa sola (comportamiento previo).
func (sq *SelfQuery) answerCapabilities() string {
	declared := sq.identity.GetCapabilities()
	floor := ""
	if len(declared) > 0 {
		floor = "Puedo (declarado en mi identidad): " + strings.Join(declared, ", ")
	}
	if sq.competence != nil {
		assessments := sq.competence.Capabilities()
		if len(assessments) == 0 {
			if floor == "" {
				return "Aún no tengo procedimientos con evidencia verificable. De lo que no puedo afirmar, no lo afirmo."
			}
			return floor + "\nNota honesta: eso es lo DECLARADO en mi identidad; todavía no tengo práctica verificada que lo respalde."
		}
		parts := make([]string, 0, len(assessments))
		for _, a := range assessments {
			parts = append(parts, fmt.Sprintf("%s → %s (%s)", a.Capability, a.Standing, a.Reason))
		}
		// "Registrada" es neutral: un estado `unverifiable` no es una
		// afirmación de capacidad. Decir "puedo afirmar" con unverifiable
		// sería precisamente la inflación que T4 prohíbe.
		answer := "Evidencia de competencia registrada: " + strings.Join(parts, "; ")
		if floor != "" {
			answer += "\nPuedo (declarado en mi identidad): " + strings.Join(declared, ", ") + " — sin evidencia de práctica que lo respalde"
		}
		return answer
	}
	return "Puedo: " + strings.Join(declared, ", ")
}

// answerWhatFailed — "¿Qué falló?"
func (sq *SelfQuery) answerWhatFailed() string {
	last := sq.history.GetLastError()
	if last == nil {
		return "Todo funcionó bien hasta ahora"
	}

	return fmt.Sprintf(
		"El último error fue: Intenté %s pero %s. Ocurrió hace poco",
		last.Action,
		last.Result,
	)
}

// answerStats — "¿Cómo te va?"
func (sq *SelfQuery) answerStats() string {
	stats := sq.history.GetStats()
	successRate := stats["success_rate"].(float64)

	return fmt.Sprintf(
		"En esta sesión: %d acciones totales. %.1f%% funcionaron bien. Promedio: %dms por acción",
		stats["total_actions"],
		successRate,
		stats["avg_duration_ms"],
	)
}

// answerModules — "¿De qué estás hecho?"
func (sq *SelfQuery) answerModules() string {
	modules := sq.registry.ListModules()
	if len(modules) == 0 {
		return "Tengo varios módulos que trabajan juntos, pero el registro está vacío"
	}

	names := []string{}
	for _, m := range modules {
		names = append(names, m.Name)
	}
	return "Estoy compuesto por estos módulos: " + strings.Join(names, ", ")
}

// answerPreviousContext — "¿Qué estábamos haciendo?"
func (sq *SelfQuery) answerPreviousContext() string {
	last := sq.history.GetLastAction()
	if last == nil {
		return "No hay sesión anterior en mi historial"
	}

	return fmt.Sprintf(
		"Estábamos: %s. La última acción fue %s",
		last.Input,
		last.Action,
	)
}

// answerLastFile — "¿Cuál fue el último archivo?"
func (sq *SelfQuery) answerLastFile() string {
	// Buscar última acción de buscar/abrir archivo
	actions := sq.history.GetByAction("buscar_archivo")
	if len(actions) == 0 {
		return "No recuerdo haber buscado archivos"
	}

	last := actions[len(actions)-1]
	return fmt.Sprintf(
		"El último archivo fue: %s",
		last.Target,
	)
}

// answerWhy — "¿Por qué...?"
func (sq *SelfQuery) answerWhy(question string) string {
	last := sq.history.GetLastAction()
	if last == nil {
		return "Sin contexto, es difícil explicar por qué"
	}

	return fmt.Sprintf(
		"Porque me lo pediste cuando dijiste '%s'. Ejecuté %s porque eso es lo que entendí",
		last.Input,
		last.Action,
	)
}

// Helper
func contains(text string, keywords []string) bool {
	for _, kw := range keywords {
		if strings.Contains(text, kw) {
			return true
		}
	}
	return false
}

// isGreetingOnly reports whether the normalized turn contains nothing but a
// greeting vocative ("hola", "buenas tardes", "hola Ahmelia", "que tal"...).
// Content words disqualify it: "hola, ¿puedes abrir X?" is not a greeting.
func isGreetingOnly(q string) bool {
	q = strings.TrimSpace(q)
	if q == "" {
		return false
	}
	cleaned := strings.NewReplacer(
		",", " ", ".", " ", "¿", " ", "?", " ", "¡", " ", "!", " ", ":", " ", ";", " ",
	).Replace(q)
	fields := strings.Fields(cleaned)
	if len(fields) == 0 {
		return false
	}
	for _, f := range fields {
		switch f {
		case "hola", "buenas", "buenos", "dia", "dias", "tardes", "noches",
			"ahmelia", "amelia", "aymelia", "que", "tal", "hey", "eh":
		default:
			return false
		}
	}
	return true
}
