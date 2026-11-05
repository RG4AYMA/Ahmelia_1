package interaction

import "strings"

// CommunicativeAct is the channel/domain-independent description of what a
// human is doing with a turn. It is classification evidence only: an act
// never authorizes execution, chooses a provider, mutates memory, decides
// initiative, or generates language.
type CommunicativeAct string

const (
	ActUnknown              CommunicativeAct = "unknown"
	ActQuestion             CommunicativeAct = "question"
	ActCommand              CommunicativeAct = "command"
	ActInform               CommunicativeAct = "inform"
	ActDeclaration          CommunicativeAct = "declaration"
	ActObservation          CommunicativeAct = "observation"
	ActFeedback             CommunicativeAct = "feedback"
	ActCorrection           CommunicativeAct = "correction"
	ActConfirmation         CommunicativeAct = "confirmation"
	ActAuthorizationInquiry CommunicativeAct = "authorization_inquiry"
	ActRejection            CommunicativeAct = "rejection"
	ActProposal             CommunicativeAct = "proposal"
	ActIdea                 CommunicativeAct = "idea"
	ActExploration          CommunicativeAct = "exploration"
	ActOpinionRequest       CommunicativeAct = "opinion_request"
	ActDecision             CommunicativeAct = "decision"
	ActContinuation         CommunicativeAct = "continuation"
	ActReference            CommunicativeAct = "reference"
	ActSocial               CommunicativeAct = "social"
	ActControl              CommunicativeAct = "control"
	ActLearning             CommunicativeAct = "learning"
	ActAnswer               CommunicativeAct = "answer"
	ActAnaphora             CommunicativeAct = "anaphora"
	ActTopicShift           CommunicativeAct = "topic_shift"
	ActCompound             CommunicativeAct = "compound"
)

// CommunicativeActEvidence is intentionally small and auditable. Evidence is
// provenance for classification, not a response instruction.
type CommunicativeActEvidence struct {
	Act        CommunicativeAct
	Confidence float64
	Source     string
}

// CommunicativeActs is the canonical classification result for one human
// turn. Multiple acts are allowed because real utterances can perform more
// than one communicative function at once.
type CommunicativeActs struct {
	Primary    CommunicativeAct
	Secondary  []CommunicativeAct
	Confidence float64
	Ambiguous  bool
	Evidence   []CommunicativeActEvidence
}

// CanonicalActs returns the only semantic act contract used by runtime
// consumers. Normally CommunicativeActs is populated by Analyze. The
// compatibility fallback exists only for detached legacy callers/tests that
// construct SemanticRequest values manually; it is never used to classify a
// live human turn.
func CanonicalActs(r SemanticRequest) CommunicativeActs {
	if r.CommunicativeActs.Primary != "" && (r.CommunicativeActs.Primary != ActUnknown || len(r.CommunicativeActs.Evidence) > 0) {
		return r.CommunicativeActs
	}
	if strings.TrimSpace(r.Raw) != "" || strings.TrimSpace(r.Normalized) != "" {
		acts := ClassifyCommunicativeActs(r)
		if acts.Primary != ActUnknown || (r.Kind == KindUnknown && r.SpeechAct == SpeechUnknown) {
			return acts
		}
	}
	// Detached compatibility projection only. No runtime parser calls this
	// path because every live turn has Raw/Normalized semantic evidence.
	var act CommunicativeAct
	switch r.Kind {
	case KindQuestion, KindReasoning:
		act = ActQuestion
	case KindAction, KindProduction:
		act = ActCommand
	case KindCorrection:
		act = ActCorrection
	case KindProposal:
		act = ActProposal
	case KindFeedback:
		act = ActFeedback
	case KindLearning:
		act = ActLearning
	case KindContinuation:
		act = ActContinuation
	case KindStop, KindPause:
		act = ActControl
	case KindInform:
		act = ActInform
	case KindConversation:
		act = ActSocial
	default:
		act = ActUnknown
	}
	return CommunicativeActs{Primary: act, Confidence: r.Confidence, Ambiguous: act == ActUnknown, Evidence: []CommunicativeActEvidence{{Act: act, Confidence: r.Confidence, Source: "compatibility:detached_request"}}}
}

