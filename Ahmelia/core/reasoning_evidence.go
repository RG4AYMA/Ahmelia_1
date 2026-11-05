package core

import (
	"fmt"
	"strings"
	"time"

	"ahmelia/cognition"
	"ahmelia/interaction"
	"ahmelia/knowledge"
	"ahmelia/reasoning_deep"
	"ahmelia/shared"
	"ahmelia/situation"
	"ahmelia/solving"
)

// Phase 2 — Legacy reasoning is a specialist evidence producer.
//
// Knowledge Core and Cognitive Mind remain useful providers, but neither can
// own the final conversational response. This adapter deliberately stops at
// evidence. ResponseIntent/Dialogue/ResponseContract remain the behavioral
// authority and ConversationalGenerator remains the language boundary.
type specialistEvidenceBundle struct {
	ReasonerText       string
	ReasonerConfidence float64
	Knowledge          *knowledge.Result
	Reasoning          *cognition.ReasoningContext
}

func (b specialistEvidenceBundle) Empty() bool {
	return strings.TrimSpace(b.ReasonerText) == "" && b.Knowledge == nil && b.Reasoning == nil
}

func (b specialistEvidenceBundle) ResponseEvidence() []interaction.ResponseEvidence {
	out := make([]interaction.ResponseEvidence, 0, 3)
	if strings.TrimSpace(b.ReasonerText) != "" {
		out = append(out, interaction.ResponseEvidence{
			Text:       strings.TrimSpace(b.ReasonerText),
			Source:     "reasoning:reasoner",
			Relevant:   true,
			Confidence: b.ReasonerConfidence,
		})
	}
	if b.Knowledge != nil && strings.TrimSpace(b.Knowledge.Content) != "" {
		out = append(out, interaction.ResponseEvidence{
			Text:       strings.TrimSpace(b.Knowledge.Content),
			Source:     "knowledge:" + strings.TrimSpace(b.Knowledge.Source),
			Relevant:   true,
			Confidence: b.Knowledge.Confidence,
		})
	}
	if b.Reasoning != nil {
		// ReasoningNeed is internal metacognitive assessment (a score and its
		// scoring signals), not evidence that answers the user's question. Keep
		// it in ReasoningContext and out of the user-facing response contract.
		if b.Reasoning.Activated && b.Reasoning.Decision != nil {
			out = append(out, interaction.ResponseEvidence{
				Text:       strings.TrimSpace(b.Reasoning.Decision.Describe()),
				Source:     "cognitive_reasoning",
				Relevant:   true,
				Confidence: reasoningConfidence(b.Reasoning),
			})
		}
	}
	if len(out) == 0 {
		out = append(out, interaction.ResponseEvidence{
			Text:       "Entiendo la solicitud, pero no tengo una fuente de conocimiento suficiente para resolverla de forma fiable.",
			Source:     "reasoning_uncertainty",
			Relevant:   true,
			Confidence: 0.4,
		})
	}
	return out
}

func (b specialistEvidenceBundle) Contribution(now time.Time) situation.Contribution {
	if now.IsZero() {
		now = time.Now()
	}
	contribution := situation.Contribution{
		ID:          fmt.Sprintf("specialist-evidence-%d", now.UnixNano()),
		Perspective: "specialist_evidence",
		Confidence:  specialistConfidence(b),
		CreatedAt:   now,
		Provenance:  []string{"core.specialistEvidence", "B96.40"},
	}
	for idx, evidence := range b.ResponseEvidence() {
		id := fmt.Sprintf("specialist-evidence:%d", idx)
		kind := "supports"
		if evidence.Source == "reasoning_uncertainty" {
			kind = "uncertain_about"
		}
		contribution.Claims = append(contribution.Claims, evidence.Text)
		contribution.Relations = append(contribution.Relations, situation.Relation{
			Source:      evidence.Source,
			Target:      "current_turn",
			Type:        kind,
			EvidenceIDs: []string{id},
			Provenance:  evidence.Source,
			Temporal:    situation.TemporalCurrent,
			Relevance:   0.95,
			Confidence:  evidence.Confidence,
			Epistemic:   situation.EpistemicDerived,
		})
	}
	return contribution
}

