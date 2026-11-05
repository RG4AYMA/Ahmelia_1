package interaction

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"ahmelia/ownership"
	"ahmelia/shared"
)

// InputChannel identifies how a request entered Ahmelia.
// All channels converge to the same semantic contract.
type InputChannel string

const (
	ChannelUnknown InputChannel = "unknown"
	ChannelVoice   InputChannel = "voice"
	ChannelText    InputChannel = "text"
	ChannelUI      InputChannel = "ui"
	ChannelVision  InputChannel = "vision"
	ChannelGesture InputChannel = "gesture"
)

// RequestKind is domain-neutral. A domain/provider is selected later.
type RequestKind string

const (
	KindUnknown       RequestKind = "unknown"
	KindConversation  RequestKind = "conversation"
	KindQuestion      RequestKind = "question"
	KindAction        RequestKind = "action"
	KindProduction    RequestKind = "production"
	KindSearch        RequestKind = "search"
	KindReasoning     RequestKind = "reasoning"
	KindClarification RequestKind = "clarification"
	KindInform        RequestKind = "inform"
	KindProposal      RequestKind = "proposal"
	KindCorrection    RequestKind = "correction"
	KindStop          RequestKind = "stop"
	KindContinuation  RequestKind = "continuation"
	KindPause         RequestKind = "pause"
	KindLearning      RequestKind = "learning"
	KindPlanning      RequestKind = "planning"
	KindFeedback      RequestKind = "feedback"
)

// Objective is the domain-neutral representation of what the user wants.
type Objective struct {
	Text        string
	Constraints []string
	OpenEnded   bool
}

// ObjectiveFrame preserves the causal structure of a user objective before
// cognition or execution consume it. Secondary clauses cannot silently replace
// the primary operation.
type ObjectiveFrame struct {
	PrimaryGoal      string
	PrimaryOperation string
	Subject          string
	Workspace        string
	Constraints      []string
	RequiredActions  []string
	Verification     []string
	Recovery         []string
	Confidence       float64
	Provenance       string
}

// ReferenceKind describes what a pronoun/ellipsis points toward.
type ReferenceKind string

const (
	ReferenceNone           ReferenceKind = ""
	ReferenceRecentWork     ReferenceKind = "recent_work"
	ReferenceRecentArtifact ReferenceKind = "recent_artifact"
	ReferenceCurrentTopic   ReferenceKind = "current_topic"
	ReferenceCurrentInput   ReferenceKind = "current_input"
	ReferenceLocation       ReferenceKind = "location"
)

// Reference is semantic evidence, not a provider-specific lookup.
type ReferenceRole string

const (
	ReferenceRoleTarget         ReferenceRole = "target"
	ReferenceRoleContext        ReferenceRole = "context"
	ReferenceRoleRequirement    ReferenceRole = "requirement"
	ReferenceRoleRecoveryTarget ReferenceRole = "recovery_target"
)

type Reference struct {
	Kind       ReferenceKind
	Expression string
	Resolved   bool
	Value      string
	Role       ReferenceRole
	Confidence float64
	Provenance string
}

// SpeechAct describes what the user is doing communicatively. It is not a
// domain and never selects a capability.
type SpeechAct string

const (
	SpeechUnknown  SpeechAct = "unknown"
	SpeechQuestion SpeechAct = "question"
	SpeechCommand  SpeechAct = "command"
	SpeechInform   SpeechAct = "inform"
	SpeechPropose  SpeechAct = "propose"
	SpeechCorrect  SpeechAct = "correct"
	SpeechConfirm  SpeechAct = "confirm"
	SpeechReject   SpeechAct = "reject"
	SpeechContinue SpeechAct = "continue"
	SpeechStop     SpeechAct = "stop"
	SpeechPause    SpeechAct = "pause"
	SpeechSocial   SpeechAct = "social"
	SpeechFeedback SpeechAct = "feedback"
)

type UserStateSignals struct {
	Frustration float64 `json:"frustration"`
	Positive    float64 `json:"positive"`
	Fatigue     float64 `json:"fatigue"`
	Urgency     float64 `json:"urgency"`
	Confidence  float64 `json:"confidence"`
}

// SemanticRequest is the stable boundary between human interaction and the
// rest of Ahmelia. It must remain independent of Programming, Search, Unity,
// Blender, a particular language, or a particular I/O technology.
// SemanticRelation is linguistic evidence about a relation expressed in the
// current turn. It is not a decision, authorization, or source-of-truth claim.
type SemanticRelation struct {
	Source     string
	Predicate  string
	Target     string
	Value      string
	Confidence float64
	Evidence   string
}

type SemanticRequest struct {
	Raw        string
	Normalized string
	Channel    InputChannel
	Kind       RequestKind
	Objective  Objective
	Frame      ObjectiveFrame
	FollowUp   bool
	Continues  bool
	Language   string
	Confidence float64
	// Subject identifies the owner of user-model semantics when known.
	// "user" and "self" are explicit referents; it is never inferred by a
	// downstream model consumer.
	Subject           UserObservationSubject
	SubjectConfidence float64
	SubjectEvidence   []ownership.Evidence
	References        []Reference

	// Conversation state signals used by response/work layers.
	NeedsClarification bool
	AddressesCurrent   bool
	// BreaksContinuity is a derived discourse boundary. It is set by the
	// context-intelligence layer for explicit topic changes and is consumed
	// by the conversation router/state owner. It is not a memory mutation.
	BreaksContinuity bool
	ExplicitDomain   string

	// Action is the domain-neutral executable intent resolved at the same
	// semantic boundary. Execution layers consume it instead of reparsing the
	// original human sentence.
	Action ActionIntent

	// General interaction metadata. These remain domain-neutral and are
	// optional hints for cognition/work/response planning.
	Entities         []string
	Relations        []SemanticRelation
	Urgency          string
	RequestedAction  string
	ExpectedResponse string

	// Rich communication semantics. These fields are deliberately general.
	SpeechAct        SpeechAct
	Topic            string
	QuestionType     string
	Directness       string
	UserSignals      UserStateSignals
	CorrectionTarget string
	Correction       CorrectionInfo

	// Meaning is the language-independent semantic representation used after
	// linguistic interpretation and before response/work decisions.
	Meaning  SemanticMeaning
	Temporal TemporalReference

	// CommunicativeActs is the canonical interaction-layer classification of
	// what the human is doing with this turn. It is evidence, never a response
	// or execution decision.
	CommunicativeActs CommunicativeActs
}

func inferQuestionSubject(normalized string) UserObservationSubject {
	return UserObservationSubject(ownership.Resolve(normalized).Subject)
}

