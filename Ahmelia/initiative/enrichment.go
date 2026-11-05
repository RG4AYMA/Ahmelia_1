package initiative

import (
	"strings"
	"time"
)

// B96.37.5 — Initiative Source Enrichment.
//
// These structures are read-only projections of already-owned domain state.
// They intentionally contain observations, timestamps and provenance inputs,
// not initiative decisions. Ownership remains with the upstream subsystem.
type InitiativeSourceEnrichment struct {
	ConversationalContext ContextEnrichment
	Cognition             CognitionEnrichment
	UserModel             UserModelEnrichment
	HistoricalMemory      HistoricalMemoryEnrichment
	Work                  WorkEnrichment
	Commitment            CommitmentEnrichment
	Autonomy              AutonomyEnrichment
	Availability          AvailabilityEnrichment
	PrivacySafety         PrivacySafetyEnrichment
	Channel               ChannelEnrichment
}

type ContextEnrichment struct {
	HasContext       bool
	Topic            string
	Objective        string
	ThreadID         string
	OpenThreads      int
	Continues        bool
	BreaksContinuity bool
	ObservedAt       time.Time
}

type CognitionEnrichment struct {
	Valid              bool
	Version            string
	Intent             string
	IntentConfidence   float64
	Goal               string
	Relevance          float64
	NeedsClarification bool
	ActionOriented     bool
	Ambiguous          bool
	ObservedAt         time.Time
}

type UserModelEnrichment struct {
	Available            bool
	ReliableFacts        int
	ReliableGoals        int
	ReliablePreferences  int
	CommunicationProfile bool
	ObservedAt           time.Time
}

type HistoricalMemoryEnrichment struct {
	Available  bool
	Required   bool
	Query      string
	ObservedAt time.Time
}

type WorkEnrichment struct {
	Exists       bool
	ID           string
	Status       string
	CurrentStep  string
	PendingSteps int
	UpdatedAt    time.Time
}

type CommitmentEnrichment struct {
	OpenCount   int
	LatestText  string
	LatestOwner string
	LatestAt    time.Time
}

type AutonomyEnrichment struct {
	ExecutionState  string
	ActiveTask      string
	WaitingForUser  bool
	LastActivity    time.Time
	LastInteraction time.Time
	StaleGoals      int
	BlockedGoals    int
	ObservedAt      time.Time
}

type AvailabilityEnrichment struct {
	Availability     UserAvailability
	Interruptibility Interruptibility
	ConversationLoad ConversationLoad
	ObservedAt       time.Time
}

type PrivacySafetyEnrichment struct {
	Policy           InformationPolicy
	PolicyKnown      bool
	Interruptibility Interruptibility
	ObservedAt       time.Time
}

type ChannelEnrichment struct {
	Channel    string
	Language   string
	Supported  bool
	ObservedAt time.Time
}

func (e InitiativeSourceEnrichment) any() bool {
	return e.ConversationalContext.HasContext || e.Cognition.Valid || e.UserModel.Available ||
		e.HistoricalMemory.Available || e.HistoricalMemory.Required || e.Work.Exists || e.Commitment.OpenCount > 0 ||
		e.Autonomy.ExecutionState != "" || e.Availability.Availability != AvailabilityUnknown || e.PrivacySafety.PolicyKnown ||
		strings.TrimSpace(e.Channel.Channel) != ""
}

func sourceObservedAt(ts time.Time, fallback time.Time) time.Time {
	if !ts.IsZero() {
		return ts
	}
	return fallback
}
