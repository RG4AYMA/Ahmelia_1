package socialinteraction

type PolitenessPolicy struct{}

func NewPolitenessPolicy() *PolitenessPolicy { return &PolitenessPolicy{} }
func (p *PolitenessPolicy) Apply(plan ResponsePlan, formality Formality) ResponsePlan {
	plan.Formality = formality
	if formality == Formal || formality == VeryFormal {
		plan.Constraints = append(plan.Constraints, "use_formal_register")
	}
	return plan
}