func (a CommunicativeActs) Has(act CommunicativeAct) bool {
	if a.Primary == act {
		return true
	}
	for _, x := range a.Secondary {
		if x == act {
			return true
		}
	}
	return false
}

// WithDerivedAct returns the classification with one additional act derived
// from a later semantic reading (e.g., the open comprehension contract). It
// never rewrites the canonical classification: the derived act joins as
// secondary evidence and only becomes primary when there was none. It is how
// a typed act from a later reading reaches the response stage.
func (a CommunicativeActs) WithDerivedAct(act CommunicativeAct, confidence float64, source string) CommunicativeActs {
	if act == "" || a.Has(act) {
		return a
	}
	out := a
	secondary := append([]CommunicativeAct(nil), a.Secondary...)
	if out.Primary == "" || out.Primary == ActUnknown {
		out.Primary = act
		out.Ambiguous = false
	} else {
		secondary = append(secondary, act)
	}
	out.Secondary = secondary
	out.Evidence = append(append([]CommunicativeActEvidence(nil), a.Evidence...), CommunicativeActEvidence{Act: act, Confidence: confidence, Source: source})
	if confidence > out.Confidence {
		out.Confidence = confidence
	}
	return out
}

// ClassifyCommunicativeActs is the single interaction-layer authority for
// communicative-act classification. It consumes the already normalized turn
// and established RequestKind; downstream layers must consume this result
// instead of reparsing the human text into a competing act taxonomy.
func ClassifyCommunicativeActs(r SemanticRequest) CommunicativeActs {
	n := normalizeForClassification(r.Normalized)
	if n == "" {
		n = normalizeForClassification(r.Raw)
	}
	if n == "" {
		return CommunicativeActs{Primary: ActUnknown, Ambiguous: true}
	}

	type candidate struct {
		act   CommunicativeAct
		score float64
		src   string
	}
	var c []candidate
	add := func(act CommunicativeAct, score float64, src string) {
		if score > 0 {
			c = append(c, candidate{act, score, src})
		}
	}

	// Canonical classification is intentionally independent of RequestKind and
	// SpeechAct. Those legacy projections may exist for compatibility, but they
	// are never allowed to determine the communicative meaning of the turn.
	if isStop(n) || isPause(n) {
		add(ActControl, .99, "semantic:control")
	}
	if isQuestion(n) {
		add(ActQuestion, .98, "semantic:question")
	}
	if isCorrection(n) || r.Correction.IsCorrection {
		add(ActCorrection, .98, "semantic:correction")
	}
	if isFeedback(n) {
		add(ActFeedback, .91, "semantic:feedback")
	}
	if isProposal(n) {
		add(ActProposal, .96, "semantic:proposal")
	}
	for _, rel := range r.Relations {
		switch strings.ToLower(strings.TrimSpace(rel.Predicate)) {
		case "proposes", "anticipates", "commits_to":
			score := rel.Confidence
			if score <= 0 {
				score = .70
			}
			if score > .90 {
				score = .90
			}
			add(ActProposal, score, "relation:"+strings.ToLower(strings.TrimSpace(rel.Predicate)))
		}
	}
	if isLearning(n) {
		add(ActLearning, .94, "semantic:learning")
	}
	if isAuthorizationStatement(n, r.Raw) {
		add(ActConfirmation, .99, "semantic:authorization_performative")
	}
	if isAuthorizationInquiry(n, r.Raw) {
		add(ActAuthorizationInquiry, .99, "semantic:authorization_inquiry")
		add(ActQuestion, .94, "semantic:authorization_inquiry")
	}
	if isConfirmationStatement(n) {
		add(ActConfirmation, .99, "semantic:confirmation")
	}
	if isRejectionStatement(n) {
		add(ActRejection, .97, "semantic:rejection")
	}
	if isDecisionStatement(n) {
		add(ActDecision, .94, "semantic:decision")
	}
	if isSearch(n) {
		// Search is an information request, not a physical capability command.
		// Keep command evidence for provenance, but make the answer-seeking act
		// authoritative so the response boundary can render the retrieved
		// knowledge instead of treating search as a local action such as
		// buscar_archivo.
		add(ActQuestion, .985, "semantic:search_request")
		add(ActCommand, .78, "semantic:search_command")
	}
	if inferAction(n).Name != "" {
		add(ActCommand, .96, "semantic:action_intent")
	}
	if isCommandStatement(n) {
		add(ActCommand, .94, "semantic:imperative_statement")
	}
	if isProduction(n) {
		add(ActCommand, .94, "semantic:production")
	}
	if r.FollowUp || r.Continues || r.AddressesCurrent {
		add(ActContinuation, .78, "semantic:continuity")
	}
	if len(r.References) > 0 {
		add(ActReference, .84, "semantic:reference")
	}
	if hasAnaphoricReference(n) {
		add(ActAnaphora, .88, "semantic:anaphora")
	}
	if isTopicShift(n) {
		add(ActTopicShift, .86, "semantic:topic_shift")
	}
	if r.QuestionType == "opinion" || strings.EqualFold(r.QuestionType, "opinión") || isOpinionRequest(n) {
		add(ActOpinionRequest, .99, "semantic:opinion_request")
	}
	if hasMeaningConceptLocal(r, ConceptDecision) {
		add(ActDecision, .93, "meaning:decision")
	}
	if hasMeaningConceptLocal(r, ConceptAgreement) {
		add(ActConfirmation, .88, "meaning:agreement")
	}
	if hasMeaningConceptLocal(r, ConceptDisagreement) {
		add(ActRejection, .88, "meaning:disagreement")
	}
	if hasMeaningConceptLocal(r, ConceptPreference) {
		add(ActDeclaration, .82, "meaning:preference")
	}
	if hasMeaningConceptLocal(r, ConceptObservation) {
		add(ActObservation, .88, "meaning:observation")
		add(ActInform, .70, "meaning:observation_as_information")
	}
	if hasMeaningConceptLocal(r, ConceptFeedback) {
		add(ActFeedback, .90, "meaning:feedback")
	}

	if isGreeting(n) || isSocial(n) || isOpenHelpTurn(n) || strings.HasPrefix(n, "hola") || strings.HasPrefix(n, "buenos ") || strings.HasPrefix(n, "buenas ") {
		add(ActSocial, .90, "semantic:social")
	}
	if isFirstPersonDeclaration(n) {
		add(ActDeclaration, .86, "semantic:declaration")
		add(ActInform, .70, "semantic:declaration_as_information")
	}
	if isExploratoryStatement(n) {
		add(ActExploration, .97, "semantic:exploration")
		add(ActInform, .70, "semantic:exploration_as_information")
	}
	if isIdeaStatement(n) {
		add(ActIdea, .94, "semantic:idea")
		add(ActInform, .70, "semantic:idea_as_information")
	}
	if isDecisionStatement(n) {
		add(ActDecision, .86, "semantic:decision_statement")
		add(ActInform, .70, "semantic:decision_as_information")
	}

	// A plain declarative turn still needs a communicative representation. Do
	// this only after stronger acts have been collected so an idea, feedback,
	// correction, etc. never gets flattened into generic information.
	if len(c) == 0 && looksInformative(n) {
		add(ActInform, .80, "semantic:inform")
	}
	if len(c) == 0 {
		add(ActUnknown, .20, "semantic:insufficient_evidence")
	}

	// Aggregate the strongest evidence per act.
	type agg struct {
		score   float64
		sources []string
	}
	m := map[CommunicativeAct]agg{}
	for _, x := range c {
		a := m[x.act]
		if x.score > a.score {
			a.score = x.score
		}
		a.sources = append(a.sources, x.src)
		m[x.act] = a
	}
	ordered := make([]candidate, 0, len(m))
	for act, a := range m {
		ordered = append(ordered, candidate{act, a.score, strings.Join(a.sources, "+")})
	}
	for i := 0; i < len(ordered); i++ {
		for j := i + 1; j < len(ordered); j++ {
			if ordered[j].score > ordered[i].score {
				ordered[i], ordered[j] = ordered[j], ordered[i]
			}
		}
	}

	out := CommunicativeActs{Primary: ordered[0].act, Confidence: ordered[0].score}
	for _, x := range ordered {
		out.Evidence = append(out.Evidence, CommunicativeActEvidence{Act: x.act, Confidence: x.score, Source: x.src})
	}
	for i := 1; i < len(ordered); i++ {
		if ordered[i].score >= ordered[0].score-.30 {
			out.Secondary = append(out.Secondary, ordered[i].act)
		}
	}
	compound := false
	if m[ActCommand].score > 0 && containsAny(n, " pero ", " y ", " primero ", ", pero", "but ", " and ", " first ") &&
		(m[ActConfirmation].score > 0 || m[ActRejection].score > 0 || m[ActCorrection].score > 0 || m[ActProposal].score > 0 || m[ActQuestion].score > 0) {
		compound = true
	}
	if m[ActQuestion].score > 0 && m[ActCommand].score > 0 && containsAny(n, " pero ", " y ", " but ", " and ") {
		compound = true
	}
	if m[ActFeedback].score > 0 && (m[ActCorrection].score > 0 || m[ActRejection].score > 0) && containsAny(n, " pero ", " y ", " but ", " and ") {
		compound = true
	}
	if compound {
		out.Secondary = append(out.Secondary, ActCompound)
		out.Evidence = append(out.Evidence, CommunicativeActEvidence{Act: ActCompound, Confidence: .95, Source: "semantic:multiple_acts"})
	}

	if len(ordered) > 1 && ordered[0].score-ordered[1].score < .12 {
		out.Ambiguous = true
	}
	return out
}

