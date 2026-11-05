package core

import (
	"context"
	"fmt"
	"strings"

	"ahmelia/autonomy"
	"ahmelia/capability"
	"ahmelia/interaction"
	"ahmelia/production"
	"ahmelia/reasoning"
)

// DirectLLMBypassResult is the observable result of the diagnostic bypass.
type DirectLLMBypassResult struct {
	Decision reasoning.DirectLLMDecision
	Result   production.AuthorityExecutionResult
	Response string
}

// ProcessDirectLLMBypass deliberately skips the normal semantic/cognitive
// orchestration. The model sees the raw input plus the registered capability
// vocabulary, proposes one action or a response, and execution still crosses
// the canonical ProductionRuntime/ExecutionGuard boundary.
//
// This is an experiment, not a second production interaction path. Normal
// ProcessWithChannel remains untouched unless main.go is explicitly launched
// with AHMELIA_DIRECT_LLM_BYPASS=1.
func (e *Engine) ProcessDirectLLMBypass(ctx context.Context, input string, channel interaction.InputChannel, maxSteps int) (DirectLLMBypassResult, error) {
	if e == nil || e.linguisticClient == nil {
		return DirectLLMBypassResult{}, fmt.Errorf("direct llm bypass: engine/LLM unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if maxSteps < 1 {
		maxSteps = 1
	}
	if maxSteps > 8 {
		maxSteps = 8
	}

	tools := make([]reasoning.DirectLLMTool, 0)
	if e.capabilityRegistry != nil {
		for _, c := range e.capabilityRegistry.List() {
			params := make([]string, 0, len(c.Inputs))
			for _, p := range c.Inputs {
				params = append(params, p.Name)
			}
			tools = append(tools, reasoning.DirectLLMTool{ID: string(c.ID), Operation: c.Action, Description: c.Description, Parameters: params})
		}
	}
	reasoner := reasoning.NewDirectLLMReasoner(e.linguisticClient)
	observation := ""
	var last reasoning.DirectLLMDecision
	var lastResult production.AuthorityExecutionResult

	for step := 1; step <= maxSteps; step++ {
		fmt.Printf("\n========== DIRECT LLM STEP %d ==========\n\nINPUT:\n%s\n\nOBSERVATION:\n%s\n", step, input, obsOrNone(observation))
		decision, err := reasoner.Decide(ctx, input, observation, tools)
		fmt.Printf("\nPROMPT SENT:\n%s\n", reasoner.LastPrompt)
		fmt.Printf("\nMODEL RAW:\n%s\n", reasoner.LastRaw)
		if err != nil {
			fmt.Printf("\nMODEL DECISION: ERROR %v\n========================================\n", err)
			return DirectLLMBypassResult{Decision: decision, Result: lastResult}, err
		}
		last = decision
		fmt.Printf("\nLEDGER: done_so_far=%q remaining=%q\n", decision.DoneSoFar, decision.Remaining)
		fmt.Printf("\nMODEL DECISION: status=%s done=%t confidence=%.2f plan=%v\n", decision.Status, decision.Done, decision.Confidence, decision.Plan)

		if decision.Inconsistent {
			fmt.Printf("\nWARNING: model chose a capability but returned status=%s (nothing will be executed)\n", decision.Status)
		}
		if decision.Status != "act" {
			fmt.Printf("\nRESPONSE:\n%s\n========================================\n", decision.Final)
			return DirectLLMBypassResult{Decision: decision, Result: lastResult, Response: strings.TrimSpace(decision.Final)}, nil
		}
		a := decision.Action
		fmt.Printf("\nACTION: capability=%s operation=%s target=%s params=%v\n", a.CapabilityID, a.Operation, a.Target, a.Parameters)

		req := interaction.SemanticRequest{
			Raw: input, Normalized: strings.ToLower(strings.TrimSpace(input)), Channel: channel,
			Kind:       interaction.KindAction,
			Objective:  interaction.Objective{Text: input, Constraints: nil, OpenEnded: false},
			Frame:      interaction.ObjectiveFrame{PrimaryGoal: input, PrimaryOperation: a.Operation, Subject: a.Target, Confidence: decision.Confidence, Provenance: "direct_llm_bypass"},
			Confidence: decision.Confidence,
			Action:     interaction.ActionIntent{Name: a.Operation, Target: a.Target, Parameters: a.Parameters, Confidence: decision.Confidence},
		}
		c, ok := e.capabilityRegistry.Get(capability.ID(a.CapabilityID))
		if !ok {
			return DirectLLMBypassResult{Decision: decision}, fmt.Errorf("direct llm bypass: unknown capability %q", a.CapabilityID)
		}
		if c.Lane == capability.LaneProduction {
			req.Kind = interaction.KindProduction
		}
		decisionRequest := interaction.CapabilityInteractionDecision{
			Executable: true,
			Request:    interaction.ToCapabilityRequest(req.Action, req.Raw),
			Reason:     "direct_llm_bypass",
		}
		lastResult = e.authorityExecuteWithOptions(decisionRequest, production.ExecutionOptions{
			Context:                     ctx,
			Approval:                    autonomy.ApprovalNone,
			RecoveryAttempt:             1,
			ExperimentalBypassCognition: true,
		}, req)
		observation = formatDirectLLMObservation(lastResult)
		fmt.Printf("\nEXECUTION RESULT:\n%s\n========================================\n", observation)
		if lastResult.AuthorizationRequired {
			return DirectLLMBypassResult{Decision: decision, Result: lastResult}, nil
		}
		// No early exit on success: the model must see the observation and
		// decide "finish" itself. That is the loop being measured.
	}
	return DirectLLMBypassResult{Decision: last, Result: lastResult}, fmt.Errorf("direct llm bypass: max steps (%d) reached without finish/respond", maxSteps)
}

func obsOrNone(o string) string {
	if strings.TrimSpace(o) == "" {
		return "<none>"
	}
	return o
}

func formatDirectLLMObservation(r production.AuthorityExecutionResult) string {
	return fmt.Sprintf("executed=%t succeeded=%t observed=%t verified=%t authorization_required=%t outcome=%s summary=%s failure=%s",
		r.Executed, r.Succeeded, r.Observed, r.Verified, r.AuthorizationRequired, r.OutcomeCode, r.Summary, r.FailureReason)
}