// Context is the transport-neutral conversation snapshot. Future adapters
// (vision/gesture/UI) can populate the same structure.
type Context struct {
	PreviousInput      string
	PreviousKind       RequestKind
	ActiveObjective    string
	ActiveWorkID       string
	ActiveWorkStatus   string
	CurrentStep        string
	LastResult         string
	ActiveWorkProvider string
	LastArtifactName   string
	LastArtifactPath   string
	LastArtifactNames  []string
	Language           string
	CurrentTopic       string
	PendingObjective   string
	Channel            InputChannel
}

type Manager struct {
	last    SemanticRequest
	ctx     Context
	history []SemanticRequest
}

func NewManager() *Manager {
	return &Manager{history: make([]SemanticRequest, 0, 16)}
}

// Analyze is the single semantic interpretation entry point.
func (m *Manager) Analyze(raw string, ctx Context) SemanticRequest {
	if m == nil {
		return Analyze(raw, ctx)
	}
	r := Analyze(raw, ctx)
	m.history = append(m.history, r)
	if len(m.history) > 32 {
		m.history = m.history[len(m.history)-32:]
	}
	m.last, m.ctx = r, ctx
	if r.Language != "" {
		m.ctx.Language = r.Language
	}
	return r
}

func Analyze(raw string, ctx Context) SemanticRequest {
	channel := ctx.Channel
	if channel == "" {
		channel = ChannelUnknown
	}
	r := Parse(raw, channel)
	semantic := normalizeForClassification(r.Normalized)
	r.Language = DetectLanguage(semantic, ctx.Language)
	r.Confidence = confidence(r.Kind)
	r.Action = inferActionForKind(r.Kind, semantic)
	r.Frame = BuildObjectiveFrame(r)
	r.SpeechAct = speechActFor(r.Kind, semantic)
	r.Topic = inferConversationTopic(semantic)
	r.QuestionType = inferQuestionType(semantic)
	r.Directness = inferDirectness(semantic)
	r.UserSignals = inferUserStateSignals(semantic)
	// Keep the richer normalized surface for correction extraction because
	// replacement text is semantic content, not merely a classifier surface.
	r.Correction = AnalyzeCorrection(r.Raw, r.Normalized)
	if r.Correction.IsCorrection {
		r.CorrectionTarget = r.Correction.Target
	}

	// Resolve ownership once at the semantic boundary. The result carries
	// graded confidence/evidence so downstream Situation/language consumers
	// never have to re-infer ownership from raw text.
	ownershipResolution := ownership.Resolve(semantic)
	r.Subject = UserObservationSubject(ownershipResolution.Subject)
	r.SubjectConfidence = ownershipResolution.Confidence
	r.SubjectEvidence = append([]ownership.Evidence(nil), ownershipResolution.Evidence...)

	r.References = resolveReferences(semantic, ctx)
	r.Continues = r.Continues || isContinuation(semantic)
	r.FollowUp = r.FollowUp || hasContextualReference(r.References)

	// A live objective is strong conversational context. A follow-up such as
	// "quiero que la funcion acepte negativos" must stay attached to the
	// current work even though it does not repeat "proyecto" or "programa".
	if ctx.ActiveObjective != "" && isLikelyWorkContinuation(semantic, r.Kind) {
		r.AddressesCurrent = true
		r.Continues = true
		r.FollowUp = true
		if r.Kind == KindConversation || r.Kind == KindUnknown {
			r.Kind = KindContinuation
			r.Confidence = 0.90
		}
	}

	if hasContextualReference(r.References) || r.Continues || r.FollowUp {
		r.AddressesCurrent = true
		if ctx.ActiveObjective != "" {
			r.Objective.Text = ctx.ActiveObjective
		}
	}

	r.Entities = extractEntities(semantic)
	r.RequestedAction = requestedActionFor(r.Kind, semantic)
	if r.Frame.PrimaryOperation != "" {
		r.RequestedAction = r.Frame.PrimaryOperation
	}
	r.ExpectedResponse = expectedResponseFor(r.Kind)
	r.Urgency = detectUrgency(semantic)
	// Build after references, correction, continuation and user signals are known.
	r.Meaning = BuildSemanticMeaning(r)
	r.Meaning.Query = classifySemanticQuery(semantic)
	r.Meaning.Attribute = inferEnvironmentAttribute(semantic, r.Meaning.Query)
	if r.Topic == "" {
		r.Topic = inferQuerySubject(semantic, r.Meaning.Query)
	}
	r.Temporal = InferTemporalReference(semantic)
	r.CommunicativeActs = ClassifyCommunicativeActs(r)

	if r.Kind == KindContinuation {
		r.Confidence = 0.94
	}
	return r
}

// Reanalyze replaces the current in-flight semantic turn after a
// clarification/correction changes its semantic surface. It deliberately
// does not append a second history entry: one human turn must remain one
// interaction record even when its internal representation is refined.
func (m *Manager) Reanalyze(raw string, ctx Context) SemanticRequest {
	if m == nil {
		return Analyze(raw, ctx)
	}
	r := Analyze(raw, ctx)
	if len(m.history) > 0 {
		m.history[len(m.history)-1] = r
	} else {
		m.history = append(m.history, r)
	}
	m.last, m.ctx = r, ctx
	if r.Language != "" {
		m.ctx.Language = r.Language
	}
	return r
}

func (m *Manager) Last() SemanticRequest {
	if m == nil {
		return SemanticRequest{}
	}
	return m.last
}

func (m *Manager) Context() Context {
	if m == nil {
		return Context{}
	}
	return m.ctx
}

func (m *Manager) History() []SemanticRequest {
	if m == nil {
		return nil
	}
	out := make([]SemanticRequest, len(m.history))
	copy(out, m.history)
	return out
}

