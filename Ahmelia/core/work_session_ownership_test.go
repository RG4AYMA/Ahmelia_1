package core

import (
	"context"
	"os"
	"strings"
	"testing"

	"ahmelia/autonomy"
	"ahmelia/capability"
	"ahmelia/integration"
	"ahmelia/interaction"
	"ahmelia/production"
)

func TestCurrentPendingAuthorizationUsesConversationOwnerNotLastWork(t *testing.T) {
	e := newB963334AuditEngine(t)
	firstReq := interaction.Analyze("crea un archivo de prueba", interaction.Context{})
	first, err := e.workStore.Begin(firstReq)
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := e.workStore.BeginAttempt(first.ID, "programming.create_file", "create_file", "programming-provider")
	if err != nil {
		t.Fatal(err)
	}
	pending := production.PendingExecution{Request: firstReq, CapabilityID: "programming.create_file", Operation: "create_file", RouteKind: "production", ProviderID: "programming-provider", AttemptNumber: attempt}
	if err := e.workStore.SetPendingExecution(first.ID, pending); err != nil {
		t.Fatal(err)
	}
	secondReq := interaction.Analyze("crea otro archivo", interaction.Context{})
	if _, err := e.workStore.Begin(secondReq); err != nil {
		t.Fatal(err)
	}
	e.conversationState.SetActiveWork(first.ID)

	work, gotPending, ok := e.currentPendingAuthorization()
	if !ok {
		t.Fatal("expected active conversation owner to expose its pending execution")
	}
	if work.ID != first.ID || gotPending.CapabilityID != pending.CapabilityID {
		t.Fatalf("resolved wrong owner: work=%s pending=%+v want=%s", work.ID, gotPending, first.ID)
	}
}

func TestCurrentPendingAuthorizationDoesNotFallBackToLastWork(t *testing.T) {
	e := newB963334AuditEngine(t)
	firstReq := interaction.Analyze("crea un archivo de prueba", interaction.Context{})
	first, err := e.workStore.Begin(firstReq)
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := e.workStore.BeginAttempt(first.ID, "programming.create_file", "create_file", "programming-provider")
	if err != nil {
		t.Fatal(err)
	}
	pending := production.PendingExecution{Request: firstReq, CapabilityID: "programming.create_file", Operation: "create_file", RouteKind: "production", ProviderID: "programming-provider", AttemptNumber: attempt}
	if err := e.workStore.SetPendingExecution(first.ID, pending); err != nil {
		t.Fatal(err)
	}
	secondReq := interaction.Analyze("crea otro archivo", interaction.Context{})
	second, err := e.workStore.Begin(secondReq)
	if err != nil {
		t.Fatal(err)
	}
	if second.ID == first.ID {
		t.Fatal("test requires distinct work IDs")
	}
	// No active conversational owner. The pending first work is not allowed to
	// leak into a confirmation merely because it is the latest persisted work.
	if _, _, ok := e.currentPendingAuthorization(); ok {
		t.Fatal("pending execution leaked without a conversation owner")
	}
}

