package socialinteraction

// B96.38.17 — engine-facing data adapter. It has no Engine dependency and does not initiate turns.
type Integration struct {
	Context         *ContextBuilder
	Signals         *SignalNormalizer
	Intents         *IntentDetector
	Cues            *CueAnalyzer
	States          *StateEvaluator
	Boundaries      *BoundaryEvaluator
	Responses       *ResponsePlanner
	Reciprocity     *ReciprocityPolicy
	Politeness      *PolitenessPolicy
	Acknowledgment  *AcknowledgmentPolicy
	Repair          *RepairDetector
	Participation   *ParticipationPolicy
	Adaptation      *ContextAdapter
	Formality       *FormalityPolicy
	Appropriateness *AppropriatenessChecker
}

func NewIntegration() *Integration {
	return &Integration{NewContextBuilder(), NewSignalNormalizer(), NewIntentDetector(), NewCueAnalyzer(), NewStateEvaluator(), NewBoundaryEvaluator(), NewResponsePlanner(), NewReciprocityPolicy(), NewPolitenessPolicy(), NewAcknowledgmentPolicy(), NewRepairDetector(), NewParticipationPolicy(), NewContextAdapter(), NewFormalityPolicy(), NewAppropriatenessChecker()}
}
func (i *Integration) Assess(input, channel string, previous SocialState) Assessment {
	ctx := i.Context.Build(input, channel)
	sig := i.Signals.Normalize(input)
	intent := i.Intents.Detect(input, sig)
	cues := i.Cues.Analyze(input, sig)
	state := i.States.Evaluate(intent, ctx.Formality, previous)
	bounds := i.Boundaries.Evaluate(input, ctx, intent)
	conf := 0.5
	if intent != UnknownIntent {
		conf = .9
	}
	return Assessment{Kind: ctx.Purpose, Context: ctx, Signals: sig, Cues: cues, Intent: intent, State: state, Formality: ctx.Formality, Boundaries: bounds, Confidence: conf}
}
func (i *Integration) Plan(a Assessment) ResponsePlan {
	p := i.Responses.Plan(a.Intent, a.State, a.Boundaries)
	p = i.Reciprocity.Apply(p, a.Context.Topic)
	p = i.Politeness.Apply(p, a.Formality)
	p = i.Adaptation.Adapt(p, a.Context)
	p.RepairRequired = i.Repair.Detect(a.Context.Topic, p)
	p = i.Participation.Apply(p)
	p, _ = i.Appropriateness.Check(p, a.Boundaries)
	return p
}