func Parse(raw string, channel InputChannel) SemanticRequest {
	n := normalize(raw)
	semantic := normalizeForClassification(n)
	r := SemanticRequest{
		Raw:        raw,
		Normalized: n,
		Channel:    channel,
		Objective:  Objective{Text: strings.TrimSpace(raw)},
	}
	if n == "" {
		return r
	}

	// Follow-ups are detected from the semantic surface, not from quoted
	// artifact content. This keeps references such as "the test" meaningful
	// while preventing literal examples from hijacking the turn.
	r.FollowUp = isFollowUp(semantic)

	// Precedence is semantic evidence, not implementation convenience:
	// explicit control > strong production > correction/action/query >
	// conversation. A weak token such as "hola" must never outrank a clear
	// objective such as "crea un programa".
	switch {
	case isStop(semantic):
		r.Kind = KindStop
	case isPause(semantic):
		r.Kind = KindPause
	case isProduction(semantic):
		r.Kind = KindProduction
	case isCorrection(semantic):
		r.Kind = KindCorrection
	case isLearning(semantic):
		r.Kind = KindLearning
	case isFeedback(semantic):
		r.Kind = KindFeedback
	case isProposal(semantic):
		r.Kind = KindProposal
	case isContinuation(semantic):
		r.Kind = KindContinuation
	case isSearch(semantic):
		r.Kind = KindSearch
	case isReasoning(semantic):
		r.Kind = KindReasoning
	case isAction(semantic):
		r.Kind = KindAction
	case looksInformative(semantic):
		r.Kind = KindInform
	case isClarification(semantic):
		r.Kind = KindClarification
	case isQuestion(semantic):
		r.Kind = KindQuestion
	case isGreeting(semantic) || isSocial(semantic):
		r.Kind = KindConversation
	default:
		r.Kind = KindConversation
	}

	r.Objective.OpenEnded = r.Kind == KindProduction && !containsAny(semantic, "solo ", "unicamente ")
	// A production objective may contain executable words such as "ejecuta",
	// "abre" or "corre" inside its requirements/conditions. Those words are
	// not top-level computer actions. Let the production lane own the whole
	// objective and only infer low-level actions for non-production turns.
	r.Action = inferActionForKind(r.Kind, semantic)
	return r
}

// ShouldAppendObjectiveDraft reports whether a new turn is plausibly part of
// an unfinished delegated objective. It is a conversational assembly rule,
// not a capability or domain rule. Questions, searches and explicit control
// turns remain independent so an unfinished draft cannot absorb unrelated talk.
func ShouldAppendObjectiveDraft(r SemanticRequest) bool {
	if r.Kind == KindStop || r.Kind == KindPause || r.Kind == KindQuestion || r.Kind == KindSearch {
		return false
	}
	if r.Kind == KindConversation {
		n := semanticSurface(r.Normalized)
		if strings.HasSuffix(strings.TrimSpace(r.Raw), ":") || objectiveSpecificationMarker(n) {
			return true
		}
		return !r.CommunicativeActs.Has(ActSocial) && !isGreeting(n)
	}
	return r.Kind == KindProduction || r.Kind == KindAction || r.Kind == KindContinuation ||
		r.Kind == KindCorrection || r.Kind == KindFeedback || r.Kind == KindProposal ||
		r.CommunicativeActs.Has(ActCommand) || r.CommunicativeActs.Has(ActContinuation)
}

// NeedsObjectiveAssembly reports whether a production request is clearly a
// delegation preamble rather than a sufficiently specified executable turn.
// It is intentionally channel- and domain-neutral: the signal is grammatical
// and structural, not tied to Programming, Blender, Unity or any provider.
func NeedsObjectiveAssembly(r SemanticRequest) bool {
	if r.Kind != KindProduction || !r.Objective.OpenEnded {
		return false
	}
	n := semanticSurface(r.Normalized)
	if n == "" || objectiveSpecificationMarker(n) || strings.Contains(n, "\n") {
		return false
	}
	for _, prefix := range []string{
		"quiero que ", "necesito que ", "me gustaria que ", "me gustaría que ",
		"i want you to ", "i need you to ",
	} {
		if strings.HasPrefix(n, prefix) {
			body := strings.TrimSpace(strings.TrimPrefix(n, prefix))
			// Hold only a genuinely open delegation preamble. A complete
			// one-sentence objective such as "Quiero que implementes una función
			// que..." must still reach the normal production lane. The
			// structural subordinate-clause test keeps this domain-neutral.
			if strings.Contains(n, ":") || countSentenceLikeBoundaries(n) > 1 {
				return false
			}
			for _, marker := range []string{" para ", " con ", " que ", " using ", " with ", " that "} {
				if strings.Contains(body, marker) {
					return false
				}
			}
			return true
		}
	}
	return false
}

// ShouldFinalizeObjectiveDraft reports whether a fragmented conversational
// objective contains an explicit user handoff. It is deliberately generic:
// it does not inspect Programming, Blender, Unity, or any provider action.
// Voice aggregation is expected to deliver a complete intervention as one
// turn; this guard is the safety boundary when a recognizer still fragments
// one intervention into several transport turns.
func ShouldFinalizeObjectiveDraft(r SemanticRequest) bool {
	n := semanticSurface(r.Normalized)
	if n == "" {
		return false
	}
	for _, marker := range []string{
		"hazlo", "hazlo ya", "adelante", "procede", "puedes hacerlo",
		"puede hacerlo", "ejecutalo", "ejecútalo", "ejecuta ahora",
		"continua", "continúa", "continue", "go ahead", "do it",
	} {
		if strings.Contains(n, marker) {
			return true
		}
	}
	return false
}

func countSentenceLikeBoundaries(n string) int {
	count := 0
	for _, r := range n {
		if r == '.' || r == '!' || r == '?' {
			count++
		}
	}
	return count
}

func inferActionForKind(kind RequestKind, semantic string) ActionIntent {
	if kind == KindProduction {
		return ActionIntent{}
	}
	return inferAction(semantic)
}

func isLikelyWorkContinuation(n string, kind RequestKind) bool {
	// A live work objective is context, not permission to classify ordinary
	// discourse as work continuation. Generic connectors such as "tambien"
	// and "ademas" are deliberately excluded: they are ubiquitous in normal
	// conversation and were a source of false continuation signals.
	if kind == KindQuestion || kind == KindSearch || kind == KindReasoning ||
		kind == KindStop || kind == KindPause {
		return false
	}

	workAnchors := []string{
		"la funcion", "la funcion", "el codigo", "el codigo", "el proyecto",
		"ese proyecto", "este proyecto", "esa funcion", "esa funcion",
		"el test", "la prueba", "ese archivo", "the function", "the project",
		"that project", "the test", "that file",
	}
	continuationSignals := []string{
		"quiero que", "haz que", "hacer que", "modifica eso", "mejorar eso",
		"cambialo", "cambialo", "mejoralo", "mejorarlo", "cambia", "modifica",
		"mejora", "mejorar", "corrige", "corrigir", "implementa", "implementar",
		"add", "make it", "change it", "modify it",
	}

	// Explicit continuation commands are already represented by isContinuation
	// and do not need another classifier here.
	if isContinuation(n) || isCurrentWorkQuestion(n) {
		return false
	}
	return containsAnyPhrase(n, workAnchors) && containsAnyPhrase(n, continuationSignals)
}

