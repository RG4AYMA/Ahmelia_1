package production

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"ahmelia/interaction"
)

func TestWorkStoreRoundTripAndArtifactRecall(t *testing.T) {
	path := filepath.Join(t.TempDir(), "production_work.json")
	store := NewWorkStore(path)
	req := interaction.Parse("Crear un modelo y verificarlo", interaction.ChannelText)
	w, err := store.Begin(req)
	if err != nil {
		t.Fatal(err)
	}
	a := ProductionArtifact{ID: "a1", Name: "modelo.obj", Path: "C:\\out\\modelo.obj", Type: "model", Producer: "test"}
	if err := store.RecordArtifact(w.ID, a); err != nil {
		t.Fatal(err)
	}
	if err := store.Complete(w.ID, true, "ok"); err != nil {
		t.Fatal(err)
	}
	store2 := NewWorkStore(path)
	got, ok := store2.FindArtifact("modelo.obj")
	if !ok || got.Path != a.Path {
		t.Fatalf("artifact=%+v ok=%v", got, ok)
	}
	last, ok := store2.Last()
	if !ok || last.Status != WorkCompleted {
		t.Fatalf("last=%+v ok=%v", last, ok)
	}
}

func TestWorkSessionDoesNotEncodeAProviderDomain(t *testing.T) {
	req := interaction.Parse("Producir algo", interaction.ChannelText)
	w := WorkSession{ID: "w", Objective: req.Objective, Status: WorkPending}
	if w.Objective.Text == "" {
		t.Fatal("objective missing")
	}
}

func TestWorkStoreProductionAttachPreservesCanonicalPlanningLink(t *testing.T) {
	path := filepath.Join(t.TempDir(), "production_work.json")
	store := NewWorkStore(path)
	req := interaction.Parse("Crear una herramienta", interaction.ChannelText)
	w, err := store.Begin(req)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AttachPlanning(w.ID, "goal-1", "plan-1"); err != nil {
		t.Fatal(err)
	}
	if err := store.AttachProduction(w.ID, "", "", "provider-1"); err != nil {
		t.Fatal(err)
	}
	got, ok := store.Get(w.ID)
	if !ok {
		t.Fatal("work missing")
	}
	if got.GoalID != "goal-1" || got.PlanID != "plan-1" || got.Provider != "provider-1" {
		t.Fatalf("production attach erased canonical planning link: %+v", got)
	}
}

func TestWorkStorePersistsMultipleAttemptsWithinOneWorkOwner(t *testing.T) {
	path := filepath.Join(t.TempDir(), "production_work.json")
	store := NewWorkStore(path)
	req := interaction.Parse("Implementar una funcionalidad", interaction.ChannelText)
	work, err := store.Begin(req)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AttachPlanning(work.ID, "goal-1", "plan-1"); err != nil {
		t.Fatal(err)
	}

	first, err := store.BeginAttempt(work.ID, "programming.modify_code", "modify_code", "programming")
	if err != nil || first != 1 {
		t.Fatalf("first attempt=%d err=%v", first, err)
	}
	failed := ActionResult{Success: false, Verified: false, WorkID: work.ID, Producer: "programming", FailedStage: "test", FailureReason: "tests failed"}
	if err := store.CompleteAttempt(work.ID, first, failed, "retry", "transient test failure"); err != nil {
		t.Fatal(err)
	}

	second, err := store.BeginAttempt(work.ID, "programming.modify_code", "modify_code", "programming")
	if err != nil || second != 2 {
		t.Fatalf("second attempt=%d err=%v", second, err)
	}
	succeeded := ActionResult{Success: true, Verified: true, Observed: true, WorkID: work.ID, Producer: "programming", Summary: "ok"}
	if err := store.CompleteAttempt(work.ID, second, succeeded, "", ""); err != nil {
		t.Fatal(err)
	}
	if err := store.Complete(work.ID, true, "ok"); err != nil {
		t.Fatal(err)
	}

	reloaded := NewWorkStore(path)
	got, ok := reloaded.Get(work.ID)
	if !ok || len(got.Attempts) != 2 {
		t.Fatalf("expected two persisted attempts: %+v ok=%t", got, ok)
	}
	if got.GoalID != "goal-1" || got.PlanID != "plan-1" {
		t.Fatalf("canonical planning link changed across attempts: %+v", got)
	}
	if got.Attempts[0].Status != "failed" || got.Attempts[0].RecoveryAction != "retry" {
		t.Fatalf("first attempt evidence incomplete: %+v", got.Attempts[0])
	}
	if got.Attempts[1].Status != "succeeded" || got.Attempts[1].Result == nil || !got.Attempts[1].Result.Verified {
		t.Fatalf("second attempt evidence incomplete: %+v", got.Attempts[1])
	}
}

