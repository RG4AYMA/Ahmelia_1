package knowledge

import (
	"ahmelia/interaction"
	"ahmelia/situation"
)

func queryTyped(fn QueryFunc, req interaction.SemanticRequest, ev situation.DecisionEvidence, kind Kind, source string) (*Result, bool) {
	if fn == nil {
		return nil, false
	}
	content, confidence, ok := fn(req, ev)
	if !ok || content == "" {
		return nil, false
	}
	return &Result{Content: content, Kind: kind, Confidence: confidence, Source: source}, true
}

func queryCompat(fn QueryFunc, topic string, kind Kind, source string) (*Result, bool) {
	req := interaction.Analyze(topic, interaction.Context{})
	return queryTyped(fn, req, situation.DecisionEvidence{}, kind, source)
}

// semanticSourceHandles centralizes provider routing from the typed semantic
// contract. It never reads Raw or Normalized; those are owned by interaction.
func semanticSourceHandles(req interaction.SemanticRequest, allowed ...interaction.SemanticQueryType) (bool, float64) {
	q := req.Meaning.Query
	for _, a := range allowed {
		if q == a {
			confidence := req.Meaning.Confidence
			if confidence <= 0 {
				confidence = req.Confidence
			}
			if confidence <= 0 {
				confidence = 0.5
			}
			if req.QuestionType != "" {
				confidence += 0.02
			}
			if len(req.Entities) > 0 || len(req.Relations) > 0 || req.Topic != "" {
				confidence += 0.02
			}
			if confidence > 0.99 {
				confidence = 0.99
			}
			return true, confidence
		}
	}
	return false, 0
}

// --- Reasoner ---------------------------------------------------------
type ReasonerSource struct{ fn QueryFunc }

func NewReasonerSource(fn QueryFunc) *ReasonerSource { return &ReasonerSource{fn: fn} }
func (r *ReasonerSource) Name() string               { return "reasoner" }
func (r *ReasonerSource) Priority() int              { return 5 }
func (r *ReasonerSource) Handles(req interaction.SemanticRequest) (bool, float64) {
	return semanticSourceHandles(req, interaction.QuerySolvingGoalAnalysis, interaction.QuerySolvingDecision, interaction.QuerySolvingHypothesis, interaction.QuerySolvingCausalExplain, interaction.QuerySolvingCausalProject, interaction.QuerySolvingStrategy)
}
func (r *ReasonerSource) QuerySemantic(req interaction.SemanticRequest, ev situation.DecisionEvidence) (*Result, bool) {
	return queryTyped(r.fn, req, ev, KindStructured, "reasoner")
}
func (r *ReasonerSource) Query(topic string) (*Result, bool) {
	return queryCompat(r.fn, topic, KindStructured, "reasoner")
}

// --- Environment -----------------------------------------------------
type EnvironmentSource struct{ fn QueryFunc }

func NewEnvironmentSource(fn QueryFunc) *EnvironmentSource { return &EnvironmentSource{fn: fn} }
func (e *EnvironmentSource) Name() string                  { return "environment" }
func (e *EnvironmentSource) Priority() int                 { return 8 }
func (e *EnvironmentSource) Handles(req interaction.SemanticRequest) (bool, float64) {
	return semanticSourceHandles(req, interaction.QueryEnvironmentResources, interaction.QueryEnvironmentTabs, interaction.QueryEnvironmentContext, interaction.QueryEnvironmentActiveWork, interaction.QueryEnvironmentOpenApps)
}
func (e *EnvironmentSource) QuerySemantic(req interaction.SemanticRequest, ev situation.DecisionEvidence) (*Result, bool) {
	return queryTyped(e.fn, req, ev, KindFact, "environment")
}
func (e *EnvironmentSource) Query(topic string) (*Result, bool) {
	return queryCompat(e.fn, topic, KindFact, "environment")
}

// --- Memory ----------------------------------------------------------
type MemorySource struct{ fn QueryFunc }

func NewMemorySource(fn QueryFunc) *MemorySource { return &MemorySource{fn: fn} }
func (m *MemorySource) Name() string             { return "memory" }
func (m *MemorySource) Priority() int            { return 10 }
func (m *MemorySource) Handles(req interaction.SemanticRequest) (bool, float64) {
	if req.Meaning.Query != interaction.QueryNone {
		return semanticSourceHandles(req, req.Meaning.Query)
	}
	// The existing MemorySource closure also contains planning/project
	// state queries that are outside the G3 Parse*Query migration. They stay
	// reachable through the semantic request kind instead of raw-text routing.
	if req.QuestionType != "" || req.Kind == interaction.KindConversation || req.Kind == interaction.KindProduction || req.Kind == interaction.KindAction {
		return true, 0.5
	}
	return false, 0
}
func (m *MemorySource) QuerySemantic(req interaction.SemanticRequest, ev situation.DecisionEvidence) (*Result, bool) {
	return queryTyped(m.fn, req, ev, KindFact, "memory")
}
func (m *MemorySource) Query(topic string) (*Result, bool) {
	return queryCompat(m.fn, topic, KindFact, "memory")
}