func requestedActionFor(k RequestKind, n string) string {
	switch k {
	case KindProduction:
		// Production is governed by the first/primary operation. Recovery verbs
		// such as "corrígelo si falla" belong to the recovery policy and must
		// never replace a primary "create/implement" operation.
		return primaryProductionOperation(n)
	case KindAction:
		return "execute"
	case KindQuestion, KindReasoning:
		return "answer"
	case KindSearch:
		return "search"
	case KindContinuation:
		if containsAny(n, "modifica", "modificar", "cambia", "cambiar", "corrige", "corregir",
			"fix ", "modify ", "change ", "mejora", "mejorar", "add ", "agrega", "anade", "anade",
			"quiero que ", "haz que ", "hacer que ", "acepte ", "aceptar ", "tambien ", "tambien ") {
			return "modify"
		}
		return "continue"
	case KindCorrection:
		return "correct"
	case KindProposal:
		return "propose"
	case KindStop, KindPause:
		return "stop"
	default:
		return ""
	}
}

func primaryProductionOperation(n string) string {
	n = strings.TrimSpace(strings.ToLower(n))
	for _, prefix := range []string{
		"quiero que ", "necesito que ", "me gustaria que ", "me gustaría que ",
		"i want you to ", "i need you to ",
	} {
		if strings.HasPrefix(n, prefix) {
			n = strings.TrimSpace(strings.TrimPrefix(n, prefix))
			break
		}
	}
	first := n
	for _, sep := range []string{"\n", ":", ".", " si ", " cuando ", " en caso de "} {
		if i := strings.Index(first, sep); i >= 0 && i < len(first) {
			first = strings.TrimSpace(first[:i])
		}
	}
	if containsAny(first, "modifica", "modificar", "corrige", "corregir", "arregla", "arreglar", "fix ", "modify ", "change ") {
		return "modify"
	}
	if containsAny(first, "crea", "crear", "implementa", "implementar", "construye", "construir", "genera", "generar", "desarrolla", "desarrollar", "create ", "implement ", "build ", "generate ") {
		return "create"
	}
	return "create"
}

// BuildObjectiveFrame derives generic objective structure from the semantic
// request. It deliberately recognizes roles, not domains: tests/verification,
// workspace and recovery clauses remain attached to the primary goal.
func BuildObjectiveFrame(r SemanticRequest) ObjectiveFrame {
	n := strings.TrimSpace(strings.ToLower(r.Raw))
	frame := ObjectiveFrame{
		PrimaryGoal:      r.Objective.Text,
		PrimaryOperation: requestedActionFor(r.Kind, n),
		Confidence:       r.Confidence,
		Provenance:       "interaction.semantic_objective_frame",
	}
	if r.Kind != KindProduction {
		return frame
	}
	if containsAny(n, "workspace", "proyecto actual", "current workspace", "current project") {
		frame.Workspace = "current_workspace"
	}
	if containsAny(n, "ejecuta los tests", "ejecuta los test", "corre los tests", "run the tests", "run tests", "añade tests", "agrega tests", "add tests") {
		frame.RequiredActions = append(frame.RequiredActions, "test")
	}
	if containsAny(n, "verifica", "verificar", "comprueba", "comprueba que", "funciona realmente", "verify", "check that") {
		frame.Verification = append(frame.Verification, "behavioral_result")
	}
	if containsAny(n, "si encuentras un problema", "si falla", "en caso de error", "if it fails", "if there is an error") &&
		containsAny(n, "corrige", "corrigelo", "corrígelo", "corregir", "arregla", "reintenta", "retry", "fix ") {
		frame.Recovery = append(frame.Recovery, "repair_and_retry")
	}
	return frame
}

func expectedResponseFor(k RequestKind) string {
	switch k {
	case KindQuestion, KindReasoning:
		return "answer"
	case KindProduction, KindAction:
		return "result"
	case KindCorrection, KindPause, KindStop:
		return "acknowledgement"
	case KindProposal:
		return "discussion"
	case KindContinuation:
		return "continuation_or_result"
	case KindConversation:
		return "conversation"
	default:
		return ""
	}
}

func detectUrgency(n string) string {
	if containsAny(n, "urgente", "urgentemente", "ya mismo", "ahora mismo", "asap", "urgent", "right now") {
		return "high"
	}
	if containsAny(n, "cuando puedas", "sin prisa", "when you can", "no rush") {
		return "low"
	}
	return ""
}

func extractEntities(n string) []string {
	var out []string
	for _, marker := range []string{
		"proyecto", "archivo", "test", "prueba", "funcion", "funcion",
		"escena", "modelo", "imagen", "video", "audio", "programa",
		"project", "file", "test", "scene", "model", "image", "video", "audio",
		"program", "function",
	} {
		if strings.Contains(n, marker) {
			out = append(out, marker)
		}
	}
	return out
}

func isSearch(n string) bool {
	return containsAny(n,
		"busca ", "buscar ", "buscame ", "buscame ",
		"en internet", "en la web", "en linea", "en linea",
		"search for ", "look up ", "find online", "on the web")
}

func isReasoning(n string) bool {
	return looksLikeArithmeticExpression(n) ||
		strings.Contains(n, " = ") ||
		containsAny(n,
			"cuanto es", "cuanto es", "calcula ", "calcular ",
			"resuelve ", "resolver ", "demuestra ", "demostrar ",
			"razona ", "razonar ", "reason ", "reasoning ",
			"por que ", "por que ", "why ", "how come ",
			"explica por que", "explica por que", "explain why")
}

func looksLikeArithmeticExpression(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" || !strings.ContainsAny(s, "+-*/") {
		return false
	}
	hasDigit := false
	for _, r := range s {
		if r >= '0' && r <= '9' {
			hasDigit = true
			continue
		}
		if strings.ContainsRune("+-*/(). ", r) {
			continue
		}
		return false
	}
	return hasDigit
}

func isLearning(s string) bool {
	// Learning/memory requests require explicit memory-directed language.
	// Generic advice markers ("deberias", "tienes que") and the fragment
	// "esto es" are intentionally not sufficient: they occur constantly in
	// ordinary explanations and commands.
	patterns := []string{
		"aprende", "recuerda", "guarda", "memoriza", "ten en cuenta",
		"cuando diga", "si digo", "esto significa", "a partir de ahora",
		"en el futuro", "no olvides", "acuerdate", "acuerdate",
	}
	return containsAnySemantic(s, patterns)
}

