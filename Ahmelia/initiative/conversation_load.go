package initiative

import (
	"errors"
	"strings"
	"time"
)

// B96.37.13 — Conversation Load & Cooldown.
// This layer evaluates conversational pressure only. It never authorizes,
// denies, or selects an initiative decision.
type CooldownState string

const (
	CooldownClear     CooldownState = "CLEAR"
	CooldownActive    CooldownState = "ACTIVE"
	CooldownSaturated CooldownState = "SATURATED"
)

type ConversationTurnObservation struct {
	At        time.Time
	UserText  string
	Assistant string
	SpeechAct string
}

type ConversationLoadAssessment struct {
	Load                    ConversationLoad `json:"load"`
	Cooldown                CooldownState    `json:"cooldown"`
	CooldownActive          bool             `json:"cooldown_active"`
	RecentInitiatives       int              `json:"recent_initiatives"`
	ConsecutiveInitiatives  int              `json:"consecutive_initiatives"`
	RecentUserResponses     int              `json:"recent_user_responses"`
	RecentTurns             int              `json:"recent_turns"`
	InteractionDensity      float64          `json:"interaction_density"`
	UserResponseRate        float64          `json:"user_response_rate"`
	TimeSinceLastInitiative time.Duration    `json:"time_since_last_initiative"`
	RepeatedIntervention    bool             `json:"repeated_intervention"`
	Saturated               bool             `json:"saturated"`
	RecoveryEligible        bool             `json:"recovery_eligible"`
	UrgencyReassessment     bool             `json:"urgency_reassessment"`
	Reasons                 []string         `json:"reasons,omitempty"`
	EvaluatedAt             time.Time        `json:"evaluated_at"`
}

// ConversationLoadPolicy contains thresholds, not decisions.
type ConversationLoadPolicy struct {
	Window               int
	ActiveCooldown       time.Duration
	SaturationCooldown   time.Duration
	HighInitiatives      int
	SaturatedInitiatives int
	HighConsecutive      int
	SaturatedConsecutive int
	HighDensity          float64
	SaturatedDensity     float64
	MinimumRecoveryGap   time.Duration
}

func DefaultConversationLoadPolicy() ConversationLoadPolicy {
	return ConversationLoadPolicy{
		Window: 8, ActiveCooldown: 30 * time.Second, SaturationCooldown: 90 * time.Second,
		HighInitiatives: 3, SaturatedInitiatives: 5, HighConsecutive: 2, SaturatedConsecutive: 4,
		HighDensity: 0.75, SaturatedDensity: 0.90, MinimumRecoveryGap: 45 * time.Second,
	}
}

type ConversationLoadEvaluator struct{ Policy ConversationLoadPolicy }

func NewConversationLoadEvaluator() *ConversationLoadEvaluator {
	p := DefaultConversationLoadPolicy()
	return &ConversationLoadEvaluator{Policy: p}
}

