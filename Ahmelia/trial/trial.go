// Package trial es el mecanismo de PRÁCTICA y de NO-REGRESIÓN: ejecuta un
// procedimiento sobre escenarios reproducibles (fixtures) y decide si una
// versión candidata puede reemplazar a la activa. No tiene LLM: la práctica
// es determinista y barata, por eso puede correr sin modelo y en reposo.
package trial

import (
	"context"
	"fmt"
	"sort"

	"ahmelia/procedure"
)

type Scenario struct {
	ID         string
	Slots      map[string]string // valores reales del fixture (nunca del texto web)
	ContextKey string
}

type Suite struct{ Scenarios []Scenario }

type Outcome struct {
	Verified     bool
	Unknown      bool // el oráculo no pudo observar: ni pasa ni falla
	FailureClass string
}

// Runner ejecuta el procedimiento en un workspace aislado y devuelve el
// veredicto del oráculo. La implementación real envuelve el runtime de
// producción; los tests usan uno falso.
type Runner interface {
	Run(ctx context.Context, p procedure.Procedure, s Scenario) (Outcome, error)
}

type Report struct {
	Per                 map[string]Outcome
	Passed, Total, Unkn int
}

func RunSuite(ctx context.Context, r Runner, p procedure.Procedure, s Suite) Report {
	rep := Report{Per: map[string]Outcome{}, Total: len(s.Scenarios)}
	for _, sc := range s.Scenarios {
		o, err := r.Run(ctx, p, sc)
		if err != nil {
			o = Outcome{Unknown: true, FailureClass: "runner_error"}
		}
		rep.Per[sc.ID] = o
		switch {
		case o.Unknown:
			rep.Unkn++
		case o.Verified:
			rep.Passed++
		}
	}
	return rep
}

type Decision struct {
	Promote      bool
	Reasons      []string
	Regressions  []string
	Improvements []string
}

// Gate: no certifica con resultados desconocidos; rechaza cualquier regresión
// estricta; sin versión activa exige todo verde; con versión activa exige
// mejora neta (evita churn sin evidencia).
func Gate(active *Report, cand Report) Decision {
	d := Decision{}
	if cand.Unkn > 0 {
		d.Reasons = append(d.Reasons, fmt.Sprintf("no certificable: %d escenario(s) con resultado desconocido", cand.Unkn))
		return d
	}
	if active != nil {
		for id, a := range active.Per {
			c := cand.Per[id]
			switch {
			case a.Verified && !c.Verified:
				d.Regressions = append(d.Regressions, id)
			case !a.Verified && c.Verified:
				d.Improvements = append(d.Improvements, id)
			}
		}
		sort.Strings(d.Regressions)
		sort.Strings(d.Improvements)
	}
	switch {
	case len(d.Regressions) > 0:
		d.Reasons = append(d.Reasons, "regresión en escenarios previamente verdes")
	case active == nil && cand.Passed == cand.Total && cand.Total > 0:
		d.Promote = true
		d.Reasons = append(d.Reasons, "primera versión: todos los escenarios verificados")
	case active == nil:
		d.Reasons = append(d.Reasons, "primera versión: no todos los escenarios pasan")
	case cand.Passed > active.Passed:
		d.Promote = true
		d.Reasons = append(d.Reasons, "mejora neta sin regresiones")
	default:
		d.Reasons = append(d.Reasons, "sin mejora neta sobre la versión activa")
	}
	return d
}

// Release evalúa una versión candidata contra la activa y, solo si la
// compuerta lo permite, la activa. Es la ÚNICA vía para cambiar comportamiento.
func Release(ctx context.Context, reg *procedure.Registry, r Runner, s Suite, id string, candidate int) (Decision, error) {
	cand, ok := reg.Get(id, candidate)
	if !ok {
		return Decision{}, fmt.Errorf("trial: %s@%d no existe", id, candidate)
	}
	var activeRep *Report
	if act, ok := reg.Active(id); ok {
		rp := RunSuite(ctx, r, act, s)
		activeRep = &rp
	}
	d := Gate(activeRep, RunSuite(ctx, r, cand, s))
	if d.Promote {
		if err := reg.Activate(id, candidate); err != nil {
			return d, err
		}
	}
	return d, nil
}