func TestWorkStoreCommitFailureDoesNotPublishCandidate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "production_work.json")
	store := NewWorkStore(path)
	req := interaction.Parse("trabajo A", interaction.ChannelText)
	work, err := store.Begin(req)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AttachPlanning(work.ID, "baseline", "plan-baseline"); err != nil {
		t.Fatal(err)
	}

	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0755); err != nil {
		t.Fatal(err)
	}

	if err := store.AttachPlanning(work.ID, "rejected-A", "plan-A"); err == nil {
		t.Fatal("expected commit failure")
	}
	got, ok := store.Get(work.ID)
	if !ok {
		t.Fatal("work disappeared after rejected commit")
	}
	if got.GoalID != "baseline" || got.PlanID != "plan-baseline" {
		t.Fatalf("rejected candidate leaked into live state: %+v", got)
	}

	if err := os.RemoveAll(path); err != nil {
		t.Fatal(err)
	}
	if err := store.AttachPlanning(work.ID, "accepted-B", "plan-B"); err != nil {
		t.Fatal(err)
	}

	reloaded := NewWorkStore(path)
	got, ok = reloaded.Get(work.ID)
	if !ok || got.GoalID != "accepted-B" || got.PlanID != "plan-B" {
		t.Fatalf("durable state=%+v ok=%v", got, ok)
	}
}

func TestWorkStoreCommitFailureMatrix(t *testing.T) {
	t.Run("success-failure", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "production_work.json")
		store := NewWorkStore(path)
		w, err := store.Begin(interaction.Parse("A", interaction.ChannelText))
		if err != nil {
			t.Fatal(err)
		}
		if err := store.AttachPlanning(w.ID, "A", "plan-A"); err != nil {
			t.Fatal(err)
		}
		backup := path + ".accepted"
		if err := os.Rename(path, backup); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(path, 0755); err != nil {
			t.Fatal(err)
		}
		if err := store.AttachPlanning(w.ID, "B-rejected", "plan-B"); err == nil {
			t.Fatal("expected B commit failure")
		}
		if err := os.RemoveAll(path); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(backup, path); err != nil {
			t.Fatal(err)
		}
		reloaded := NewWorkStore(path)
		got, ok := reloaded.Get(w.ID)
		if !ok || got.GoalID != "A" || got.PlanID != "plan-A" {
			t.Fatalf("expected A only, got=%+v ok=%v", got, ok)
		}
	})

	t.Run("both-fail", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "production_work.json")
		store := NewWorkStore(path)
		w, err := store.Begin(interaction.Parse("A", interaction.ChannelText))
		if err != nil {
			t.Fatal(err)
		}
		backup := path + ".accepted"
		if err := os.Rename(path, backup); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(path, 0755); err != nil {
			t.Fatal(err)
		}
		if err := store.AttachPlanning(w.ID, "A-rejected", "plan-A"); err == nil {
			t.Fatal("expected first commit failure")
		}
		if err := store.AttachPlanning(w.ID, "B-rejected", "plan-B"); err == nil {
			t.Fatal("expected second commit failure")
		}
		if err := os.RemoveAll(path); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(backup, path); err != nil {
			t.Fatal(err)
		}
		reloaded := NewWorkStore(path)
		got, ok := reloaded.Get(w.ID)
		if !ok || got.GoalID != "" || got.PlanID != "" {
			t.Fatalf("expected baseline state, got=%+v ok=%v", got, ok)
		}
	})
}

