package core

import (
	"context"
	"os"
	"testing"

	"ahmelia/capability"
	"ahmelia/interaction"
	"ahmelia/production"
)

type b9643VerifiedProductionProvider struct{}

func (b9643VerifiedProductionProvider) ID() string { return "programming" }

func (b9643VerifiedProductionProvider) Execute(_ context.Context, req production.ProviderRequest) production.ProviderResult {
	projectID := req.Project.ID
	if projectID == "" {
		projectID = "survival-game"
	}
	return production.ProviderResult{
		Success: true, Verified: true, Observed: true, Summary: "test production verified",
		OutcomeCode: "test_production_verified", Producer: "programming",
		Project: production.ProductionProject{ID: projectID, Name: projectID},
	}
}

func TestB9643WorkSessionCreatesCanonicalGoalPlanAndPersistentOwner(t *testing.T) {
	t.Setenv("AHMELIA_SKIP_PROGRAM_INDEX_SCAN", "1")
	t.Setenv("AHMELIA_DISABLE_TTS", "1")
	e := buildTestEngine(t)

	req := interaction.Parse("Quiero crear una herramienta para organizar mis archivos", interaction.ChannelText)
	route := capability.Route{Kind: capability.IntentProduction}

	workID, err := e.ensureWorkSessionForProduction(req, route, "")
	if err != nil {
		t.Fatal(err)
	}
	if workID == "" {
		t.Fatal("expected persistent work owner")
	}
	work, ok := e.workStore.Get(workID)
	if !ok {
		t.Fatalf("work %q not found", workID)
	}
	if work.GoalID == "" || work.PlanID == "" {
		t.Fatalf("work is not linked to canonical goal/plan: %+v", work)
	}
	if work.Status != "pending" {
		t.Fatalf("work must remain pending before physical execution, got %q", work.Status)
	}
	goal, ok := e.goalStore.Get(work.GoalID)
	if !ok || goal == nil {
		t.Fatalf("canonical goal missing: id=%q ok=%t", work.GoalID, ok)
	}
	if goal.Name == "" || goal.Status == "" {
		t.Fatalf("invalid canonical goal: %+v", goal)
	}
	if e.conversationState.ActiveWork() != workID {
		t.Fatalf("conversation/work ownership diverged: active=%q work=%q", e.conversationState.ActiveWork(), workID)
	}
}

func TestB9643WorkSessionReusesCurrentWorkOnContinuation(t *testing.T) {
	t.Setenv("AHMELIA_SKIP_PROGRAM_INDEX_SCAN", "1")
	t.Setenv("AHMELIA_DISABLE_TTS", "1")
	e := buildTestEngine(t)

	first := interaction.Parse("Quiero crear una herramienta para organizar mis archivos", interaction.ChannelText)
	id, err := e.ensureWorkSessionForProduction(first, capability.Route{Kind: capability.IntentProduction}, "")
	if err != nil {
		t.Fatal(err)
	}

	continuation := first
	continuation.Kind = interaction.KindContinuation
	continuation.Continues = true
	continuation.FollowUp = true
	continuation.Normalized = "continua"
	continuation.Objective.Text = "continuar la herramienta"

	reused, err := e.ensureWorkSessionForProduction(continuation, capability.Route{Kind: capability.IntentProduction}, "")
	if err != nil {
		t.Fatal(err)
	}
	if reused != id {
		t.Fatalf("continuation created/switched work: got=%q want=%q", reused, id)
	}
}

func TestB9645ExplicitResumeRestoresDurableConversationOwner(t *testing.T) {
	t.Setenv("AHMELIA_SKIP_PROGRAM_INDEX_SCAN", "1")
	t.Setenv("AHMELIA_DISABLE_TTS", "1")
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })

	cases := []string{
		"Volvamos al proyecto",
		"Retomemos el trabajo",
		"Sigamos con el proyecto",
	}
	for _, phrase := range cases {
		t.Run(phrase, func(t *testing.T) {
			e := buildTestEngine(t)
			initial := interaction.Parse("Crear una escena de supervivencia", interaction.ChannelText)
			work, err := e.workStore.Begin(initial)
			if err != nil {
				t.Fatal(err)
			}
			if err := e.workStore.AttachProduction(work.ID, "goal-resume", "plan-resume", "test-provider"); err != nil {
				t.Fatal(err)
			}
			e.conversationState.SetActiveWork("")
			if err := e.conversationState.Save(interaction.ConversationStatePath); err != nil {
				t.Fatal(err)
			}

			resume := interaction.Parse(phrase, interaction.ChannelText)
			got, err := e.ensureWorkSessionForProduction(resume, capability.Route{Kind: capability.IntentProduction}, "")
			if err != nil {
				t.Fatal(err)
			}
			if got != work.ID {
				t.Fatalf("explicit resume switched owner: got=%q want=%q", got, work.ID)
			}
			if active := e.conversationState.ActiveWork(); active != work.ID {
				t.Fatalf("resume returned an execution owner without restoring conversation ownership: active=%q want=%q", active, work.ID)
			}
			snap := e.conversationState.Snapshot()
			if len(snap.WorkLinks) == 0 {
				t.Fatal("resume did not record a conversation-to-work link")
			}
			lastLink := snap.WorkLinks[len(snap.WorkLinks)-1]
			if lastLink.WorkID != work.ID || lastLink.Relation != interaction.WorkRelationResume {
				t.Fatalf("resume link does not preserve canonical owner/relation: %+v", lastLink)
			}
			reloaded := interaction.LoadConversationState(interaction.ConversationStatePath)
			if persisted := reloaded.ActiveWork(); persisted != work.ID {
				t.Fatalf("resumed owner was not durable: persisted=%q want=%q", persisted, work.ID)
			}
		})
	}
}

