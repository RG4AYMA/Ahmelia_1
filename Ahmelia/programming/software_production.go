package programming

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// B96.18 — Software Production Loop
//
// This is the programming-domain orchestration loop. It specializes B95's
// production lifecycle with the software-specific sequence:
// understand -> change -> build -> test -> verify.
// It does not own retry, recovery, authorization, safety, or final delivery.

type SoftwareProductionRequest struct {
	ID                       string
	AttemptNumber            int
	WorkID                   string
	Project                  CodeProject
	Analysis                 *ProjectAnalysis
	Changes                  *MultiFilePlan
	Build                    *BuildRequest
	Tests                    *TestRequest
	Verification             CodeVerificationRequest
	EffectEvaluationRecorder func([]byte) error
}

func (r SoftwareProductionRequest) Validate() error {
	if strings.TrimSpace(r.ID) == "" {
		return fmt.Errorf("programming software production: ID vacío")
	}
	if err := r.Project.Validate(); err != nil {
		return err
	}
	if r.Build != nil && r.Build.Project.ID != r.Project.ID {
		return fmt.Errorf("programming software production: build project mismatch")
	}
	if r.Tests != nil && r.Tests.Project.ID != r.Project.ID {
		return fmt.Errorf("programming software production: test project mismatch")
	}
	if r.Changes != nil && r.Changes.Project.ID != r.Project.ID {
		return fmt.Errorf("programming software production: changes project mismatch")
	}
	if r.Verification.Project.ID != "" && r.Verification.Project.ID != r.Project.ID {
		return fmt.Errorf("programming software production: verification project mismatch")
	}
	return nil
}

type SoftwareProductionStatus string

const (
	SoftwareProductionUnknown   SoftwareProductionStatus = "unknown"
	SoftwareProductionSucceeded SoftwareProductionStatus = "succeeded"
	SoftwareProductionFailed    SoftwareProductionStatus = "failed"
	SoftwareProductionCancelled SoftwareProductionStatus = "cancelled"
)

type SoftwareProductionResult struct {
	ID                    string
	ProjectID             string
	DependencyPreparation *DependencyResult
	Status                SoftwareProductionStatus
	Analysis              *ProjectAnalysis
	Changes               *MultiFileResult
	Build                 *Build
	Tests                 *TestRun
	Verification          CodeVerificationEvidence
	// ErrorAnalysis is populated when Build/Test/Runtime evidence fails.
	// B96.11 structures the failure; it does not decide recovery.
	ErrorAnalysis *ErrorAnalysis
	FailedStage   string
	FailureReason string
	StartedAt     time.Time
	FinishedAt    time.Time
}

type SoftwareProductionLoop struct {
	Editor             *MultiFileEditor
	DependencyPreparer DependencyPreparer
	BuildExecutor      BuildExecutor
	TestExecutor       TestExecutor
	Verifier           CodeVerifier
	mu                 sync.Mutex
}

func NewSoftwareProductionLoop(editor *MultiFileEditor, build BuildExecutor, tests TestExecutor, verifier CodeVerifier) (*SoftwareProductionLoop, error) {
	if editor == nil {
		return nil, fmt.Errorf("programming software production: editor nil")
	}
	if build == nil {
		return nil, fmt.Errorf("programming software production: build executor nil")
	}
	if tests == nil {
		return nil, fmt.Errorf("programming software production: test executor nil")
	}
	if verifier == nil {
		return nil, fmt.Errorf("programming software production: verifier nil")
	}
	return &SoftwareProductionLoop{
		Editor: editor, DependencyPreparer: NewGoDependencyManager(), BuildExecutor: build, TestExecutor: tests, Verifier: verifier,
	}, nil
}