func TestWorkStoreConcurrentAcceptedCommitsAreNotLost(t *testing.T) {
	for run := 0; run < 100; run++ {
		t.Run(fmt.Sprintf("run-%03d", run), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "production_work.json")
			store := NewWorkStore(path)
			a, err := store.Begin(interaction.Parse("A", interaction.ChannelText))
			if err != nil {
				t.Fatal(err)
			}
			b, err := store.Begin(interaction.Parse("B", interaction.ChannelText))
			if err != nil {
				t.Fatal(err)
			}

			var wg sync.WaitGroup
			wg.Add(2)
			var errs [2]error
			go func() {
				defer wg.Done()
				errs[0] = store.AttachPlanning(a.ID, "A", "plan-A")
			}()
			go func() {
				defer wg.Done()
				errs[1] = store.AttachPlanning(b.ID, "B", "plan-B")
			}()
			wg.Wait()
			for i, err := range errs {
				if err != nil {
					t.Fatalf("writer %d failed: %v", i, err)
				}
			}

			reloaded := NewWorkStore(path)
			ga, oka := reloaded.Get(a.ID)
			gb, okb := reloaded.Get(b.ID)
			if !oka || !okb || ga.GoalID != "A" || gb.GoalID != "B" {
				t.Fatalf("lost accepted commit: A=%+v/%v B=%+v/%v", ga, oka, gb, okb)
			}
		})
	}
}

func TestWorkStoreInterruptedCommitCandidateIsNotAccepted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "production_work.json")
	store := NewWorkStore(path)
	w, err := store.Begin(interaction.Parse("A", interaction.ChannelText))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AttachPlanning(w.ID, "accepted", "plan-accepted"); err != nil {
		t.Fatal(err)
	}

	// Model the crash window after the candidate has been written but before
	// rename: the durable file remains authoritative and the orphaned .tmp is
	// not loaded by a reconstructed WorkStore.
	candidateWork := cloneWork(w)
	candidateWork.GoalID = "crash-candidate"
	candidateWork.PlanID = "plan-crash-candidate"
	candidate := map[string]WorkSession{w.ID: candidateWork}
	data, err := json.MarshalIndent(candidate, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+".tmp", data, 0644); err != nil {
		t.Fatal(err)
	}

	reloaded := NewWorkStore(path)
	got, ok := reloaded.Get(w.ID)
	if !ok || got.GoalID != "accepted" || got.PlanID != "plan-accepted" {
		t.Fatalf("unaccepted crash candidate became durable: %+v ok=%v", got, ok)
	}
}

func TestSetPendingExecutionRequiresMatchingDurableAttemptIdentity(t *testing.T) {
	store := NewWorkStore(filepath.Join(t.TempDir(), "work.json"))
	req := interaction.Analyze("crear un archivo protegido", interaction.Context{})
	work, err := store.Begin(req)
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := store.BeginAttempt(work.ID, "files.create", "create_file", "files-provider")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		pending PendingExecution
	}{
		{
			name:    "wrong operation",
			pending: PendingExecution{Request: req, CapabilityID: "files.create", Operation: "delete_file", RouteKind: "production", ProviderID: "files-provider", AttemptNumber: attempt},
		},
		{
			name:    "wrong provider",
			pending: PendingExecution{Request: req, CapabilityID: "files.create", Operation: "create_file", RouteKind: "production", ProviderID: "other-provider", AttemptNumber: attempt},
		},
		{
			name:    "missing route identity",
			pending: PendingExecution{Request: req, CapabilityID: "files.create", Operation: "create_file", ProviderID: "files-provider", AttemptNumber: attempt},
		},
		{
			name:    "missing attempt",
			pending: PendingExecution{Request: req, CapabilityID: "files.create", Operation: "create_file", RouteKind: "production", ProviderID: "files-provider", AttemptNumber: attempt + 1},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := store.SetPendingExecution(work.ID, tc.pending); err == nil {
				t.Fatalf("expected invalid pending contract to be rejected: %+v", tc.pending)
			}
			got, ok := store.Get(work.ID)
			if !ok {
				t.Fatal("work disappeared")
			}
			if got.PendingExecution != nil {
				t.Fatalf("invalid pending authorization was persisted: %+v", got.PendingExecution)
			}
			if got.Attempts[0].Status != "running" {
				t.Fatalf("rejected transition mutated attempt status: %q", got.Attempts[0].Status)
			}
		})
	}

	valid := PendingExecution{Request: req, CapabilityID: "files.create", Operation: "create_file", RouteKind: "production", ProviderID: "files-provider", AttemptNumber: attempt}
	if err := store.SetPendingExecution(work.ID, valid); err != nil {
		t.Fatalf("valid pending authorization rejected: %v", err)
	}
	got, _ := store.Get(work.ID)
	if got.PendingExecution == nil || got.PendingExecution.Reason != "authorization_required" || got.Attempts[0].Status != "awaiting_authorization" || got.Status != WorkBlocked {
		t.Fatalf("pending authorization transition not committed atomically: %+v", got)
	}
}

