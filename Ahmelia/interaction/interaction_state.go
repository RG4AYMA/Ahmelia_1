package interaction

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// InteractionSource identifies the origin of an interaction. It is broader
// than language and deliberately allows non-conversational inputs.
type InteractionSource string

const (
	InteractionSourceUnknown     InteractionSource = "unknown"
	InteractionSourceVoice       InteractionSource = "voice"
	InteractionSourceText        InteractionSource = "text"
	InteractionSourceUI          InteractionSource = "ui"
	InteractionSourceVision      InteractionSource = "vision"
	InteractionSourceGesture     InteractionSource = "gesture"
	InteractionSourceSensor      InteractionSource = "sensor"
	InteractionSourceEnvironment InteractionSource = "environment"
)

// InteractionStatus describes the situation of an interaction, not Ahmelia's
// internal cognition pipeline. Cognition owns processing stages separately.
type InteractionStatus string

const (
	InteractionIdle        InteractionStatus = "idle"
	InteractionActive      InteractionStatus = "active"
	InteractionWaiting     InteractionStatus = "waiting"
	InteractionInterrupted InteractionStatus = "interrupted"
	InteractionEnded       InteractionStatus = "ended"
)

// InteractionFocus is the cross-modal focus shared by interaction consumers.
// It is a coordination view, not a second owner of objectives or work.
type InteractionFocus struct {
	Topic     string
	Objective string
	WorkID    string
	ThreadID  string
}

// ActiveInteractionContext is the focused semantic context of the current
// interaction. It is intentionally smaller than ConversationSnapshot: it
// answers what is active now without becoming a second history store.
//
// The fields are a coordination view. ConversationState remains authoritative
// for turns/threads, the work system remains authoritative for work lifecycle,
// and cognition remains authoritative for reasoning state.
type ActiveInteractionContext struct {
	SessionID       string
	Source          InteractionSource
	Status          InteractionStatus
	TurnID          uint64
	Topic           string
	Objective       string
	ThreadID        string
	WorkID          string
	WorkStatus      string
	WorkProvider    string
	CurrentStep     string
	LastResult      string
	RequestKind     RequestKind
	SpeechAct       SpeechAct
	InteractionNeed InteractionNeed
	Confidence      float64
	UnresolvedRefs  int
	HasActiveThread bool
	References      []InteractionReference
	Correction      InteractionCorrection
	UpdatedAt       time.Time
}

// InteractionMetadata contains transport/runtime facts that should not be
// confused with semantic meaning or conversational history.
type InteractionMetadata struct {
	Source      InteractionSource
	Status      InteractionStatus
	StartedAt   time.Time
	UpdatedAt   time.Time
	TurnID      uint64
	HasSemantic bool
}

// InteractionStateSnapshot is the domain-neutral view of the current
// interaction situation. ConversationState, WorkStore, UserProfile and any
// future perception/environment systems remain authoritative for their own
// data; this type only coordinates their current views.
// InteractionResumePoint is a temporary coordination anchor for an interrupted interaction.
type InteractionResumePoint struct {
	ID        string
	Reason    string
	Status    InteractionStatus
	Focus     InteractionFocus
	Semantic  SemanticRequest
	Work      WorkContext
	HasWork   bool
	TurnID    uint64
	CreatedAt time.Time
}

type InteractionStateSnapshot struct {
	SessionID    string
	Metadata     InteractionMetadata
	Focus        InteractionFocus
	Semantic     SemanticRequest
	Conversation ConversationSnapshot
	Work         WorkContext
	HasWork      bool
}

// InteractionState is the cross-modal interaction coordinator. It does not
// replace ConversationState and does not own work lifecycle, memory, user
// profile, environment, or cognition state.
type InteractionState struct {
	mu sync.RWMutex

	conversation  *ConversationState
	snapshot      InteractionStateSnapshot
	interruptions []InteractionResumePoint
}

const InteractionStatePath = "memory/interaction_state.json"

// interactionStatePersistence is the durable envelope for the interaction
// coordinator. ConversationState remains authoritative for semantic history;
// the envelope merely restores the coordinator and its current projection.
type interactionStatePersistence struct {
	Version       int                      `json:"version"`
	Conversation  *ConversationState       `json:"conversation"`
	Snapshot      InteractionStateSnapshot `json:"snapshot"`
	Interruptions []InteractionResumePoint `json:"interruptions,omitempty"`
}

