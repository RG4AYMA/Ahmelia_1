package integration

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"ahmelia/autonomy"
	"ahmelia/production"
)

// scriptedGenerator returns a fixed file set on every call and records prompts.
type scriptedGenerator struct {
	mu      sync.Mutex
	files   []map[string]string
	prompts []string
}

func (g *scriptedGenerator) GenerateStructuredContext(_ context.Context, prompt string, _ json.RawMessage, _ int) (string, error) {
	g.mu.Lock()
	g.prompts = append(g.prompts, prompt)
	g.mu.Unlock()
	b, _ := json.Marshal(map[string]any{"files": g.files})
	return string(b), nil
}

// withCoreRecoveryText reproduces, byte for byte, what core.productionRecoveryRequest
// appends to the objective (core/production_recovery.go) so the harness follows the
// real two-channel path without importing core (which would create an import cycle).
func withCoreRecoveryText(base, failure, stage string) string {
	return base + "\n\n[Recovery evidence]\n" + failure + "\nFailed stage: " + stage
}

// INV-1 (owner: production recovery policy). A policy configured as
// "retry once, then replan" (production.NewTechnicalRecoveryService) must stop
// choosing retry once the retry budget is consumed. The decision must not depend
// on a magic constant living in a different package (core).
func TestINV1_RecoveryPolicyEscalatesAfterRetryBudget(t *testing.T) {
	svc := production.NewTechnicalRecoveryService()
	first, err := svc.Recover(context.Background(), "programming.create", "tests failed: x", 1)
	if err != nil {
		t.Fatal(err)
	}
	if first.Decision.Action != "retry" {
		t.Fatalf("attempt 1 should retry, got %s", first.Decision.Action)
	}
	for n := 2; n <= 4; n++ {
		r, err := svc.Recover(context.Background(), "programming.create", "tests failed: x", n)
		if err != nil {
			t.Fatal(err)
		}
		if r.Decision.Action == "retry" {
			t.Errorf("attempt %d still decided %q: policy is blind to the attempt count", n, r.Decision.Action)
		}
	}
}

// INV-2 (owner: recovery-progress contract). A retry whose synthesized plan does
// not change the artifacts named by the observed failure is NOT an attempt: it must
// not be executed (no new build/test cycle) and must not produce another `retry`.
func TestINV2_NoEffectiveRepairIsNotExecutedNorRetried(t *testing.T) {
	gen := &stubbornGenerator{}
	rt, req, route := newF10Runtime(t, gen)

	first := rt.ExecuteFromAuthority(context.Background(), req, route, production.ExecutionOptions{Approval: autonomy.ApprovalGranted, RecoveryAttempt: 1})
	if first.Success {
		t.Fatalf("setup: first attempt unexpectedly succeeded")
	}
	req2 := req
	req2.Objective.Text = withCoreRecoveryText(req.Objective.Text, first.FailureReason, first.FailedStage)
	second := rt.ExecuteFromAuthority(context.Background(), req2, route, production.ExecutionOptions{Approval: autonomy.ApprovalGranted, RecoveryAttempt: 2, WorkID: first.WorkID})

	if second.FailedStage == "test" || second.FailedStage == "build" {
		t.Errorf("second attempt re-ran %q with a plan that does not change the failing artifacts; it should be rejected before execution", second.FailedStage)
	}
	if second.Recovery != nil && second.Recovery.Action == "retry" {
		t.Errorf("same defect twice in a row must not yield another retry (got %q)", second.Recovery.Action)
	}
	if !strings.Contains(strings.ToLower(second.FailureReason), "progress") && !strings.Contains(strings.ToLower(second.FailureReason), "no_effective_repair") {
		t.Errorf("closure reason must name the lack of progress, got %q", second.FailureReason)
	}
}

// INV-3 (owner: evidence contract). When several files fail, EVERY file named by
// a diagnostic must be visible to the synthesizer on retry, and every diagnostic
// must be present in the evidence it receives.
func TestINV3_AllDiagnosedFilesAndDiagnosticsReachRetrySynthesis(t *testing.T) {
	gen := &scriptedGenerator{files: []map[string]string{
		{"path": "alpha.go", "role": "source", "content": "package main\n\nfunc A() int { return X }\n\nfunc main() {}\n"},
		{"path": "beta.go", "role": "source", "content": "package main\n\nfunc B() int { return Y }\n"},
		{"path": "calc_test.go", "role": "test", "content": "package main\n\nimport \"testing\"\n\nfunc TestOK(t *testing.T) {}\n"},
	}}
	rt, req, route := newF10Runtime(t, gen)
	req.Objective.Text = "Quiero crear un programa Go llamado calcx"

	first := rt.ExecuteFromAuthority(context.Background(), req, route, production.ExecutionOptions{Approval: autonomy.ApprovalGranted, RecoveryAttempt: 1})
	if first.Success {
		t.Fatalf("setup: first attempt unexpectedly succeeded")
	}
	req2 := req
	req2.Objective.Text = withCoreRecoveryText(req.Objective.Text, first.FailureReason, first.FailedStage)
	_ = rt.ExecuteFromAuthority(context.Background(), req2, route, production.ExecutionOptions{Approval: autonomy.ApprovalGranted, RecoveryAttempt: 2, WorkID: first.WorkID})
	if len(gen.prompts) < 2 {
		t.Fatalf("expected a second synthesis, got %d", len(gen.prompts))
	}
	p2 := gen.prompts[1]
	for _, want := range []string{"--- alpha.go ---", "--- beta.go ---", "undefined: X", "undefined: Y"} {
		if !strings.Contains(p2, want) {
			t.Errorf("retry prompt is missing %q (evidence/file lost between attempts)", want)
		}
	}
}

// INV-4 (owner: artifact model). Existence + hash is integrity evidence, not
// behavioral verification. A failed work must not expose artifacts claiming
// Verified=true.
func TestINV4_FailedWorkDoesNotClaimVerifiedArtifacts(t *testing.T) {
	gen := &stubbornGenerator{}
	rt, req, route := newF10Runtime(t, gen)
	first := rt.ExecuteFromAuthority(context.Background(), req, route, production.ExecutionOptions{Approval: autonomy.ApprovalGranted, RecoveryAttempt: 1})
	if first.Success {
		t.Fatalf("setup: first attempt unexpectedly succeeded")
	}
	w, ok := rt.work.Get(first.WorkID)
	if !ok {
		t.Fatalf("work %q missing", first.WorkID)
	}
	for _, a := range w.Artifacts {
		if a.Verified {
			t.Errorf("artifact %q is Verified=true (%q) although the attempt failed at stage %q", a.Name, a.VerificationReason, first.FailedStage)
		}
	}
}
