package integration

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"ahmelia/planning"
)

type fakeStructuredGoalPlanner struct {
	response string
	prompt   string
	schema   json.RawMessage
}

func (f *fakeStructuredGoalPlanner) GenerateStructuredContext(_ context.Context, prompt string, schema json.RawMessage, _ int) (string, error) {
	f.prompt = prompt
	f.schema = append(json.RawMessage(nil), schema...)
	return f.response, nil
}

func TestOllamaGoalTaskDecomposerBuildsGoalSpecificGraph(t *testing.T) {
	client := &fakeStructuredGoalPlanner{response: `{"tasks":[{"title":"Inspect workspace","description":"Inspect existing project files and scene","depends_on":[],"estimated_minutes":10},{"title":"Integrate gameplay","description":"Connect the scene, assets and scripts","depends_on":[0],"estimated_minutes":30},{"title":"Verify behavior","description":"Run a behavioral check and inspect the result","depends_on":[0,1],"estimated_minutes":15}]}`}
	decomposer := NewOllamaGoalTaskDecomposer(client)
	goal := planning.NewGoal("Survival prototype", "Create a Unity survival prototype with models and code", planning.PriorityHigh, "test")
	planningCtx := planning.WithEnvironmentBaseline(context.Background(), `{"focused_app":"Unity","open_apps":["Unity","Blender"]}`)
	blueprints, err := decomposer.DecomposeGoal(planningCtx, *goal)
	if err != nil {
		t.Fatal(err)
	}
	if len(blueprints) != 3 || len(blueprints[2].Dependencies) != 2 {
		t.Fatalf("unexpected graph: %#v", blueprints)
	}
	if !strings.Contains(client.prompt, "Unity survival prototype") || !strings.Contains(client.prompt, "Observed environment baseline") || !strings.Contains(client.prompt, "Blender") || len(client.schema) == 0 {
		t.Fatalf("goal context/schema missing from model request: prompt=%q schema=%s", client.prompt, client.schema)
	}
}

func TestOllamaGoalTaskDecomposerRejectsMalformedGraphShape(t *testing.T) {
	client := &fakeStructuredGoalPlanner{response: `{"tasks":[{"title":"Step","description":"Do work","depends_on":[0],"estimated_minutes":10}]}`}
	decomposer := NewOllamaGoalTaskDecomposer(client)
	goal := planning.NewGoal("Unknown", "Try an unprogrammed task", planning.PriorityMedium, "test")
	blueprints, err := decomposer.DecomposeGoal(context.Background(), *goal)
	if err != nil {
		t.Fatal(err)
	}
	planner := planning.NewPlanner()
	planner.SetGoalTaskDecomposer(decomposer)
	plan := planner.GeneratePlan(goal)
	if !plan.Feasible || len(plan.Tasks) == 0 || len(blueprints) != 1 {
		t.Fatalf("planner should validate and safely fall back from malformed graph: plan=%+v blueprints=%+v", plan, blueprints)
	}
}

func TestOllamaGoalTaskDecomposerPassesSameGoalAcrossEnvironmentVariants(t *testing.T) {
	variants := []struct {
		name     string
		baseline string
		contains string
	}{
		{name: "known-empty", baseline: "{}", contains: "{}"},
		{name: "known-editor", baseline: `{"focused_app":"Unity"}`, contains: "Unity"},
		{name: "known-multi-app", baseline: `{"open_apps":["Unity","Blender"]}`, contains: "Blender"},
		{name: "new-no-focused-app", baseline: `{"focused_app":"","open_apps":["Terminal"]}`, contains: "Terminal"},
		{name: "new-resource-pressure", baseline: `{"cpu_usage":96,"ram_usage":91}`, contains: `"cpu_usage":96`},
		{name: "new-observation-unavailable", baseline: "OBSERVATION_UNAVAILABLE: manager missing", contains: "OBSERVATION_UNAVAILABLE"},
		{name: "new-project-state-partial", baseline: `{"files":[],"status":"partial"}`, contains: `"status":"partial"`},
		{name: "new-large-baseline", baseline: strings.Repeat("x", 7000), contains: "truncated"},
	}
	for _, variant := range variants {
		t.Run(variant.name, func(t *testing.T) {
			client := &fakeStructuredGoalPlanner{response: `{"tasks":[{"title":"Inspect state","description":"Inspect observed context before deciding what to change","depends_on":[],"estimated_minutes":10}]}`}
			decomposer := NewOllamaGoalTaskDecomposer(client)
			goal := planning.NewGoal("Build a Unity scene", "Create and verify a small survival scene", planning.PriorityMedium, "test")
			ctx := planning.WithEnvironmentBaseline(context.Background(), variant.baseline)
			if _, err := decomposer.DecomposeGoal(ctx, *goal); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(client.prompt, variant.contains) {
				t.Fatalf("same goal prompt did not preserve baseline variant %q", variant.name)
			}
			if !strings.Contains(client.prompt, "do not infer unobserved facts") {
				t.Fatalf("prompt lacks uncertainty boundary for variant %q", variant.name)
			}
		})
	}
}