// Save persists the interaction coordinator so an interaction can survive a
// process/session boundary. It stores semantic state and temporary resume
// anchors, but does not persist cognition internals or claim work ownership.
// The write is atomic to avoid leaving a partially written interaction state.
func (s *InteractionState) Save(path string) error {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	payload := interactionStatePersistence{
		Version:       1,
		Snapshot:      cloneInteractionStateSnapshot(s.snapshot),
		Interruptions: append([]InteractionResumePoint(nil), s.interruptions...),
	}
	if s.conversation != nil {
		// Never copy ConversationState itself: it owns a sync.RWMutex. Build a
		// detached persistence value field-by-field instead.
		conv := &ConversationState{
			SessionID: s.conversation.SessionID, ActiveTopic: s.conversation.ActiveTopic,
			ActiveObjective: s.conversation.ActiveObjective, ActiveThreadID: s.conversation.ActiveThreadID,
			ActiveWorkID: s.conversation.ActiveWorkID, Language: s.conversation.Language,
			LastUserInput: s.conversation.LastUserInput, LastAssistant: s.conversation.LastAssistant,
			LastTurnAt: s.conversation.LastTurnAt, UserState: s.conversation.UserState,
			Turns:       append([]TurnRecord(nil), s.conversation.Turns...),
			OpenThreads: append([]ConversationThread(nil), s.conversation.OpenThreads...),
			Decisions:   append([]ConversationDecision(nil), s.conversation.Decisions...),
			Commitments: append([]ConversationCommitment(nil), s.conversation.Commitments...),
			Objectives:  append([]InteractionObjective(nil), s.conversation.Objectives...),
			WorkLinks:   append([]ConversationWorkLink(nil), s.conversation.WorkLinks...),
		}
		payload.Conversation = conv
	}
	s.mu.RUnlock()

	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".interaction-state-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	// Re-apply the private mode after rename. Replacing an existing file can
	// otherwise preserve the destination's previous permissions on POSIX.
	// Windows does not expose POSIX permission bits reliably, so this call is
	// best-effort there.
	if err := os.Chmod(path, 0600); err != nil {
		return err
	}
	return nil
}

// LoadInteractionState restores the interaction coordinator when durable
// state is available. Missing or corrupt state intentionally falls back to a
// clean coordinator, matching ConversationState's recovery semantics.
func LoadInteractionState(path string) *InteractionState {
	data, err := os.ReadFile(path)
	if err != nil {
		return NewInteractionState(NewConversationState())
	}
	var payload interactionStatePersistence
	if err := json.Unmarshal(data, &payload); err != nil || payload.Version != 1 {
		return NewInteractionState(NewConversationState())
	}
	conversation := payload.Conversation
	if conversation == nil {
		conversation = NewConversationState()
	}
	if conversation.SessionID == "" {
		conversation.SessionID = newConversationID()
	}
	if conversation.Turns == nil {
		conversation.Turns = make([]TurnRecord, 0, 64)
	}
	if conversation.OpenThreads == nil {
		conversation.OpenThreads = make([]ConversationThread, 0, 8)
	}
	if conversation.Decisions == nil {
		conversation.Decisions = make([]ConversationDecision, 0, 32)
	}
	if conversation.Commitments == nil {
		conversation.Commitments = make([]ConversationCommitment, 0, 32)
	}
	if conversation.Objectives == nil {
		conversation.Objectives = make([]InteractionObjective, 0, 32)
	}
	if conversation.WorkLinks == nil {
		conversation.WorkLinks = make([]ConversationWorkLink, 0, 32)
	}

	state := &InteractionState{
		conversation:  conversation,
		snapshot:      cloneInteractionStateSnapshot(payload.Snapshot),
		interruptions: append([]InteractionResumePoint(nil), payload.Interruptions...),
	}
	// ConversationState is authoritative after restoration. Refresh the
	// projection from it while preserving the persisted semantic/work view.
	state.snapshot.Conversation = conversation.Snapshot()
	if state.snapshot.SessionID == "" {
		state.snapshot.SessionID = conversation.SessionID
	}
	if state.snapshot.Metadata.UpdatedAt.IsZero() {
		state.snapshot.Metadata.UpdatedAt = time.Now()
	}
	return state
}