func TestPendingAuthorizationContinuationPreservesSameWorkWithoutImplicitApproval(t *testing.T) {
	e := newB963334AuditEngine(t)
	firstReq := interaction.Analyze("Quiero que implementes una pequeña utilidad de programación.", interaction.Context{})
	first, err := e.workStore.Begin(firstReq)
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := e.workStore.BeginAttempt(first.ID, "programming.create_file", "create_file", "programming-provider")
	if err != nil {
		t.Fatal(err)
	}
	pending := production.PendingExecution{
		Request:       firstReq,
		CapabilityID:  "programming.create_file",
		Operation:     "create_file",
		RouteKind:     "production",
		ProviderID:    "programming-provider",
		AttemptNumber: attempt,
		Reason:        "explicit confirmation required before execution",
	}
	if err := e.workStore.SetPendingExecution(first.ID, pending); err != nil {
		t.Fatal(err)
	}
	e.conversationState.SetActiveWork(first.ID)

	// Simulate the exact transport-fragmented continuation seen in Delivery38:
	// it is part of the same work, but it is NOT authorization.
	continuation := interaction.Analyze(
		"Si encuentras un problema durante la ejecución, analízalo y corrígelo antes de finalizar.",
		interaction.Context{ActiveWorkID: first.ID, ActiveWorkStatus: "blocked", ActiveObjective: firstReq.Objective.Text},
	)
	turn := &processTurn{
		input:       continuation.Raw,
		channel:     interaction.ChannelVoice,
		semanticReq: continuation,
		workBridgeDecision: interaction.WorkBridgeDecision{
			Relation: interaction.WorkRelationContinue,
			WorkID:   first.ID,
		},
		authorizationActs: interaction.CanonicalActs(continuation),
	}

	// Keep the test deterministic: this branch must not depend on Ollama.
	e.conversationGenerator = nil

	if !e.resumePendingAuthorizationIfConfirmed(turn) {
		t.Fatal("pending authorization continuation was not consumed")
	}
	if e.conversationState.ActiveWork() != first.ID {
		t.Fatalf("active WorkSession changed: got=%q want=%q", e.conversationState.ActiveWork(), first.ID)
	}
	got, ok := e.workStore.Get(first.ID)
	if !ok {
		t.Fatal("canonical WorkSession disappeared")
	}
	if got.PendingExecution == nil {
		t.Fatal("pending authorization was cleared without explicit approval")
	}
	if !strings.Contains(got.PendingExecution.Request.Raw, "Si encuentras un problema") {
		t.Fatalf("continued objective was not persisted into pending execution: %q", got.PendingExecution.Request.Raw)
	}
}

func TestPendingAuthorizationSetsDirectVoiceModeAndConditionalTurnDoesNotApprove(t *testing.T) {
	e := newB963334AuditEngine(t)
	firstReq := interaction.Analyze("crea el archivo de prueba", interaction.Context{})
	work, err := e.workStore.Begin(firstReq)
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := e.workStore.BeginAttempt(work.ID, "programming.create_file", "create_file", "programming-provider")
	if err != nil {
		t.Fatal(err)
	}
	pending := production.PendingExecution{Request: firstReq, CapabilityID: "programming.create_file", Operation: "create_file", RouteKind: "production", ProviderID: "programming-provider", AttemptNumber: attempt}
	if err := e.workStore.SetPendingExecution(work.ID, pending); err != nil {
		t.Fatal(err)
	}
	e.conversationState.SetActiveWork(work.ID)

	conditional := interaction.Analyze("si autorizo, la ejecucion", interaction.Context{})
	turn := &processTurn{input: conditional.Raw, channel: interaction.ChannelVoice, semanticReq: conditional, authorizationActs: interaction.CanonicalActs(conditional)}
	if !turn.authorizationActs.Has(interaction.ActAuthorizationInquiry) {
		t.Fatalf("conditional authorization was not represented as an inquiry: %+v", turn.authorizationActs)
	}
	if !e.resumePendingAuthorizationIfConfirmed(turn) {
		t.Fatal("conditional authorization inquiry did not stay with the pending Work owner")
	}
	voiceMode, err := os.ReadFile("memory/voice_mode.txt")
	if err != nil {
		t.Fatalf("pending authorization did not expose the voice continuation mode: %v", err)
	}
	if strings.TrimSpace(string(voiceMode)) != "direct" {
		t.Fatalf("voice mode = %q, want direct while authorization is pending", voiceMode)
	}
	updated, ok := e.workStore.Get(work.ID)
	if !ok || updated.PendingExecution == nil {
		t.Fatal("conditional statement cleared pending authorization")
	}
}

