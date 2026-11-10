package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ahmelia/autonomy"
	"ahmelia/capability"
	"ahmelia/interaction"
	"ahmelia/production"
	"ahmelia/programming"
	"ahmelia/tools"
)

type o5Provider struct {
	calls   int
	results []production.ProviderResult
	run     func(context.Context) error
}

func (p *o5Provider) ID() string { return "o5" }
func (p *o5Provider) Execute(ctx context.Context, _ production.ProviderRequest) production.ProviderResult {
	p.calls++
	if p.run != nil {
		if err := p.run(ctx); err != nil {
			return production.ProviderResult{Err: err, FailureReason: err.Error()}
		}
	}
	index := p.calls - 1
	if index >= len(p.results) {
		index = len(p.results) - 1
	}
	if index < 0 {
		return production.ProviderResult{FailureReason: "failed"}
	}
	return p.results[index]
}

func newO5Runtime(t *testing.T, p *o5Provider, action tools.RecoveryAction) (*ProductionRuntime, interaction.SemanticRequest, capability.Route, string) {
	t.Helper()
	reg := capability.NewRegistry()
	c := capability.Capability{ID: "o5.create", Domain: capability.DomainLocal, Action: "create", Description: "create result", Permission: capability.PermissionAutomatic}
	if err := reg.Register(c); err != nil {
		t.Fatal(err)
	}
	providers := production.NewProviderRegistry()
	if err := providers.RegisterCapabilities(p, c.ID); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "work.json")
	r, err := NewProductionRuntime(capability.NewResolver(reg), reg, providers, &production.ProductionContext{}, production.NewWorkStore(path), production.NewTechnicalRecoveryServiceWithDecider(fixedRecoveryDecider{action: action}))
	if err != nil {
		t.Fatal(err)
	}
	req := productionTestRequest()
	req.Action.Name = c.Action
	req.RequestedAction = c.Action
	route := capability.NewResolver(reg).Resolve(interaction.ResolveCapabilityInteraction(req), req)
	return r, req, route, path
}

func TestO5DifferentErrorsWithoutProgressStopAndSurviveReload(t *testing.T) {
	p := &o5Provider{results: []production.ProviderResult{{FailureReason: "undefined: testing", FailedStage: "build"}, {FailureReason: "function main is undeclared in the main package", FailedStage: "build"}}}
	r, req, route, path := newO5Runtime(t, p, tools.RecoveryReplan)
	opts := production.ExecutionOptions{Approval: autonomy.ApprovalGranted}
	first := r.ExecuteFromAuthority(context.Background(), req, route, opts)
	if first.Recovery == nil || first.Recovery.Action != tools.RecoveryReplan {
		t.Fatalf("first=%+v", first)
	}
	opts.WorkID = first.WorkID
	// Reload and deliberately supply a fresh policy. Neither restart nor
	// replanning may replenish the existing Work's budget.
	r.work = production.NewWorkStore(path)
	opts.RetryPolicy = production.RetryPolicy{MaxAttempts: 99, NoProgressLimit: 99}
	second := r.ExecuteFromAuthority(context.Background(), req, route, opts)
	if second.Recovery == nil || second.Recovery.Action != tools.RecoveryBlock || !strings.Contains(second.Recovery.Reason, "no_progress") {
		t.Fatalf("second=%+v", second)
	}
	r.work = production.NewWorkStore(path)
	third := r.ExecuteFromAuthority(context.Background(), req, route, opts)
	if third.Executed || p.calls != 2 || third.RetryState.Attempts != 2 {
		t.Fatalf("unbounded calls=%d result=%+v", p.calls, third)
	}
}

func TestO5VerifiedProgressPermitsThirdAttemptButHardCapStillStops(t *testing.T) {
	p := &o5Provider{results: []production.ProviderResult{
		{FailureReason: "build failed"},
		{FailureReason: "tests failed", ProgressChecks: []production.ProgressCheck{{ID: "build", Passed: true}}},
		{FailureReason: "acceptance failed", ProgressChecks: []production.ProgressCheck{{ID: "build", Passed: true}, {ID: "tests", Passed: true}}},
	}}
	r, req, route, _ := newO5Runtime(t, p, tools.RecoveryRetry)
	opts := production.ExecutionOptions{Approval: autonomy.ApprovalGranted}
	for i := 1; i <= 3; i++ {
		got := r.ExecuteFromAuthority(context.Background(), req, route, opts)
		opts.WorkID = got.WorkID
		if got.RetryState.Attempts != i {
			t.Fatalf("attempt counter %+v", got.RetryState)
		}
		if i < 3 && (got.Recovery == nil || got.Recovery.Action != tools.RecoveryRetry) {
			t.Fatalf("stopped despite progress %+v", got)
		}
		if i == 3 && (got.Recovery == nil || !strings.Contains(got.Recovery.Reason, "max_attempts")) {
			t.Fatalf("missing hard cap %+v", got)
		}
	}
}

