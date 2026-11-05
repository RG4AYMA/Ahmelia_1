package socialinteraction

import "strings"

type BoundaryEvaluator struct{}

func NewBoundaryEvaluator() *BoundaryEvaluator { return &BoundaryEvaluator{} }
func (b *BoundaryEvaluator) Evaluate(input string, ctx Context, intent SocialIntent) []Boundary {
	l := strings.ToLower(input)
	joke := intent == Humor || intent == PlayfulInteraction
	out := []Boundary{{Allowed: true, Reason: "no social boundary conflict detected"}}
	if joke && (ctx.Formality == Formal || ctx.Formality == VeryFormal) {
		out = []Boundary{{Allowed: false, Reason: "humor is constrained in formal context"}}
	}
	if hasAny(l, "no quiero hablar", "déjame", "dejame") {
		out = []Boundary{{Allowed: false, Reason: "user signals desire to disengage"}}
	}
	return out
}
