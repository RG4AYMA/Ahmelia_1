package interaction

import "strings"

// ResponseKind describes what Ahmelia should do conversationally before a
// domain capability is selected. It is deliberately domain-neutral.
type ResponseKind string

const (
	ResponseNone              ResponseKind = ""
	ResponseAcknowledge       ResponseKind = "acknowledge"
	ResponseInvite            ResponseKind = "invite"
	ResponseClarify           ResponseKind = "clarify"
	ResponseCorrectionAck     ResponseKind = "correction_ack"
	ResponsePauseAck          ResponseKind = "pause_ack"
	ResponseContinuationState ResponseKind = "continuation_state"
	ResponseOptions           ResponseKind = "options"
	ResponseContextAnswer     ResponseKind = "context_answer"
)

// InteractionDecision is the small contract between the communication core
// and Engine. It says whether this turn is safe to resolve conversationally
// without invoking a domain provider or the legacy classifier.
type InteractionDecision struct {
	Handle bool
	Kind   ResponseKind
	Reply  string
}

// DecideResponse handles only interaction-level turns. It never selects
// Programming, Search, Unity, Blender, a file type, or another domain.
func DecideResponse(r SemanticRequest, ctx Context) InteractionDecision {
	lang := r.Language
	if lang == "" {
		lang = ctx.Language
	}
	if strings.HasPrefix(lang, "en") {
		return decideEnglish(r, ctx)
	}
	return decideSpanish(r, ctx)
}

func decideSpanish(r SemanticRequest, ctx Context) InteractionDecision {
	acts := CanonicalActs(r)
	if acts.Has(ActControl) {
		return InteractionDecision{Handle: true, Kind: ResponsePauseAck, Reply: "Entendido. Me quedo en espera y no haré nada hasta que me indiques continuar."}
	}
	if acts.Has(ActCorrection) {
		return InteractionDecision{Handle: true, Kind: ResponseCorrectionAck, Reply: "Entendido. No doy por válida mi interpretación anterior. Dime qué querías hacer y ajustamos el objetivo."}
	}
	if acts.Has(ActProposal) {
		if r.AddressesCurrent && ctx.ActiveObjective != "" {
			return InteractionDecision{Handle: true, Kind: ResponseAcknowledge, Reply: "Sí, podemos plantear esa mejora sobre el trabajo actual. No haré cambios todavía; primero podemos concretar qué quieres mejorar."}
		}
		return InteractionDecision{Handle: true, Kind: ResponseAcknowledge, Reply: "Sí, podemos explorar esa propuesta. Cuéntame qué quieres conseguir y la convertimos en un objetivo concreto."}
	}
	if acts.Has(ActSocial) && !acts.Has(ActQuestion) {
		if isOpenHelpTurn(r.Normalized) {
			return InteractionDecision{Handle: true, Kind: ResponseInvite, Reply: "Claro. Cuando quieras, dime qué quieres conseguir y trabajamos sobre ello paso a paso."}
		}
		return InteractionDecision{Handle: true, Kind: ResponseAcknowledge, Reply: "Te escucho. Dime qué tienes en mente y lo vemos juntos."}
	}
	if acts.Has(ActQuestion) {
		if r.AddressesCurrent || r.FollowUp {
			return InteractionDecision{}
		}
		if isOpenHelpTurn(r.Normalized) {
			if ctx.ActiveObjective != "" {
				return InteractionDecision{Handle: true, Kind: ResponseOptions, Reply: "Ahora mismo podemos continuar con el objetivo que tenemos activo, revisar lo que ya hice, cambiar el objetivo o empezar uno nuevo. Tú decides qué camino seguimos."}
			}
			return InteractionDecision{Handle: true, Kind: ResponseOptions, Reply: "Podemos conversar, resolver una duda, investigar algo, crear o modificar algo, o planificar un objetivo. Dime qué quieres conseguir."}
		}
	}
	if acts.Has(ActFeedback) {
		return InteractionDecision{Handle: true, Kind: ResponseAcknowledge, Reply: "Gracias por decírmelo. Sí, todavía tengo cosas por mejorar, pero el avance cuenta."}
	}
	if acts.Has(ActDeclaration) || acts.Has(ActObservation) || acts.Has(ActInform) || acts.Has(ActConfirmation) || acts.Has(ActRejection) || acts.Has(ActIdea) || acts.Has(ActExploration) {
		return InteractionDecision{Handle: true, Kind: ResponseAcknowledge, Reply: "Entendido. Lo tengo en cuenta."}
	}
	return InteractionDecision{}
}

func decideEnglish(r SemanticRequest, ctx Context) InteractionDecision {
	acts := CanonicalActs(r)
	if acts.Has(ActControl) {
		return InteractionDecision{Handle: true, Kind: ResponsePauseAck, Reply: "Understood. I will wait and won't do anything until you tell me to continue."}
	}
	if acts.Has(ActCorrection) {
		return InteractionDecision{Handle: true, Kind: ResponseCorrectionAck, Reply: "Understood. I won't assume my previous interpretation was correct. Tell me what you meant and we'll adjust the objective."}
	}
	if acts.Has(ActProposal) {
		return InteractionDecision{Handle: true, Kind: ResponseAcknowledge, Reply: "Yes, we can explore that proposal. Tell me what you want to achieve and we'll turn it into a concrete objective."}
	}
	if acts.Has(ActSocial) {
		if isOpenHelpTurn(r.Normalized) {
			return InteractionDecision{Handle: true, Kind: ResponseInvite, Reply: "Sure. Whenever you're ready, tell me what you want to achieve and we'll work through it together."}
		}
		return InteractionDecision{Handle: true, Kind: ResponseAcknowledge, Reply: "I'm listening. Tell me what's on your mind and we'll work through it together."}
	}
	if acts.Has(ActQuestion) {
		if r.AddressesCurrent || r.FollowUp {
			return InteractionDecision{}
		}
		if isOpenHelpTurn(r.Normalized) {
			if ctx.ActiveObjective != "" {
				return InteractionDecision{Handle: true, Kind: ResponseOptions, Reply: "We can continue the active objective, review what I've done, change the objective, or start a new one. You decide where we go next."}
			}
			return InteractionDecision{Handle: true, Kind: ResponseOptions, Reply: "We can talk, answer a question, investigate something, create or modify something, or plan an objective. Tell me what you want to achieve."}
		}
	}
	if acts.Has(ActFeedback) {
		return InteractionDecision{Handle: true, Kind: ResponseAcknowledge, Reply: "Thanks for telling me. I still have things to improve, but the progress matters."}
	}
	if acts.Has(ActDeclaration) || acts.Has(ActObservation) || acts.Has(ActInform) || acts.Has(ActConfirmation) || acts.Has(ActRejection) || acts.Has(ActIdea) || acts.Has(ActExploration) {
		return InteractionDecision{Handle: true, Kind: ResponseAcknowledge, Reply: "Understood. I'll keep that in mind."}
	}
	return InteractionDecision{}
}

func isOpenHelpTurn(n string) bool {
	return containsAny(n,
		"qué podemos hacer ahora", "que podemos hacer ahora",
		"qué podemos hacer", "que podemos hacer",
		"qué hacemos ahora", "que hacemos ahora",
		"quiero que me ayudes con algo", "todavía no te digo qué",
		"todavia no te digo que", "qué podemos hacer juntos",
		"what can we do now", "what can we do", "what should we do",
		"i want you to help me with something", "i haven't told you what yet",
	)
}
