package socialinteraction

type ReciprocityPolicy struct{}

func NewReciprocityPolicy() *ReciprocityPolicy { return &ReciprocityPolicy{} }
func (p *ReciprocityPolicy) Apply(plan ResponsePlan, input string) ResponsePlan {
	if len(input) < 30 {
		plan.Brief = true
	}
	return plan
}
