package core

import (
	"errors"
	"strings"
	"unicode"

	"ahmelia/cognition"
	"ahmelia/interaction"
	"ahmelia/language"
	"ahmelia/linguistics"
)

// ResponseOrchestrator is the final behavioral hand-off before Language.
// Engine branches can provide grounded evidence, but they no longer own the
// conversational decision or the normal wording path. This is intentionally
// small: it is a convergence point, not a new cognitive authority.
type ResponseOrchestrator struct {
	generator *language.ConversationalGenerator
}

func NewResponseOrchestrator(generator *language.ConversationalGenerator) *ResponseOrchestrator {
	return &ResponseOrchestrator{generator: generator}
}

func (o *ResponseOrchestrator) Render(req interaction.SemanticRequest, contract interaction.ResponseContract, snap interaction.ConversationSnapshot, strategy interface{}, socialPlan interface{}, facts language.RuntimeFacts) (string, string, error) {
	if o == nil || !contract.Valid() {
		return "", "", nil
	}
	// The concrete B96.33/B96.38 contracts are supplied by the Engine helper
	// below. Keeping this public surface intentionally narrow avoids making the
	// orchestrator a new source of cognition or social decisions.
	return "", "", nil
}

func responseFallback(contract interaction.ResponseContract) string {
	// Metacognitive diagnostics are semantic evidence, never a fallback
	// sentence. If Language is unavailable, emit a generic response rather
	// than leaking diagnostic[kind] key=value into the user-visible output.
	if contract.Source == "metacognitive_diagnostic" {
		facts := make([]string, 0, len(contract.Evidence))
		for _, evidence := range contract.Evidence {
			text := strings.TrimSpace(evidence.Text)
			// Only the agenda's already projected, user-relevant state facts are
			// safe to expose in this fallback. Internal diagnostics remain private.
			if evidence.Relevant && text != "" && evidence.Source == "reasoning_deep.mental_agenda" {
				facts = append(facts, text)
			}
		}
		if len(facts) > 0 {
			return "Mi estado actual: " + strings.Join(facts, "; ") + "."
		}
		return "Puedo explicarte mi estado actual, pero no tengo datos suficientes para describirlo."
	}
	if contract.Source == "initiative_intervention" {
		switch contract.Move {
		case interaction.MoveAsk:
			return "Quiero preguntarte algo para continuar."
		case interaction.MovePropose:
			return "Tengo una propuesta relacionada con lo que estamos haciendo."
		case interaction.MoveContinue:
			return "Podemos continuar desde aquí."
		case interaction.MoveAcknowledge:
			return "Tengo algo que señalar sobre lo que estamos haciendo."
		case interaction.MoveInvite:
			return "Encontré algo útil relacionado con lo que estamos haciendo."
		default:
			return "Tengo algo que aportar sobre lo que estamos haciendo."
		}
	}
	for _, evidence := range contract.Evidence {
		if evidence.Relevant && strings.TrimSpace(evidence.Text) != "" {
			return evidence.Text
		}
	}
	switch contract.Move {
	case interaction.MoveAcknowledge:
		return "Entendido."
	case interaction.MoveClarify:
		return "Necesito un poco más de contexto para responderte bien."
	case interaction.MoveInvite:
		return "Podemos seguir desde ahí."
	case interaction.MoveContinue:
		return "Continuemos."
	default:
		return "Entendido."
	}
}

// renderCanonicalTextResult converts a post-cognition specialist/result text
// into the same ResponseContract/Language boundary used by evidence collected
// before Situation. It is intentionally not a new authority: the caller already
// owns the result/decision and this helper only performs the language handoff.
func (e *Engine) renderCanonicalTextResult(req interaction.SemanticRequest, source, text string, confidence float64, move interaction.ResponseMove) bool {
	if e == nil || strings.TrimSpace(text) == "" {
		return false
	}
	if !interaction.ResponseMoveIsValid(move) {
		move = interaction.MoveAnswer
	}
	contract := interaction.NewResponseContract(req, e.lastResponseIntent, move)
	contract.Source = strings.TrimSpace(source)
	contract.AddEvidence(text, contract.Source, true, confidence)
	return e.renderResponseContract(req, contract, e.buildConversationalRuntimeFacts(e.conversationState.Snapshot()))
}

func (e *Engine) renderResponseContract(req interaction.SemanticRequest, contract interaction.ResponseContract, facts language.RuntimeFacts) bool {
	strategy := e.lastConversationalCognition.Strategy
	if e.lastBehavioralDecision.Version == "B96.40.1" {
		strategy = e.lastBehavioralDecision.Strategy
	}
	return e.renderResponseContractWithStrategy(req, contract, facts, strategy)
}