func specialistConfidence(b specialistEvidenceBundle) float64 {
	best := 0.0
	if b.Knowledge != nil {
		best = b.Knowledge.Confidence
	}
	if b.ReasonerConfidence > best {
		best = b.ReasonerConfidence
	}
	if c := reasoningConfidence(b.Reasoning); c > best {
		best = c
	}
	if best <= 0 {
		best = 0.4
	}
	return clampReasoningConfidence(best)
}

func reasoningConfidence(ctx *cognition.ReasoningContext) float64 {
	if ctx == nil {
		return 0.4
	}
	if ctx.ReasoningNeed == nil {
		if ctx.Activated {
			return 0.75
		}
		return 0.4
	}
	if !ctx.Activated {
		return 0.45
	}
	// The score is an activation signal, not truth probability. Keep the
	// response evidence conservatively below absolute confidence.
	score := float64(ctx.ReasoningNeed.Score)
	confidence := 0.65 + score*0.05
	if confidence > 0.9 {
		confidence = 0.9
	}
	return confidence
}

func clampReasoningConfidence(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// shouldCollectSpecialistEvidence identifies the exact legacy gateway family
// without reparsing raw text. It consumes the established semantic contract and
// excludes turns already owned by a more specific authority.
func shouldCollectSpecialistEvidence(req interaction.SemanticRequest, historicalRequired bool) bool {
	acts := interaction.CanonicalActs(req)
	if !acts.Has(interaction.ActQuestion) && !acts.Has(interaction.ActOpinionRequest) && !acts.Has(interaction.ActLearning) {
		return false
	}
	selfOwnedLearningQuery := interaction.HasSemanticQuery(req, interaction.QueryHistoryLearning)
	if acts.Has(interaction.ActCommand) || historicalRequired || (selfReferentialQuestion(req) && !selfOwnedLearningQuery) {
		return false
	}
	if _, ok := interaction.DetectUserModelQuery(req); ok {
		return false
	}
	return true
}

// collectSpecialistEvidence preserves the useful provider order that used to
// formerly mixed inside the legacy semantic reasoning gateway, but stops at evidence.
func (e *Engine) collectSpecialistEvidence(req interaction.SemanticRequest) specialistEvidenceBundle {
	bundle := specialistEvidenceBundle{}
	input := strings.TrimSpace(req.Raw)
	if input == "" {
		input = strings.TrimSpace(req.Normalized)
	}
	queryKnowledge := func() bool {
		if e.knowledgeEngine == nil || (req.Raw == "" && req.Normalized == "" && req.Meaning.Query == interaction.QueryNone) {
			return false
		}
		result := e.knowledgeEngine.QuerySemantic(req, situation.DecisionEvidence{})
		if result == nil || result.Kind == knowledge.KindUnknown || strings.TrimSpace(result.Content) == "" {
			return false
		}
		bundle.Knowledge = result
		if e.confidenceCalculator != nil {
			sourceLabel := result.Source
			switch result.Source {
			case "reasoner":
				sourceLabel = "Razonador (evidencia)"
			case "self":
				sourceLabel = "Autoconocimiento (evidencia)"
			}
			e.confidenceCalculator.AddSource("question_"+input[:min(20, len(input))], &reasoning_deep.ConfidenceSource{
				Name: sourceLabel, Type: "evidence", Weight: 0.8, Value: result.Confidence,
			})
		}
		if e.mentalAgenda != nil {
			e.mentalAgenda.Observe(result.Source+" aportó evidencia", 3, result.Confidence)
			e.mentalAgenda.SaveState()
		}
		return true
	}

	// A typed semantic query already has a domain owner. Let that owner answer
	// before the legacy raw-text Reasoner, whose broad phrase matcher can claim
	// a more specific query (for example learning as a generic insight).
	if req.Meaning.Query != interaction.QueryNone && queryKnowledge() {
		return bundle
	}

	// The legacy Reasoner is a specialist provider now, not a KnowledgeEngine
	// response source. This preserves its unique analysis capabilities while
	// preventing Reasoner → KnowledgeEngine → LanguageGenerator bypasses.
	if e.reasoner != nil && input != "" {
		if answer, ok := e.reasoner.Query(input); ok && strings.TrimSpace(answer) != "" {
			bundle.ReasonerText = answer
			bundle.ReasonerConfidence = 0.85
			if e.mentalAgenda != nil {
				e.mentalAgenda.Observe("reasoner aportó evidencia", 3, bundle.ReasonerConfidence)
				e.mentalAgenda.SaveState()
			}
			return bundle
		}
	}

	if queryKnowledge() {
		return bundle
	}

	bundle.Reasoning = e.buildDeliberativeReasoningContext(input)
	if bundle.Reasoning != nil && !bundle.Reasoning.Activated && e.mentalAgenda != nil {
		e.mentalAgenda.Doubt("Brecha de conocimiento: " + input)
		e.mentalAgenda.Worry("No existe una fuente disponible para esta pregunta", 2)
	}
	return bundle
}

// buildDeliberativeReasoningContext contains only the useful Cognitive Mind
// work that the old gateway performed. It never composes language and never
// chooses the final conversational response.
func (e *Engine) buildDeliberativeReasoningContext(input string) *cognition.ReasoningContext {
	rc := cognition.NewReasoningContext(input)
	if e.pipeline != nil {
		e.pipeline.Enter(cognition.StageReason, "deliberative reasoning evidence: "+input)
	}
	if e.internalState != nil {
		e.internalState.Enter(cognition.StateThinking, "razonando mediante evidencia")
	}

	cacheKey := shared.Normalize(input)
	if e.reasoningCache != nil {
		if cached, hit := e.reasoningCache.Get(cacheKey); hit {
			return cached
		}
	}

	rc.GoalAnalysis = e.goalAnalyzer.Analyze(input)
	rc.ReasoningNeed = solving.ScoreReasoningNeed(rc.GoalAnalysis)
	if rc.ReasoningNeed.ShouldActivate() {
		rc.Activated = true
		rc.TaskPlan = e.problemDecomposer.Decompose(rc.GoalAnalysis)
		rc.KnowledgeMap = e.knowledgeMapper.Map(rc.TaskPlan)
		rc.DependencyGraph = e.dependencySolver.Solve(rc.TaskPlan, rc.KnowledgeMap)
		rc.HypothesisSet = e.hypothesisGenerator.Generate(rc.KnowledgeMap, rc.DependencyGraph)
		rc.ConstraintReport = e.constraintSolver.Check(rc.GoalAnalysis, rc.KnowledgeMap, rc.DependencyGraph)
		rc.Decision = e.decisionEngine.Decide(rc.GoalAnalysis.MainGoal, rc.HypothesisSet, rc.ConstraintReport)
	}
	if e.reasoningCache != nil {
		e.reasoningCache.Store(cacheKey, rc)
	}
	return rc
}

func (e *Engine) renderSpecialistEvidence(req interaction.SemanticRequest, bundle specialistEvidenceBundle) bool {
	if e == nil || e.lastResponseIntent.Kind == interaction.ResponseIntentNone || !e.lastResponseIntent.ShouldSpeak {
		return false
	}
	move := e.lastNaturalDialogue.Move
	if !interaction.ResponseMoveIsValid(move) {
		move = interaction.MoveAnswer
		if interaction.CanonicalActs(req).Has(interaction.ActLearning) {
			move = interaction.MoveAcknowledge
		}
	}
	contract := interaction.NewResponseContract(req, e.lastResponseIntent, move)
	contract.Source = "specialist_evidence"
	for _, evidence := range bundle.ResponseEvidence() {
		contract.AddEvidence(evidence.Text, evidence.Source, evidence.Relevant, evidence.Confidence)
	}
	facts := e.buildConversationalRuntimeFacts(e.conversationState.Snapshot())
	return e.renderResponseContract(req, contract, facts)
}
