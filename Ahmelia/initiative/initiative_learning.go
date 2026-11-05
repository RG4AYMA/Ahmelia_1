package initiative

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// B96.37.16 observes B96.37.15 evidence and produces bounded adaptation
// evidence. It never returns an initiative Decision and never mutates Policy.
type LearningPatternKind string

const (
	PatternOutcome      LearningPatternKind = "OUTCOME"
	PatternUserResponse LearningPatternKind = "USER_RESPONSE"
	PatternAction       LearningPatternKind = "ACTION"
	PatternTiming       LearningPatternKind = "TIMING"
	PatternInterruption LearningPatternKind = "INTERRUPTION"
	PatternTopic        LearningPatternKind = "TOPIC"
)

type AdaptationScope string

const (
	ScopeGlobal       AdaptationScope = "GLOBAL"
	ScopeAction       AdaptationScope = "ACTION"
	ScopeTiming       AdaptationScope = "TIMING"
	ScopeInterruption AdaptationScope = "INTERRUPTION"
	ScopeTopic        AdaptationScope = "TOPIC"
)

type LearningEvidence struct {
	ID           string              `json:"id"`
	InitiativeID string              `json:"initiative_id"`
	OutcomeID    string              `json:"outcome_id"`
	Pattern      LearningPatternKind `json:"pattern"`
	Key          string              `json:"key"`
	Outcome      InitiativeOutcome   `json:"outcome"`
	Confidence   float64             `json:"confidence"`
	Evidence     string              `json:"evidence,omitempty"`
	ObservedAt   time.Time           `json:"observed_at"`
	ExpiresAt    time.Time           `json:"expires_at,omitempty"`
	Provenance   Provenance          `json:"provenance"`
}

type AdaptationProposal struct {
	ID            string              `json:"id"`
	Pattern       LearningPatternKind `json:"pattern"`
	Key           string              `json:"key"`
	Scope         AdaptationScope     `json:"scope"`
	Signal        string              `json:"signal"`
	Weight        float64             `json:"weight"`
	Confidence    float64             `json:"confidence"`
	EvidenceCount int                 `json:"evidence_count"`
	CreatedAt     time.Time           `json:"created_at"`
	ExpiresAt     time.Time           `json:"expires_at,omitempty"`
	Validated     bool                `json:"validated"`
	Reversed      bool                `json:"reversed"`
	Provenance    Provenance          `json:"provenance"`
}

type LearningPolicy struct {
	MinimumEvidence   int
	MinimumConfidence float64
	MaxWeight         float64
	EvidenceTTL       time.Duration
	AdaptationTTL     time.Duration
	MaxEvidence       int
}

func DefaultLearningPolicy() LearningPolicy {
	return LearningPolicy{MinimumEvidence: 3, MinimumConfidence: .70, MaxWeight: .75, EvidenceTTL: 24 * time.Hour, AdaptationTTL: 7 * 24 * time.Hour, MaxEvidence: 512}
}

type LearningStore interface {
	PutEvidence(LearningEvidence) error
	ListEvidence() []LearningEvidence
	PutAdaptation(AdaptationProposal) error
	GetAdaptation(string) (AdaptationProposal, bool)
	ListAdaptations() []AdaptationProposal
}

type InMemoryLearningStore struct {
	mu          sync.RWMutex
	evidence    map[string]LearningEvidence
	adaptations map[string]AdaptationProposal
}

