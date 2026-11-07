package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"ahmelia/planning"
)

// StructuredGoalPlanner is the narrow model capability needed to derive a
// task graph. It does not execute tasks, authorize effects, or own plan state.
type StructuredGoalPlanner interface {
	GenerateStructuredContext(context.Context, string, json.RawMessage, int) (string, error)
}

// OllamaGoalTaskDecomposer adapts the shared local model to the canonical
// planning.Planner. The planner still validates and materializes every task;
// model output is only a proposal and never becomes execution authority.
type OllamaGoalTaskDecomposer struct {
	client  StructuredGoalPlanner
	timeout time.Duration
}

func NewOllamaGoalTaskDecomposer(client StructuredGoalPlanner) *OllamaGoalTaskDecomposer {
	return &OllamaGoalTaskDecomposer{client: client, timeout: 45 * time.Second}
}

// NewOllamaGoalTaskDecomposerWithTimeout is the background-lane constructor.
// It keeps the same validation and fallback; only the model budget changes.
func NewOllamaGoalTaskDecomposerWithTimeout(client StructuredGoalPlanner, timeout time.Duration) *OllamaGoalTaskDecomposer {
	if timeout <= 0 {
		timeout = 45 * time.Second
	}
	return &OllamaGoalTaskDecomposer{client: client, timeout: timeout}
}

func boundedPlanningBaseline(ctx context.Context) string {
	baseline := planning.EnvironmentBaseline(ctx)
	const maxBytes = 6000
	const marker = "…[truncated]"
	if len(baseline) > maxBytes {
		cutoff := maxBytes - len(marker)
		for cutoff > 0 && !utf8.ValidString(baseline[:cutoff]) {
			cutoff--
		}
		baseline = baseline[:cutoff] + marker
	}
	return baseline
}

func (d *OllamaGoalTaskDecomposer) DecomposeGoal(ctx context.Context, goal planning.Goal) ([]planning.GoalTaskBlueprint, error) {
	if d == nil || d.client == nil {
		return nil, fmt.Errorf("adaptive goal planner unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	timeout := d.timeout
	if timeout <= 0 {
		timeout = 45 * time.Second
	}
	bounded, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	prompt := fmt.Sprintf(`You are planning a real task graph for an autonomous agent.
Create a concrete, minimal but sufficient sequence of steps for this goal.
Do not assume that requested files, applications, assets, or components already exist.
Include inspection/discovery when the current state affects what must be done.
Include integration and behavioral verification when the goal spans multiple artifacts or systems.
Tasks must be actionable, specific to this goal, and useful even when the goal does not match a predefined template.
Order tasks topologically. depends_on contains zero-based indexes of earlier tasks only.
Do not claim that an operation has already happened. Do not put authorization decisions in the plan.
Return only the required JSON structure.

Goal name: %s
Goal description: %s
Priority: %v
Observed environment baseline (may be empty or partial; do not infer unobserved facts):
%s`, goal.Name, goal.Description, goal.Priority, boundedPlanningBaseline(ctx))

	var schema = json.RawMessage(`{
  "type":"object",
  "properties":{"tasks":{"type":"array","minItems":1,"maxItems":24,"items":{"type":"object","properties":{"title":{"type":"string"},"description":{"type":"string"},"depends_on":{"type":"array","items":{"type":"integer","minimum":0}},"estimated_minutes":{"type":"integer","minimum":1,"maximum":10080}},"required":["title","description","depends_on","estimated_minutes"],"additionalProperties":false}}},
  "required":["tasks"],"additionalProperties":false
}`)
	raw, err := d.client.GenerateStructuredContext(bounded, prompt, schema, 1800)
	if err != nil {
		return nil, fmt.Errorf("generate task graph: %w", err)
	}
	var response struct {
		Tasks []struct {
			Title            string `json:"title"`
			Description      string `json:"description"`
			DependsOn        []int  `json:"depends_on"`
			EstimatedMinutes int    `json:"estimated_minutes"`
		} `json:"tasks"`
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&response); err != nil {
		return nil, fmt.Errorf("decode task graph: %w", err)
	}
	if len(response.Tasks) == 0 || len(response.Tasks) > 24 {
		return nil, fmt.Errorf("model returned %d tasks; expected 1..24", len(response.Tasks))
	}
	blueprints := make([]planning.GoalTaskBlueprint, 0, len(response.Tasks))
	for i, task := range response.Tasks {
		if strings.TrimSpace(task.Title) == "" || strings.TrimSpace(task.Description) == "" {
			return nil, fmt.Errorf("model returned empty title or description at task %d", i)
		}
		minutes := task.EstimatedMinutes
		if minutes <= 0 {
			minutes = 60
		}
		dependencies := append([]int(nil), task.DependsOn...)
		blueprints = append(blueprints, planning.GoalTaskBlueprint{
			Title: task.Title, Description: task.Description,
			Dependencies: dependencies, TimeEstimate: time.Duration(minutes) * time.Minute,
		})
	}
	return blueprints, nil
}