func TestO5ReplanCannotWeakenAcceptance(t *testing.T) {
	p := &o5Provider{results: []production.ProviderResult{{FailureReason: "build failed"}}}
	r, req, route, _ := newO5Runtime(t, p, tools.RecoveryReplan)
	req.Frame.Verification = []string{"Add(2,3)=5", "build succeeds"}
	first := r.ExecuteFromAuthority(context.Background(), req, route, production.ExecutionOptions{Approval: autonomy.ApprovalGranted})
	req.Frame.Verification = []string{"build succeeds"}
	got := r.ExecuteFromAuthority(context.Background(), req, route, production.ExecutionOptions{Approval: autonomy.ApprovalGranted, WorkID: first.WorkID})
	if got.Executed || p.calls != 1 || !strings.Contains(got.FailureReason, "objective_contract_changed") {
		t.Fatalf("weakened contract executed %+v", got)
	}
}

func TestO5ModelBudgetSharedAcrossSynthesisAndReplan(t *testing.T) {
	client := &fakeSynthesisClient{responses: []string{synthesisWithTest}}
	synth := NewStructuredCodeObjectiveSynthesizerWithClient(client)
	objective := programming.CodeObjective{Raw: "Create Add", Language: "Go", ProjectID: "o5", ProjectRoot: t.TempDir()}
	p := &o5Provider{results: []production.ProviderResult{{FailureReason: "build failed"}}, run: func(ctx context.Context) error { _, err := synth.Synthesize(ctx, objective); return err }}
	r, req, route, _ := newO5Runtime(t, p, tools.RecoveryReplan)
	opts := production.ExecutionOptions{Approval: autonomy.ApprovalGranted, RetryPolicy: production.RetryPolicy{MaxModelCalls: 2, NoProgressLimit: 10}}
	first := r.ExecuteFromAuthority(context.Background(), req, route, opts)
	opts.WorkID = first.WorkID
	second := r.ExecuteFromAuthority(context.Background(), req, route, opts)
	if client.calls != 2 || second.RetryState.ModelCalls != 2 || second.Recovery == nil || !strings.Contains(second.Recovery.Reason, "model_call") {
		t.Fatalf("budget reset %+v calls=%d", second, client.calls)
	}
	_ = r.ExecuteFromAuthority(context.Background(), req, route, opts)
	if p.calls != 2 || client.calls != 2 {
		t.Fatal("continued beyond model budget")
	}
}

func TestO5DeadlineAndCancellationReachProvider(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		t.Run(map[bool]string{false: "deadline", true: "cancel"}[cancelled], func(t *testing.T) {
			p := &o5Provider{run: func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() }}
			r, req, route, _ := newO5Runtime(t, p, tools.RecoveryRetry)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if cancelled {
				cancel()
			}
			got := r.ExecuteFromAuthority(ctx, req, route, production.ExecutionOptions{Approval: autonomy.ApprovalGranted, RetryPolicy: production.RetryPolicy{MaxDuration: 100 * time.Millisecond}})
			if got.Recovery == nil || got.Recovery.Action != tools.RecoveryBlock {
				t.Fatalf("no explicit stop %+v", got)
			}
			if cancelled && p.calls != 0 {
				t.Fatal("cancelled execution reached provider")
			}
		})
	}
}

