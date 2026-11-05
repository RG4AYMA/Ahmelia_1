package interaction

import "strings"

// InteractionInterruptionDecision describes a temporary interaction transition.
type InteractionInterruptionDecision struct {
	Action       string
	ResumeID     string
	Reason       string
	RestoredTurn uint64
	Topic        string
	Objective    string
	ThreadID     string
	WorkID       string
	Confidence   float64
	Success      bool
}

type InteractionInterruptionCoordinator struct{}

func NewInteractionInterruptionCoordinator() *InteractionInterruptionCoordinator {
	return &InteractionInterruptionCoordinator{}
}

func (c *InteractionInterruptionCoordinator) Interrupt(state *InteractionState, reason string) InteractionInterruptionDecision {
	if state == nil {
		return InteractionInterruptionDecision{Action: "interrupt", Reason: "interaction state unavailable", Confidence: 0.1}
	}
	point := state.Interrupt(strings.TrimSpace(reason))
	if point.ID == "" {
		return InteractionInterruptionDecision{Action: "interrupt", Reason: "no resumable interaction anchor available", Confidence: 0.2}
	}
	return InteractionInterruptionDecision{Action: "interrupt", ResumeID: point.ID, Reason: point.Reason, RestoredTurn: point.TurnID, Topic: point.Focus.Topic, Objective: point.Focus.Objective, ThreadID: point.Focus.ThreadID, WorkID: point.Focus.WorkID, Confidence: 0.98, Success: true}
}

func (c *InteractionInterruptionCoordinator) Resume(state *InteractionState) InteractionInterruptionDecision {
	if state == nil {
		return InteractionInterruptionDecision{Action: "resume", Reason: "interaction state unavailable", Confidence: 0.1}
	}
	point, ok := state.Resume()
	if !ok {
		return InteractionInterruptionDecision{Action: "resume", Reason: "no pending interrupted interaction", Confidence: 0.2}
	}
	return InteractionInterruptionDecision{Action: "resume", ResumeID: point.ID, Reason: "interrupted interaction restored", RestoredTurn: point.TurnID, Topic: point.Focus.Topic, Objective: point.Focus.Objective, ThreadID: point.Focus.ThreadID, WorkID: point.Focus.WorkID, Confidence: 0.98, Success: true}
}
