package socialinteraction

type ResponsePlanner struct{}

func NewResponsePlanner() *ResponsePlanner { return &ResponsePlanner{} }
func (p *ResponsePlanner) Plan(intent SocialIntent, state SocialState, boundaries []Boundary) ResponsePlan {
	allowed := true
	for _, b := range boundaries {
		if !b.Allowed {
			allowed = false
		}
	}
	plan := ResponsePlan{Participation: Respond, Intent: intent, State: state, Formality: Neutral, Brief: true}
	if !allowed {
		plan.Constraints = []string{"respect_social_boundary"}
	}
	switch intent {
	case Greeting:
		plan.Participation = Acknowledge
	case Farewell:
		plan.Participation = Close
	case Thanking, Apology, Congratulation, Acknowledgment:
		plan.Participation = Acknowledge
	case EmotionalSupportRequest:
		plan.Participation = Support
	case Humor, PlayfulInteraction:
		plan.Participation = React
	}
	return plan
}