func isFeedback(s string) bool {
	// Feedback must contain an evaluative phrase or an explicit progress/
	// deficiency construction. Never treat a generic word such as "bien" or
	// "mal" as feedback by itself: e.g. "no redacte bien" is not evaluation
	// of Ahmelia. The old substring rule produced exactly that false positive.
	patterns := []string{
		"bien hecho", "buen trabajo", "muy bien", "excelente", "genial",
		"perfecto", "incorrecto", "correcto", "gracias", "me encanta",
		"me gusta", "me alegra", "eso esta bien", "eso esta bien",
		"eso esta mal", "eso esta mal", "no era eso", "exacto", "equivocado",
		"fallaste", "acertaste", "has mejorado", "ha mejorado", "mejoraste",
		"mejorado ultimamente", "mejorado ultimamente", "aun falta", "aun falta",
		"todavia falta", "todavia falta", "cosas por mejorar",
		"cosas por mejorar aun", "cosas por mejorar aun", "te falta por mejorar",
		"falta por mejorar", "hay cosas que mejorar", "hay cosas por mejorar",
	}
	if containsAnySemantic(s, patterns) {
		return true
	}
	return hasProgressFeedbackStructure(s)
}

func hasProgressFeedbackStructure(s string) bool {
	n := semanticSurface(s)
	progress := containsAnyPhrase(n, []string{
		"mejorando", "mejorado", "mejoras", "mejorar", "progreso", "avanzando", "avanza",
	})
	deficit := containsAnyPhrase(n, []string{
		"falta", "faltan", "queda", "quedan", "todavia", "todavia", "aun", "aun",
	})
	return progress && deficit
}

func containsAnySemantic(s string, patterns []string) bool {
	s = semanticSurface(s)
	for _, p := range patterns {
		if phrasePresent(s, semanticSurface(p)) {
			return true
		}
	}
	return false
}

// containsAnyPhrase applies token-boundary matching to normalized semantic
// phrases. It is intentionally stricter than strings.Contains so common words
// cannot become semantic acts merely because they occur somewhere in a turn.
func containsAnyPhrase(s string, patterns []string) bool {
	s = semanticSurface(s)
	for _, p := range patterns {
		if phrasePresent(s, semanticSurface(p)) {
			return true
		}
	}
	return false
}

func semanticSurface(s string) string {
	// One interaction-wide normalization contract: accents and punctuation are
	// normalized here before lexical/semantic phrase matching.
	return strings.Join(strings.Fields(shared.Normalize(s)), " ")
}

func isQuestion(n string) bool {
	if strings.HasSuffix(strings.TrimSpace(n), "?") {
		return true
	}
	// Do not use the bare token "como" as a question trigger. In Spanish it
	// is also a normal comparative/conjunctive word ("hazlo como te dije").
	// Unaccented variants remain supported only when followed by an explicitly
	// interrogative construction.
	return containsAnyPhrase(n, []string{
		"por que", "por que",
		"queria saber si", "queria saber si", "quiero saber si", "quisiera saber si",
		"where is", "what is", "how do", "what does", "who is", "who created",
		"how can", "what can", "what do you", "who can", "when does", "where can",
		"donde", "donde", "que es", "que es", "que significa", "que significa",
		"quien", "quien", "que sabes", "que sabes", "que puedes", "que puedes",
		"que haces", "que haces", "como estas", "como estas", "que tal", "que tal", "como funciona", "como funciona", "como se",
		"como puedo", "como puedes", "como hago", "como hacer", "como crear",
		"how does", "how can", "what can", "what do you",
	})
}

func isAction(n string) bool {
	// inferAction is the single executable-action vocabulary. Keeping a
	// second hand-written verb list here caused a real semantic gap:
	// interaction/action_intent.go already understood "cambia a chrome",
	// but Parse() classified it as ordinary conversation because this
	// predicate did not know the window action vocabulary. Reuse the same
	// authority so every executable interaction crosses the same boundary.
	return inferAction(n).Name != ""
}

func isProduction(n string) bool {
	// Callers cross both the normalized interaction boundary and direct helper
	// boundaries. Reuse the classification normalizer here so accents do not
	// change production intent while punctuation that carries grammar (such as
	// "Nuevo objetivo:") remains available to the directive-frame parser.
	n = normalizeForClassification(n)
	// A production noun and an action verb can appear in a report, desire, or
	// description of a user's own effort. They become an executable production
	// intent only when the utterance itself has a top-level directive/delegation
	// frame. This keeps subordinate phrases such as "estoy intentando terminar
	// el formulario" on the conversational path.
	if !hasProductionRequestFrame(n) {
		return false
	}
	verb := containsWholeProductionTerm(n,
		"crea", "crear", "construye", "construir",
		"genera", "generar", "disena", "disenar", "desarrolla", "desarrollar",
		"implementa", "implementar", "implementes", "implemente",
		"modifica", "modificar", "modifiques", "modifique",
		"prepara", "preparar", "prepares", "prepare",
		"corrige", "corregir", "corrijas", "corrija",
		"termina", "terminar", "haz", "hacer",
		"avanza", "avanzar", "avances", "trabaja", "trabajar",
		"empieza", "empezar", "comienza", "comenzar", "entregame", "entregar",
		"create", "build", "generate", "develop", "implement",
		"modify", "prepare", "fix", "make")
	object := containsWholeProductionTerm(n,
		"archivo", "archivos", "codigo", "programa", "proyecto",
		"funcion", "funciones", "metodo", "metodos", "utilidad", "herramienta", "script",
		"test", "tests", "bloque", "escena", "modelo", "capacidad", "imagen", "video",
		"audio", "software", "file", "files", "code", "program",
		"project", "scene", "model", "image")
	return verb && object
}

func hasProductionRequestFrame(n string) bool {
	n = strings.TrimSpace(n)
	if hasProductionDirectivePrefix(n) {
		return true
	}
	// A request may open with analysis and then contain a separate imperative
	// sentence that asks for an artifact (for example, "Analyze the gap. Build
	// a small component and deliver it"). Recognize directive sentence frames
	// without promoting embedded requirements such as "the script creates a
	// file" or reported user activity such as "I am creating a file".
	for _, clause := range strings.FieldsFunc(n, func(r rune) bool {
		return r == '.' || r == '!' || r == '?' || r == ';' || r == ':'
	}) {
		if hasProductionDirectivePrefix(strings.TrimSpace(clause)) {
			return true
		}
	}
	return false
}

