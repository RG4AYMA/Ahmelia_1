package socialinteraction

type StateEvaluator struct{}

func NewStateEvaluator() *StateEvaluator { return &StateEvaluator{} }
func (e *StateEvaluator) Evaluate(intent SocialIntent, formal Formality, previous SocialState) SocialState {
	if intent == Farewell {
		return StateClosing
	}
	if intent == Greeting {
		return StateOpening
	}
	if intent == UnknownIntent {
		if previous != "" {
			return previous
		}
		return StateNeutral
	}
	if formal == Formal || formal == VeryFormal {
		return StateFormal
	}
	if intent == Humor || intent == PlayfulInteraction {
		return StatePlayful
	}
	return StateEngaged
}
