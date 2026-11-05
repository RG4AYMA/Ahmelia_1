package integration

import (
	"testing"
	"time"

	"ahmelia/production"
)

func TestProjectAttemptsCountsOnlyVerifiedAndNeverInflatesContexts(t *testing.T) {
	now := time.Now()
	ref := "proc:verify_go@2"
	w1 := production.WorkSession{ID: "w1", Artifacts: []production.ProductionArtifact{{ProjectID: "projA"}},
		Attempts: []production.WorkAttempt{
			{SkillID: ref, CapabilityID: "programming", Operation: "build", FinishedAt: now, Result: &production.ActionResult{Success: true, Verified: true}},
			{SkillID: ref, CapabilityID: "programming", Operation: "test", FinishedAt: now, Result: &production.ActionResult{Success: true, Verified: false}}, // éxito NO verificado
			{SkillID: "otro", FinishedAt: now, Result: &production.ActionResult{Verified: true}},
			{SkillID: ref, FinishedAt: time.Time{}}, // intento sin cerrar
		}}
	w2 := production.WorkSession{ID: "w2", Attempts: []production.WorkAttempt{
		{SkillID: ref, CapabilityID: "programming", Operation: "build", FinishedAt: now, Result: &production.ActionResult{Verified: true}}}}
	got := ProjectAttempts([]production.WorkSession{w1, w2}, ref)
	if len(got) != 3 {
		t.Fatalf("%d %+v", len(got), got)
	}
	if !got[0].Verified || got[1].Verified || got[0].ContextKey != "projA" || got[2].ContextKey != "unknown" {
		t.Fatalf("%+v", got)
	}
}
