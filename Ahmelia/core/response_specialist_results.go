package core

import (
	"fmt"
	"strings"
	"time"

	contextintelligence "ahmelia/contextintelligence"
	"ahmelia/dialogue"
	"ahmelia/interaction"
	"ahmelia/knowledge"
	memoryintelligence "ahmelia/memory_intelligence"
	"ahmelia/situation"
	"ahmelia/solving"
)

// Phase 3 — specialist results keep their domain semantics while losing the
// authority to speak. Each result is collected before Situation/Cognition and
// rendered only after ResponseIntent + Dialogue through ResponseContract.

type DeterministicAnswer struct {
	Value      string
	Source     string
	Confidence float64
}

type SkillResult struct {
	Search *solving.SkillSearchResult
	Source string
}

func (r SkillResult) Text() string {
	if r.Search == nil {
		return ""
	}
	if len(r.Search.Matches) == 1 && r.Search.Matches[0].Note != nil {
		return r.Search.Matches[0].Note.Describe()
	}
	return r.Search.Describe()
}

func (r SkillResult) Confidence() float64 {
	if r.Search == nil || !r.Search.HasMatches() {
		return 0.4
	}
	if len(r.Search.Matches) == 1 {
		return 0.95
	}
	return 0.8
}

type SocialMemoryResult struct {
	Query  string
	Person *dialogue.Person
	Prompt string
}

func (r SocialMemoryResult) Text() string {
	if strings.TrimSpace(r.Prompt) != "" {
		return r.Prompt
	}
	if r.Person == nil {
		return fmt.Sprintf("No tengo ningún registro de %s todavía.", r.Query)
	}
	return r.Person.Describe()
}

func (r SocialMemoryResult) Confidence() float64 {
	if r.Person != nil {
		return 0.95
	}
	if strings.TrimSpace(r.Prompt) != "" {
		return 1
	}
	return 0.4
}

type ConversationalMemoryResult struct {
	Query   string
	Textual string
	Found   bool
}

func (r ConversationalMemoryResult) Text() string { return strings.TrimSpace(r.Textual) }
func (r ConversationalMemoryResult) Confidence() float64 {
	if r.Found {
		return 0.9
	}
	return 0.4
}

type LongMemoryResult struct {
	Response       string
	ActionProposal string
	Target         string
	Confidence     float64
}

type SpecialistResponseResults struct {
	Historical           *dialogue.HistoricalRecallResult
	ConversationalMemory *ConversationalMemoryResult
	Knowledge            *knowledge.Result
	Skill                *SkillResult
	Social               *SocialMemoryResult
	Deterministic        *DeterministicAnswer
	LongMemory           *LongMemoryResult
}

func (r SpecialistResponseResults) Empty() bool {
	return r.Historical == nil && r.ConversationalMemory == nil && r.Knowledge == nil &&
		r.Skill == nil && r.Social == nil && r.Deterministic == nil && r.LongMemory == nil
}

func (r SpecialistResponseResults) Primary() (source, text string, confidence float64, ok bool) {
	// Primary is a rendering hint only. It is not a cognitive decision: the
	// complete typed result set is preserved in Situation and ResponseContract.
	// The order exists only to provide a stable trace/source label when a
	// downstream renderer needs one.
	switch {
	case r.Historical != nil && strings.TrimSpace(r.Historical.Response) != "":
		return "historical_memory", strings.TrimSpace(r.Historical.Response), 0.9, true
	case r.ConversationalMemory != nil && r.ConversationalMemory.Text() != "":
		return "conversational_memory", r.ConversationalMemory.Text(), r.ConversationalMemory.Confidence(), true
	case r.Deterministic != nil && strings.TrimSpace(r.Deterministic.Value) != "":
		return r.Deterministic.Source, strings.TrimSpace(r.Deterministic.Value), r.Deterministic.Confidence, true
	case r.Social != nil && r.Social.Text() != "":
		return "social_memory", r.Social.Text(), r.Social.Confidence(), true
	case r.Skill != nil && r.Skill.Text() != "":
		source = r.Skill.Source
		if source == "" {
			source = "skill_library"
		}
		return source, r.Skill.Text(), r.Skill.Confidence(), true
	case r.Knowledge != nil && strings.TrimSpace(r.Knowledge.Content) != "":
		source = "knowledge"
		if strings.TrimSpace(r.Knowledge.Source) != "" {
			source += ":" + strings.TrimSpace(r.Knowledge.Source)
		}
		confidence = r.Knowledge.Confidence
		if confidence <= 0 {
			confidence = 0.4
		}
		return source, strings.TrimSpace(r.Knowledge.Content), confidence, true
	case r.LongMemory != nil && strings.TrimSpace(r.LongMemory.Response) != "":
		confidence = r.LongMemory.Confidence
		if confidence <= 0 {
			confidence = 0.4
		}
		return "long_memory", strings.TrimSpace(r.LongMemory.Response), confidence, true
	}
	return "", "", 0, false
}

