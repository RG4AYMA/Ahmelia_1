package socialinteraction

type ContextAdapter struct{}

func NewContextAdapter() *ContextAdapter { return &ContextAdapter{} }
func (a *ContextAdapter) Adapt(plan ResponsePlan, ctx Context) ResponsePlan {
	plan.Formality = ctx.Formality
	if ctx.Group {
		plan.Constraints = append(plan.Constraints, "group_context")
	}
	if ctx.Public {
		plan.Constraints = append(plan.Constraints, "public_context")
	}
	return plan
}