func cloneInteractionStateSnapshot(in InteractionStateSnapshot) InteractionStateSnapshot {
	out := in
	out.Conversation.Turns = append([]TurnRecord(nil), in.Conversation.Turns...)
	out.Conversation.OpenThreads = append([]ConversationThread(nil), in.Conversation.OpenThreads...)
	out.Conversation.Decisions = append([]ConversationDecision(nil), in.Conversation.Decisions...)
	out.Conversation.Commitments = append([]ConversationCommitment(nil), in.Conversation.Commitments...)
	out.Conversation.Objectives = append([]InteractionObjective(nil), in.Conversation.Objectives...)
	out.Conversation.WorkLinks = append([]ConversationWorkLink(nil), in.Conversation.WorkLinks...)
	out.Semantic.References = append([]Reference(nil), in.Semantic.References...)
	out.Semantic.Entities = append([]string(nil), in.Semantic.Entities...)
	out.Semantic.Objective.Constraints = append([]string(nil), in.Semantic.Objective.Constraints...)
	return out
}

func NewInteractionState(conversation *ConversationState) *InteractionState {
	s := &InteractionState{conversation: conversation}
	s.snapshot.Metadata.Status = InteractionIdle
	if conversation != nil {
		s.snapshot.SessionID = conversation.Snapshot().SessionID
	}
	return s
}

// Observe updates the coordination view from already-authoritative state.
// It never mutates ConversationState or the supplied WorkContext.
func (s *InteractionState) Observe(req SemanticRequest, work WorkContext, hasWork bool) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	conv := ConversationSnapshot{}
	if s.conversation != nil {
		conv = s.conversation.Snapshot()
	}

	now := time.Now()
	if s.snapshot.Metadata.StartedAt.IsZero() {
		s.snapshot.Metadata.StartedAt = now
	}
	s.snapshot.Metadata.UpdatedAt = now
	s.snapshot.Metadata.Source = interactionSource(req.Channel)
	s.snapshot.Metadata.Status = InteractionActive
	s.snapshot.Metadata.TurnID = lastTurnID(conv)
	s.snapshot.Metadata.HasSemantic = strings.TrimSpace(req.Raw) != "" || CanonicalActs(req).Primary != ActUnknown
	s.snapshot.SessionID = conv.SessionID
	s.snapshot.Semantic = req
	s.snapshot.Conversation = conv
	s.snapshot.Work = work
	s.snapshot.HasWork = hasWork && strings.TrimSpace(work.ID) != ""
	s.snapshot.Focus = InteractionFocus{
		Topic:     req.Topic,
		Objective: chooseInteractionObjective(req, conv),
		WorkID:    chooseInteractionWorkID(work, conv),
		ThreadID:  conv.ActiveThreadID,
	}
}

// SetStatus changes only the interaction coordination status. It does not
// advance or rewind cognition stages.
func (s *InteractionState) SetStatus(status InteractionStatus) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snapshot.Metadata.Status = status
	s.snapshot.Metadata.UpdatedAt = time.Now()
}