func TestRecoverPendingAuthorizationOwnerRestoresUniqueAndClearsAmbiguousOwners(t *testing.T) {
	e := newB963334AuditEngine(t)
	createPending := func(objective string) production.WorkSession {
		t.Helper()
		req := interaction.Analyze(objective, interaction.Context{})
		work, err := e.workStore.Begin(req)
		if err != nil {
			t.Fatal(err)
		}
		attempt, err := e.workStore.BeginAttempt(work.ID, "programming.create_file", "create_file", "programming-provider")
		if err != nil {
			t.Fatal(err)
		}
		pending := production.PendingExecution{Request: req, CapabilityID: "programming.create_file", Operation: "create_file", RouteKind: "production", ProviderID: "programming-provider", AttemptNumber: attempt}
		if err := e.workStore.SetPendingExecution(work.ID, pending); err != nil {
			t.Fatal(err)
		}
		return work
	}

	first := createPending("crea el primer archivo")
	e.conversationState.SetActiveWork("stale-owner")
	if got := recoverPendingAuthorizationOwner(e.conversationState, e.workStore, e.sess); got != 1 {
		t.Fatalf("pending authorization count = %d, want 1", got)
	}
	if got := e.conversationState.ActiveWork(); got != first.ID {
		t.Fatalf("unique authorization owner = %q, want %q", got, first.ID)
	}
	voiceMode, err := os.ReadFile("memory/voice_mode.txt")
	if err != nil || strings.TrimSpace(string(voiceMode)) != "direct" {
		t.Fatalf("unique pending authorization did not restore direct voice mode: mode=%q err=%v", voiceMode, err)
	}

	_ = createPending("crea el segundo archivo")
	if got := recoverPendingAuthorizationOwner(e.conversationState, e.workStore, e.sess); got != 2 {
		t.Fatalf("pending authorization count = %d, want 2", got)
	}
	if got := e.conversationState.ActiveWork(); got != "" {
		t.Fatalf("ambiguous pending contracts retained active owner %q", got)
	}
	voiceMode, err = os.ReadFile("memory/voice_mode.txt")
	if err != nil || strings.TrimSpace(string(voiceMode)) != "wake" {
		t.Fatalf("ambiguous authorization state did not fail closed to wake mode: mode=%q err=%v", voiceMode, err)
	}
}

func TestPendingAuthorizationOwnershipSurvivesRestartWithNewerWorkPresent(t *testing.T) {
	dir := t.TempDir()
	workPath := dir + "/production_work.json"
	conversationPath := dir + "/conversation_state.json"

	firstReq := interaction.Analyze("crea el archivo original", interaction.Context{})
	storeBeforeRestart := production.NewWorkStore(workPath)
	first, err := storeBeforeRestart.Begin(firstReq)
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := storeBeforeRestart.BeginAttempt(first.ID, "programming.create_file", "create_file", "programming-provider")
	if err != nil {
		t.Fatal(err)
	}
	pending := production.PendingExecution{
		Request: firstReq, CapabilityID: "programming.create_file",
		Operation: "create_file", RouteKind: "production", ProviderID: "programming-provider", AttemptNumber: attempt,
		Reason: "explicit confirmation required before execution",
	}
	if err := storeBeforeRestart.SetPendingExecution(first.ID, pending); err != nil {
		t.Fatal(err)
	}

	conversationBeforeRestart := interaction.NewConversationState()
	conversationBeforeRestart.SetActiveWork(first.ID)
	if err := conversationBeforeRestart.Save(conversationPath); err != nil {
		t.Fatal(err)
	}

	// A second, newer record is deliberately persisted after the authorization
	// request. Recency must not replace the durable conversational owner.
	secondReq := interaction.Analyze("crea otro archivo independiente", interaction.Context{})
	second, err := storeBeforeRestart.Begin(secondReq)
	if err != nil {
		t.Fatal(err)
	}
	if second.ID == first.ID {
		t.Fatal("test requires distinct work identities")
	}

	// Recreate both persistence owners as a process restart would.
	storeAfterRestart := production.NewWorkStore(workPath)
	conversationAfterRestart := interaction.LoadConversationState(conversationPath)
	conversationAfterRestart.ReconcileObjectiveOwnership(false)
	if conversationAfterRestart.ActiveWork() != "" {
		t.Fatal("test requires the ordinary idle reconciliation to clear the old conversational pointer")
	}
	pendingOwners := storeAfterRestart.PendingAuthorizations()
	if len(pendingOwners) != 1 || pendingOwners[0].ID != first.ID {
		t.Fatalf("pending owner recovery candidates = %+v, want only %s", pendingOwners, first.ID)
	}
	conversationAfterRestart.SetActiveWork(pendingOwners[0].ID)
	engine := &Engine{
		workStore:         NewPersistentWorkStore(storeAfterRestart),
		conversationState: conversationAfterRestart,
	}

	gotWork, gotPending, ok := engine.currentPendingAuthorization()
	if !ok {
		t.Fatal("pending authorization was not recoverable after restart")
	}
	if gotWork.ID != first.ID {
		t.Fatalf("authorization rebound to work %q, want original %q (newer work %q)", gotWork.ID, first.ID, second.ID)
	}
	if gotPending.CapabilityID != pending.CapabilityID || gotPending.Operation != pending.Operation || gotPending.AttemptNumber != pending.AttemptNumber {
		t.Fatalf("pending execution contract changed across restart: got=%+v want=%+v", gotPending, pending)
	}
	if conversationAfterRestart.ActiveWork() != first.ID {
		t.Fatalf("active work identity after restart=%q, want %q", conversationAfterRestart.ActiveWork(), first.ID)
	}
}