func TestWorkStoreCorruptPersistedStateFailsClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "production_work.json")
	corrupt := []byte(`{"work-broken":`)
	if err := os.WriteFile(path, corrupt, 0o600); err != nil {
		t.Fatal(err)
	}
	store := NewWorkStore(path)
	if err := store.LoadError(); err == nil {
		t.Fatal("expected corrupt persisted state to be reported")
	}
	if _, err := store.Begin(interaction.Parse("new work must not overwrite corrupt state", interaction.ChannelText)); err == nil {
		t.Fatal("expected mutation to fail closed when prior state is corrupt")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(corrupt) {
		t.Fatalf("corrupt state was overwritten: before=%q after=%q", corrupt, after)
	}
}

func TestWorkStoreMissingPersistedStateIsValidFirstRun(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-created-yet.json")
	store := NewWorkStore(path)
	if err := store.LoadError(); err != nil {
		t.Fatalf("missing state should be valid on first run: %v", err)
	}
	if _, err := store.Begin(interaction.Parse("first work", interaction.ChannelText)); err != nil {
		t.Fatalf("first work should persist successfully: %v", err)
	}
}

func TestFinalizeAttemptAtomicallyPersistsAttemptResultAndWorkClosure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "production_work.json")
	store := NewWorkStore(path)
	work, err := store.Begin(interaction.Parse("create and verify artifact", interaction.ChannelText))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AttachProduction(work.ID, "", "", "provider-test"); err != nil {
		t.Fatal(err)
	}
	number, err := store.BeginAttempt(work.ID, "capability-test", "create", "provider-test")
	if err != nil {
		t.Fatal(err)
	}
	result := ActionResult{Executed: true, Success: true, OutcomeStatus: OutcomeSucceeded, Observed: true, Verified: true, Summary: "verified", WorkID: work.ID, AttemptNumber: number, ObservationActionID: "action-1", ObservationOperation: "create", ObservationTarget: "/tmp/effect", ObservationSource: "filesystem", ObservationCapturedAt: time.Now().UTC(), ObservationFacts: []ObservedPostconditionFact{{Subject: "/tmp/effect", Predicate: "filesystem.exists", Expected: "true", Observed: "true"}}}
	if err := store.FinalizeAttempt(work.ID, number, result, "", "", WorkCompleted, "verified"); err != nil {
		t.Fatal(err)
	}
	reloaded := NewWorkStore(path)
	got, ok := reloaded.Get(work.ID)
	if !ok {
		t.Fatal("work missing after reload")
	}
	if got.Status != WorkCompleted || len(got.Attempts) != 1 || got.Attempts[0].Status != "succeeded" {
		t.Fatalf("lifecycle was not committed atomically: status=%q attempts=%+v", got.Status, got.Attempts)
	}
	if got.LastActionResult == nil || got.LastActionResult.AttemptNumber != number || got.LastActionResult.OutcomeStatus != OutcomeSucceeded {
		t.Fatalf("last action result missing or inconsistent: %+v", got.LastActionResult)
	}
	if got.LastActionResult.ObservationActionID != "action-1" || got.LastActionResult.ObservationSource != "filesystem" || len(got.LastActionResult.ObservationFacts) != 1 || got.LastActionResult.ObservationFacts[0].Observed != "true" || got.LastActionResult.ObservationCapturedAt.IsZero() {
		t.Fatalf("structured observation provenance/facts did not survive durable reload: %+v", got.LastActionResult)
	}
}

