package initiative

import (
	"fmt"
	"time"
)

type RuntimeSnapshot struct {
	Input SignalInput
	Now   time.Time
}
type SnapshotSource interface {
	Name() string
	CollectSnapshot(RuntimeSnapshot) []InitiativeSignal
}
type Adapter struct {
	SourceName string
	CollectFn  func(RuntimeSnapshot) []InitiativeSignal
}

func (a Adapter) Name() string { return a.SourceName }
func (a Adapter) CollectSnapshot(s RuntimeSnapshot) []InitiativeSignal {
	if a.CollectFn == nil {
		return nil
	}
	return a.CollectFn(s)
}

type RuntimePipeline struct{ Sources []SnapshotSource }

func NewRuntimePipeline(sources ...SnapshotSource) *RuntimePipeline {
	return &RuntimePipeline{Sources: append([]SnapshotSource(nil), sources...)}
}

type SourceFailure struct {
	Source string
	Error  string
}

// SignalsWithReport isolates individual source failures while retaining an
// auditable report for tests/diagnostics. A failed source never authorizes a
// decision and never prevents healthy sources from contributing evidence.
func (p *RuntimePipeline) SignalsWithReport(in SignalInput, now time.Time) ([]InitiativeSignal, []SourceFailure) {
	if p == nil {
		return nil, nil
	}
	if now.IsZero() {
		now = time.Now()
	}
	if in.Now.IsZero() {
		in.Now = now
	}
	snap := RuntimeSnapshot{Input: in, Now: now}
	var all []InitiativeSignal
	var failures []SourceFailure
	for _, src := range p.Sources {
		if src == nil {
			continue
		}
		func() {
			defer func() {
				if r := recover(); r != nil {
					failures = append(failures, SourceFailure{Source: src.Name(), Error: fmt.Sprint(r)})
				}
			}()
			all = append(all, src.CollectSnapshot(snap)...)
		}()
	}
	all = EnrichAndCorrelate(snap.Input, all, now)
	return NormalizeSignals(all, now), failures
}

func (p *RuntimePipeline) Signals(in SignalInput, now time.Time) []InitiativeSignal {
	signals, _ := p.SignalsWithReport(in, now)
	return signals
}
func (p *RuntimePipeline) Opportunity(in SignalInput, now time.Time) InitiativeOpportunity {
	return BuildOpportunity(p.Signals(in, now), in)
}
func FreshSignal(s InitiativeSignal, now time.Time) bool {
	if now.IsZero() {
		now = time.Now()
	}
	return s.ExpiresAt.IsZero() || !now.After(s.ExpiresAt)
}
func WithExpiry(s InitiativeSignal, ttl time.Duration) InitiativeSignal {
	if ttl > 0 {
		base := s.ObservedAt
		if base.IsZero() {
			base = time.Now()
		}
		s.ExpiresAt = base.Add(ttl)
	}
	return s
}

type Runtime struct {
	Engine   *SignalEngine
	Pipeline *RuntimePipeline
	Learner  *InitiativeLearner // B96.37.16 evidence/adaptation boundary; not a decision authority
	Now      func() time.Time
}

func NewRuntime(sources ...SignalSource) *Runtime {
	return &Runtime{Engine: NewSignalEngine(sources...), Pipeline: NewRuntimePipeline(RuntimeSourceAdapters()...), Learner: NewInitiativeLearner(nil), Now: time.Now}
}
func (r *Runtime) Collect(in SignalInput) []InitiativeSignal {
	if r == nil {
		return nil
	}
	if in.Now.IsZero() {
		if r.Now != nil {
			in.Now = r.Now()
		} else {
			in.Now = time.Now()
		}
	}
	if r.Pipeline == nil {
		r.Pipeline = NewRuntimePipeline(RuntimeSourceAdapters()...)
	}
	return r.Pipeline.Signals(in, in.Now)
}
func (r *Runtime) Opportunity(in SignalInput) InitiativeOpportunity {
	if r == nil {
		return BuildOpportunity(nil, in)
	}
	if in.Now.IsZero() {
		if r.Now != nil {
			in.Now = r.Now()
		} else {
			in.Now = time.Now()
		}
	}
	return BuildOpportunity(r.Collect(in), in)
}

// ObserveInitiativeOutcome feeds B96.37.15 outcome evidence into B96.37.16.
// It records adaptation evidence only; it does not re-evaluate or decide an initiative.
func (r *Runtime) ObserveInitiativeOutcome(out InitiativeOutcomeRecord) (LearningEvidence, error) {
	if r == nil {
		return LearningEvidence{}, fmt.Errorf("initiative runtime unavailable")
	}
	if r.Learner == nil {
		r.Learner = NewInitiativeLearner(nil)
	}
	return r.Learner.ObserveOutcome(out)
}

// InitiativeAdaptationSignals exposes bounded adaptation evidence to consumers
// such as B96.37.2. The signals contain no Decision or Action authority.
func (r *Runtime) InitiativeAdaptationSignals(now time.Time) []AdaptationSignal {
	if r == nil || r.Learner == nil {
		return nil
	}
	return r.Learner.Signals(now)
}