func NewInMemoryLearningStore() *InMemoryLearningStore {
	return &InMemoryLearningStore{evidence: map[string]LearningEvidence{}, adaptations: map[string]AdaptationProposal{}}
}
func (s *InMemoryLearningStore) PutEvidence(x LearningEvidence) error {
	if strings.TrimSpace(x.ID) == "" {
		return errors.New("learning evidence requires id")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.evidence[x.ID] = x
	return nil
}
func (s *InMemoryLearningStore) ListEvidence() []LearningEvidence {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]LearningEvidence, 0, len(s.evidence))
	for _, x := range s.evidence {
		out = append(out, x)
	}
	return out
}
func (s *InMemoryLearningStore) PutAdaptation(x AdaptationProposal) error {
	if strings.TrimSpace(x.ID) == "" {
		return errors.New("adaptation requires id")
	}
	if x.Weight < 0 || x.Weight > 1 {
		return errors.New("adaptation weight must be between 0 and 1")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.adaptations[x.ID] = x
	return nil
}
func (s *InMemoryLearningStore) GetAdaptation(id string) (AdaptationProposal, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	x, ok := s.adaptations[id]
	return x, ok
}
func (s *InMemoryLearningStore) ListAdaptations() []AdaptationProposal {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]AdaptationProposal, 0, len(s.adaptations))
	for _, x := range s.adaptations {
		out = append(out, x)
	}
	return out
}

type InitiativeLearner struct {
	Store  LearningStore
	Policy LearningPolicy
}

func NewInitiativeLearner(store LearningStore) *InitiativeLearner {
	if store == nil {
		store = NewInMemoryLearningStore()
	}
	return &InitiativeLearner{Store: store, Policy: DefaultLearningPolicy()}
}

func LearningKey(o InitiativeOutcomeRecord) (LearningPatternKind, string, AdaptationScope) {
	action := string(o.InitiativeAction)
	reason := string(o.InitiativeReason)
	switch o.InitiativeAction {
	case ActionAsk, ActionSuggest, ActionPropose, ActionResume, ActionComment, ActionWarn, ActionContribute, ActionRemind, ActionShareDiscovery, ActionPresentPossibility, ActionContinue, ActionRedirect, ActionDisagree:
		return PatternAction, action, ScopeAction
	}
	if reason != "" {
		return PatternOutcome, reason, ScopeGlobal
	}
	return PatternOutcome, action, ScopeGlobal
}

func (l *InitiativeLearner) ObserveOutcome(o InitiativeOutcomeRecord) (LearningEvidence, error) {
	if l == nil || l.Store == nil {
		return LearningEvidence{}, errors.New("initiative learner unavailable")
	}
	if strings.TrimSpace(o.InitiativeID) == "" {
		return LearningEvidence{}, errors.New("outcome requires initiative id")
	}
	now := o.ObservedAt
	if now.IsZero() {
		now = time.Now()
	}
	p, k, scope := LearningKey(o)
	// Unknown/expired evidence is retained as observation but cannot create a learning signal.
	if o.Outcome == OutcomeUnknown || o.Outcome == OutcomeExpired {
		p = PatternOutcome
		k = string(o.Outcome)
		scope = ScopeGlobal
	}
	e := LearningEvidence{ID: "learning-" + o.ID + "-" + string(p) + "-" + k, InitiativeID: o.InitiativeID, OutcomeID: o.ID, Pattern: p, Key: k, Outcome: o.Outcome, Confidence: clamp(o.Confidence), Evidence: o.Evidence, ObservedAt: now, Provenance: Provenance{Component: "initiative.InitiativeLearner", Subsystem: "B96.37.16", SignalType: string(o.Outcome), OriginID: o.ID, CreatedAt: now}}
	if l.Policy.EvidenceTTL > 0 {
		e.ExpiresAt = now.Add(l.Policy.EvidenceTTL)
	}
	if err := l.Store.PutEvidence(e); err != nil {
		return e, err
	}
	_ = scope // scope is derived again during proposal generation.
	return e, nil
}

