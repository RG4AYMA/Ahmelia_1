package programming

// CompatibilityJudgment is the structured result of comparing one expected
// programming effect with one primary filesystem observation.
//
// Compatible answers only whether the observed state is compatible with the
// expected CREATE effect. It makes no claim about execution history, causality,
// completion, authorization, or what should happen next.
type CompatibilityJudgment struct {
	Compatible bool `json:"compatible"`
}

// EvaluateCreateCompatibility is the programming-domain compatibility
// boundary for the recovered CREATE experiment. It depends only on the
// expected MultiFileChange and the primary ObservedFile evidence.
func EvaluateCreateCompatibility(change MultiFileChange, observed ObservedFile) CompatibilityJudgment {
	return CompatibilityJudgment{
		Compatible: CreateCompatible(change, observed),
	}
}