func TestFinalizeAttemptRejectsEpistemicallyIncoherentClosure(t *testing.T) {
	cases := []struct {
		name     string
		result   ActionResult
		terminal WorkStatus
	}{
		{
			name:     "unknown cannot complete work",
			result:   ActionResult{Executed: true, Success: false, OutcomeStatus: OutcomeUnknown, Observed: false, Verified: false},
			terminal: WorkCompleted,
		},
		{
			name:     "unknown cannot fail work terminally",
			result:   ActionResult{Executed: true, Success: false, OutcomeStatus: OutcomeUnknown, Observed: false, Verified: false},
			terminal: WorkFailed,
		},
		{
			name:     "success without observation cannot complete work",
			result:   ActionResult{Executed: true, Success: true, OutcomeStatus: OutcomeSucceeded, Observed: false, Verified: true},
			terminal: WorkCompleted,
		},
		{
			name:     "success without verification cannot complete work",
			result:   ActionResult{Executed: true, Success: true, OutcomeStatus: OutcomeSucceeded, Observed: true, Verified: false},
			terminal: WorkCompleted,
		},
		{
			name:     "failed outcome cannot report success",
			result:   ActionResult{Executed: true, Success: true, OutcomeStatus: OutcomeFailed, Observed: true, Verified: false},
			terminal: WorkFailed,
		},
		{
			name:     "success cannot close work as failed",
			result:   ActionResult{Executed: true, Success: true, OutcomeStatus: OutcomeSucceeded, Observed: true, Verified: true},
			terminal: WorkFailed,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := NewWorkStore(filepath.Join(t.TempDir(), "production_work.json"))
			work, err := store.Begin(interaction.Parse("preserve outcome semantics", interaction.ChannelText))
			if err != nil {
				t.Fatal(err)
			}
			if err := store.AttachProduction(work.ID, "", "", "provider-test"); err != nil {
				t.Fatal(err)
			}
			number, err := store.BeginAttempt(work.ID, "capability-test", "execute", "provider-test")
			if err != nil {
				t.Fatal(err)
			}
			if err := store.FinalizeAttempt(work.ID, number, tc.result, "", "", tc.terminal, "test closure"); err == nil {
				t.Fatal("expected incoherent result/closure contract to be rejected")
			}
			got, ok := store.Get(work.ID)
			if !ok {
				t.Fatal("work disappeared after rejected finalization")
			}
			if got.Status == WorkCompleted || got.Status == WorkFailed || got.LastActionResult != nil || got.Attempts[number-1].Status != "running" {
				t.Fatalf("rejected candidate leaked into canonical state: %+v", got)
			}
		})
	}
}

func TestFinalizeAttemptPersistenceFailureDoesNotPublishCandidate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "production_work.json")
	store := NewWorkStore(path)
	work, err := store.Begin(interaction.Parse("preserve canonical state", interaction.ChannelText))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AttachProduction(work.ID, "", "", "provider-test"); err != nil {
		t.Fatal(err)
	}
	number, err := store.BeginAttempt(work.ID, "capability-test", "create", "provider-test")
	if err != nil {
		t.Fatal(err)
	}
	// Force the durable rename to fail after the candidate has been prepared.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	result := ActionResult{Executed: true, Success: true, OutcomeStatus: OutcomeSucceeded, Observed: true, Verified: true, Summary: "verified"}
	if err := store.FinalizeAttempt(work.ID, number, result, "", "", WorkCompleted, "verified"); err == nil {
		t.Fatal("expected durable finalization to fail")
	}
	got, ok := store.Get(work.ID)
	if !ok {
		t.Fatal("work disappeared after failed commit")
	}
	if got.Status == WorkCompleted || len(got.Attempts) != 1 || got.Attempts[0].Status != "running" || got.LastActionResult != nil {
		t.Fatalf("failed candidate leaked into live state: %+v", got)
	}
}

func TestFinalizeRepairAttemptAtomicallyUpdatesObjectiveAndClosure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "production_work.json")
	store := NewWorkStore(path)
	work, err := store.Begin(interaction.Parse("initial objective", interaction.ChannelText))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AttachProduction(work.ID, "", "", "provider-test"); err != nil {
		t.Fatal(err)
	}
	number, err := store.BeginAttempt(work.ID, "capability-test", "repair", "provider-test")
	if err != nil {
		t.Fatal(err)
	}
	result := ActionResult{Executed: true, Success: true, OutcomeStatus: OutcomeSucceeded, Observed: true, Verified: true, Summary: "replacement verified"}
	if err := store.FinalizeRepairAttempt(work.ID, number, result, "replacement objective", "verified repair"); err != nil {
		t.Fatal(err)
	}
	reloaded := NewWorkStore(path)
	got, ok := reloaded.Get(work.ID)
	if !ok {
		t.Fatal("work missing after reload")
	}
	if got.Objective.Text != "replacement objective" || got.Status != WorkCompleted || got.Attempts[0].Status != "succeeded" {
		t.Fatalf("repair transition did not commit coherently: objective=%q work=%q attempt=%q", got.Objective.Text, got.Status, got.Attempts[0].Status)
	}
}

