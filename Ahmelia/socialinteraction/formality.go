package socialinteraction

type FormalityPolicy struct{}

func NewFormalityPolicy() *FormalityPolicy               { return &FormalityPolicy{} }
func (p *FormalityPolicy) Resolve(ctx Context) Formality { return ctx.Formality }