func hasProductionDirectivePrefix(n string) bool {
	for _, prefix := range []string{"hola ahmelia, ", "hola ahmelia ", "ahmelia, ", "ahmelia ", "amelia, ", "amelia "} {
		n = strings.TrimPrefix(n, prefix)
	}
	for _, prefix := range []string{"nuevo objetivo: ", "objetivo: ", "new objective: ", "objective: "} {
		n = strings.TrimPrefix(n, prefix)
	}
	// Direct imperatives and terse task objectives remain valid production
	// requests, including the established "crear una herramienta" form.
	for _, prefix := range []string{
		"crea ", "crear ", "construye ", "construir ", "genera ", "generar ",
		"disena ", "disenar ", "desarrolla ", "desarrollar ", "implementa ", "implementar ",
		"modifica ", "modificar ", "prepara ", "preparar ", "corrige ", "corregir ",
		"termina ", "terminar ", "haz ", "hacer ", "avanza ", "avanzar ",
		"trabaja ", "trabajar ", "empieza ", "empezar ", "comienza ", "comenzar ",
		"create ", "build ", "generate ", "develop ", "implement ",
		"modify ", "prepare ", "fix ", "make ",
		"quiero crear ", "quiero construir ", "quiero generar ", "quiero desarrollar ",
		"quiero disenar ", "quiero implementar ", "quiero modificar ", "quiero preparar ", "quiero corregir ",
		"quiero terminar ", "quiero hacer ", "quiero avanzar ", "quiero trabajar ",
		"quiero empezar ", "quiero comenzar ",
		"i want to create ", "i want to build ", "i want to generate ", "i want to develop ",
		"i want to implement ", "i want to modify ", "i want to prepare ", "i want to make ",
	} {
		if strings.HasPrefix(n, prefix) {
			return true
		}
	}
	for _, prefix := range []string{"quiero que ", "necesito que "} {
		if !strings.HasPrefix(n, prefix) {
			continue
		}
		body := strings.TrimPrefix(n, prefix)
		if containsWholeProductionTerm(body,
			"implementes", "crees", "construyas", "generes", "desarrolles",
			"modifiques", "prepares", "corrijas", "hagas", "termines", "avances", "trabajes", "empieces", "comiences",
			"implement", "create", "build", "generate", "develop", "modify", "prepare", "fix", "make") {
			return true
		}
	}
	return false
}

func containsWholeProductionTerm(n string, terms ...string) bool {
	words := strings.FieldsFunc(n, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) })
	for _, word := range words {
		for _, term := range terms {
			if word == term {
				return true
			}
		}
	}
	return false
}

func isClarification(n string) bool {
	// "cual/cual" by itself is ordinary question language ("¿cual
	// prefieres?"). It must never demote a real question into the
	// clarification state. Clarification is reserved for explicit
	// meta-communication about not understanding or asking Ahmelia to
	// disambiguate something already discussed.
	return containsAny(n,
		"que quieres decir", "que quieres decir", "what do you mean",
		"no entendi", "no entendi", "i don't understand",
		"no entiendo", "i don't get it", "which one de esas",
		"cual de las dos", "cual de las dos", "a cual te refieres",
		"a cual te refieres", "a que te refieres", "a que te refieres")
}

func isCorrection(n string) bool {
	return containsAny(n, "no, ", "no es ", "eso no", "me referia", "me referia",
		"corrijo", "corrige ", "correction:", "i meant", "that's not")
}

func isProposal(n string) bool {
	return containsAny(n, "te propongo", "propongo ", "podriamos", "podriamos", "deberiamos", "deberiamos",
		"creo que deberiamos", "creo que deberiamos", "we should", "we could", "i suggest", "suggestion:")
}

func isStop(n string) bool {
	n = strings.TrimSpace(strings.Trim(n, " .!?¿¡"))
	if n == "olvidalo" || n == "cancela" || n == "cancelar" ||
		n == "cancel" || n == "stop" || n == "detente" || n == "detener" ||
		n == "salir" || n == "exit" || n == "quit" || n == "goodbye" || n == "adios" {
		return true
	}
	for _, prefix := range []string{"salir, ", "salir ", "exit, ", "stop, ", "detente, ", "cancela, "} {
		if strings.HasPrefix(n, prefix) {
			return true
		}
	}
	return false
}

func isPause(n string) bool {
	n = strings.Trim(n, " .!?¿¡")
	return n == "espera" || n == "espera un momento" ||
		n == "wait" || n == "hold on" || n == "don't do anything" ||
		n == "no hagas nada" || n == "no hagas nada todavia" ||
		n == "no hagas nada aun" ||
		strings.HasPrefix(n, "espera, ") ||
		strings.HasPrefix(n, "wait, ") ||
		strings.HasPrefix(n, "no hagas nada ") ||
		strings.HasPrefix(n, "no abras ") ||
		strings.HasPrefix(n, "no ejecutes ") ||
		strings.HasPrefix(n, "no abras ningun ") ||
		strings.HasPrefix(n, "no quiero que abras ")
}

func looksInformative(n string) bool {
	return containsAny(n, "te informo", "para que sepas", "just so you know",
		"por cierto", "by the way")
}

func speechActFor(k RequestKind, n string) SpeechAct {
	switch k {
	case KindQuestion, KindReasoning:
		return SpeechQuestion
	case KindAction, KindProduction:
		return SpeechCommand
	case KindInform:
		return SpeechInform
	case KindProposal:
		return SpeechPropose
	case KindCorrection:
		return SpeechCorrect
	case KindStop:
		return SpeechStop
	case KindPause:
		return SpeechPause
	case KindContinuation:
		return SpeechContinue
	case KindConversation:
		if isSocial(n) || isGreeting(n) {
			return SpeechSocial
		}
		return SpeechInform
	default:
		return SpeechUnknown
	}
}

func inferQuestionType(n string) string {
	if !isQuestion(n) && !isReasoning(n) {
		return ""
	}
	switch {
	case containsAny(n, "cuanto", "cuanto", "how much", "how many"):
		return "quantity"
	case containsAny(n, "quien", "quien", "who"):
		return "person"
	case containsAny(n, "donde", "donde", "where"):
		return "location"
	case containsAny(n, "cuando", "cuando", "when"):
		return "time"
	case containsAny(n, "por que", "por que", "why"):
		return "explanation"
	case containsAny(n, "como", "como", "how"):
		return "procedure_or_explanation"
	default:
		return "general"
	}
}

func inferDirectness(n string) string {
	n = strings.TrimSpace(n)
	if n == "" {
		return ""
	}
	if containsAny(n, "podrias", "podrias", "puedes", "puedo", "podemos", "could you", "can you", "would you") {
		return "polite_request"
	}
	if isAction(n) || isProduction(n) {
		return "direct_command"
	}
	if isQuestion(n) {
		return "question"
	}
	return "statement"
}

