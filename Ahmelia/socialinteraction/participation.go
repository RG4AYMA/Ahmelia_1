package socialinteraction

type ParticipationPolicy struct{}

func NewParticipationPolicy() *ParticipationPolicy { return &ParticipationPolicy{} }
func (p *ParticipationPolicy) Apply(plan ResponsePlan) ResponsePlan {
	if plan.RepairRequired {
		plan.Participation = Repair
	}
	return plan
}