// Evaluate derives load from the conversation snapshot. Initiative history is
// inferred conservatively from assistant turns whose content has initiative
// speech acts, while direct user turns remain first-class feedback.
func (e *ConversationLoadEvaluator) Evaluate(turns []ConversationTurnObservation, op InitiativeOpportunity, now time.Time) ConversationLoadAssessment {
	if now.IsZero() {
		now = time.Now()
	}
	p := DefaultConversationLoadPolicy()
	if e != nil {
		p = e.Policy
	}
	if p.Window <= 0 {
		p.Window = 8
	}
	if len(turns) > p.Window {
		turns = turns[len(turns)-p.Window:]
	}
	a := ConversationLoadAssessment{Load: LoadLow, Cooldown: CooldownClear, EvaluatedAt: now, RecentTurns: len(turns)}
	for _, t := range turns {
		if strings.TrimSpace(t.Assistant) != "" {
			if assistantLooksInitiative(t) {
				a.RecentInitiatives++
			}
		}
		if strings.TrimSpace(t.UserText) != "" {
			a.RecentUserResponses++
		}
	}
	if a.RecentTurns > 0 {
		a.InteractionDensity = float64(a.RecentInitiatives) / float64(a.RecentTurns)
	}
	if a.RecentInitiatives > 0 {
		// Consecutive initiative turns are counted from the tail until a user response.
		for i := len(turns) - 1; i >= 0; i-- {
			if strings.TrimSpace(turns[i].Assistant) == "" || !assistantLooksInitiative(turns[i]) {
				break
			}
			a.ConsecutiveInitiatives++
		}
		for i := len(turns) - 1; i >= 0; i-- {
			if assistantLooksInitiative(turns[i]) {
				a.TimeSinceLastInitiative = now.Sub(turns[i].At)
				if a.TimeSinceLastInitiative < 0 {
					a.TimeSinceLastInitiative = 0
				}
				break
			}
		}
	}
	if a.RecentTurns > 0 {
		a.UserResponseRate = float64(a.RecentUserResponses) / float64(a.RecentTurns)
	}
	if a.RecentInitiatives >= p.SaturatedInitiatives || a.ConsecutiveInitiatives >= p.SaturatedConsecutive || (a.RecentTurns >= 4 && a.InteractionDensity >= p.SaturatedDensity) {
		a.Load, a.Saturated, a.Cooldown = LoadSaturated, true, CooldownSaturated
		a.Reasons = append(a.Reasons, "conversation_saturation")
	} else if a.RecentInitiatives >= p.HighInitiatives || a.ConsecutiveInitiatives >= p.HighConsecutive || (a.RecentTurns >= 4 && a.InteractionDensity >= p.HighDensity) {
		a.Load = LoadHigh
		a.Reasons = append(a.Reasons, "conversation_load_high")
	} else if a.RecentInitiatives > 0 {
		a.Load = LoadNormal
	}
	if a.TimeSinceLastInitiative > 0 && a.TimeSinceLastInitiative < p.ActiveCooldown {
		a.Cooldown, a.CooldownActive = CooldownActive, true
		a.Reasons = append(a.Reasons, "initiative_cooldown")
	}
	if a.Saturated && a.TimeSinceLastInitiative > 0 && a.TimeSinceLastInitiative < p.SaturationCooldown {
		a.Cooldown, a.CooldownActive = CooldownSaturated, true
	}
	a.RepeatedIntervention = a.ConsecutiveInitiatives >= 2 || a.RecentInitiatives >= p.HighInitiatives
	if a.RepeatedIntervention {
		a.Reasons = append(a.Reasons, "repeated_intervention")
	}
	a.RecoveryEligible = a.TimeSinceLastInitiative >= p.MinimumRecoveryGap || a.RecentUserResponses > a.RecentInitiatives
	if a.RecoveryEligible && !a.Saturated {
		a.Reasons = append(a.Reasons, "load_recovery_possible")
	}
	if op.Urgency == UrgencyCritical {
		a.UrgencyReassessment = true
		a.Reasons = append(a.Reasons, "critical_urgency_reassessment")
	}
	return a
}

func assistantLooksInitiative(t ConversationTurnObservation) bool {
	if strings.TrimSpace(t.Assistant) == "" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(t.SpeechAct)) {
	case "question", "propose", "suggest", "warning":
		return true
	}
	// Conservative fallback for older records without SpeechAct metadata.
	text := strings.TrimSpace(t.Assistant)
	return strings.HasSuffix(text, "?") || strings.HasPrefix(strings.ToLower(text), "por cierto")
}

func (a ConversationLoadAssessment) Validate() error {
	switch a.Load {
	case LoadLow, LoadNormal, LoadHigh, LoadSaturated:
	default:
		return errors.New("invalid conversation load")
	}
	switch a.Cooldown {
	case CooldownClear, CooldownActive, CooldownSaturated:
	default:
		return errors.New("invalid cooldown state")
	}
	if a.EvaluatedAt.IsZero() {
		return errors.New("conversation load requires evaluation time")
	}
	if a.InteractionDensity < 0 || a.InteractionDensity > 1 {
		return errors.New("invalid interaction density")
	}
	if a.UserResponseRate < 0 || a.UserResponseRate > 1 {
		return errors.New("invalid user response rate")
	}
	if a.CooldownActive != (a.Cooldown != CooldownClear) {
		return errors.New("cooldown state mismatch")
	}
	return nil
}

func (e *ConversationLoadEvaluator) EnrichOpportunity(op InitiativeOpportunity, turns []ConversationTurnObservation, now time.Time) (InitiativeOpportunity, ConversationLoadAssessment, error) {
	a := e.Evaluate(turns, op, now)
	if err := a.Validate(); err != nil {
		return op, a, err
	}
	op.ConversationLoad = a.Load
	op.CooldownActive = a.CooldownActive
	return op, a, nil
}