func (e *Engine) generateResponseContractWithStrategy(req interaction.SemanticRequest, contract interaction.ResponseContract, facts language.RuntimeFacts, strategy cognition.ConversationalResponseStrategy) (string, bool) {
	if e == nil || !contract.Valid() {
		return "", false
	}
	if e.lastBehavioralDecision.Version == "B96.40.1" &&
		(contract.Source == "b96.39_response_contract" || contract.Source == "initiative_intervention") &&
		!e.lastBehavioralDecision.SpeakToCurrentTurn {
		return "", false
	}
	if e.flowTrace != nil {
		source := contract.Source
		if source == "" {
			source = "response_contract"
		}
		e.flowTrace.Mark("RESPONSE", "intent="+string(contract.Intent.Kind)+" move="+string(contract.Move)+" source="+source+" subject="+string(contract.Subject))
		evidenceSources := make([]string, 0, len(contract.Evidence))
		seen := map[string]bool{}
		for _, evidence := range contract.Evidence {
			provider := strings.TrimSpace(evidence.Source)
			if evidence.Relevant && provider != "" && !seen[provider] {
				seen[provider] = true
				evidenceSources = append(evidenceSources, provider)
			}
		}
		if len(evidenceSources) > 0 {
			e.flowTrace.Mark("RESPONSE EVIDENCE", "sources="+strings.Join(evidenceSources, ","))
		}
	}
	// Feedback (escena feedback_recibido): cuando el acto tipado del turno es
	// feedback, la superficie es fija y SIN arrastrar el tema previo — el eco
	// de tema muere aquí. La observación del usuario sigue viajando como
	// evidencia (trazabilidad), pero el modelo queda fuera de este acuse.
	if contract.Intent.Kind == interaction.ResponseIntentAcknowledge && req.CommunicativeActs.Has(interaction.ActFeedback) {
		contract.AddEvidence("El usuario ha expresado esta observación: "+strings.TrimSpace(req.Raw), "user_turn", true, .99)
		if contract.RenderScene == nil {
			contract.RenderScene = &interaction.RenderScene{Scene: string(linguistics.MoveFeedbackRecibido)}
		}
	}
	// Fase 2 (2026-10-18): si el contrato trae una escena cerrada, la
	// superficie la escribe el renderizador determinista (linguistics/render.go)
	// y el modelo pequeño queda fuera de la redacción final de esa escena.
	// La única condición es el conjunto Fase 2; si el renderizador rechaza
	// (slots incompletos), el turno cae a los caminos existentes sin perder
	// habla.
	if text, ok := e.applyFase2Scene(contract); ok {
		return text, true
	}
	// Self-knowledge is deterministic authority. A high-confidence factual self
	// answer must not be reinterpreted by the language model, especially when
	// recent conversation history contains user-owned facts that could leak.
	// Sources: self_knowledge (legacy), user_model, self_runtime_state,
	// deterministic_answer, and knowledge:self (SelfKnowledgeSource via knowledge engine).
	isDeterministicSource := contract.Source == "self_knowledge" ||
		contract.Source == "user_model" ||
		contract.Source == "self_runtime_state" ||
		contract.Source == "deterministic_answer" ||
		strings.HasPrefix(contract.Source, "knowledge:self")
	if isDeterministicSource && hasHighConfidenceEvidenceForSource(contract, contract.Source) {
		fallback := responseFallback(contract)
		if strings.TrimSpace(fallback) != "" {
			if e.flowTrace != nil {
				e.flowTrace.Mark("LANGUAGE ORIGIN", "renderer=code evidence_source="+contract.Source)
			}
			return fallback, true
		}
	}
	if e.conversationGenerator != nil {
		natural, err := e.conversationGenerator.GenerateGrounded(req, e.conversationState.Snapshot(), contract.Intent, strategy, e.lastSocialPlan, facts, responseEvidenceTexts(contract))
		if err == nil && strings.TrimSpace(natural) != "" {
			// B96.45-G2 (restaurado 2026-10-16): un texto canónico del sistema
			// no puede perder su evidencia al ser reescrito. La redacción del
			// modelo se conserva solo si retiene ≥25% de los tokens de
			// contenido de la evidencia (render-then-verify); si no, se
			// entrega el texto exacto del código y la degradación queda
			// observable en el trace.
			if contract.Source == "deterministic_system_response" && !renderIsFaithful(natural, responseEvidenceTexts(contract)) {
				if e.flowTrace != nil {
					e.flowTrace.Mark("LANGUAGE DEGRADATION", "renderer=code_fallback reason=unfaithful_render source="+contract.Source)
					e.flowTrace.Mark("LANGUAGE ORIGIN", "renderer=code_fallback reason=render_unfaithful source="+contract.Source)
				}
			} else {
				if e.flowTrace != nil {
					e.flowTrace.Mark("LANGUAGE ORIGIN", "renderer=model:"+e.conversationGenerator.RendererID())
				}
				return natural, true
			}
		} else if e.flowTrace != nil {
			reason := "provider_error"
			switch {
			case err == nil:
				reason = "empty_model_output"
			case errors.Is(err, language.ErrConversationalGenerationTimeout):
				reason = "generation_timeout"
			case strings.Contains(strings.ToLower(err.Error()), "unavailable"):
				reason = "client_unavailable"
			case strings.Contains(strings.ToLower(err.Error()), "invalid conversational json"), strings.Contains(strings.ToLower(err.Error()), "empty conversational response"):
				reason = "invalid_model_output"
			}
			e.flowTrace.Mark("LANGUAGE ORIGIN", "renderer=code_fallback reason=model_generation_failed category="+reason)
			e.flowTrace.Mark("LANGUAGE DEGRADATION", "renderer=code_fallback category="+reason)
		}
	} else if e.flowTrace != nil {
		e.flowTrace.Mark("LANGUAGE ORIGIN", "renderer=code_fallback reason=model_renderer_not_configured")
	}
	// Fase 2 (2026-10-18): una iniciativa sin oración generada no habla. El
	// fallback de este contrato es genérico por construcción — su evidencia es
	// metadata semántica, nunca superficie — así que hablarlo es la
	// intervención vacía del defecto vivo («Encontré algo útil» sin nada útil
	// que decir, corridas 18/10). La decisión de intervenir ya se tomó; sin
	// contenido, la entrega se cierra en silencio, el mismo principio que la
	// lectura diferida sin acto tipado.
	if contract.Source == "initiative_intervention" {
		return "", false
	}
	fallback := responseFallback(contract)
	if strings.TrimSpace(fallback) == "" {
		return "", false
	}
	return fallback, true
}