// Run executes one software-production attempt. Recovery/retry decisions remain
// outside this loop: a failed stage is returned as structured state for B95.
func (l *SoftwareProductionLoop) Run(ctx context.Context, req SoftwareProductionRequest) (SoftwareProductionResult, error) {
	if l == nil {
		return SoftwareProductionResult{}, fmt.Errorf("programming software production: loop unavailable")
	}
	if err := req.Validate(); err != nil {
		return SoftwareProductionResult{}, err
	}
	if ctx == nil {
		ctx = context.Background()
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	result := SoftwareProductionResult{
		ID: req.ID, ProjectID: req.Project.ID, Status: SoftwareProductionUnknown,
		StartedAt: time.Now().UTC(),
	}
	finish := func() SoftwareProductionResult {
		result.FinishedAt = time.Now().UTC()
		return result
	}
	if err := ctx.Err(); err != nil {
		result.Status = SoftwareProductionCancelled
		result.FailedStage = "context"
		result.FailureReason = err.Error()
		return finish(), nil
	}

	// B96.4 analysis is supplied by the caller because analysis is an observation
	// concern and may come from a real workspace analyzer.
	result.Analysis = req.Analysis

	if req.Changes != nil {
		changes, err := l.Editor.Apply(*req.Changes)
		if err != nil {
			result.Status = SoftwareProductionFailed
			result.FailedStage = "edit"
			result.FailureReason = err.Error()
			return finish(), nil
		}
		result.Changes = &changes
		if !changes.Success {
			result.Status = SoftwareProductionFailed
			result.FailedStage = "edit"
			result.FailureReason = changes.FailureReason
			return finish(), nil
		}
		if req.EffectEvaluationRecorder != nil {
			if err := recordEffectEvaluations(req.Project, req.WorkID, req.AttemptNumber, changes, req.EffectEvaluationRecorder); err != nil {
				result.Status = SoftwareProductionFailed
				result.FailedStage = "observation"
				result.FailureReason = "effect evaluation persistence failed: " + err.Error()
				return finish(), nil
			}
		}
	}

	if err := ctx.Err(); err != nil {
		result.Status = SoftwareProductionCancelled
		result.FailedStage = "context"
		result.FailureReason = err.Error()
		return finish(), nil
	}

	// Dependency metadata is part of a reproducible Go production attempt.
	// Prepare only when a module manifest exists; missing-manifest handling stays
	// with synthesis/build contracts, while non-Go projects use their own path.
	if strings.EqualFold(req.Project.Language, "go") && fileExists(filepath.Join(req.Project.Root, "go.mod")) && l.DependencyPreparer != nil {
		timeout := 2 * time.Minute
		if req.Build != nil && req.Build.Timeout > 0 {
			timeout = req.Build.Timeout
		} else if req.Tests != nil && req.Tests.Timeout > 0 {
			timeout = req.Tests.Timeout
		}
		depCtx, cancel := context.WithTimeout(ctx, timeout)
		depResult := l.DependencyPreparer.Prepare(depCtx, DependencyRequest{
			ID: "dependencies-" + req.Project.ID, Project: req.Project,
			Operation: DependencyResolve, Timeout: timeout,
		})
		cancel()
		result.DependencyPreparation = &depResult
		if !depResult.Success {
			result.Status = SoftwareProductionFailed
			result.FailedStage = "dependencies"
			if len(depResult.Errors) > 0 {
				result.FailureReason = "dependency preparation failed: " + depResult.Errors[0]
			} else if strings.TrimSpace(depResult.Output) != "" {
				result.FailureReason = "dependency preparation failed: " + strings.TrimSpace(depResult.Output)
			} else {
				result.FailureReason = "dependency preparation failed"
			}
			return finish(), nil
		}
	}

	if req.Build != nil {
		build := l.BuildExecutor.Build(ctx, *req.Build)
		result.Build = &build
		if build.Status != BuildSuccess || build.ExitCode != 0 {
			if output := buildFailureOutput(build); output != "" {
				if analysis, analysisErr := AnalyzeErrors(ErrorAnalysisRequest{
					ProjectID: req.Project.ID, Language: req.Project.Language, Output: output,
				}); analysisErr == nil {
					result.ErrorAnalysis = &analysis
				}
			}
			result.Status = SoftwareProductionFailed
			result.FailedStage = "build"
			result.FailureReason = buildFailureReason(build, result.ErrorAnalysis)
			return finish(), nil
		}
	}

	if err := ctx.Err(); err != nil {
		result.Status = SoftwareProductionCancelled
		result.FailedStage = "context"
		result.FailureReason = err.Error()
		return finish(), nil
	}

	if req.Tests != nil {
		tests := l.TestExecutor.RunTests(ctx, *req.Tests)
		result.Tests = &tests
		if tests.Status != TestPassed || tests.ExitCode != 0 {
			// B96.11 is part of the canonical failure path now. Preserve the
			// complete TestRun and structure its output before Production/B95
			// decides whether to retry, replan, ask, or block.
			if analysis, analysisErr := AnalyzeErrors(ErrorAnalysisRequest{
				ProjectID: req.Project.ID,
				Language:  req.Project.Language,
				Output:    tests.Output,
			}); analysisErr == nil {
				result.ErrorAnalysis = &analysis
			}
			result.Status = SoftwareProductionFailed
			result.FailedStage = "test"
			result.FailureReason = testsFailureReason(tests, result.ErrorAnalysis)
			return finish(), nil
		}
	}

	verifyReq := req.Verification
	if verifyReq.Project.ID == "" {
		verifyReq.Project = req.Project
	}
	verifyReq.Build = result.Build
	verifyReq.Tests = result.Tests
	evidence := l.Verifier.VerifyCode(verifyReq)
	result.Verification = evidence
	if !evidence.Success {
		result.Status = SoftwareProductionFailed
		result.FailedStage = "verify"
		result.FailureReason = verificationFailureReason(evidence)
		return finish(), nil
	}

	result.Status = SoftwareProductionSucceeded
	return finish(), nil
}

func buildFailureOutput(build Build) string {
	if strings.TrimSpace(build.Output) != "" {
		return build.Output
	}
	return strings.Join(build.Errors, "\n")
}

func buildFailureReason(build Build, analysis *ErrorAnalysis) string {
	if analysis != nil && len(analysis.Diagnostics) > 0 {
		return formatDiagnosticFailure("build failed", analysis.Diagnostics)
	}
	// Go prefixes compiler output with a package header ("# module/path").
	// Using only Errors[0] preserved that header and dropped the actual
	// diagnostic, e.g. "function main is undeclared in the main package".
	// Keep every meaningful line so the next synthesis can repair the cause.
	lines := make([]string, 0, len(build.Errors))
	for _, line := range build.Errors {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "# ") {
			continue
		}
		lines = append(lines, trimmed)
	}
	if len(lines) > 0 {
		return "build failed: " + strings.Join(lines, "; ")
	}
	if len(build.Errors) > 0 {
		return "build failed: " + build.Errors[0]
	}
	if strings.TrimSpace(build.Output) != "" {
		return "build failed: " + strings.TrimSpace(build.Output)
	}
	return "build failed"
}

