package production

import (
	"context"
	"fmt"

	"ahmelia/autonomy"
	"ahmelia/planning"
	"ahmelia/tools"
)

// B95.18 — Multi-Step Production
//
// Composes the existing B95.17 single-step loop across an existing Plan.
// Production does not create another Task/Plan/Action model. A BindingProvider
// supplies the already selected CapabilityBinding for each runnable Task.
// Failed steps stop the run; recovery/replanning remains owned by B95.11.
type BindingProvider interface {
	BindTask(ctx context.Context, task *planning.Task) (CapabilityBinding, error)
}

type MultiStepProduction struct {
	production *Production
	tasks      *TaskSystem
	loop       *ProductionLoop
	bindings   BindingProvider
}

func NewMultiStepProduction(p *Production, tasks *TaskSystem, loop *ProductionLoop, bindings BindingProvider) (*MultiStepProduction, error) {
	if p == nil {
		return nil, fmt.Errorf("production: multi-step production nil")
	}
	if tasks == nil {
		return nil, fmt.Errorf("production: multi-step task system nil")
	}
	if loop == nil {
		return nil, fmt.Errorf("production: multi-step loop nil")
	}
	if bindings == nil {
		return nil, fmt.Errorf("production: multi-step binding provider nil")
	}
	return &MultiStepProduction{production: p, tasks: tasks, loop: loop, bindings: bindings}, nil
}

type MultiStepRunResult struct {
	ProductionID string
	Steps        []ProductionRunResult
	Completed    bool
	Failure      string
}

// Run executes runnable tasks in plan order until the plan is complete or one
// step fails. It never silently retries or replans. Each step goes through the
// canonical B95.17 execution path.
func (m *MultiStepProduction) Run(ctx context.Context, approvals map[string]autonomy.Approval, env tools.Environment) (MultiStepRunResult, error) {
	if m == nil {
		return MultiStepRunResult{}, fmt.Errorf("production: multi-step unavailable")
	}
	if err := ctx.Err(); err != nil {
		return MultiStepRunResult{}, err
	}
	if env == nil {
		return MultiStepRunResult{}, fmt.Errorf("production: multi-step environment nil")
	}

	result := MultiStepRunResult{ProductionID: m.production.ID, Steps: []ProductionRunResult{}}
	for {
		if err := ctx.Err(); err != nil {
			result.Failure = err.Error()
			return result, err
		}
		task := m.tasks.NextRunnable()
		if task == nil {
			if allTasksComplete(m.tasks.Tasks()) {
				if m.production.Status == ProductionPending {
					_ = m.production.Start()
				}
				if m.production.Status == ProductionRunning {
					if err := m.production.Complete(); err != nil {
						return result, err
					}
				}
				result.Completed = true
				return result, nil
			}
			result.Failure = "no hay tarea ejecutable: plan incompleto o bloqueado"
			return result, nil
		}
		binding, err := m.bindings.BindTask(ctx, task)
		if err != nil {
			result.Failure = err.Error()
			return result, nil
		}
		approval := approvals[binding.Action.ID]
		step, err := m.loop.run(ctx, binding, approval, env, false)
		if err != nil {
			return result, err
		}
		result.Steps = append(result.Steps, step)
		if !step.Success {
			result.Failure = step.Failure
			return result, nil
		}
	}
}

func allTasksComplete(tasks []*planning.Task) bool {
	if len(tasks) == 0 {
		return false
	}
	for _, task := range tasks {
		if task == nil || !task.IsCompleted() {
			return false
		}
	}
	return true
}

// StaticBindingProvider is useful when the caller has already selected all
// bindings. It never changes the bindings and rejects duplicate task IDs.
type StaticBindingProvider struct{ bindings map[string]CapabilityBinding }

func NewStaticBindingProvider(bindings []CapabilityBinding) (*StaticBindingProvider, error) {
	out := make(map[string]CapabilityBinding, len(bindings))
	for _, b := range bindings {
		if b.Task == nil || b.Task.ID == "" {
			return nil, fmt.Errorf("production: binding task inválida")
		}
		if _, exists := out[b.Task.ID]; exists {
			return nil, fmt.Errorf("production: binding duplicado para task %q", b.Task.ID)
		}
		out[b.Task.ID] = b
	}
	return &StaticBindingProvider{bindings: out}, nil
}

func (s *StaticBindingProvider) BindTask(_ context.Context, task *planning.Task) (CapabilityBinding, error) {
	if s == nil || task == nil {
		return CapabilityBinding{}, fmt.Errorf("production: static binding unavailable")
	}
	b, ok := s.bindings[task.ID]
	if !ok {
		return CapabilityBinding{}, fmt.Errorf("production: no existe binding para task %q", task.ID)
	}
	return b, nil
}