// renderVerbatimSystemResponse delivers harness-decided text without letting
// the language layer rewrite it. It is reserved for owner commands whose
// content is exact (IDs, states, "no se ejecutó" clauses). If the canonical
// contract path cannot render, the text is delivered as-is.
func (e *Engine) renderVerbatimSystemResponse(req interaction.SemanticRequest, source, text string, move interaction.ResponseMove, kind interaction.ResponseIntentKind, needsInput bool) bool {
	if e == nil || strings.TrimSpace(text) == "" {
		return false
	}
	snap := e.canonicalSystemSnapshot()
	contract := e.buildCanonicalSystemContract(req, source, move, kind, needsInput, snap)
	contract.Source = "deterministic_answer"
	contract.AddEvidence(text, "deterministic_answer", true, .99)
	if e.renderResponseContract(req, contract, e.buildConversationalRuntimeFacts(snap)) {
		return true
	}
	e.deliverRenderedResponse(text)
	return true
}

// renderCanonicalSystemResponse is the final semantic-to-language hand-off for
// system-generated outcomes that do not come from a domain result renderer.
// The caller supplies the already-decided communicative intent and movement;
// this helper never chooses a capability, provider, or execution behavior.
func (e *Engine) renderCanonicalSystemResponse(req interaction.SemanticRequest, source, text string, move interaction.ResponseMove, kind interaction.ResponseIntentKind, needsInput bool) bool {
	if e == nil || strings.TrimSpace(text) == "" {
		return false
	}
	snap := e.canonicalSystemSnapshot()
	contract := e.buildCanonicalSystemContract(req, source, move, kind, needsInput, snap)
	contract.AddEvidence(text, contract.Source, true, contract.Intent.Confidence)
	return e.renderResponseContract(req, contract, e.buildConversationalRuntimeFacts(snap))
}

// renderCanonicalSceneResponse is the Fase-2 variant of
// renderCanonicalSystemResponse: the spoken surface comes from the contract's
// closed scene rendered deterministically (linguistics.Render). fallbackText,
// when non-empty, preserves the pre-scene wording only for the degraded path
// in which the renderer refuses the scene (incomplete slots).
func (e *Engine) renderCanonicalSceneResponse(req interaction.SemanticRequest, source string, scene interaction.RenderScene, fallbackText string, move interaction.ResponseMove, kind interaction.ResponseIntentKind, needsInput bool) bool {
	if e == nil {
		return false
	}
	snap := e.canonicalSystemSnapshot()
	contract := e.buildCanonicalSystemContract(req, source, move, kind, needsInput, snap)
	contract.RenderScene = &scene
	if strings.TrimSpace(fallbackText) != "" {
		contract.AddEvidence(fallbackText, contract.Source, true, contract.Intent.Confidence)
	}
	return e.renderResponseContract(req, contract, e.buildConversationalRuntimeFacts(snap))
}

func (e *Engine) canonicalSystemSnapshot() interaction.ConversationSnapshot {
	snap := interaction.ConversationSnapshot{}
	if e.conversationState != nil {
		snap = e.conversationState.Snapshot()
	}
	return snap
}