func TestB9643WorkSessionPersistsPlanningLink(t *testing.T) {
	t.Setenv("AHMELIA_SKIP_PROGRAM_INDEX_SCAN", "1")
	t.Setenv("AHMELIA_DISABLE_TTS", "1")
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	_ = old
	e := buildTestEngine(t)
	req := interaction.Parse("Quiero preparar una escena de prueba", interaction.ChannelText)
	id, err := e.ensureWorkSessionForProduction(req, capability.Route{Kind: capability.IntentProduction}, "")
	if err != nil {
		t.Fatal(err)
	}
	work, ok := e.workStore.Get(id)
	if !ok {
		t.Fatal("work missing")
	}
	persisted, ok := e.workStore.Raw().Get(id)
	if !ok || persisted.GoalID != work.GoalID || persisted.PlanID != work.PlanID {
		t.Fatalf("planning link was not preserved: persisted=%+v work=%+v", persisted, work)
	}
}

func TestB9643WorkSessionE2EProducesWorkGoalPlanAndVerifiedResult(t *testing.T) {
	t.Setenv("AHMELIA_DISABLE_TTS", "1")
	e := newInteractionE2EEngine(t)
	installF9LanguageStubs(t, e)
	if err := e.productionProviders.RegisterCapabilities(b9643VerifiedProductionProvider{}, capability.ID("programming.create_file")); err != nil {
		t.Fatal(err)
	}
	var responses []string
	e.SetResponseCallback(func(msg string) { responses = append(responses, msg) })

	e.Process("Empieza creando un programa base para el juego de supervivencia.")

	work, ok := e.workStore.Last()
	if !ok {
		t.Fatal("E2E production request did not create WorkSession")
	}
	if work.GoalID == "" || work.PlanID == "" {
		t.Fatalf("E2E work lost canonical Goal/Plan link: %+v", work)
	}
	if work.LastActionResult == nil || !work.LastActionResult.Success || !work.LastActionResult.Verified {
		t.Fatalf("E2E result is not successfully verified: %+v", work.LastActionResult)
	}
	if e.conversationState.ActiveWork() != work.ID {
		t.Fatalf("conversation/work link diverged: active=%q work=%q", e.conversationState.ActiveWork(), work.ID)
	}
	if len(responses) == 0 {
		t.Fatal("E2E production session produced no response")
	}
}

func TestB9644RecoveryWorkIDReusesCanonicalWorkOwner(t *testing.T) {
	t.Setenv("AHMELIA_SKIP_PROGRAM_INDEX_SCAN", "1")
	t.Setenv("AHMELIA_DISABLE_TTS", "1")
	e := buildTestEngine(t)

	first := interaction.Parse("Quiero crear una herramienta para organizar mis archivos", interaction.ChannelText)
	workID, err := e.ensureWorkSessionForProduction(first, capability.Route{Kind: capability.IntentProduction}, "")
	if err != nil {
		t.Fatal(err)
	}
	if workID == "" {
		t.Fatal("missing canonical work owner")
	}

	retry := first
	retry.Kind = interaction.KindContinuation
	retry.Continues = true
	retry.FollowUp = true
	retry.Normalized = "reintentar"
	reused, err := e.ensureWorkSessionForProduction(retry, capability.Route{Kind: capability.IntentProduction}, workID)
	if err != nil {
		t.Fatal(err)
	}
	if reused != workID {
		t.Fatalf("recovery path switched work owner: got=%q want=%q", reused, workID)
	}
	last, ok := e.workStore.Last()
	if !ok || last.ID != workID {
		t.Fatalf("canonical work changed during recovery continuation: %+v ok=%t", last, ok)
	}
}

// A Goal must not become runnable before the same objective has a durable
// WorkSession and planning link. Exercise several semantically different
// production requests against the same persistence failure, because the
// invariant belongs to the ownership transition, not to a phrase.
func TestB9644WorkSessionPersistenceFailureNeverActivatesGoal(t *testing.T) {
	cases := []string{
		"crear una herramienta para organizar archivos",
		"preparar una escena de Unity con un objeto coleccionable",
		"construir un programa Go con pruebas automatizadas",
		"generar un modelo 3D y sus materiales",
		"Quiero crear un programa de escritorio para automatizar tareas repetitivas",
	}
	for _, objective := range cases {
		t.Run(objective, func(t *testing.T) {
			oldWD, err := os.Getwd()
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Chdir(t.TempDir()); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chdir(oldWD) })
			t.Setenv("AHMELIA_SKIP_PROGRAM_INDEX_SCAN", "1")
			t.Setenv("AHMELIA_DISABLE_TTS", "1")
			e := buildTestEngine(t)
			// A directory cannot be replaced by the WorkStore's atomic file
			// rename, deterministically forcing Begin's durable commit to fail.
			e.workStore = NewPersistentWorkStore(production.NewWorkStore(t.TempDir()))
			req := interaction.Parse(objective, interaction.ChannelText)
			_, err = e.ensureWorkSessionForProduction(req, capability.Route{Kind: capability.IntentProduction}, "")
			if err == nil {
				t.Fatal("expected WorkSession persistence failure")
			}
			if current := e.goalStore.Current(); current != nil {
				t.Fatalf("Goal became runnable without durable WorkSession: goal=%q status=%q", current.ID, current.Status)
			}
			if pending := e.goalStore.Pending(); len(pending) != 0 {
				t.Fatalf("Goal entered pending queue without durable WorkSession: %+v", pending)
			}
			if active := e.conversationState.ActiveWork(); active != "" {
				t.Fatalf("conversation points to non-durable WorkSession: %q", active)
			}
		})
	}
}