func hasAnaphoricReference(n string) bool {
	for _, x := range []string{"eso", "esa ", "ese ", "aquello", "aquella", "aquel ", "lo anterior", "la anterior", "el anterior", "that ", "those ", "it ", "the previous"} {
		if strings.Contains(" "+n, " "+x) {
			return true
		}
	}
	return false
}

func isTopicShift(n string) bool {
	return containsAny(n, "cambiando de tema", "otro tema", "hablemos de otra cosa", "por cierto", "a propósito", "a proposito", "cambiemos de tema", "changing the subject", "another topic", "by the way")
}

func hasMeaningConceptLocal(r SemanticRequest, c SemanticConcept) bool {
	return r.Meaning.Concepts != nil && containsSemanticConceptForActs(r.Meaning.Concepts, c)
}
func containsSemanticConceptForActs(cs []SemanticConcept, target SemanticConcept) bool {
	for _, c := range cs {
		if c == target {
			return true
		}
	}
	return false
}

func isCommandStatement(n string) bool {
	n = strings.TrimSpace(n)
	if n == "" {
		return false
	}
	verbs := []string{"haz ", "hazlo ", "hazlo", "hagan ", "cambia ", "cambiar ", "modifica ", "modificar ", "corrige ", "corrigir ", "crea ", "crear ", "abre ", "abrir ", "cierra ", "cerrar ", "ejecuta ", "ejecutar ", "mueve ", "mover ", "renombra ", "renombrar ", "busca ", "buscar ", "do ", "make ", "change ", "modify ", "create ", "open ", "close ", "run ", "move ", "rename ", "find "}
	for _, v := range verbs {
		if strings.HasPrefix(n, v) || strings.Contains(n, ", "+v) || strings.Contains(n, " pero "+v) || strings.Contains(n, " y "+v) || strings.Contains(n, " but "+v) || strings.Contains(n, " and "+v) {
			return true
		}
	}
	return false
}