func (r SpecialistResponseResults) ResponseEvidence() []interaction.ResponseEvidence {
	out := make([]interaction.ResponseEvidence, 0, 7)
	if r.Historical != nil && strings.TrimSpace(r.Historical.Response) != "" {
		out = append(out, interaction.ResponseEvidence{Text: strings.TrimSpace(r.Historical.Response), Source: "historical_memory", Relevant: true, Confidence: 0.9})
	}
	if r.ConversationalMemory != nil && r.ConversationalMemory.Text() != "" {
		out = append(out, interaction.ResponseEvidence{Text: r.ConversationalMemory.Text(), Source: "conversational_memory", Relevant: true, Confidence: r.ConversationalMemory.Confidence()})
	}
	if r.Deterministic != nil && strings.TrimSpace(r.Deterministic.Value) != "" {
		out = append(out, interaction.ResponseEvidence{Text: strings.TrimSpace(r.Deterministic.Value), Source: r.Deterministic.Source, Relevant: true, Confidence: r.Deterministic.Confidence})
	}
	if r.Social != nil && r.Social.Text() != "" {
		out = append(out, interaction.ResponseEvidence{Text: r.Social.Text(), Source: "social_memory", Relevant: true, Confidence: r.Social.Confidence()})
	}
	if r.Skill != nil && r.Skill.Text() != "" {
		source := r.Skill.Source
		if source == "" {
			source = "skill_library"
		}
		out = append(out, interaction.ResponseEvidence{Text: r.Skill.Text(), Source: source, Relevant: true, Confidence: r.Skill.Confidence()})
	}
	if r.Knowledge != nil && strings.TrimSpace(r.Knowledge.Content) != "" {
		source := "knowledge"
		if strings.TrimSpace(r.Knowledge.Source) != "" {
			source += ":" + strings.TrimSpace(r.Knowledge.Source)
		}
		confidence := r.Knowledge.Confidence
		if confidence <= 0 {
			confidence = 0.4
		}
		out = append(out, interaction.ResponseEvidence{Text: strings.TrimSpace(r.Knowledge.Content), Source: source, Relevant: true, Confidence: confidence})
	}
	if r.LongMemory != nil && strings.TrimSpace(r.LongMemory.Response) != "" {
		confidence := r.LongMemory.Confidence
		if confidence <= 0 {
			confidence = 0.4
		}
		out = append(out, interaction.ResponseEvidence{Text: strings.TrimSpace(r.LongMemory.Response), Source: "long_memory", Relevant: true, Confidence: confidence})
	}
	return out
}