func TestAttachProductionDoesNotMutateWorkAwaitingReconciliation(t *testing.T) {
	store := NewWorkStore(filepath.Join(t.TempDir(), "production_work.json"))
	work, err := store.Begin(interaction.Parse("crear un artefacto", interaction.ChannelText))
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := store.BeginAttempt(work.ID, "files.create", "create_file", "files-provider")
	if err != nil {
		t.Fatal(err)
	}
	unknown := ActionResult{
		Executed: true, Success: false, OutcomeStatus: OutcomeUnknown,
		Observed: false, Verified: false, WorkID: work.ID, AttemptNumber: attempt,
		FailureReason: "effect_not_reconciled",
	}
	if err := store.FinalizeAttempt(work.ID, attempt, unknown, "", "reconciliation_required", "", "uncertain effect"); err != nil {
		t.Fatal(err)
	}
	before, ok := store.Get(work.ID)
	if !ok {
		t.Fatal("work missing before rejected attachment")
	}
	if err := store.AttachProduction(work.ID, "", "", "files-provider"); err == nil {
		t.Fatal("expected production attachment to be rejected while prior effect is unverified")
	}
	after, ok := store.Get(work.ID)
	if !ok {
		t.Fatal("work missing after rejected attachment")
	}
	if after.Status != before.Status {
		t.Fatalf("rejected attachment mutated Work status: before=%q after=%q", before.Status, after.Status)
	}
	if len(after.Events) != len(before.Events) {
		t.Fatalf("rejected attachment appended events: before=%d after=%d", len(before.Events), len(after.Events))
	}
	if len(after.Attempts) != 1 || after.Attempts[0].Status != "unverified" {
		t.Fatalf("uncertain attempt state changed: %+v", after.Attempts)
	}
}

func TestAttachProductionRejectsOrphanedRunningAttemptBeforeLifecycleMutation(t *testing.T) {
	store := NewWorkStore(filepath.Join(t.TempDir(), "production_work.json"))
	work, err := store.Begin(interaction.Parse("ejecutar una operación", interaction.ChannelText))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.BeginAttempt(work.ID, "files.create", "create_file", "files-provider"); err != nil {
		t.Fatal(err)
	}
	before, _ := store.Get(work.ID)
	if err := store.AttachProduction(work.ID, "", "", "files-provider"); err == nil {
		t.Fatal("expected a still-running attempt to block another production attachment")
	}
	after, _ := store.Get(work.ID)
	if after.Status != before.Status || len(after.Events) != len(before.Events) {
		t.Fatalf("blocked attachment mutated lifecycle: before=%+v after=%+v", before, after)
	}
}

func TestLegacyCompleteAttemptRejectsVerifiedSuccessWithoutObservation(t *testing.T) {
	store := NewWorkStore(filepath.Join(t.TempDir(), "production_work.json"))
	work, err := store.Begin(interaction.Parse("crear un artefacto", interaction.ChannelText))
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := store.BeginAttempt(work.ID, "files.create", "create_file", "files-provider")
	if err != nil {
		t.Fatal(err)
	}
	result := ActionResult{Success: true, Verified: true, Observed: false, WorkID: work.ID, AttemptNumber: attempt, Summary: "provider claims success"}
	before, _ := store.Get(work.ID)
	if err := store.CompleteAttempt(work.ID, attempt, result, "", ""); err == nil {
		t.Fatal("expected legacy completion to reject success without observation")
	}
	after, _ := store.Get(work.ID)
	if after.Attempts[0].Status != before.Attempts[0].Status || after.Attempts[0].Result != before.Attempts[0].Result || len(after.Events) != len(before.Events) {
		t.Fatalf("rejected legacy completion mutated Work: before=%+v after=%+v", before, after)
	}
}

