package programming

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ahmelia/production"
)

// ReconcileMultiFilePlan observes the durable effect specification of an
// interrupted programming attempt. It never mutates the project: it checks the
// expected files, then runs the project's build and tests. The returned
// ActionResult is suitable for WorkStore.ReconcileUnknownAttempt.
func ReconcileMultiFilePlan(ctx context.Context, plan MultiFilePlan) (production.ActionResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := plan.Validate(); err != nil {
		return production.ActionResult{}, fmt.Errorf("reconciliation: invalid plan: %w", err)
	}
	root := strings.TrimSpace(plan.Project.Root)
	if root == "" {
		return production.ActionResult{}, fmt.Errorf("reconciliation: project root is empty")
	}
	info, err := os.Stat(root)
	if err != nil {
		return production.ActionResult{}, fmt.Errorf("reconciliation: project root: %w", err)
	}
	if !info.IsDir() {
		return production.ActionResult{}, fmt.Errorf("reconciliation: project root is not a directory")
	}

	for _, change := range plan.Changes {
		path := filepath.Join(root, filepath.FromSlash(change.File.Path))
		switch change.Operation {
		case SourceCreate, SourceWrite:
			content, err := os.ReadFile(path)
			if err != nil {
				return reconciliationFailure("expected file %s is missing: %v", change.File.Path, err), nil
			}
			if normalizeReconciliationContent(string(content)) != normalizeReconciliationContent(change.Content) {
				return reconciliationFailure("expected file %s does not match the persisted effect", change.File.Path), nil
			}
		case SourceDelete:
			if _, err := os.Stat(path); err == nil {
				return reconciliationFailure("expected file %s to be absent but it exists", change.File.Path), nil
			} else if !os.IsNotExist(err) {
				return reconciliationFailure("cannot inspect expected absent file %s: %v", change.File.Path, err), nil
			}
		default:
			return production.ActionResult{}, fmt.Errorf("reconciliation: unsupported operation %q for %s", change.Operation, change.File.Path)
		}
	}

	buildCommand := strings.TrimSpace(plan.Project.BuildCommand)
	if buildCommand == "" {
		buildCommand = "go build ./..."
	}
	testCommand := strings.TrimSpace(plan.Project.TestCommand)
	if testCommand == "" {
		testCommand = "go test ./..."
	}
	timeout := 2 * time.Minute
	build := NewOSBuildExecutor().Build(ctx, BuildRequest{
		ID: "reconcile-build", Project: plan.Project, Command: buildCommand,
		WorkingDir: root, Timeout: timeout,
	})
	if build.Status != BuildSuccess || build.ExitCode != 0 {
		return reconciliationFailure("reconciliation build failed: %s", strings.TrimSpace(build.Output)), nil
	}
	tests := NewOSTestExecutor().RunTests(ctx, TestRequest{
		ID: "reconcile-tests", Project: plan.Project, Command: testCommand,
		WorkingDir: root, Timeout: timeout,
	})
	if tests.Status != TestPassed || tests.ExitCode != 0 {
		return reconciliationFailure("reconciliation tests failed: %s", strings.TrimSpace(tests.Output)), nil
	}

	return production.ActionResult{
		Executed: true, Success: true, Verified: true, Observed: true,
		OutcomeStatus:       production.OutcomeSucceeded,
		Summary:             "reconciled: persisted project files are present and build/tests pass",
		VerificationSource:  "programming.reconciliation",
		VerificationProfile: "files+build+tests",
	}, nil
}

func reconciliationFailure(format string, args ...interface{}) production.ActionResult {
	reason := fmt.Sprintf(format, args...)
	return production.ActionResult{
		Executed: true, Success: false, Verified: false, Observed: true,
		OutcomeStatus:      production.OutcomeFailed,
		Summary:            "reconciliation found that the interrupted effect did not complete correctly",
		FailedStage:        "reconciliation",
		FailureReason:      reason,
		VerificationSource: "programming.reconciliation",
	}
}

func normalizeReconciliationContent(value string) string {
	return strings.ReplaceAll(strings.TrimSpace(value), "\r\n", "\n")
}