func verificationFailureReason(e CodeVerificationEvidence) string {
	for _, check := range e.Checks {
		if !check.Success {
			if strings.TrimSpace(check.Reason) == "" {
				return "code verification failed: " + check.Name
			}
			return "code verification failed: " + check.Name + ": " + check.Reason
		}
	}
	return "code verification failed"
}

func testsFailureReason(t TestRun, analysis *ErrorAnalysis) string {
	if analysis != nil && len(analysis.Diagnostics) > 0 {
		return formatDiagnosticFailure("tests failed", analysis.Diagnostics)
	}
	if len(t.Failures) > 0 {
		return "tests failed: " + t.Failures[0]
	}
	if strings.TrimSpace(t.Output) != "" {
		return "tests failed: " + strings.TrimSpace(t.Output)
	}
	return "tests failed"
}

// formatDiagnosticFailure preserves the full set of observed compiler/test
// diagnostics across the provider-neutral recovery boundary. Keeping only the
// first diagnostic caused later failing files and symbols to disappear from
// the next synthesis request.
func formatDiagnosticFailure(prefix string, diagnostics []Diagnostic) string {
	parts := make([]string, 0, len(diagnostics))
	for _, d := range diagnostics {
		location := d.File
		if d.Line > 0 {
			location = fmt.Sprintf("%s:%d", location, d.Line)
			if d.Column > 0 {
				location = fmt.Sprintf("%s:%d", location, d.Column)
			}
		}
		parts = append(parts, fmt.Sprintf("%s: %s", location, d.Message))
	}
	return prefix + ": " + strings.Join(parts, "; ")
}

func recordEffectEvaluations(project CodeProject, workID string, attemptNumber int, changes MultiFileResult, record func([]byte) error) error {
	if attemptNumber < 1 {
		attemptNumber = 1
	}
	for _, item := range changes.Changes {
		change := item.Change
		if change.Operation != SourceCreate && change.Operation != SourceWrite && change.Operation != SourceEdit {
			continue
		}
		observed, err := ObserveFile(project.Root, change.File.Path)
		if err != nil {
			return err
		}
		evaluation := EffectEvaluation{ID: fmt.Sprintf("effect-eval-%s-%d-%s", workID, attemptNumber, change.ID), WorkID: workID, AttemptNumber: attemptNumber, ExpectedChangeID: change.ID, Observed: observed, Judgment: EvaluateContentCompatibility(change, observed), EvaluatedAt: time.Now().UTC()}
		if err := evaluation.Validate(); err != nil {
			return err
		}
		payload, err := json.Marshal(evaluation)
		if err != nil {
			return err
		}
		if err := record(payload); err != nil {
			return err
		}
	}
	return nil
}
