package socialinteraction

// B96.38.19 — deterministic E2E harness for the social layer in isolation.
type E2E struct{ I *Integration }

func NewE2E() *E2E { return &E2E{NewIntegration()} }
func (e *E2E) Run(input string) (Assessment, ResponsePlan) {
	a := e.I.Assess(input, "text", StateNeutral)
	return a, e.I.Plan(a)
}