func inferUserStateSignals(n string) UserStateSignals {
	var u UserStateSignals
	if containsAny(n, "estoy cansado", "estoy cansada", "cansado", "cansada", "agotado", "agotada", "llevo horas", "i'm tired", "i am tired", "exhausted") {
		u.Fatigue = 0.9
	}
	if containsAny(n, "no funciona", "otra vez", "fallaste", "me desespera", "frustrado", "frustrada", "this doesn't work", "again", "frustrated") {
		u.Frustration = 0.75
	}
	if containsAny(n, "jajaja", "haha", "xd", "me encanta", "genial", "perfecto", "awesome", "love it") {
		u.Positive = 0.7
	}
	if detectUrgency(n) == "high" {
		u.Urgency = 0.9
	}
	u.Confidence = 0.5
	if u.Fatigue > 0 || u.Frustration > 0 || u.Positive > 0 || u.Urgency > 0 {
		u.Confidence = 0.85
	}
	return u
}

func inferConversationTopic(n string) string {
	n = strings.TrimSpace(n)
	if n == "" {
		return ""
	}
	for _, marker := range []string{"sobre ", "about ", "con ", "with ", "en ", "for "} {
		if idx := strings.LastIndex(n, marker); idx >= 0 && idx+len(marker) < len(n) {
			rest := strings.Trim(strings.TrimSpace(n[idx+len(marker):]), " .!?¿¡")
			if rest != "" && len(strings.Fields(rest)) <= 8 {
				return rest
			}
		}
	}
	for _, entity := range extractEntities(n) {
		return entity
	}
	return ""
}

func inferEnvironmentAttribute(n string, q SemanticQueryType) string {
	if q == QueryEnvironmentResources {
		for attr, markers := range map[string][]string{
			"cpu": {"cpu"}, "ram": {"ram"}, "gpu": {"gpu"}, "memory": {"memoria"}, "battery": {"bateria"},
		} {
			for _, m := range markers {
				if phrasePresent(n, m) {
					return attr
				}
			}
		}
		return ""
	}
	switch q {
	case QueryEnvironmentTabs:
		return "tabs"
	case QueryEnvironmentContext:
		// Context is a typed environment query, but it is not one of the
		// nine concrete environment attributes; keep Attribute empty rather
		// than inventing a parallel attribute vocabulary.
		return ""
	case QueryEnvironmentActiveWork:
		if phrasePresent(n, "en que proyecto") {
			return "active_project"
		}
		return "active_file"
	case QueryEnvironmentOpenApps:
		return "open_apps"
	}
	return ""
}

func inferQuerySubject(n string, q SemanticQueryType) string {
	triggers := semanticQuerySpecs[q]
	best := ""
	for _, trigger := range triggers {
		if semanticPhrasePresent(n, trigger) && len(trigger) > len(best) {
			best = trigger
		}
	}
	if best == "" {
		return ""
	}
	idx := strings.Index(n, best)
	if idx < 0 {
		return ""
	}
	rest := strings.TrimSpace(n[idx+len(best):])
	rest = strings.Trim(rest, " .!?¿¡:;")
	if rest == "" || len(strings.Fields(rest)) > 12 {
		return ""
	}
	return rest
}

func confidence(k RequestKind) float64 {
	switch k {
	case KindConversation:
		return 0.92
	case KindSearch:
		return 0.96
	case KindReasoning:
		return 0.92
	case KindAction:
		return 0.86
	case KindProduction:
		return 0.90
	case KindContinuation:
		return 0.94
	case KindQuestion:
		return 0.82
	case KindCorrection, KindClarification, KindProposal, KindPause:
		return 0.84
	default:
		return 0.30
	}
}

func DetectLanguage(n, previous string) string {
	// Evidence is weighted by phrases rather than individual words, so a
	// proper noun such as "Unity" cannot flip the conversation language.
	es := scoreLanguage(n, []string{
		" que ", " que ", "donde", "donde", "como", "como", "quiero",
		"puedes", "puedo", "esta", "esta", "crear", "crea", "haz ",
		"hacer", "por que", "porque", "que ", "el ", " la ", " un ", " una ",
	})
	en := scoreLanguage(n, []string{
		" what ", " where ", " how ", " why ", " when ", " can you ",
		" i want ", " create ", " make ", " do ", " is ", " the ", " a ",
		"project", "created", "you just", "continue", "with that",
	})
	// Strong imperative markers are sufficient to detect a language even
	// when the utterance is short ("Open Unity"). Proper nouns are never
	// counted as language evidence.
	if containsAny(n, "open ", "continue ", "resume ", "where is ", "what is ", "what were ",
		"i want you to ", "i want to ", "i haven't ", "i have not ") {
		return "en"
	}
	if containsAny(n, "abre ", "abrir ", "continua ", "continua ", "donde esta ", "donde esta ", "que es ", "que es ") {
		return "es"
	}
	switch {
	case es >= 2 && es > en:
		return "es"
	case en >= 2 && en > es:
		return "en"
	case previous != "":
		return previous
	default:
		return ""
	}
}

func scoreLanguage(s string, terms []string) int {
	padded := " " + s + " "
	score := 0
	for _, t := range terms {
		if strings.Contains(padded, t) {
			score++
		}
	}
	return score
}

