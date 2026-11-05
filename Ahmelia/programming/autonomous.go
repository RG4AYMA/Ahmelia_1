package programming

import (
	"context"
	"fmt"
	"sync"

	"ahmelia/autonomy"
	"ahmelia/planning"
)

// B96.19 — Autonomous Programming.
//
// Connects the existing autonomy/production authorities to the programming
// domain. It does not create a Programming Brain: GoalSelector chooses the
// goal, GoalPlanningAdapter supplies the existing plan, and the caller-owned
// SoftwareProductionRequest describes the programming work. Execution remains
// owned by B96.18 and authority/recovery remain in B95.

type AutonomousProgrammingRequestProvider interface {
	BuildProgrammingRequest(context.Context, *planning.Goal, *planning.Plan) (SoftwareProductionRequest, error)
}

type AutonomousProgramming struct {
	mu       sync.Mutex
	selector *autonomy.GoalSelector
	planner  *autonomy.GoalPlanningAdapter
	provider AutonomousProgrammingRequestProvider
	loop     *SoftwareProductionLoop
	active   bool
}

func NewAutonomousProgramming(selector *autonomy.GoalSelector, planner *autonomy.GoalPlanningAdapter, provider AutonomousProgrammingRequestProvider, loop *SoftwareProductionLoop) (*AutonomousProgramming, error) {
	if selector == nil {
		return nil, fmt.Errorf("programming: autonomous selector nil")
	}
	if planner == nil {
		return nil, fmt.Errorf("programming: autonomous planner nil")
	}
	if provider == nil {
		return nil, fmt.Errorf("programming: autonomous request provider nil")
	}
	if loop == nil {
		return nil, fmt.Errorf("programming: autonomous production loop nil")
	}
	return &AutonomousProgramming{selector: selector, planner: planner, provider: provider, loop: loop}, nil
}

// RunNext selects an existing goal, obtains its existing plan, translates that
// plan into a programming-domain request, and executes one canonical
// SoftwareProductionLoop attempt. No Goal or Plan is created here.
func (a *AutonomousProgramming) RunNext(ctx context.Context) (SoftwareProductionResult, bool, error) {
	if a == nil {
		return SoftwareProductionResult{}, false, fmt.Errorf("programming: autonomous programming unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return SoftwareProductionResult{}, false, err
	}

	a.mu.Lock()
	if a.active {
		a.mu.Unlock()
		return SoftwareProductionResult{}, false, fmt.Errorf("programming: autonomous programming already running")
	}
	a.active = true
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		a.active = false
		a.mu.Unlock()
	}()

	goal := a.selector.SelectGoal()
	if goal == nil {
		return SoftwareProductionResult{}, false, nil
	}
	plan, err := a.planner.EnsurePlan(goal)
	if err != nil {
		return SoftwareProductionResult{}, false, err
	}
	if plan == nil || plan.GoalID != goal.ID {
		return SoftwareProductionResult{}, false, fmt.Errorf("programming: invalid plan for goal %q", goal.ID)
	}

	req, err := a.provider.BuildProgrammingRequest(ctx, goal, plan)
	if err != nil {
		return SoftwareProductionResult{}, false, err
	}
	if req.ID == "" {
		return SoftwareProductionResult{}, false, fmt.Errorf("programming: provider returned request with empty ID")
	}
	if err := req.Validate(); err != nil {
		return SoftwareProductionResult{}, false, err
	}

	result, err := a.loop.Run(ctx, req)
	return result, true, err
}

func (a *AutonomousProgramming) Active() bool {
	if a == nil {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.active
}

// StaticProgrammingRequestProvider is a deterministic adapter for callers
// that already prepared programming requests for known goal IDs. It does not
// create goals or plans and rejects mismatched goal IDs.
type StaticProgrammingRequestProvider struct {
	requests map[string]SoftwareProductionRequest
}

func NewStaticProgrammingRequestProvider(requests map[string]SoftwareProductionRequest) (*StaticProgrammingRequestProvider, error) {
	if len(requests) == 0 {
		return nil, fmt.Errorf("programming: no autonomous requests configured")
	}
	out := make(map[string]SoftwareProductionRequest, len(requests))
	for goalID, req := range requests {
		if goalID == "" {
			return nil, fmt.Errorf("programming: empty goal ID in request provider")
		}
		if err := req.Validate(); err != nil {
			return nil, err
		}
		out[goalID] = req
	}
	return &StaticProgrammingRequestProvider{requests: out}, nil
}

func (p *StaticProgrammingRequestProvider) BuildProgrammingRequest(_ context.Context, goal *planning.Goal, _ *planning.Plan) (SoftwareProductionRequest, error) {
	if p == nil || goal == nil {
		return SoftwareProductionRequest{}, fmt.Errorf("programming: static request provider unavailable")
	}
	req, ok := p.requests[goal.ID]
	if !ok {
		return SoftwareProductionRequest{}, fmt.Errorf("programming: no request for goal %q", goal.ID)
	}
	return req, nil
}