func isFirstPersonDeclaration(n string) bool {
	return strings.HasPrefix(n, "mi ") || strings.HasPrefix(n, "mis ") ||
		strings.HasPrefix(n, "prefiero ") || strings.HasPrefix(n, "me gusta ") ||
		strings.HasPrefix(n, "me gustan ") || strings.HasPrefix(n, "soy ") ||
		strings.HasPrefix(n, "estoy ") || strings.HasPrefix(n, "llevo ") || strings.HasPrefix(n, "he estado ") || strings.HasPrefix(n, "tengo ") ||
		strings.HasPrefix(n, "my ") || strings.HasPrefix(n, "i am ") ||
		strings.HasPrefix(n, "i'm ") || strings.HasPrefix(n, "i have ") || strings.HasPrefix(n, "i've been ") || strings.HasPrefix(n, "i have been ") ||
		strings.HasPrefix(n, "i prefer ") || strings.HasPrefix(n, "i like ")
}

func isExploratoryStatement(n string) bool {
	return strings.Contains(n, "estoy pensando") || strings.Contains(n, "estoy considerando") ||
		strings.Contains(n, "no estoy seguro") || strings.Contains(n, "no me decido") ||
		strings.Contains(n, "aún no sé") || strings.Contains(n, "aun no se") ||
		strings.Contains(n, "i'm thinking") || strings.Contains(n, "i am thinking") ||
		strings.Contains(n, "i'm considering") || strings.Contains(n, "i am considering") ||
		strings.Contains(n, "not sure") || strings.Contains(n, "haven't decided")
}