func TestLegacyCompleteCannotCloseWorkWithUnknownAttempt(t *testing.T) {
	store := NewWorkStore(filepath.Join(t.TempDir(), "production_work.json"))
	work, err := store.Begin(interaction.Parse("crear un artefacto", interaction.ChannelText))
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := store.BeginAttempt(work.ID, "files.create", "create_file", "files-provider")
	if err != nil {
		t.Fatal(err)
	}
	unknown := ActionResult{Executed: true, Success: false, OutcomeStatus: OutcomeUnknown, WorkID: work.ID, AttemptNumber: attempt, FailureReason: "effect_unknown"}
	if err := store.FinalizeAttempt(work.ID, attempt, unknown, "", "reconciliation_required", "", "unknown effect"); err != nil {
		t.Fatal(err)
	}
	before, _ := store.Get(work.ID)
	if err := store.Complete(work.ID, true, "legacy completion must not bypass reconciliation"); err == nil {
		t.Fatal("expected legacy completion to reject an unverified attempt")
	}
	after, _ := store.Get(work.ID)
	if after.Status != before.Status || len(after.Events) != len(before.Events) {
		t.Fatalf("rejected completion mutated Work: before=%+v after=%+v", before, after)
	}
}

func TestWorkStoreRestartConvertsInterruptedRunningAttemptToUnverified(t *testing.T) {
	path := filepath.Join(t.TempDir(), "production_work.json")
	before := NewWorkStore(path)
	w, err := before.Begin(interaction.Parse("Crear un archivo", interaction.ChannelText))
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := before.BeginAttempt(w.ID, "files.create", "create_file", "files-provider")
	if err != nil {
		t.Fatal(err)
	}

	after := NewWorkStore(path)
	got, ok := after.Get(w.ID)
	if !ok {
		t.Fatal("work missing after restart")
	}
	if len(got.Attempts) != 1 || got.Attempts[0].Number != attempt {
		t.Fatalf("attempt identity changed after restart: %+v", got.Attempts)
	}
	if got.Attempts[0].Status != "unverified" {
		t.Fatalf("interrupted running attempt=%q, want unverified", got.Attempts[0].Status)
	}
	if got.Attempts[0].Result != nil {
		t.Fatal("restart invented an attempt result")
	}
	if len(got.Events) == 0 || got.Events[len(got.Events)-1].Type != "attempt_interrupted_on_restart" {
		t.Fatalf("missing restart reconciliation event: %+v", got.Events)
	}
	if _, err := after.BeginAttempt(w.ID, "files.create", "create_file", "files-provider"); err == nil {
		t.Fatal("new attempt bypassed reconciliation after restart")
	}

	again := NewWorkStore(path)
	reloaded, ok := again.Get(w.ID)
	if !ok || len(reloaded.Attempts) != 1 || reloaded.Attempts[0].Status != "unverified" {
		t.Fatalf("restart normalization was not durable: %+v", reloaded)
	}
}

func TestWorkStoreRestartPreservesExplicitAuthorizationInsteadOfTreatingItAsInterruptedEffect(t *testing.T) {
	path := filepath.Join(t.TempDir(), "production_work.json")
	before := NewWorkStore(path)
	req := interaction.Parse("Crear un archivo protegido", interaction.ChannelText)
	w, err := before.Begin(req)
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := before.BeginAttempt(w.ID, "files.create", "create_file", "files-provider")
	if err != nil {
		t.Fatal(err)
	}
	if err := before.SetPendingExecution(w.ID, PendingExecution{Request: req, CapabilityID: "files.create", Operation: "create_file", RouteKind: "production", ProviderID: "files-provider", AttemptNumber: attempt, Reason: "authorization_required"}); err != nil {
		t.Fatal(err)
	}

	after := NewWorkStore(path)
	got, ok := after.Get(w.ID)
	if !ok || got.PendingExecution == nil {
		t.Fatalf("authorization state lost after restart: %+v", got)
	}
	if got.Attempts[0].Status != "awaiting_authorization" {
		t.Fatalf("authorization attempt was converted incorrectly: %q", got.Attempts[0].Status)
	}
	if _, err := after.BeginAttempt(w.ID, "files.create", "create_file", "files-provider"); err == nil {
		t.Fatal("new attempt bypassed pending authorization")
	}
}