// buildCanonicalSystemContract centraliza la forma del contrato canónico de
// sistema compartida por renderCanonicalSystemResponse y su variante de
// escena Fase-2.
func (e *Engine) buildCanonicalSystemContract(req interaction.SemanticRequest, source string, move interaction.ResponseMove, kind interaction.ResponseIntentKind, needsInput bool, snap interaction.ConversationSnapshot) interaction.ResponseContract {
	if !interaction.ResponseMoveIsValid(move) {
		move = interaction.MoveAnswer
	}
	intent := e.lastResponseIntent
	if intent.Kind == interaction.ResponseIntentNone || kind != interaction.ResponseIntentNone {
		intent.Kind = kind
	}
	if intent.Kind == interaction.ResponseIntentNone {
		intent.Kind = interaction.ResponseIntentAnswer
	}
	intent.NeedsUserInput = needsInput
	intent.ShouldSpeak = true
	intent.ShouldAct = false
	intent.ShouldContinue = needsInput
	if intent.Confidence <= 0 {
		intent.Confidence = .99
	}
	if intent.Language == "" {
		intent.Language = snap.Language
	}
	contract := interaction.NewResponseContract(req, intent, move)
	contract.Source = "deterministic_system_response"
	contract.Reason = strings.TrimSpace(source)
	contract.ShouldAct = false
	contract.NeedsInput = needsInput
	return contract
}

// deliverRenderedResponse is the only low-level output primitive allowed to
// emit already-rendered language. All semantic response decisions must cross
// ResponseContract/Language before reaching this method.
func (e *Engine) deliverRenderedResponse(text string) {
	if e == nil || strings.TrimSpace(text) == "" {
		return
	}
	e.sayAsync(text)
}

func (e *Engine) renderResponseContractWithStrategy(req interaction.SemanticRequest, contract interaction.ResponseContract, facts language.RuntimeFacts, strategy cognition.ConversationalResponseStrategy) bool {
	text, ok := e.generateResponseContractWithStrategy(req, contract, facts, strategy)
	if !ok {
		return false
	}
	e.deliverRenderedResponse(text)
	return true
}

func responseEvidenceTexts(contract interaction.ResponseContract) []string {
	out := make([]string, 0, len(contract.Evidence))
	for _, evidence := range contract.Evidence {
		if evidence.Relevant && strings.TrimSpace(evidence.Text) != "" {
			out = append(out, evidence.Text)
		}
	}
	return out
}

// renderIsFaithful — B96.45-G2 (restaurado 2026-10-16): verificación léxica
// post-render. El texto generado debe conservar al menos el 25% de los tokens
// de contenido (≥4 runas) de la evidencia; un acuse genérico ("Entendido.")
// no conserva ninguno. Es deliberadamente léxico: cuando haya cómputo local
// disponible se reemplaza por verificación semántica sin cambiar los callers.
const (
	faithfulMinTokenRatio      = 0.25
	faithfulContentTokenMinLen = 4
)

func renderIsFaithful(natural string, evidence []string) bool {
	content := make(map[string]bool)
	for _, token := range tokenizeForFidelity(strings.Join(evidence, " ")) {
		if len([]rune(token)) >= faithfulContentTokenMinLen {
			content[token] = true
		}
	}
	if len(content) == 0 {
		return true
	}
	rendered := make(map[string]bool)
	for _, token := range tokenizeForFidelity(natural) {
		rendered[token] = true
	}
	kept := 0
	for token := range content {
		if rendered[token] {
			kept++
		}
	}
	return float64(kept)/float64(len(content)) >= faithfulMinTokenRatio
}

func tokenizeForFidelity(text string) []string {
	var tokens []string
	var current strings.Builder
	for _, r := range strings.ToLower(text) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			current.WriteRune(r)
			continue
		}
		if current.Len() > 0 {
			tokens = append(tokens, current.String())
			current.Reset()
		}
	}
	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}
	return tokens
}

func hasHighConfidenceEvidence(contract interaction.ResponseContract) bool {
	return hasHighConfidenceEvidenceForSource(contract, "")
}

func hasHighConfidenceEvidenceForSource(contract interaction.ResponseContract, source string) bool {
	for _, evidence := range contract.Evidence {
		if !evidence.Relevant || strings.TrimSpace(evidence.Text) == "" {
			continue
		}
		if source != "" && !strings.EqualFold(strings.TrimSpace(evidence.Source), source) {
			continue
		}
		// Source-specific authorities already produced grounded prose. For the
		// User Model, requiring a confirmed evidence item is more important than
		// a cosmetic confidence threshold because downstream language must not
		// change its factual value.
		if evidence.Confidence >= 0.99 || source == "user_model" {
			return true
		}
	}
	return false
}
