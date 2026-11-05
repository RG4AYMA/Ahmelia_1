package competence

import "fmt"

// Standing es el nivel de lo que Ahmelia puede AFIRMAR sobre un procedimiento.
// Se DERIVA de evidencia en cada consulta; nunca se almacena (no hay deriva
// entre "lo que dice el estado" y "lo que demuestran los datos").
type Standing string

const (
	StandingHeard        Standing = "heard"        // texto/claim; no compila contra capacidades
	StandingCompiled     Standing = "compiled"     // pasos tipados válidos; sin práctica suficiente
	StandingTrialed      Standing = "trialed"      // aprobó práctica en sandbox
	StandingDemonstrated Standing = "demonstrated" // verificado en trabajo real, en >=N contextos
	StandingRegressed    Standing = "regressed"    // dejó de pasar algo que antes pasaba
	StandingUnverifiable Standing = "unverifiable" // sin oráculo: no se puede afirmar más
)

type Tally struct{ Runs, Pass int }

type Evidence struct {
	Compiled       bool
	HasOracle      bool // la capacidad declara cómo se verifica su resultado
	Trials         Tally
	Real           []Attempt // proyección de WorkAttempts con SkillID = Procedure.Ref()
	SuiteRegressed bool
}

type Policy struct {
	MinTrials, MinReal, MinContexts int
	LowerBound                      float64 // cota inferior de Wilson exigida
}

func DefaultPolicy() Policy { return Policy{MinTrials: 5, MinReal: 5, MinContexts: 2, LowerBound: 0.6} }

func Derive(e Evidence, p Policy) (Standing, string) {
	switch {
	case !e.Compiled:
		return StandingHeard, "no compila contra capacidades registradas"
	case !e.HasOracle:
		return StandingUnverifiable, "sin oráculo declarado no hay afirmación de competencia"
	case e.SuiteRegressed:
		return StandingRegressed, "falla un escenario que antes pasaba"
	}
	tr := wilson(e.Trials.Pass, e.Trials.Runs)
	if e.Trials.Runs < p.MinTrials || tr.Lo < p.LowerBound {
		return StandingCompiled, fmt.Sprintf("práctica insuficiente: %d/%d (lo=%.2f)", e.Trials.Pass, e.Trials.Runs, tr.Lo)
	}
	ok, ctx := 0, map[string]bool{}
	for _, a := range e.Real {
		if a.Verified {
			ok++
			ctx[a.ContextKey] = true
		}
	}
	rl := wilson(ok, len(e.Real))
	if len(e.Real) < p.MinReal || len(ctx) < p.MinContexts || rl.Lo < p.LowerBound {
		return StandingTrialed, fmt.Sprintf("real: %d/%d en %d contexto(s) (lo=%.2f)", ok, len(e.Real), len(ctx), rl.Lo)
	}
	return StandingDemonstrated, fmt.Sprintf("real: %d/%d en %d contextos (lo=%.2f)", ok, len(e.Real), len(ctx), rl.Lo)
}