func isIdeaStatement(n string) bool {
	return strings.Contains(n, "tengo la idea") || strings.Contains(n, "tengo una idea") || strings.Contains(n, "se me ocurre") ||
		strings.Contains(n, "estaba pensando en") || strings.Contains(n, "podría ser") ||
		strings.Contains(n, "podriamos") || strings.Contains(n, "podríamos") ||
		strings.Contains(n, "i have an idea") || strings.Contains(n, "i was thinking") ||
		strings.Contains(n, "could be") || strings.Contains(n, "we could")
}

func isDecisionStatement(n string) bool {
	return strings.HasPrefix(n, "decidí ") || strings.HasPrefix(n, "decidi ") ||
		strings.HasPrefix(n, "he decidido ") || strings.HasPrefix(n, "decidimos ") ||
		strings.HasPrefix(n, "decidamos ") || strings.HasPrefix(n, "i decided ") ||
		strings.HasPrefix(n, "we decided ")
}

func isConfirmationStatement(n string) bool {
	n = strings.TrimSpace(strings.Trim(n, " .!?¿¡"))
	if strings.HasPrefix(n, "sí, ") || strings.HasPrefix(n, "si, ") || strings.HasPrefix(n, "yes, ") {
		return true
	}
	return n == "sí" || n == "si" || n == "exacto" || n == "correcto" ||
		n == "así es" || n == "asi es" || n == "yes" || n == "exactly" || n == "correct" ||
		n == "sí, exactamente" || n == "si, exactamente"
}

// isAuthorizationStatement recognizes the first-person performative itself,
// regardless of conversational preambles or object clitics ("bueno, entonces
// la autorizo"). This is a policy-relevant speech act, so lexical evidence is
// deliberately conservative: questions, negation, and conditional clauses do
// not grant permission. The pending-work authority still binds the grant to
// the exact operation that is waiting.
func isAuthorizationStatement(n, raw string) bool {
	n = strings.TrimSpace(n)
	if n == "" || hasQuestionSyntax(n, raw) || hasAuthorizationNegationOrCondition(n, raw) {
		return false
	}
	for _, token := range strings.Fields(n) {
		switch strings.Trim(token, "\"'()[]{}") {
		case "autorizo", "apruebo":
			return true
		}
	}
	return strings.Contains(n, "doy mi autorizacion")
}

func isAuthorizationInquiry(n, raw string) bool {
	n = strings.TrimSpace(n)
	mentionsAuthorization := strings.Contains(n, "autoriz") || strings.Contains(n, "aprob") || strings.Contains(n, "permiso")
	if !mentionsAuthorization {
		return false
	}
	return hasAuthorizationNegationOrCondition(n, raw) || hasQuestionSyntax(n, raw)
}

func hasQuestionSyntax(normalized, raw string) bool {
	return strings.Contains(raw, "?") || strings.Contains(raw, "¿") || isQuestion(normalized)
}

func hasAuthorizationNegationOrCondition(n, raw string) bool {
	affirmativeSi := strings.Contains(strings.ToLower(raw), "sí")
	for _, token := range strings.Fields(n) {
		switch strings.Trim(token, "\"'()[]{}.,;:") {
		case "no", "nunca", "jamas", "cuando", "siempre", "unless", "if":
			return true
		case "si":
			if !affirmativeSi {
				return true
			}
		}
	}
	return false
}

func isRejectionStatement(n string) bool {
	n = strings.TrimSpace(strings.Trim(n, " .!?¿¡"))
	return n == "no" || n == "nope" || n == "that's not" || n == "no es así" || n == "no es asi"
}

func isOpinionRequest(n string) bool {
	return strings.Contains(n, "qué opinas") || strings.Contains(n, "que opinas") ||
		strings.Contains(n, "qué piensas") || strings.Contains(n, "que piensas") ||
		strings.Contains(n, "qué te parece") || strings.Contains(n, "que te parece") ||
		strings.Contains(n, "what do you think") || strings.Contains(n, "what's your opinion")
}
