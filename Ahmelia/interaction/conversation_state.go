package interaction

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ObjectiveDraft is a conversation-owned draft for an unfinished delegated objective.
// It is deliberately not WorkSession: no capability, provider, authorization,
// execution or production lifecycle is created until the objective is complete.
type ObjectiveDraft struct {
	ID         string    `json:"id"`
	Text       string    `json:"text"`
	SourceTurn uint64    `json:"source_turn,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	Fragments  int       `json:"fragments,omitempty"`
}

func cloneObjectiveDraft(in *ObjectiveDraft) *ObjectiveDraft {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}

// ConversationState is the durable semantic state of the current dialogue.
// It is deliberately independent from any capability or production system.
type ConversationState struct {
	mu sync.RWMutex

	SessionID          string
	Turns              []TurnRecord
	ActiveTopic        string
	ActiveObjective    string
	ActiveThreadID     string
	ActiveWorkID       string `json:"active_work_id,omitempty"`
	Language           string
	OpenThreads        []ConversationThread
	Decisions          []ConversationDecision
	Commitments        []ConversationCommitment
	Objectives         []InteractionObjective `json:"objectives,omitempty"`
	WorkLinks          []ConversationWorkLink `json:"work_links,omitempty"`
	PendingObjective   *ObjectiveDraft        `json:"pending_objective,omitempty"`
	OpenQuestions      []OpenQuestion         `json:"open_questions,omitempty"`
	NextOpenQuestionID uint64                 `json:"next_open_question_id,omitempty"`
	LastUserInput      string
	LastAssistant      string
	LastTurnAt         time.Time
	UserState          UserState
}

// TurnRecord keeps the meaning of a turn, not only its raw text.
type TurnRecord struct {
	ID                uint64
	At                time.Time
	UserText          string
	Assistant         string
	Kind              RequestKind
	SpeechAct         SpeechAct
	CommunicativeActs CommunicativeActs `json:"communicative_acts,omitempty"`
	// Subject is the canonical ownership boundary for person-referential
	// semantics. Persisting it prevents the user/self distinction from being
	// lost when a turn crosses the conversation-state boundary.
	Subject      UserObservationSubject `json:"subject,omitempty"`
	Language     string
	Topic        string
	References   []Reference
	Confidence   float64
	Interrupted  bool
	Relation     TurnRelation `json:"relation,omitempty"`
	ParentTurnID uint64       `json:"parent_turn_id,omitempty"`
	Objective    string       `json:"objective,omitempty"`
}

// ConversationThread represents a topic that can remain alive while the
// user temporarily changes subject.
type ConversationThread struct {
	ID        string
	Topic     string
	Objective string
	Status    string // active | open | paused | completed | abandoned
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ConversationDecision is a decision made during conversation, so later
// pronouns such as "la segunda" can be resolved against something concrete.
type ConversationDecision struct {
	Topic      string
	Decision   string
	SourceTurn uint64
	At         time.Time
}

// ConversationCommitment records something Ahmelia or the user committed to.
type ConversationCommitment struct {
	Owner      string // user | ahmelia | shared
	Text       string
	Status     string // open | fulfilled | cancelled
	SourceTurn uint64
	At         time.Time
}

func NewConversationState() *ConversationState {
	return &ConversationState{
		SessionID:     newConversationID(),
		Turns:         make([]TurnRecord, 0, 64),
		OpenThreads:   make([]ConversationThread, 0, 8),
		Decisions:     make([]ConversationDecision, 0, 32),
		Commitments:   make([]ConversationCommitment, 0, 32),
		OpenQuestions: make([]OpenQuestion, 0, 8),
	}
}

// RecordTurn appends semantic history and updates the active conversational
// focus. It never selects or executes a domain capability.
func (s *ConversationState) RecordTurn(req SemanticRequest, assistant string) uint64 {
	return s.RecordTurnLinked(req, assistant, RelationIndependent, 0)
}

// RecordTurnLinked records how the turn participates in the dialogue graph.
// The relationship is durable so a short answer can remain attached to the
// question/objective that gave it meaning after the current call returns.
func (s *ConversationState) RecordTurnLinked(req SemanticRequest, assistant string, relation TurnRelation, parentTurnID uint64) uint64 {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	// Update interaction signals as part of the same durable turn transition.
	ctx := ConversationSnapshot{ActiveTopic: s.ActiveTopic, ActiveObjective: s.ActiveObjective, ActiveThreadID: s.ActiveThreadID, ActiveWorkID: s.ActiveWorkID, Language: s.Language, LastUserInput: s.LastUserInput, LastAssistant: s.LastAssistant, LastTurnAt: s.LastTurnAt, UserState: s.UserState, PendingObjective: cloneObjectiveDraft(s.PendingObjective)}
	assessment := AnalyzeUserState(req, ctx)
	s.UserState = assessment.Current

	id := uint64(len(s.Turns) + 1)
	topic := req.Topic
	if topic == "" && !CanonicalActs(req).Has(ActSocial) && !CanonicalActs(req).Has(ActQuestion) {
		topic = s.ActiveTopic
	}
	objective := strings.TrimSpace(req.Objective.Text)
	if objective == "" {
		objective = s.ActiveObjective
	}

	acts := cloneCommunicativeActs(CanonicalActs(req))
	s.Turns = append(s.Turns, TurnRecord{
		ID: id, At: time.Now(), UserText: req.Raw, Assistant: assistant,
		Kind: req.Kind, SpeechAct: req.SpeechAct, CommunicativeActs: acts, Subject: req.Subject, Language: req.Language,
		Topic: topic, References: cloneReferences(req.References),
		Confidence: req.Confidence, Relation: relation, ParentTurnID: parentTurnID,
		Objective: objective,
	})
	if len(s.Turns) > 256 {
		s.Turns = append([]TurnRecord(nil), s.Turns[len(s.Turns)-256:]...)
	}

	acts = CanonicalActs(req)
	if acts.Has(ActCommand) || acts.Has(ActContinuation) || acts.Has(ActProposal) {
		// A delegation preamble is conversationally real but not yet an
		// accepted operational objective. Keep it in PendingObjective instead
		// of promoting an incomplete phrase to ActiveObjective.
		if strings.TrimSpace(req.Objective.Text) != "" && !NeedsObjectiveAssembly(req) {
			s.ActiveObjective = strings.TrimSpace(req.Objective.Text)
		}
	}
	if topic != "" {
		s.ActiveTopic = topic
	}
	if req.Language != "" {
		s.Language = req.Language
	}
	s.LastUserInput = req.Raw
	s.LastAssistant = assistant
	s.LastTurnAt = time.Now()
	return id
}

// RecordAssistantInitiative records an autonomous conversational turn without
// fabricating a user utterance. It is owned by ConversationState so initiative
// history remains visible to B96.37.13 cooldown/load evaluation.
func (s *ConversationState) RecordAssistantInitiative(text, topic string) uint64 {
	if s == nil || strings.TrimSpace(text) == "" {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	id := uint64(len(s.Turns) + 1)
	s.Turns = append(s.Turns, TurnRecord{ID: id, At: now, Assistant: strings.TrimSpace(text), Kind: KindConversation, SpeechAct: SpeechInform, Language: s.Language, Topic: strings.TrimSpace(topic), Objective: s.ActiveObjective})
	if len(s.Turns) > 256 {
		s.Turns = append([]TurnRecord(nil), s.Turns[len(s.Turns)-256:]...)
	}
	s.LastAssistant = strings.TrimSpace(text)
	s.LastTurnAt = now
	return id
}

func (s *ConversationState) SetAssistantResponse(text string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.LastAssistant = strings.TrimSpace(text)
	if len(s.Turns) > 0 {
		s.Turns[len(s.Turns)-1].Assistant = s.LastAssistant
	}
}

func (s *ConversationState) Snapshot() ConversationSnapshot {
	if s == nil {
		return ConversationSnapshot{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := ConversationSnapshot{
		SessionID: s.SessionID, ActiveTopic: s.ActiveTopic,
		ActiveObjective: s.ActiveObjective, ActiveThreadID: s.ActiveThreadID, ActiveWorkID: s.ActiveWorkID, Language: s.Language,
		LastUserInput: s.LastUserInput, LastAssistant: s.LastAssistant,
		LastTurnAt: s.LastTurnAt, UserState: s.UserState,
	}
	out.Turns = cloneTurnRecords(s.Turns)
	out.OpenThreads = append([]ConversationThread(nil), s.OpenThreads...)
	out.Decisions = append([]ConversationDecision(nil), s.Decisions...)
	out.Commitments = append([]ConversationCommitment(nil), s.Commitments...)
	out.Objectives = append([]InteractionObjective(nil), s.Objectives...)
	out.WorkLinks = append([]ConversationWorkLink(nil), s.WorkLinks...)
	out.PendingObjective = cloneObjectiveDraft(s.PendingObjective)
	out.OpenQuestions = append([]OpenQuestion(nil), s.OpenQuestions...)
	return out
}

func cloneCommunicativeActs(in CommunicativeActs) CommunicativeActs {
	out := in
	out.Secondary = append([]CommunicativeAct(nil), in.Secondary...)
	out.Evidence = append([]CommunicativeActEvidence(nil), in.Evidence...)
	return out
}

func cloneReferences(in []Reference) []Reference {
	return append([]Reference(nil), in...)
}

func cloneTurnRecords(in []TurnRecord) []TurnRecord {
	if in == nil {
		return nil
	}
	out := make([]TurnRecord, len(in))
	for i, turn := range in {
		out[i] = turn
		out[i].CommunicativeActs = cloneCommunicativeActs(turn.CommunicativeActs)
		out[i].References = cloneReferences(turn.References)
	}
	return out
}

type ConversationSnapshot struct {
	SessionID        string
	Turns            []TurnRecord
	ActiveTopic      string
	ActiveObjective  string
	ActiveThreadID   string
	ActiveWorkID     string `json:"active_work_id,omitempty"`
	Language         string
	OpenThreads      []ConversationThread
	Decisions        []ConversationDecision
	Commitments      []ConversationCommitment
	Objectives       []InteractionObjective `json:"objectives,omitempty"`
	WorkLinks        []ConversationWorkLink `json:"work_links,omitempty"`
	PendingObjective *ObjectiveDraft        `json:"pending_objective,omitempty"`
	OpenQuestions    []OpenQuestion         `json:"open_questions,omitempty"`
	LastUserInput    string
	LastAssistant    string
	LastTurnAt       time.Time
	UserState        UserState
}

func (s *ConversationState) AddDecision(topic, decision string, turnID uint64) {
	if s == nil || strings.TrimSpace(decision) == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Decisions = append(s.Decisions, ConversationDecision{
		Topic: topic, Decision: decision, SourceTurn: turnID, At: time.Now(),
	})
	if len(s.Decisions) > 64 {
		s.Decisions = s.Decisions[len(s.Decisions)-64:]
	}
}

// RegisterOpenQuestion records that Ahmelia asked the user a question and is
// waiting for the answer. A previous open question for the same key is
// cancelled: only the latest ask can admit an answer. The question stores the
// semantic key/category resolved from the user's own query, so the answer
// does not need to re-derive them.
func (s *ConversationState) RegisterOpenQuestion(kind, category, key, question string, turnID uint64) uint64 {
	if s == nil || strings.TrimSpace(key) == "" {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for i := range s.OpenQuestions {
		q := &s.OpenQuestions[i]
		if q.Status == OpenQuestionOpen && q.Key == strings.TrimSpace(key) {
			q.Status = OpenQuestionCancelled
			q.ClosedAt = now
		}
	}
	if len(s.OpenQuestions) >= 32 {
		s.OpenQuestions = append([]OpenQuestion(nil), s.OpenQuestions[len(s.OpenQuestions)-31:]...)
	}
	s.NextOpenQuestionID++
	s.OpenQuestions = append(s.OpenQuestions, OpenQuestion{
		ID: s.NextOpenQuestionID, SessionID: s.SessionID, Kind: strings.TrimSpace(kind),
		Category: strings.TrimSpace(category), Key: strings.TrimSpace(key),
		Question: strings.TrimSpace(question), AskedTurn: turnID, AskedAt: now,
		Status: OpenQuestionOpen,
	})
	return s.NextOpenQuestionID
}

// ActiveOpenQuestion returns the newest still-valid question Ahmelia asked,
// expiring entries that no longer qualify: another session, older than the
// freshness window, or too many turns away. Expiry is lazy and visible in the
// persisted state (status "expired").
func (s *ConversationState) ActiveOpenQuestion(now time.Time, currentTurn uint64) (OpenQuestion, bool) {
	if s == nil {
		return OpenQuestion{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var best *OpenQuestion
	for i := range s.OpenQuestions {
		q := &s.OpenQuestions[i]
		if q.Status != OpenQuestionOpen {
			continue
		}
		stale := q.SessionID != s.SessionID ||
			now.Sub(q.AskedAt) > OpenQuestionMaxAge ||
			currentTurn < q.AskedTurn ||
			currentTurn-q.AskedTurn > OpenQuestionMaxTurnDistance
		if stale {
			q.Status = OpenQuestionExpired
			q.ClosedAt = now
			continue
		}
		if best == nil || q.ID > best.ID {
			best = q
		}
	}
	if best == nil {
		return OpenQuestion{}, false
	}
	return *best, true
}

// CloseOpenQuestion closes an open question once it has served its purpose
// (answered) or cannot admit anything anymore (cancelled/expired).
func (s *ConversationState) CloseOpenQuestion(id uint64, status OpenQuestionStatus, turnID uint64) {
	if s == nil || id == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.OpenQuestions {
		if s.OpenQuestions[i].ID == id {
			s.OpenQuestions[i].Status = status
			s.OpenQuestions[i].AnsweredTurn = turnID
			s.OpenQuestions[i].ClosedAt = time.Now()
			return
		}
	}
}

// BeginObjectiveDraft stores an unfinished conversational delegation without
// creating operational work. A second specification turn can extend it until
// the normal execution boundary accepts the completed objective.
func (s *ConversationState) BeginObjectiveDraft(text string, turnID uint64) *ObjectiveDraft {
	if s == nil || strings.TrimSpace(text) == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	if s.PendingObjective != nil && strings.TrimSpace(s.PendingObjective.Text) != "" {
		s.PendingObjective.Text = ComposeObjective(s.PendingObjective.Text, text)
		s.PendingObjective.UpdatedAt = now
		return cloneObjectiveDraft(s.PendingObjective)
	}
	s.PendingObjective = &ObjectiveDraft{ID: newConversationID(), Text: strings.TrimSpace(text), SourceTurn: turnID, CreatedAt: now, UpdatedAt: now, Fragments: 1}
	return cloneObjectiveDraft(s.PendingObjective)
}

// AppendObjectiveDraft extends the existing unfinished delegation. Empty or
// label-only fragments do not mutate the draft.
func (s *ConversationState) AppendObjectiveDraft(text string) *ObjectiveDraft {
	if s == nil || strings.TrimSpace(text) == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.PendingObjective == nil {
		return nil
	}
	merged := ComposeObjective(s.PendingObjective.Text, text)
	if merged != s.PendingObjective.Text {
		s.PendingObjective.Text = merged
		s.PendingObjective.UpdatedAt = time.Now()
		s.PendingObjective.Fragments++
	}
	return cloneObjectiveDraft(s.PendingObjective)
}

func (s *ConversationState) PendingObjectiveDraft() (ObjectiveDraft, bool) {
	if s == nil {
		return ObjectiveDraft{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.PendingObjective == nil || strings.TrimSpace(s.PendingObjective.Text) == "" {
		return ObjectiveDraft{}, false
	}
	return *cloneObjectiveDraft(s.PendingObjective), true
}

// DiscardObjectiveDraft abandons an unfinished conversational delegation
// without touching operational work or the rest of conversation history.
func (s *ConversationState) DiscardObjectiveDraft() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.PendingObjective = nil
}

// CommitObjectiveDraft removes the conversational draft after the completed
// objective has crossed into the normal Work/Execution authority.
func (s *ConversationState) CommitObjectiveDraft() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.PendingObjective = nil
}

// ReconcileObjectiveOwnership repairs the persisted boundary between
// conversational drafts and operational objectives after process restart.
// Older state may contain an incomplete delegation in ActiveObjective even
// though no live Work owns it. The conversation layer remains the authority
// for the draft; this method never creates Work or executes anything.
func (s *ConversationState) ReconcileObjectiveOwnership(hasOperationalWork bool) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if hasOperationalWork {
		return
	}
	// A terminal/missing Work must not remain the implicit active owner after
	// restart. Explicit resume remains possible through the WorkBridge history
	// boundary, but ordinary turns start without operational work attached.
	s.ActiveWorkID = ""
	if s.PendingObjective != nil && strings.TrimSpace(s.PendingObjective.Text) != "" {
		if s.PendingObjective.Fragments < 1 {
			s.PendingObjective.Fragments = 1
		}
		if strings.EqualFold(strings.TrimSpace(s.ActiveObjective), strings.TrimSpace(s.PendingObjective.Text)) {
			s.ActiveObjective = ""
		}
		return
	}
	active := strings.TrimSpace(s.ActiveObjective)
	if active == "" {
		return
	}
	req := Analyze(active, Context{})
	if NeedsObjectiveAssembly(req) {
		now := time.Now()
		s.PendingObjective = &ObjectiveDraft{ID: newConversationID(), Text: active, SourceTurn: 0, CreatedAt: now, UpdatedAt: now, Fragments: 1}
		s.ActiveObjective = ""
	}
}

// AddObjective records an accepted interaction objective and makes it active.
// It does not create operational work or invoke a capability.
func (s *ConversationState) AddObjective(topic, text string, turnID uint64, decision string) {
	if s == nil || strings.TrimSpace(text) == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for i := range s.Objectives {
		if s.Objectives[i].Status == "active" {
			s.Objectives[i].Status = "open"
			s.Objectives[i].UpdatedAt = now
		}
	}
	o := InteractionObjective{ID: newConversationID(), Topic: strings.TrimSpace(topic), Text: strings.TrimSpace(text),
		Status: "active", SourceTurn: turnID, Decision: strings.TrimSpace(decision), CreatedAt: now, UpdatedAt: now}
	s.Objectives = append(s.Objectives, o)
	if len(s.Objectives) > 64 {
		s.Objectives = append([]InteractionObjective(nil), s.Objectives[len(s.Objectives)-64:]...)
	}
	s.ActiveObjective = o.Text
}

// ActiveObjectiveRecord returns the durable objective currently marked active.
func (s *ConversationState) ActiveObjectiveRecord() (InteractionObjective, bool) {
	if s == nil {
		return InteractionObjective{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for i := len(s.Objectives) - 1; i >= 0; i-- {
		if s.Objectives[i].Status == "active" {
			return s.Objectives[i], true
		}
	}
	return InteractionObjective{}, false
}

// CompleteObjective changes objective state only. Work execution remains
// outside the interaction package.
func (s *ConversationState) CompleteObjective(id string) bool {
	if s == nil || strings.TrimSpace(id) == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.Objectives {
		if s.Objectives[i].ID == id && s.Objectives[i].Status == "active" {
			s.Objectives[i].Status = "completed"
			s.Objectives[i].UpdatedAt = time.Now()
			if s.ActiveObjective == s.Objectives[i].Text {
				s.ActiveObjective = ""
			}
			return true
		}
	}
	return false
}

func (s *ConversationState) AddCommitment(owner, text string, turnID uint64) {
	if s == nil || strings.TrimSpace(text) == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Commitments = append(s.Commitments, ConversationCommitment{
		Owner: owner, Text: text, Status: "open", SourceTurn: turnID, At: time.Now(),
	})
}

// SetActiveWork records which operational work session the conversation is
// currently associated with. It does not alter the work session itself.
func (s *ConversationState) SetActiveWork(workID string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ActiveWorkID = strings.TrimSpace(workID)
}

// SetActiveObjective records the current semantic objective without owning
// operational work. It is used when a verified post-execution correction
// replaces the objective represented by the existing work session.
func (s *ConversationState) SetActiveObjective(objective string) {
	if s == nil {
		return
	}
	objective = strings.TrimSpace(objective)
	if objective == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ActiveObjective = objective
}

// RecordWorkLink persists the conversation↔work relationship for audit and
// later reference resolution. Older links are retained as history.
func (s *ConversationState) RecordWorkLink(link ConversationWorkLink) {
	if s == nil || strings.TrimSpace(link.WorkID) == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if link.At.IsZero() {
		link.At = time.Now()
	}
	s.WorkLinks = append(s.WorkLinks, link)
	if len(s.WorkLinks) > 128 {
		s.WorkLinks = append([]ConversationWorkLink(nil), s.WorkLinks[len(s.WorkLinks)-128:]...)
	}
	if link.Relation != WorkRelationPreserve {
		s.ActiveWorkID = strings.TrimSpace(link.WorkID)
	}
}

// ActiveWork returns the last operational work session associated with the
// conversation, without claiming that it is currently executing.
func (s *ConversationState) ActiveWork() string {
	if s == nil {
		return ""
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ActiveWorkID
}

func (s *ConversationState) OpenThread(topic, objective string) string {
	if s == nil || strings.TrimSpace(topic) == "" {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	id := newConversationID()
	now := time.Now()
	s.OpenThreads = append(s.OpenThreads, ConversationThread{
		ID: id, Topic: strings.TrimSpace(topic), Objective: strings.TrimSpace(objective),
		Status: "active", CreatedAt: now, UpdatedAt: now,
	})
	return id
}

func (s *ConversationState) PauseActiveThread() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.OpenThreads {
		if s.OpenThreads[i].Status == "active" {
			s.OpenThreads[i].Status = "open"
			s.OpenThreads[i].UpdatedAt = time.Now()
		}
	}
}

func (s *ConversationState) ResumeThread(topic string) bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	target := strings.ToLower(strings.TrimSpace(topic))
	for i := range s.OpenThreads {
		if strings.Contains(strings.ToLower(s.OpenThreads[i].Topic), target) ||
			strings.Contains(target, strings.ToLower(s.OpenThreads[i].Topic)) {
			s.OpenThreads[i].Status = "active"
			s.OpenThreads[i].UpdatedAt = time.Now()
			s.ActiveTopic = s.OpenThreads[i].Topic
			if s.OpenThreads[i].Objective != "" {
				s.ActiveObjective = s.OpenThreads[i].Objective
			}
			return true
		}
	}
	return false
}

func (s *ConversationState) OpenThreadsSnapshot() []ConversationThread {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := append([]ConversationThread(nil), s.OpenThreads...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].UpdatedAt.Before(out[j].UpdatedAt) })
	return out
}

// ActivateThread records an explicit conversational focus without deleting
// other open threads. A thread is a semantic lane, not a capability.
func (s *ConversationState) ActivateThread(threadID string) bool {
	if s == nil || strings.TrimSpace(threadID) == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.OpenThreads {
		if s.OpenThreads[i].ID != threadID {
			continue
		}
		for j := range s.OpenThreads {
			if s.OpenThreads[j].Status == "active" {
				s.OpenThreads[j].Status = "open"
			}
		}
		s.OpenThreads[i].Status = "active"
		s.OpenThreads[i].UpdatedAt = time.Now()
		s.ActiveThreadID = threadID
		s.ActiveTopic = s.OpenThreads[i].Topic
		if s.OpenThreads[i].Objective != "" {
			s.ActiveObjective = s.OpenThreads[i].Objective
		}
		return true
	}
	return false
}

// FindThread returns the best existing semantic lane for a topic. It does
// not create a new lane merely because a word overlaps; callers decide when
// a new topic is substantial enough to deserve its own thread.
func (s *ConversationState) FindThread(topic string) (ConversationThread, bool) {
	if s == nil || strings.TrimSpace(topic) == "" {
		return ConversationThread{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	target := strings.ToLower(strings.TrimSpace(topic))
	var best ConversationThread
	bestScore := 0
	for _, t := range s.OpenThreads {
		if t.Status == "completed" || t.Status == "abandoned" {
			continue
		}
		tt := strings.ToLower(strings.TrimSpace(t.Topic))
		score := 0
		if tt == target {
			score = 3
		} else if strings.Contains(tt, target) || strings.Contains(target, tt) {
			score = 2
		}
		if score > 0 && t.Status == "active" {
			score++
		}
		if score > bestScore {
			best, bestScore = t, score
		}
	}
	return best, bestScore > 0
}

// EnsureThread keeps the current objective attached to a semantic lane.
// Existing lanes are reused; genuinely new topics create a new lane while
// preserving the previous one as open.
func (s *ConversationState) EnsureThread(topic, objective string) ConversationThread {
	if s == nil || strings.TrimSpace(topic) == "" {
		return ConversationThread{}
	}
	if existing, ok := s.FindThread(topic); ok {
		_ = s.ActivateThread(existing.ID)
		return existing
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.OpenThreads {
		if s.OpenThreads[i].Status == "active" {
			s.OpenThreads[i].Status = "open"
		}
	}
	now := time.Now()
	t := ConversationThread{
		ID: newConversationID(), Topic: strings.TrimSpace(topic),
		Objective: strings.TrimSpace(objective), Status: "active",
		CreatedAt: now, UpdatedAt: now,
	}
	s.OpenThreads = append(s.OpenThreads, t)
	s.ActiveThreadID = t.ID
	s.ActiveTopic = t.Topic
	if t.Objective != "" {
		s.ActiveObjective = t.Objective
	}
	return t
}

// ClearActiveContext ends the current semantic focus without deleting the
// thread or its history. The previous lane remains resumable, but it cannot
// leak into an unrelated turn after an explicit topic boundary.
func (s *ConversationState) ClearActiveContext() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.OpenThreads {
		if s.OpenThreads[i].Status == "active" {
			s.OpenThreads[i].Status = "open"
			s.OpenThreads[i].UpdatedAt = time.Now()
		}
	}
	s.ActiveThreadID = ""
	s.ActiveTopic = ""
	s.ActiveObjective = ""
}

// ActiveThread returns the currently focused semantic lane.
func (s *ConversationState) ActiveThread() (ConversationThread, bool) {
	if s == nil {
		return ConversationThread{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.ActiveThreadID != "" {
		for _, t := range s.OpenThreads {
			if t.ID == s.ActiveThreadID && t.Status != "completed" && t.Status != "abandoned" {
				return t, true
			}
		}
	}
	for _, t := range s.OpenThreads {
		if t.Status == "active" {
			return t, true
		}
	}
	return ConversationThread{}, false
}

const ConversationStatePath = "memory/conversation_state.json"

// Save persists semantic conversation state. Raw user text is kept because
// it is part of the conversational record; callers should place this file
// under the same local/private memory boundary as the rest of Ahmelia.
func (s *ConversationState) Save(path string) error {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	data, err := json.MarshalIndent(s, "", "  ")
	s.mu.RUnlock()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// LoadConversationState restores durable conversation context when available.
// Corrupt or missing state intentionally falls back to a clean session.
func LoadConversationState(path string) *ConversationState {
	data, err := os.ReadFile(path)
	if err != nil {
		return NewConversationState()
	}
	var s ConversationState
	if err := json.Unmarshal(data, &s); err != nil {
		return NewConversationState()
	}
	if s.Turns == nil {
		s.Turns = make([]TurnRecord, 0, 64)
	}
	if s.OpenThreads == nil {
		s.OpenThreads = make([]ConversationThread, 0, 8)
	}
	if s.Decisions == nil {
		s.Decisions = make([]ConversationDecision, 0, 32)
	}
	if s.Commitments == nil {
		s.Commitments = make([]ConversationCommitment, 0, 32)
	}
	if s.WorkLinks == nil {
		s.WorkLinks = make([]ConversationWorkLink, 0, 32)
	}
	if s.OpenQuestions == nil {
		s.OpenQuestions = make([]OpenQuestion, 0, 8)
	}
	if s.SessionID == "" {
		s.SessionID = newConversationID()
	}
	return &s
}

// LastTurn returns the latest semantic turn, regardless of whether it was
// a question. It is the anchor for conversational repair: "no, me refería
// a..." can correct a command, proposal, answer, or ordinary statement.
func (s *ConversationState) LastTurn() (TurnRecord, bool) {
	if s == nil {
		return TurnRecord{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.Turns) == 0 {
		return TurnRecord{}, false
	}
	return s.Turns[len(s.Turns)-1], true
}

// LastQuestion returns the most recent user turn that actually asked a
// question. This is the anchor used to interpret short replies such as
// "azul", "la segunda" or "sí" as answers instead of unrelated commands.
func (s *ConversationState) LastQuestion() (TurnRecord, bool) {
	if s == nil {
		return TurnRecord{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for i := len(s.Turns) - 1; i >= 0; i-- {
		t := s.Turns[i]
		if t.CommunicativeActs.Has(ActQuestion) || t.CommunicativeActs.Has(ActOpinionRequest) || strings.HasSuffix(strings.TrimSpace(t.Assistant), "?") {
			return t, true
		}
	}
	return TurnRecord{}, false
}

var conversationIDFallback atomic.Uint64

func newConversationID() string {
	// A timestamp alone is not a safe identifier: several threads can be
	// created within the same clock tick, especially on platforms with a
	// coarser wall-clock resolution. IDs are part of conversational identity,
	// so collisions here could merge two independent dialogue lanes.
	var b [12]byte
	stamp := time.Now().UTC().Format("20060102T150405.000000000")
	if _, err := rand.Read(b[:]); err == nil {
		return stamp + "-" + hex.EncodeToString(b[:])
	}
	return stamp + "-" + strconv.FormatUint(conversationIDFallback.Add(1), 10)
}