func (r SpecialistResponseResults) Contributions(now time.Time) []situation.Contribution {
	if now.IsZero() {
		now = time.Now()
	}
	var out []situation.Contribution
	appendOne := func(source, text string, confidence float64) {
		text = strings.TrimSpace(text)
		if text == "" {
			return
		}
		if confidence <= 0 {
			confidence = 0.4
		}
		out = append(out, situation.Contribution{
			ID:          fmt.Sprintf("specialist-response-%s-%d", source, now.UnixNano()+int64(len(out))),
			Perspective: "specialist_response:" + source,
			Confidence:  confidence,
			CreatedAt:   now,
			Provenance:  []string{"core.specialistResponseResults", "B96.39", "B96.40"},
			Claims:      []string{text},
			Relations: []situation.Relation{{
				Source:      source,
				Target:      "current_turn",
				Type:        "supports",
				EvidenceIDs: []string{"specialist-response:" + source},
				Provenance:  source,
				Temporal:    situation.TemporalCurrent,
				Relevance:   1,
				Confidence:  confidence,
				Epistemic:   situation.EpistemicDerived,
			}},
		})
	}
	if r.Historical != nil {
		appendOne("historical_memory", r.Historical.Response, 0.9)
	}
	if r.ConversationalMemory != nil {
		appendOne("conversational_memory", r.ConversationalMemory.Text(), r.ConversationalMemory.Confidence())
	}
	if r.Deterministic != nil {
		appendOne(r.Deterministic.Source, r.Deterministic.Value, r.Deterministic.Confidence)
	}
	if r.Social != nil {
		appendOne("social_memory", r.Social.Text(), r.Social.Confidence())
	}
	if r.Skill != nil {
		source := r.Skill.Source
		if source == "" {
			source = "skill_library"
		}
		appendOne(source, r.Skill.Text(), r.Skill.Confidence())
	}
	if r.Knowledge != nil {
		source := "knowledge"
		if strings.TrimSpace(r.Knowledge.Source) != "" {
			source += ":" + strings.TrimSpace(r.Knowledge.Source)
		}
		confidence := r.Knowledge.Confidence
		if confidence <= 0 {
			confidence = 0.4
		}
		appendOne(source, r.Knowledge.Content, confidence)
	}
	if r.LongMemory != nil {
		appendOne("long_memory", r.LongMemory.Response, r.LongMemory.Confidence)
	}
	return out
}

// Contribution is kept for compatibility with existing callers that want a
// single contribution, but it never replaces the plural Contributions API.
func (r SpecialistResponseResults) Contribution(now time.Time) (situation.Contribution, bool) {
	all := r.Contributions(now)
	if len(all) == 0 {
		return situation.Contribution{}, false
	}
	return all[0], true
}

