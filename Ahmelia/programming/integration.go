package programming

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// B96.20 — Programming Integration.
//
// Final integration boundary for the programming domain. It composes the
// existing programming services without introducing a new authority. B95
// remains responsible for production authority, authorization, safety,
// recovery and final verification decisions.
type ProgrammingIntegration struct {
	loop    *SoftwareProductionLoop
	mu      sync.Mutex
	running bool
}

type ProgrammingIntegrationResult struct {
	RequestID  string
	ProjectID  string
	Status     SoftwareProductionStatus
	Production SoftwareProductionResult
	Stages     []ProgrammingIntegrationStage
	StartedAt  time.Time
	FinishedAt time.Time
}

type ProgrammingIntegrationStage struct {
	Name    string
	Success bool
	Detail  string
}

func NewProgrammingIntegration(loop *SoftwareProductionLoop) (*ProgrammingIntegration, error) {
	if loop == nil {
		return nil, fmt.Errorf("programming integration: production loop nil")
	}
	return &ProgrammingIntegration{loop: loop}, nil
}

// Run executes one canonical programming production flow. It deliberately
// delegates execution to B96.18 rather than duplicating its lifecycle.
func (i *ProgrammingIntegration) Run(ctx context.Context, req SoftwareProductionRequest) (ProgrammingIntegrationResult, error) {
	if i == nil || i.loop == nil {
		return ProgrammingIntegrationResult{}, fmt.Errorf("programming integration: unavailable")
	}
	if err := req.Validate(); err != nil {
		return ProgrammingIntegrationResult{}, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return ProgrammingIntegrationResult{}, err
	}

	i.mu.Lock()
	if i.running {
		i.mu.Unlock()
		return ProgrammingIntegrationResult{}, fmt.Errorf("programming integration: already running")
	}
	i.running = true
	i.mu.Unlock()
	defer func() {
		i.mu.Lock()
		i.running = false
		i.mu.Unlock()
	}()

	started := time.Now().UTC()
	result := ProgrammingIntegrationResult{RequestID: req.ID, ProjectID: req.Project.ID, Status: SoftwareProductionUnknown, StartedAt: started}
	productionResult, err := i.loop.Run(ctx, req)
	if err != nil {
		return ProgrammingIntegrationResult{}, err
	}
	result.Production = productionResult
	result.Status = productionResult.Status

	if productionResult.Analysis != nil {
		result.Stages = append(result.Stages, ProgrammingIntegrationStage{Name: "analysis", Success: true, Detail: "project analysis supplied"})
	}
	if req.Changes != nil {
		ok := productionResult.Changes != nil && productionResult.Changes.Success
		detail := "changes applied"
		if !ok {
			detail = productionResult.FailureReason
		}
		result.Stages = append(result.Stages, ProgrammingIntegrationStage{Name: "changes", Success: ok, Detail: detail})
	}
	if req.Build != nil {
		ok := productionResult.Build != nil && productionResult.Build.Status == BuildSuccess && productionResult.Build.ExitCode == 0
		detail := "build succeeded"
		if !ok {
			detail = "build failed"
		}
		result.Stages = append(result.Stages, ProgrammingIntegrationStage{Name: "build", Success: ok, Detail: detail})
	}
	if req.Tests != nil {
		ok := productionResult.Tests != nil && productionResult.Tests.Status == TestPassed && productionResult.Tests.ExitCode == 0
		detail := "tests passed"
		if !ok {
			detail = "tests failed"
		}
		result.Stages = append(result.Stages, ProgrammingIntegrationStage{Name: "tests", Success: ok, Detail: detail})
	}
	result.Stages = append(result.Stages, ProgrammingIntegrationStage{Name: "verification", Success: productionResult.Verification.Success, Detail: verificationDetail(productionResult.Verification)})
	result.FinishedAt = time.Now().UTC()
	return result, nil
}

func (i *ProgrammingIntegration) Active() bool {
	if i == nil {
		return false
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.running
}

func verificationDetail(e CodeVerificationEvidence) string {
	if e.Success {
		return "code verification succeeded"
	}
	return fmt.Sprintf("code verification failed (%d checks)", len(e.Checks))
}