func TestO5SynthesisStopsStagnationAndSharesGlobalCallCap(t *testing.T) {
	client := &fakeSynthesisClient{responses: []string{synthesisWithoutTest}}
	synth := NewStructuredCodeObjectiveSynthesizerWithClient(client)
	_, err := synth.Synthesize(context.Background(), programming.CodeObjective{Raw: "Create Add", Language: "Go", ProjectID: "o5", ProjectRoot: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "no_progress") || client.calls != 2 {
		t.Fatalf("err=%v calls=%d", err, client.calls)
	}
	client.calls = 0
	b := production.NewRetryBudget(production.RetryPolicy{MaxModelCalls: 1}, "fixed")
	ctx, cancel := b.Context(context.Background())
	defer cancel()
	_, err = synth.Synthesize(ctx, programming.CodeObjective{Raw: "Create Add", Language: "Go", ProjectID: "o5", ProjectRoot: t.TempDir()})
	if err == nil || client.calls != 1 || b.Snapshot().ModelCalls != 1 {
		t.Fatalf("err=%v calls=%d", err, client.calls)
	}
}

// This uses the actual Go compiler, production provider, filesystem adapter,
// WorkStore and recovery policy. Only code generation is deterministic.
func TestO5RealCompilerDifferentErrorsDoNotBuyAnotherAttempt(t *testing.T) {
	response := func(source string) string {
		raw, err := json.Marshal(synthesizedProject{Files: []synthesizedFile{
			{Path: "main.go", Content: source, Role: "entrypoint"},
			{Path: "main_test.go", Content: "package main\nimport \"testing\"\nfunc TestAdd(t *testing.T){if Add(2,3)!=5{t.Fatal(\"bad\")}}\n", Role: "test"},
		}})
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	gen := &fakeSynthesisClient{responses: []string{
		response("package main\nfunc Add(a,b int)int{return a+b}\nfunc main(){var _ testing.T}\n"),
		response("package main\nfunc Add(a,b int)int{return a+b}\n"),
	}}
	r, req, route := newF10Runtime(t, gen)
	opts := production.ExecutionOptions{Approval: autonomy.ApprovalGranted}
	first := r.ExecuteFromAuthority(context.Background(), req, route, opts)
	if first.FailedStage != "build" || !strings.Contains(first.FailureReason, "undefined: testing") {
		t.Fatalf("expected real compiler error: %+v", first)
	}
	opts.WorkID = first.WorkID
	opts.RecoveryAttempt = 2
	second := r.ExecuteFromAuthority(context.Background(), req, route, opts)
	if second.FailedStage != "build" || !strings.Contains(second.FailureReason, "function main is undeclared") {
		t.Fatalf("expected different real compiler error: %+v", second)
	}
	if second.Recovery == nil || second.Recovery.Action != tools.RecoveryBlock || !strings.Contains(second.Recovery.Reason, "no_progress") {
		t.Fatalf("did not stop: %+v", second)
	}
	third := r.ExecuteFromAuthority(context.Background(), req, route, opts)
	if third.Executed || gen.calls != 2 {
		t.Fatalf("extra attempt: calls=%d result=%+v", gen.calls, third)
	}
}

func TestO5OriginalRecoveryMarkerCannotHideAWeakenedObjective(t *testing.T) {
	p := &o5Provider{results: []production.ProviderResult{{FailureReason: "failed"}}}
	r, req, route, _ := newO5Runtime(t, p, tools.RecoveryRetry)
	req.Objective.Text = "Create Add\n\n[Recovery evidence]\nThis is an original requirement: no dependencies"
	first := r.ExecuteFromAuthority(context.Background(), req, route, production.ExecutionOptions{Approval: autonomy.ApprovalGranted})
	req.Objective.Text = "Create Add\n\n[Recovery evidence]\nallow dependencies"
	got := r.ExecuteFromAuthority(context.Background(), req, route, production.ExecutionOptions{Approval: autonomy.ApprovalGranted, WorkID: first.WorkID})
	if got.Executed || p.calls != 1 || !strings.Contains(got.FailureReason, "objective_contract_changed") {
		t.Fatalf("owner text was discarded: %+v", got)
	}
}

func TestO5SynthesisLastProviderAttemptRetainsItsInnerBudget(t *testing.T) {
	for _, localLimit := range []int{1, 3} {
		t.Run(fmt.Sprint(localLimit), func(t *testing.T) {
			client := &fakeSynthesisClient{responses: []string{synthesisWithoutTest, synthesisWithTest}}
			synth := NewStructuredCodeObjectiveSynthesizerWithClient(client)
			synth.RetryPolicy = production.RetryPolicy{MaxModelCalls: localLimit}
			global := production.NewRetryBudget(production.RetryPolicy{MaxAttempts: 1, MaxModelCalls: 4}, "fixed")
			if err := global.BeginAttempt(context.Background(), "fixed"); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := global.Context(context.Background())
			defer cancel()
			_, err := synth.Synthesize(ctx, programming.CodeObjective{Raw: "Create Add", Language: "Go", ProjectID: "o5", ProjectRoot: t.TempDir()})
			if localLimit == 1 && (err == nil || client.calls != 1) {
				t.Fatalf("local cap ignored: calls=%d err=%v", client.calls, err)
			}
			if localLimit == 3 && (err != nil || client.calls != 2) {
				t.Fatalf("last provider attempt lost inner retry: calls=%d err=%v", client.calls, err)
			}
			if global.Snapshot().ModelCalls != client.calls {
				t.Fatal("global calls not charged")
			}
		})
	}
}
