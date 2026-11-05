package core

import (
	"testing"
	"time"

	"ahmelia/interaction"
	"ahmelia/production"
	"ahmelia/situation"
)

func TestProductionProvenanceReachesSituationForImmediateAndPersistedResults(t *testing.T) {
	const (
		skillID             = "software.development"
		generationSource    = "opencode:test-model"
		verificationProfile = "build=passed;tests=not_run"
		objectiveStatus     = "unassessed"
		objectiveContract   = `{"primary_goal":"create feature","provenance":"semantic_request"}`
		targetEnvironment   = `C:\workspace\demo`
	)

	assertProvenance := func(t *testing.T, observations []situation.Observation, wantTemporal situation.TemporalStatus) {
		t.Helper()
		want := map[string]string{
			"production_skill_identity":        skillID,
			"production_generation_provenance": generationSource,
			"production_verification_scope":    verificationProfile,
			"production_objective_status":      objectiveStatus,
			"production_objective_contract":    objectiveContract,
			"production_target_environment":    targetEnvironment,
		}
		found := make(map[string]string, len(want))
		for _, observation := range observations {
			if _, expected := want[observation.Kind]; expected {
				found[observation.Kind] = observation.Object
				if observation.Temporal != wantTemporal {
					t.Errorf("%s temporal=%q want=%q", observation.Kind, observation.Temporal, wantTemporal)
				}
			}
		}
		for kind, expected := range want {
			if found[kind] != expected {
				t.Errorf("%s object=%q want=%q", kind, found[kind], expected)
			}
		}
	}

	t.Run("immediate outcome", func(t *testing.T) {
		store := production.NewWorkStore("")
		req := interaction.SemanticRequest{Action: interaction.ActionIntent{Name: "create_file"}, Objective: interaction.Objective{Text: "crear herramienta"}}
		work, err := store.Begin(req)
		if err != nil {
			t.Fatal(err)
		}
		attempt, err := store.BeginAttempt(work.ID, "programming.create_file", "create_file", "programming", skillID)
		if err != nil {
			t.Fatal(err)
		}
		engine := &Engine{workStore: NewPersistentWorkStore(store)}
		engine.convergeActionOutcome(req, production.ActionResult{
			WorkID: work.ID, AttemptNumber: attempt, ProductionID: "project-1", Producer: "programming",
			GenerationSource: generationSource, VerificationProfile: verificationProfile,
			ObjectiveStatus: objectiveStatus, ObjectiveContractJSON: objectiveContract,
			Project: production.ProductionProject{Path: targetEnvironment},
		})
		assertProvenance(t, engine.pendingSituationObservations, situation.TemporalCurrent)
	})

	t.Run("persisted outcome reconstruction", func(t *testing.T) {
		at := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
		work := production.WorkSession{
			ID: "work-persisted", UpdatedAt: at,
			Attempts: []production.WorkAttempt{{Number: 1, SkillID: skillID}},
			LastActionResult: &production.ActionResult{
				WorkID: "work-persisted", AttemptNumber: 1, ProductionID: "project-1", Producer: "programming",
				GenerationSource: generationSource, VerificationProfile: verificationProfile,
				ObjectiveStatus: objectiveStatus, ObjectiveContractJSON: objectiveContract,
				Project: production.ProductionProject{Path: targetEnvironment},
			},
		}
		assertProvenance(t, persistedWorkOutcomeObservations(work), situation.TemporalHistorical)
	})
}