type authorizationIdentityTestProvider struct {
	id    string
	calls int
}

func (p *authorizationIdentityTestProvider) ID() string { return p.id }
func (p *authorizationIdentityTestProvider) Execute(_ context.Context, _ production.ProviderRequest) production.ProviderResult {
	p.calls++
	return production.ProviderResult{Success: true, Observed: true, Verified: true, Summary: "exact persisted provider executed", Producer: p.id}
}

func TestCanonicalAuthorityResumesExactPersistedCapabilityNotRediscoveredCandidate(t *testing.T) {
	e := newB963334AuditEngine(t)
	registry := capability.NewRegistry()
	firstCapability := capability.Capability{ID: "demo.candidate-a", Domain: capability.DomainLocal, Action: "protected_action", Description: "candidate A", Lane: capability.LaneProduction, Permission: capability.PermissionAskFirst}
	secondCapability := capability.Capability{ID: "demo.candidate-b", Domain: capability.DomainLocal, Action: "protected_action", Description: "candidate B", Lane: capability.LaneProduction, Permission: capability.PermissionAskFirst}
	if err := registry.Register(firstCapability); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(secondCapability); err != nil {
		t.Fatal(err)
	}
	providerA := &authorizationIdentityTestProvider{id: "provider-a"}
	providerB := &authorizationIdentityTestProvider{id: "provider-b"}
	providers := production.NewProviderRegistry()
	if err := providers.RegisterCapabilities(providerA, firstCapability.ID); err != nil {
		t.Fatal(err)
	}
	if err := providers.RegisterCapabilities(providerB, secondCapability.ID); err != nil {
		t.Fatal(err)
	}
	runtime, err := integration.NewProductionRuntime(capability.NewResolver(registry), registry, providers, &production.ProductionContext{}, e.workStore.Raw(), production.NewTechnicalRecoveryService())
	if err != nil {
		t.Fatal(err)
	}
	e.capabilityRegistry = registry
	e.capabilityResolver = capability.NewResolver(registry)
	e.productionRuntime = runtime

	req := interaction.Analyze("ejecuta la operación protegida", interaction.Context{})
	req.Kind = interaction.KindProduction
	req.Action.Name = "protected_action"
	req.RequestedAction = "protected_action"
	work, err := e.workStore.Begin(req)
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := e.workStore.BeginAttempt(work.ID, string(secondCapability.ID), secondCapability.Action, providerB.ID())
	if err != nil {
		t.Fatal(err)
	}
	pending := production.PendingExecution{Request: req, CapabilityID: string(secondCapability.ID), Operation: secondCapability.Action, RouteKind: string(capability.IntentProduction), ProviderID: providerB.ID(), AttemptNumber: attempt}
	if err := e.workStore.SetPendingExecution(work.ID, pending); err != nil {
		t.Fatal(err)
	}
	e.conversationState.SetActiveWork(work.ID)

	approval := interaction.Analyze("autorizo la ejecucion de la operacion protegida pendiente", interaction.Context{})
	turn := &processTurn{input: approval.Raw, channel: interaction.ChannelText, semanticReq: approval, authorizationActs: interaction.CanonicalActs(approval)}
	if !turn.authorizationActs.Has(interaction.ActConfirmation) {
		t.Fatalf("explicit authorization was not classified as confirmation: %+v", turn.authorizationActs)
	}
	if !e.resumePendingAuthorizationIfConfirmed(turn) {
		t.Fatal("explicit authorization did not resume the pending canonical operation")
	}
	if providerA.calls != 0 || providerB.calls != 1 {
		t.Fatalf("authorization resume rediscovered/substituted provider: providerA=%d providerB=%d", providerA.calls, providerB.calls)
	}
	updated, ok := e.workStore.Get(work.ID)
	if !ok || len(updated.Attempts) != 1 || updated.Attempts[0].Number != attempt || updated.Attempts[0].Status != "succeeded" || updated.PendingExecution != nil {
		t.Fatalf("persisted work/attempt did not converge after resume: %+v", updated)
	}
}