func (e *Engine) collectResponseSpecialistResults(req interaction.SemanticRequest, historicalRequired bool, contextResolution contextintelligence.Result, previousTurn interaction.TurnRecord, hasPreviousTurn bool, input, normInput string) SpecialistResponseResults {
	results := SpecialistResponseResults{}

	if historicalRequired && e.conversationHistory != nil {
		historicalQuery := req.Raw
		if contextResolution.Historical.Required && contextResolution.Historical.Query != "" {
			historicalQuery = contextResolution.Historical.Query
		}
		if hasPreviousTurn && historicalQuery == req.Raw {
			historicalQuery = dialogue.ResolveHistoricalReferenceQuery(historicalQuery, previousTurn.Topic, previousTurn.Objective)
		}
		recall := dialogue.ResolveHistoricalRecallExcluding(e.conversationHistory, historicalQuery, time.Now(), e.conversationEpisodeID)
		if !recall.Found {
			recall.Response = fmt.Sprintf("No tengo ningún registro de haber hablado de %s antes.", dialogue.ExtractMemoryTarget(normInput))
			if strings.TrimSpace(dialogue.ExtractMemoryTarget(normInput)) == "" {
				recall.Response = "No encontré un registro histórico relevante para esta pregunta."
			}
		}
		results.Historical = &recall
		// B96.36 explicitly requested historical context, so historical recall is
		// the contextual source for this turn. This is a context gate, not a
		// specialist response priority rule.
		return results
	}

	if answer, ok := interaction.SolveQuestion(req); ok {
		results.Deterministic = &DeterministicAnswer{Value: answer, Source: "deterministic_answer", Confidence: 1}
	}

	if dialogue.ParseConversationalMemoryQuery(normInput) && e.conversationalMemory != nil {
		target := dialogue.ExtractMemoryTarget(normInput)
		if target != "" {
			if e.conversationHistory != nil {
				recall := dialogue.ResolveHistoricalRecallExcluding(e.conversationHistory, target, time.Now(), e.conversationEpisodeID)
				if recall.Found && strings.TrimSpace(recall.Response) != "" {
					results.Historical = &recall
				}
			}
			if results.Historical == nil {
				results.ConversationalMemory = &ConversationalMemoryResult{
					Query:   target,
					Textual: fmt.Sprintf("No tengo ningún registro de haber hablado de %s antes.", target),
					Found:   false,
				}
			}
		} else if strings.Contains(normInput, "de que hablamos hace tiempo") && e.conversationHistory != nil {
			matches := e.conversationHistory.Latest(time.Now(), 5, dialogue.RecentThreshold)
			if len(matches) > 0 {
				var parts []string
				for _, m := range matches {
					label := strings.Join(m.Episode.Topics, ", ")
					if label == "" {
						label = "sin tema registrado"
					}
					parts = append(parts, fmt.Sprintf("%s (%s)", label, m.Episode.Start.Format("02/01/2006")))
				}
				results.ConversationalMemory = &ConversationalMemoryResult{Query: normInput, Textual: "Recuerdo estas conversaciones anteriores: " + strings.Join(parts, "; ") + ".", Found: true}
			}
		} else {
			pending := e.conversationalMemory.PendingTopics()
			if len(pending) == 0 {
				results.ConversationalMemory = &ConversationalMemoryResult{Query: normInput, Textual: "No tengo ningún tema pendiente registrado.", Found: false}
			} else {
				names := make([]string, 0, len(pending))
				for _, p := range pending {
					names = append(names, p.Name)
				}
				results.ConversationalMemory = &ConversationalMemoryResult{Query: normInput, Textual: "Temas pendientes: " + strings.Join(names, ", ") + ".", Found: true}
			}
		}
	}

	if dialogue.ParseSocialMemoryQuery(normInput) {
		target := dialogue.ExtractPersonName(normInput)
		if target == "" {
			results.Social = &SocialMemoryResult{Query: target, Prompt: "¿De quién o de qué querés que te cuente?"}
		} else {
			if e.socialMemory != nil {
				if p, ok := e.socialMemory.Recall(target); ok {
					results.Social = &SocialMemoryResult{Query: target, Person: p}
				}
			}
			if results.Social == nil && e.knowledgeDiscoverer != nil && e.skillLibrary != nil {
				if discovery := e.knowledgeDiscoverer.Discover(target); discovery.Domain != "" {
					if note, ok := e.skillLibrary.Get(discovery.Domain); ok && note != nil {
						search := &solving.SkillSearchResult{Query: target, Matches: []solving.SkillMatch{{Domain: discovery.Domain, Note: note, Hits: 1}}}
						results.Skill = &SkillResult{Search: search, Source: "skill_library"}
					}
				}
			}
			if results.Social == nil && results.Skill == nil && e.skillLibrary != nil {
				search := solving.NewSemanticSkillSearcher().Search(target, e.skillLibrary)
				if search.HasMatches() {
					results.Skill = &SkillResult{Search: search, Source: "skill_library"}
				}
			}
			if results.Social == nil && results.Skill == nil && e.knowledgeMemory != nil {
				if entry, ok := e.knowledgeMemory.Recall(target); ok {
					since := entry.FirstValidatedAt
					if t, err := time.Parse(time.RFC3339, entry.FirstValidatedAt); err == nil {
						since = t.Format("2006-01-02")
					}
					results.Knowledge = &knowledge.Result{Content: fmt.Sprintf("%s es confiable desde %s (confianza %.0f%%).", entry.Label, since, entry.Confidence*100), Kind: knowledge.KindFact, Confidence: entry.Confidence, Source: "knowledge_memory"}
				}
			}
			if results.Social == nil && results.Skill == nil && results.Knowledge == nil {
				results.Knowledge = &knowledge.Result{Content: fmt.Sprintf("No tengo ningún registro de %s todavía.", target), Kind: knowledge.KindUnknown, Confidence: 0.4, Source: "knowledge"}
			}
		}
	}

	_, isRelationsQuery := memoryintelligence.ParseRelationsQuery(req)
	isPatternsQuery := memoryintelligence.ParsePatternsQuery(req)
	isHabitsQuery := memoryintelligence.ParseHabitsQuery(req)
	_, isExperienceQuery := memoryintelligence.ParseExperienceQuery(req)
	isInsightsQuery := memoryintelligence.ParseInsightsQuery(req)
	_, _, _, isTimelineQuery := memoryintelligence.ParseTimelineQuery(req)
	if (isTimelineQuery || isRelationsQuery || isPatternsQuery || isHabitsQuery || isExperienceQuery || isInsightsQuery) && e.knowledgeEngine != nil {
		if r := e.knowledgeEngine.QuerySemantic(req, e.lastDecisionEvidence); r != nil && r.Kind != knowledge.KindUnknown && strings.TrimSpace(r.Content) != "" {
			results.Knowledge = r
		}
	}

	// Self-knowledge queries (subject=self) route through the knowledge engine
	// to the SelfKnowledgeSource. This covers both explicit QuerySelfKnowledge
	// triggers and any question where ownership resolved to SubjectSelf.
	// Check BEFORE skill search so "que sabes sobre ti misma" goes to self-knowledge,
	// not to skill library search for notes about "ti misma".
	isSelfKnowledgeQuery := req.Subject == interaction.UserObservationSubjectSelf || req.Meaning.Query == interaction.QuerySelfKnowledge
	if isSelfKnowledgeQuery && e.knowledgeEngine != nil {
		if r := e.knowledgeEngine.QuerySemantic(req, e.lastDecisionEvidence); r != nil && r.Kind != knowledge.KindUnknown && strings.TrimSpace(r.Content) != "" {
			results.Knowledge = r
		}
	}

	// Skill search only for non-self queries about external topics
	if solving.ParseSemanticSkillSearchQuery(req) && !isSelfKnowledgeQuery && e.skillLibrary != nil {
		search := solving.NewSemanticSkillSearcher().Search(input, e.skillLibrary)
		if search.HasMatches() {
			results.Skill = &SkillResult{Search: search, Source: "skill_library"}
		}
	}

	if e.longMem != nil {
		if k, ok := e.longMem.Recall(input); ok && k != nil {
			if strings.TrimSpace(k.Response) != "" || (k.Action != "" && k.Action != "desconocido") {
				results.LongMemory = &LongMemoryResult{Response: k.Response, ActionProposal: k.Action, Target: k.Target, Confidence: k.Confidence}
			}
		}
	}

	return results
}

func (e *Engine) renderSpecialistResponseResults(req interaction.SemanticRequest, results SpecialistResponseResults) bool {
	if e == nil || results.Empty() || e.lastResponseIntent.Kind == interaction.ResponseIntentNone || !e.lastResponseIntent.ShouldSpeak {
		return false
	}
	move := e.lastNaturalDialogue.Move
	if !interaction.ResponseMoveIsValid(move) {
		move = interaction.MoveAnswer
	}
	contract := interaction.NewResponseContract(req, e.lastResponseIntent, move)
	source, _, _, ok := results.Primary()
	if !ok {
		return false
	}
	contract.Source = source
	for _, evidence := range results.ResponseEvidence() {
		contract.AddEvidence(evidence.Text, evidence.Source, evidence.Relevant, evidence.Confidence)
	}
	return e.renderResponseContract(req, contract, e.buildConversationalRuntimeFacts(e.conversationState.Snapshot()))
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