// ActiveContext returns the semantic context that is active for the current
// interaction. It is derived from the latest coordination snapshot and never
// creates or persists a parallel context store.
func (s *InteractionState) ActiveContext() ActiveInteractionContext {
	if s == nil {
		return ActiveInteractionContext{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return activeContextFromSnapshot(s.snapshot)
}

func activeContextFromSnapshot(s InteractionStateSnapshot) ActiveInteractionContext {
	ctx := ActiveInteractionContext{
		SessionID:    s.SessionID,
		Source:       s.Metadata.Source,
		Status:       s.Metadata.Status,
		TurnID:       s.Metadata.TurnID,
		Topic:        strings.TrimSpace(s.Focus.Topic),
		Objective:    strings.TrimSpace(s.Focus.Objective),
		ThreadID:     strings.TrimSpace(s.Focus.ThreadID),
		WorkID:       strings.TrimSpace(s.Focus.WorkID),
		WorkStatus:   strings.TrimSpace(s.Work.Status),
		WorkProvider: strings.TrimSpace(s.Work.Provider),
		CurrentStep:  strings.TrimSpace(s.Work.CurrentStep),
		LastResult:   strings.TrimSpace(s.Work.LastResult),
		RequestKind:  s.Semantic.Kind,
		SpeechAct:    s.Semantic.SpeechAct,
		Confidence:   s.Semantic.Confidence,
		References:   projectInteractionReferences(s.Semantic.References, s.Metadata.TurnID, s.Metadata.UpdatedAt),
		Correction:   projectInteractionCorrection(s.Semantic.Correction, s.Semantic.CorrectionTarget, s.Metadata.TurnID, s.Metadata.UpdatedAt),
		UpdatedAt:    s.Metadata.UpdatedAt,
	}
	for _, ref := range s.Semantic.References {
		if strings.TrimSpace(ref.Expression) != "" && !ref.Resolved {
			ctx.UnresolvedRefs++
		}
	}
	ctx.InteractionNeed = s.Conversation.UserState.InteractionNeed
	if ctx.ThreadID != "" {
		for _, thread := range s.Conversation.OpenThreads {
			if thread.ID == ctx.ThreadID && thread.Status != "completed" && thread.Status != "abandoned" {
				ctx.HasActiveThread = true
				break
			}
		}
	}
	return ctx
}

// Interrupt captures the current interaction anchor without changing authoritative conversation or work state.
func (s *InteractionState) Interrupt(reason string) InteractionResumePoint {
	if s == nil {
		return InteractionResumePoint{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	point := InteractionResumePoint{ID: newConversationID(), Reason: strings.TrimSpace(reason), Status: s.snapshot.Metadata.Status, Focus: s.snapshot.Focus, Semantic: s.snapshot.Semantic, Work: s.snapshot.Work, HasWork: s.snapshot.HasWork, TurnID: s.snapshot.Metadata.TurnID, CreatedAt: now}
	s.interruptions = append(s.interruptions, point)
	s.snapshot.Metadata.Status = InteractionInterrupted
	s.snapshot.Metadata.UpdatedAt = now
	return point
}

// Resume restores the most recent interaction anchor while refreshing the latest authoritative conversation snapshot.
func (s *InteractionState) Resume() (InteractionResumePoint, bool) {
	if s == nil {
		return InteractionResumePoint{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.interruptions) == 0 {
		return InteractionResumePoint{}, false
	}
	idx := len(s.interruptions) - 1
	point := s.interruptions[idx]
	s.interruptions = s.interruptions[:idx]
	if s.conversation != nil {
		if point.Focus.ThreadID != "" {
			s.conversation.ActivateThread(point.Focus.ThreadID)
		}
		s.snapshot.Conversation = s.conversation.Snapshot()
		s.snapshot.SessionID = s.snapshot.Conversation.SessionID
		s.snapshot.Metadata.TurnID = lastTurnID(s.snapshot.Conversation)
	}
	s.snapshot.Focus = point.Focus
	s.snapshot.Semantic = point.Semantic
	s.snapshot.Work = point.Work
	s.snapshot.HasWork = point.HasWork
	s.snapshot.Metadata.Status = InteractionActive
	s.snapshot.Metadata.UpdatedAt = time.Now()
	return point, true
}

// PendingResumePoints returns detached interruption anchors for diagnostics and policy.
func (s *InteractionState) PendingResumePoints() []InteractionResumePoint {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]InteractionResumePoint(nil), s.interruptions...)
}

func (s *InteractionState) Snapshot() InteractionStateSnapshot {
	if s == nil {
		return InteractionStateSnapshot{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := s.snapshot
	out.Conversation.Turns = append([]TurnRecord(nil), s.snapshot.Conversation.Turns...)
	out.Conversation.OpenThreads = append([]ConversationThread(nil), s.snapshot.Conversation.OpenThreads...)
	out.Conversation.Decisions = append([]ConversationDecision(nil), s.snapshot.Conversation.Decisions...)
	out.Conversation.Commitments = append([]ConversationCommitment(nil), s.snapshot.Conversation.Commitments...)
	out.Conversation.WorkLinks = append([]ConversationWorkLink(nil), s.snapshot.Conversation.WorkLinks...)
	out.Semantic.References = append([]Reference(nil), s.snapshot.Semantic.References...)
	out.Semantic.Entities = append([]string(nil), s.snapshot.Semantic.Entities...)
	out.Semantic.Objective.Constraints = append([]string(nil), s.snapshot.Semantic.Objective.Constraints...)
	return out
}

func interactionSource(channel InputChannel) InteractionSource {
	switch channel {
	case ChannelVoice:
		return InteractionSourceVoice
	case ChannelText:
		return InteractionSourceText
	case ChannelUI:
		return InteractionSourceUI
	case ChannelVision:
		return InteractionSourceVision
	case ChannelGesture:
		return InteractionSourceGesture
	default:
		return InteractionSourceUnknown
	}
}

func chooseInteractionObjective(req SemanticRequest, conv ConversationSnapshot) string {
	if v := strings.TrimSpace(req.Objective.Text); v != "" {
		return v
	}
	return strings.TrimSpace(conv.ActiveObjective)
}

func chooseInteractionWorkID(work WorkContext, conv ConversationSnapshot) string {
	if id := strings.TrimSpace(work.ID); id != "" {
		return id
	}
	return strings.TrimSpace(conv.ActiveWorkID)
}

func lastTurnID(conv ConversationSnapshot) uint64 {
	if len(conv.Turns) == 0 {
		return 0
	}
	return conv.Turns[len(conv.Turns)-1].ID
}
