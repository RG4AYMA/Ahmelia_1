package core

import (
	"fmt"
	"sync"

	"ahmelia/autonomy"
	"ahmelia/planning"
)

// GoalStore is the Core-owned persistent Goal authority. It does not introduce
// another Goal model or another persistence file: it owns the existing
// Planner + AutonomousState + GoalPlanningAdapter graph and delegates durable
// storage to PersistAutonomyState().
type GoalStore struct {
	mu        sync.RWMutex
	planner   *planning.Planner
	state     *autonomy.AutonomousState
	adapter   *autonomy.GoalPlanningAdapter
	persister func() error
}

func NewGoalStore(planner *planning.Planner, state *autonomy.AutonomousState, adapter *autonomy.GoalPlanningAdapter) (*GoalStore, error) {
	if planner == nil || state == nil || adapter == nil {
		return nil, fmt.Errorf("goal store: planner/state/adapter no configurados")
	}
	return &GoalStore{planner: planner, state: state, adapter: adapter}, nil
}

func (s *GoalStore) SetPersister(persist func() error) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.persister = persist
}

func (s *GoalStore) Register(goal *planning.Goal) error {
	if s == nil || s.planner == nil {
		return fmt.Errorf("goal store no configurado")
	}
	if goal == nil || goal.ID == "" {
		return fmt.Errorf("goal store: goal inválido")
	}
	s.mu.Lock()
	s.planner.RegisterGoal(goal)
	s.mu.Unlock()
	return nil
}

func (s *GoalStore) Get(id string) (*planning.Goal, bool) {
	if s == nil || s.planner == nil || id == "" {
		return nil, false
	}
	for _, goal := range s.planner.Goals() {
		if goal != nil && goal.ID == id {
			return goal, true
		}
	}
	return nil, false
}

func (s *GoalStore) All() []*planning.Goal {
	if s == nil || s.planner == nil {
		return nil
	}
	return s.planner.Goals()
}

func (s *GoalStore) Current() *planning.Goal {
	if s == nil || s.state == nil {
		return nil
	}
	return s.state.CurrentGoal()
}

func (s *GoalStore) Pending() []*planning.Goal {
	if s == nil || s.state == nil {
		return nil
	}
	return s.state.PendingGoals()
}

func (s *GoalStore) SetCurrent(goal *planning.Goal) error {
	if s == nil || s.state == nil {
		return fmt.Errorf("goal store no configurado")
	}
	s.mu.Lock()
	s.state.SetCurrentGoal(goal)
	s.mu.Unlock()
	return s.persist()
}

func (s *GoalStore) AddPending(goal *planning.Goal) error {
	if s == nil || s.state == nil {
		return fmt.Errorf("goal store no configurado")
	}
	if goal == nil || goal.ID == "" {
		return fmt.Errorf("goal store: goal inválido")
	}
	s.mu.Lock()
	s.state.AddPendingGoal(goal)
	s.mu.Unlock()
	return s.persist()
}

func (s *GoalStore) RemovePending(id string) error {
	if s == nil || s.state == nil {
		return fmt.Errorf("goal store no configurado")
	}
	s.mu.Lock()
	s.state.RemovePendingGoal(id)
	s.mu.Unlock()
	return s.persist()
}

// ActivateOrQueue applies the single current/pending ownership rule used by
// both conversational and autonomous goal flows.
func (s *GoalStore) ActivateOrQueue(goal *planning.Goal) error {
	if goal == nil {
		return fmt.Errorf("goal store: goal inválido")
	}
	if s.Current() == nil {
		return s.SetCurrent(goal)
	}
	return s.AddPending(goal)
}

func (s *GoalStore) Persist() error { return s.persist() }

func (s *GoalStore) persist() error {
	if s == nil {
		return fmt.Errorf("goal store no configurado")
	}
	s.mu.RLock()
	persister := s.persister
	s.mu.RUnlock()
	if persister == nil {
		return nil
	}
	return persister()
}