func TestCanonicalAuthorityPersistsWorkOwnerBeforeAuthorizationSuspensionForBypassCaller(t *testing.T) {
	e := newB963334AuditEngine(t)
	registry := capability.NewRegistry()
	c := capability.Capability{ID: "demo.bypass-protected", Domain: capability.DomainLocal, Action: "protected_action", Description: "protected production operation", Lane: capability.LaneProduction, Permission: capability.PermissionAskFirst}
	if err := registry.Register(c); err != nil {
		t.Fatal(err)
	}
	provider := &authorizationIdentityTestProvider{id: "bypass-protected-provider"}
	providers := production.NewProviderRegistry()
	if err := providers.RegisterCapabilities(provider, c.ID); err != nil {
		t.Fatal(err)
	}
	runtime, err := integration.NewProductionRuntime(capability.NewResolver(registry), registry, providers, &production.ProductionContext{}, e.workStore.Raw(), production.NewTechnicalRecoveryService())
	if err != nil {
		t.Fatal(err)
	}
	e.capabilityRegistry = registry
	e.capabilityResolver = capability.NewResolver(registry)
	e.productionRuntime = runtime
	e.capabilityApprovalResolver = nil // bypass test must exercise the real ask-first suspension

	req := interaction.Analyze("ejecuta la operación protegida", interaction.Context{})
	req.Kind = interaction.KindProduction
	req.Action.Name = c.Action
	req.RequestedAction = c.Action
	decision := interaction.ResolveCapabilityInteraction(req)
	result := e.authorityExecuteWithOptions(decision, production.ExecutionOptions{
		Approval: autonomy.ApprovalNone, ExperimentalBypassCognition: true,
	}, req)
	if !result.Handled || !result.AuthorizationRequired {
		t.Fatalf("expected canonical authorization suspension, got %+v", result)
	}
	if provider.calls != 0 {
		t.Fatalf("provider executed before authorization: calls=%d", provider.calls)
	}

	// The diagnostic bypass does not run the ordinary turn cleanup. The
	// canonical Work owner therefore must already be durable at the authority
	// boundary, before the pending execution can survive a process restart.
	recoveredConversation := interaction.LoadConversationState(interaction.ConversationStatePath)
	activeID := recoveredConversation.ActiveWork()
	if activeID == "" {
		t.Fatal("active Work owner was only in memory; restart would lose the pending authorization owner")
	}
	recoveredWorkStore := production.NewWorkStore("memory/production_work.json")
	pending, ok := recoveredWorkStore.PendingExecution(activeID)
	if !ok {
		t.Fatalf("recovered active Work %q has no pending authorization", activeID)
	}
	if pending.CapabilityID != string(c.ID) || pending.ProviderID != provider.ID() || pending.Operation != c.Action || pending.AttemptNumber != 1 {
		t.Fatalf("recovered authorization identity changed: %+v", pending)
	}
}
