package planning

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

// Proposer es UNA estrategia de descomposición (LLM local, plantilla
// aprendida, receta extraída de un Work previo). El Planner no conoce nombres
// de objetivos: la selección entre estrategias es por evidencia, no por switch.
type Proposer struct {
	Name string
	D    GoalTaskDecomposer
}

// BlueprintCritic puntúa un plan candidato en [0,1] y explica por qué.
type BlueprintCritic interface {
	Score(goal Goal, bp []GoalTaskBlueprint) (float64, []string)
}

type WeightedCritic struct {
	C BlueprintCritic
	W float64
}

type CompositeCritic []WeightedCritic

func (cc CompositeCritic) Score(g Goal, bp []GoalTaskBlueprint) (float64, []string) {
	var sum, wsum float64
	var why []string
	for _, c := range cc {
		s, r := c.C.Score(g, bp)
		sum += c.W * s
		wsum += c.W
		why = append(why, r...)
	}
	if wsum == 0 {
		return 0, why
	}
	return sum / wsum, why
}

// StructuralCritic puntúa SOLO estructura del grafo (sin leer texto):
// paralelismo disponible, tamaño razonable y horizonte total.
type StructuralCritic struct{ MinTasks, MaxTasks int }

func (s StructuralCritic) Score(_ Goal, bp []GoalTaskBlueprint) (float64, []string) {
	n := len(bp)
	if n == 0 {
		return 0, []string{"empty"}
	}
	lo, hi := s.MinTasks, s.MaxTasks
	if lo == 0 {
		lo = 3
	}
	if hi == 0 {
		hi = 12
	}
	finish := make([]time.Duration, n)
	var total, cp time.Duration
	for i, b := range bp {
		var start time.Duration
		for _, d := range b.Dependencies {
			if d >= 0 && d < i && finish[d] > start {
				start = finish[d]
			}
		}
		finish[i] = start + b.TimeEstimate
		total += b.TimeEstimate
		if finish[i] > cp {
			cp = finish[i]
		}
	}
	par := 0.0
	if cp > 0 {
		par = float64(total)/float64(cp) - 1 // 0 = serial puro
		if par > 1 {
			par = 1
		}
	}
	size := 1.0
	if n < lo {
		size = float64(n) / float64(lo)
	} else if n > hi {
		size = float64(hi) / float64(n)
	}
	horizon := 1.0
	if h := total.Hours(); h > 40 {
		horizon = 40 / h
	}
	score := 0.35*par + 0.35*size + 0.30*horizon
	return score, []string{fmt.Sprintf("structural par=%.2f size=%.2f horizon=%.2f", par, size, horizon)}
}

type RankedPlan struct {
	Strategy   string
	Blueprints []GoalTaskBlueprint
	Score      float64
	Reasons    []string
}

// Deliberate genera K candidatos con estrategias distintas EN PARALELO, valida
// cada uno con la misma materialización que el camino adaptativo, los puntúa
// con el crítico y devuelve el mejor plan más el ranking completo.
// Si NINGUNA estrategia produce un plan válido devuelve error: no hay
// fallback silencioso a plantillas por nombre de objetivo.
func (p *Planner) Deliberate(ctx context.Context, goal *Goal, props []Proposer, critic BlueprintCritic) (*Plan, []RankedPlan, error) {
	if goal == nil {
		return nil, nil, errors.New("planning: nil goal")
	}
	if len(props) == 0 || critic == nil {
		return nil, nil, errors.New("planning: proposers and critic required")
	}
	type res struct {
		rp  RankedPlan
		err error
	}
	out := make([]res, len(props))
	var wg sync.WaitGroup
	for i, pr := range props {
		wg.Add(1)
		go func(i int, pr Proposer) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					out[i].err = fmt.Errorf("proposer %s panicked: %v", pr.Name, r)
				}
			}()
			bp, err := pr.D.DecomposeGoal(ctx, *goal)
			if err == nil {
				_, err = materializeTaskBlueprints(goal, bp) // validación estructural
			}
			if err != nil {
				out[i].err = fmt.Errorf("%s: %w", pr.Name, err)
				return
			}
			s, why := critic.Score(*goal, bp)
			out[i].rp = RankedPlan{Strategy: pr.Name, Blueprints: bp, Score: s, Reasons: why}
		}(i, pr)
	}
	wg.Wait()
	var ranked []RankedPlan
	var errs []error
	for _, r := range out {
		if r.err != nil {
			errs = append(errs, r.err)
			continue
		}
		ranked = append(ranked, r.rp)
	}
	if len(ranked) == 0 {
		return nil, nil, errors.Join(errs...)
	}
	sort.SliceStable(ranked, func(i, j int) bool { return ranked[i].Score > ranked[j].Score })
	tasks, err := materializeTaskBlueprints(goal, ranked[0].Blueprints)
	if err != nil {
		return nil, ranked, err
	}
	plan := &Plan{ID: fmt.Sprintf("plan-%d", time.Now().UnixNano()), GoalID: goal.ID, CreatedAt: time.Now(), Tasks: tasks, Priority: goal.Priority}
	for _, t := range tasks {
		plan.TotalEstimate += t.TimeEstimate
	}
	plan.Feasible, plan.Issues = p.validatePlan(plan)
	for _, e := range errs {
		plan.Issues = append(plan.Issues, "strategy discarded: "+e.Error())
	}
	p.orderTasksByDependencies(plan)
	return plan, ranked, nil
}