func resolveReferences(n string, ctx Context) []Reference {
	var refs []Reference
	add := func(k ReferenceKind, expr, value string) {
		role := ReferenceRoleTarget
		if k == ReferenceRecentArtifact && (containsAny(n, "test", "tests", "prueba", "pruebas")) {
			role = ReferenceRoleRequirement
		}
		if k == ReferenceCurrentInput && containsAny(n, "corrígelo", "corrigelo", "corrige lo", "fix it", "if it fails", "si falla") {
			role = ReferenceRoleRecoveryTarget
		}
		refs = append(refs, Reference{Kind: k, Expression: expr, Resolved: value != "", Value: value, Role: role, Confidence: 0.90, Provenance: "semantic_surface"})
	}
	// Discourse-closure phrases are not anaphora. In particular, "eso es todo"
	// means the speaker is closing the clarification, not referring back to the
	// current topic. Treating it as a reference silently resurrects stale context
	// (for example the previous "color favorito") and makes later turns appear
	// to be about a topic the user has explicitly left.
	if isContextClosure(n) {
		return refs
	}
	switch {
	case containsAny(n, "lo que acabas de", "lo ultimo que", "lo ultimo que", "lo que hiciste", "lo que creaste", "el ultimo", "el ultimo", "what you just created", "what you created"):
		value := ctx.ActiveWorkID
		if value == "" {
			value = ctx.LastArtifactPath
		}
		add(ReferenceRecentWork, "recent work", value)
	case containsAny(n, "que archivos", "que archivos", "what files", "files you created"):
		add(ReferenceRecentArtifact, "recent artifacts", ctx.ActiveWorkID)
	case containsAny(n, "el test", "la prueba", "los tests", "the test", "the tests"):
		value := ctx.ActiveWorkID
		if ctx.LastArtifactName != "" {
			value = ctx.LastArtifactName
		}
		add(ReferenceRecentArtifact, "test artifact", value)
	case containsAny(n, "resultado", "result", "que paso", "que paso", "what happened", "resultado tuvo"):
		add(ReferenceCurrentInput, "recent result", ctx.LastResult)
	case containsAny(n, "eso", "ese archivo", "ese proyecto", "hazlo", "do it", "that"):
		add(ReferenceCurrentInput, "current referent", ctx.ActiveObjective)
	case containsAny(n, "donde", "donde", "where"):
		if ctx.LastArtifactPath != "" {
			add(ReferenceLocation, "location", ctx.LastArtifactPath)
		} else {
			add(ReferenceLocation, "location", "")
		}
	case containsAny(n, "aqui", "aqui", "ahi", "ahi", "alli", "alli", "here", "there"):
		add(ReferenceLocation, "location", ctx.LastArtifactPath)
	}
	if isContinuation(n) || isCurrentWorkQuestion(n) {
		value := ctx.ActiveWorkID
		if value == "" {
			value = ctx.ActiveObjective
		}
		add(ReferenceRecentWork, "current work", value)
	}
	return refs
}

func isContextClosure(n string) bool {
	n = strings.TrimSpace(n)
	return containsAny(n,
		"eso es todo", "eso era todo", "solo eso", "solo eso",
		"eso nomas", "eso nomas", "eso y ya", "y eso es todo",
		"that's all", "that is all", "that's it", "that is it",
		"solo estaba comprobando", "solo estaba comprobando",
		"no estoy comprobando", "no estaba comprobando",
	)
}

func hasContextualReference(refs []Reference) bool {
	for _, r := range refs {
		if r.Kind == ReferenceNone {
			continue
		}
		// A requirement or recovery clause is part of the current objective,
		// not evidence that the speaker is referring to a previous turn/work.
		if r.Role == ReferenceRoleRequirement || r.Role == ReferenceRoleRecoveryTarget {
			continue
		}
		return true
	}
	return false
}

func isContinuation(n string) bool {
	return containsAny(n,
		"hazlo", "haz eso", "continua", "continua", "sigue", "siguiente",
		"retoma", "retomemos", "continua con", "sigue con",
		"continue", "continue with", "keep going", "resume", "carry on")
}

func isCurrentWorkQuestion(n string) bool {
	return containsAny(n,
		"que estabamos haciendo", "que estabamos haciendo",
		"que haciamos", "que haciamos", "que estabamos trabajando",
		"que estabamos trabajando", "que falta", "que falta",
		"que queda", "que queda", "que resultado tuvo", "que resultado tuvo",
		"what were we doing", "what were we working on", "what is missing",
		"what happened", "how did it go")
}

func isFollowUp(n string) bool {
	return containsAny(n,
		"donde esta", "donde esta", "donde fue", "que archivos creaste",
		"que resultado", "que resultado", "que paso", "que paso",
		"y el test", "y la prueba", "the test", "the tests",
		"que estabamos haciendo", "que estabamos haciendo", "what were we doing",
		"que haciamos", "que haciamos", "what were we working on",
		"que archivos creaste", "que creaste", "como vas", "como vas",
		"que falta", "que falta", "que queda", "que queda",
		"continua", "continua", "sigue con", "retoma", "retomemos",
		"corrige eso", "corrige el error", "hazlo otra vez",
		"where is", "what files", "how is it going", "what is missing",
		"continue", "resume")
}

func isGreeting(n string) bool {
	// Greeting is an interaction act, not a substring. Keep this deliberately
	// strict so names, code literals and examples cannot become greetings.
	t := strings.Trim(n, " .!?¿¡,")
	switch t {
	case "hola", "buenas", "buenos dias",
		"buenas tardes", "buenas noches", "hey", "hello", "hi":
		return true
	}
	return false
}

func isSocial(n string) bool {
	t := strings.Trim(n, " .!?¿¡,")
	switch t {
	case "como estas", "que tal",
		"gracias", "jajaja", "xd", "how are you", "thanks":
		return true
	}
	return false
}

// normalizeForClassification removes only quoted literals from the semantic
// classification surface. The original request remains untouched in Raw and
// Objective.Text, so providers still receive exactly what the user asked for.
func normalizeForClassification(n string) string {
	// Quoted literals are removed before classification so examples/code do not
	// become interaction acts. Text outside quotes is normalized through the
	// shared contract for accents/case while preserving punctuation that carries
	// parser meaning (commas, question marks, etc.).
	var out strings.Builder
	var segment strings.Builder
	quote := rune(0)
	flush := func() {
		part := shared.Normalize(segment.String())
		segment.Reset()
		if part == "" {
			return
		}
		if out.Len() > 0 {
			last, _ := utf8.DecodeLastRuneInString(out.String())
			if !unicode.IsSpace(last) && last != ',' && last != ';' && last != ':' {
				out.WriteByte(' ')
			}
		}
		out.WriteString(part)
	}
	for _, r := range n {
		if quote != 0 {
			if r == quote || (quote == '“' && r == '”') {
				quote = 0
				out.WriteByte(' ')
			}
			continue
		}
		switch r {
		case '"', '\'', '“':
			flush()
			quote = r
			out.WriteByte(' ')
		case '”':
			flush()
			out.WriteByte(' ')
		case '¿':
			// Opening Spanish question marker is structural only.
		case '?':
			flush()
			if out.Len() > 0 && !unicode.IsSpace(lastRune(out.String())) {
				out.WriteByte(' ')
			}
			out.WriteRune('?')
		case '!', ',', '.', ';', ':':
			flush()
			out.WriteRune(r)
			out.WriteByte(' ')
		default:
			segment.WriteRune(r)
		}
	}
	flush()
	return strings.Join(strings.Fields(out.String()), " ")
}

func lastRune(s string) rune {
	r, _ := utf8.DecodeLastRuneInString(s)
	return r
}

func normalize(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return ' '
		}
		return r
	}, s)
}

func containsAny(s string, terms ...string) bool {
	for _, term := range terms {
		if strings.Contains(s, term) {
			return true
		}
	}
	return false
}
