package socialinteraction

type AppropriatenessChecker struct{}

func NewAppropriatenessChecker() *AppropriatenessChecker { return &AppropriatenessChecker{} }
func (c *AppropriatenessChecker) Check(plan ResponsePlan, boundaries []Boundary) (ResponsePlan, bool) {
	for _, b := range boundaries {
		if !b.Allowed {
			plan.Constraints = append(plan.Constraints, b.Reason)
			if plan.Participation == React {
				plan.Participation = Acknowledge
			}
		}
	}
	return plan, true
}