// --- Self ------------------------------------------------------------
type SelfKnowledgeSource struct{ fn QueryFunc }

func NewSelfKnowledgeSource(fn QueryFunc) *SelfKnowledgeSource { return &SelfKnowledgeSource{fn: fn} }
func (s *SelfKnowledgeSource) Name() string                    { return "self" }
func (s *SelfKnowledgeSource) Priority() int                   { return 9 }
func (s *SelfKnowledgeSource) Handles(req interaction.SemanticRequest) (bool, float64) {
	if req.Subject != interaction.UserObservationSubjectSelf && req.Meaning.Query != interaction.QuerySelfKnowledge {
		return false, 0
	}
	if req.QuestionType == "" && req.Meaning.Query == interaction.QueryNone {
		return false, 0
	}
	return true, 0.8
}
func (s *SelfKnowledgeSource) QuerySemantic(req interaction.SemanticRequest, ev situation.DecisionEvidence) (*Result, bool) {
	return queryTyped(s.fn, req, ev, KindFact, "self")
}
func (s *SelfKnowledgeSource) Query(topic string) (*Result, bool) {
	return queryCompat(s.fn, topic, KindFact, "self")
}

// --- Learning --------------------------------------------------------
type LearningSource struct{ fn QueryFunc }

func NewLearningSource(fn QueryFunc) *LearningSource { return &LearningSource{fn: fn} }
func (l *LearningSource) Name() string               { return "learning" }
func (l *LearningSource) Priority() int              { return 30 }
func (l *LearningSource) Handles(req interaction.SemanticRequest) (bool, float64) {
	return req.Meaning.Query != interaction.QueryNone && req.Kind == interaction.KindLearning, 0.7
}
func (l *LearningSource) QuerySemantic(req interaction.SemanticRequest, ev situation.DecisionEvidence) (*Result, bool) {
	return queryTyped(l.fn, req, ev, KindOpinion, "learning")
}
func (l *LearningSource) Query(topic string) (*Result, bool) {
	return queryCompat(l.fn, topic, KindOpinion, "learning")
}

// --- Mental state ----------------------------------------------------
type MentalStateSource struct{ fn QueryFunc }

func NewMentalStateSource(fn QueryFunc) *MentalStateSource { return &MentalStateSource{fn: fn} }
func (m *MentalStateSource) Name() string                  { return "mental_state" }
func (m *MentalStateSource) Priority() int                 { return 18 }
func (m *MentalStateSource) Handles(req interaction.SemanticRequest) (bool, float64) {
	return semanticSourceHandles(req, interaction.QueryMentalEvolution, interaction.QueryMentalDetailed, interaction.QueryMentalState)
}
func (m *MentalStateSource) QuerySemantic(req interaction.SemanticRequest, ev situation.DecisionEvidence) (*Result, bool) {
	return queryTyped(m.fn, req, ev, KindFact, "mental_state")
}
func (m *MentalStateSource) Query(topic string) (*Result, bool) {
	return queryCompat(m.fn, topic, KindFact, "mental_state")
}

// --- Local files / Internet compatibility shells --------------------
type LocalFilesSource struct{ fn QueryFunc }

func NewLocalFilesSource(fn QueryFunc) *LocalFilesSource                            { return &LocalFilesSource{fn: fn} }
func (l *LocalFilesSource) Name() string                                            { return "local_files" }
func (l *LocalFilesSource) Priority() int                                           { return 40 }
func (l *LocalFilesSource) Handles(req interaction.SemanticRequest) (bool, float64) { return false, 0 }
func (l *LocalFilesSource) QuerySemantic(req interaction.SemanticRequest, ev situation.DecisionEvidence) (*Result, bool) {
	return queryTyped(l.fn, req, ev, KindStructured, "local_files")
}
func (l *LocalFilesSource) Query(topic string) (*Result, bool) {
	return queryCompat(l.fn, topic, KindStructured, "local_files")
}

type InternetSource struct {
	fn      QueryFunc
	Enabled bool
}

func NewInternetSource(fn QueryFunc) *InternetSource {
	return NewInternetSourceWithConfig(fn, InternetSourceConfig{Enabled: false})
}
func (i *InternetSource) Name() string  { return "internet" }
func (i *InternetSource) Priority() int { return 100 }
func (i *InternetSource) Handles(req interaction.SemanticRequest) (bool, float64) {
	if !i.Enabled || req.Kind != interaction.KindSearch {
		return false, 0
	}
	confidence := req.Confidence
	if confidence <= 0 {
		confidence = 0.3
	}
	return true, confidence
}
func (i *InternetSource) QuerySemantic(req interaction.SemanticRequest, ev situation.DecisionEvidence) (*Result, bool) {
	if !i.Enabled {
		return nil, false
	}
	return queryTyped(i.fn, req, ev, KindFact, "internet")
}
func (i *InternetSource) Query(topic string) (*Result, bool) {
	if !i.Enabled {
		return nil, false
	}
	return queryCompat(i.fn, topic, KindFact, "internet")
}