func (l *InitiativeLearner) Propose(pattern LearningPatternKind, key string, scope AdaptationScope, now time.Time) (AdaptationProposal, bool, error) {
	if l == nil || l.Store == nil {
		return AdaptationProposal{}, false, errors.New("initiative learner unavailable")
	}
	if now.IsZero() {
		now = time.Now()
	}
	ev := l.activeMatching(pattern, key, now)
	if len(ev) < l.Policy.MinimumEvidence {
		return AdaptationProposal{}, false, nil
	}
	total := 0.0
	accepted := 0
	rejected := 0
	for _, e := range ev {
		total += e.Confidence
		if e.Outcome == OutcomeAccepted || e.Outcome == OutcomeEngaged || e.Outcome == OutcomeCompleted {
			accepted++
		}
		if e.Outcome == OutcomeRejected || e.Outcome == OutcomeInterrupted || e.Outcome == OutcomeFailed {
			rejected++
		}
	}
	conf := total / float64(len(ev))
	if conf < l.Policy.MinimumConfidence {
		return AdaptationProposal{}, false, nil
	}
	balance := float64(accepted-rejected) / float64(len(ev))
	weight := 0.5 + 0.5*balance
	if weight < 0 {
		weight = 0
	}
	if weight > l.Policy.MaxWeight {
		weight = l.Policy.MaxWeight
	}
	signal := "neutral"
	if accepted > rejected {
		signal = "favorable"
	} else if rejected > accepted {
		signal = "caution"
	}
	x := AdaptationProposal{ID: "adapt-" + newID(now), Pattern: pattern, Key: key, Scope: scope, Signal: signal, Weight: weight, Confidence: conf, EvidenceCount: len(ev), CreatedAt: now, Validated: true, Provenance: Provenance{Component: "initiative.InitiativeLearner", Subsystem: "B96.37.16", SignalType: "adaptation", OriginID: key, CreatedAt: now}}
	if l.Policy.AdaptationTTL > 0 {
		x.ExpiresAt = now.Add(l.Policy.AdaptationTTL)
	}
	if err := l.Store.PutAdaptation(x); err != nil {
		return x, false, err
	}
	return x, true, nil
}
func (l *InitiativeLearner) activeMatching(p LearningPatternKind, key string, now time.Time) []LearningEvidence {
	out := []LearningEvidence{}
	for _, e := range l.Store.ListEvidence() {
		if e.Pattern == p && e.Key == key && (e.ExpiresAt.IsZero() || now.Before(e.ExpiresAt)) {
			out = append(out, e)
			if l.Policy.MaxEvidence > 0 && len(out) >= l.Policy.MaxEvidence {
				break
			}
		}
	}
	return out
}
func (l *InitiativeLearner) Validate(a AdaptationProposal, now time.Time) bool {
	if now.IsZero() {
		now = time.Now()
	}
	return a.Validated && !a.Reversed && (a.ExpiresAt.IsZero() || now.Before(a.ExpiresAt)) && a.Confidence >= l.Policy.MinimumConfidence
}
func (l *InitiativeLearner) Reverse(id string, now time.Time) (AdaptationProposal, error) {
	if l == nil || l.Store == nil {
		return AdaptationProposal{}, errors.New("initiative learner unavailable")
	}
	a, ok := l.Store.GetAdaptation(id)
	if !ok {
		return a, errors.New("adaptation not found")
	}
	if now.IsZero() {
		now = time.Now()
	}
	a.Reversed = true
	a.Validated = false
	if err := l.Store.PutAdaptation(a); err != nil {
		return a, err
	}
	return a, nil
}

// AdaptationSignal is deliberately evidence-only. Consumers such as B96.37.2
// may use it as an input; it contains no decision or action authority.
type AdaptationSignal struct {
	Pattern    LearningPatternKind `json:"pattern"`
	Key        string              `json:"key"`
	Signal     string              `json:"signal"`
	Weight     float64             `json:"weight"`
	Confidence float64             `json:"confidence"`
	Scope      AdaptationScope     `json:"scope"`
	Provenance Provenance          `json:"provenance"`
}

func (l *InitiativeLearner) Signals(now time.Time) []AdaptationSignal {
	if l == nil || l.Store == nil {
		return nil
	}
	out := []AdaptationSignal{}
	for _, a := range l.Store.ListAdaptations() {
		if l.Validate(a, now) {
			out = append(out, AdaptationSignal{Pattern: a.Pattern, Key: a.Key, Signal: a.Signal, Weight: a.Weight, Confidence: a.Confidence, Scope: a.Scope, Provenance: a.Provenance})
		}
	}
	return out
}
func (a AdaptationSignal) String() string { return fmt.Sprintf("%s:%s=%s", a.Pattern, a.Key, a.Signal) }
