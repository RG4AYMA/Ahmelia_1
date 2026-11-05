package interaction

import (
	"fmt"
	"strings"
	"time"
)

// ProposalPurpose describes why Ahmelia is suggesting a course of action.
// It is domain-neutral and remains separate from capability execution.
type ProposalPurpose string

const (
	ProposalPurposeAdvance     ProposalPurpose = "advance"
	ProposalPurposeExplore     ProposalPurpose = "explore"
	ProposalPurposeOrganize    ProposalPurpose = "organize"
	ProposalPurposeImprove     ProposalPurpose = "improve"
	ProposalPurposeResume      ProposalPurpose = "resume"
	ProposalPurposePersonalize ProposalPurpose = "personalize"
)

// ProposalCandidate is a semantic proposal, not an executable action.
// Response planning/language render it for the user; downstream work begins
// only after the proposal is accepted through the normal interaction path.
type ProposalCandidate struct {
	Purpose         ProposalPurpose
	Title           string
	Proposal        string
	Rationale       string
	ExpectedBenefit string
	Priority        float64
	Confidence      float64
	Topic           string
	Objective       string
	RequiresConsent bool
	CreatedAt       time.Time
}

// ProposalGenerationContext contains only conversational evidence. The
// generator is intentionally capability-agnostic.
type ProposalGenerationContext struct {
	Request  SemanticRequest
	Snapshot ConversationSnapshot
	Profile  *UserProfile
}

// ProposalGenerator turns conversational evidence into ranked proposals.
// It does not execute, authorize, or silently commit the user to anything.
type ProposalGenerator struct {
	MaxCandidates int
}

func DefaultProposalGenerator() ProposalGenerator {
	return ProposalGenerator{MaxCandidates: 3}
}

func (g ProposalGenerator) Generate(ctx ProposalGenerationContext) []ProposalCandidate {
	max := g.MaxCandidates
	if max <= 0 {
		max = 3
	}

	req := ctx.Request
	s := ctx.Snapshot

	// A direct need must be handled before unsolicited proposals.
	acts := CanonicalActs(req)
	if acts.Has(ActQuestion) || acts.Has(ActCommand) || acts.Has(ActCorrection) || acts.Has(ActControl) || req.NeedsClarification {
		return nil
	}
	state := s.UserState
	if state.Urgency >= 0.65 || state.Frustration >= 0.65 || state.Confusion >= 0.65 {
		return nil
	}

	var out []ProposalCandidate
	topic := strings.TrimSpace(s.ActiveTopic)
	if topic == "" {
		topic = strings.TrimSpace(req.Topic)
	}
	objective := strings.TrimSpace(s.ActiveObjective)
	if objective == "" {
		objective = strings.TrimSpace(req.Objective.Text)
	}

	// Prefer proposals that advance something already alive in the dialogue.
	if objective != "" {
		label := objective
		if topic != "" {
			label = topic
		}
		out = append(out, ProposalCandidate{
			Purpose:         ProposalPurposeAdvance,
			Title:           "Siguiente paso",
			Proposal:        fmt.Sprintf("Podríamos avanzar con %s y revisar el resultado antes de decidir el siguiente paso.", label),
			Rationale:       "there is an active conversational objective that can be advanced incrementally",
			ExpectedBenefit: "mantener el objetivo activo sin convertir la propuesta en una acción automática",
			Priority:        0.60, Confidence: 0.76, Topic: topic, Objective: objective,
			RequiresConsent: true, CreatedAt: time.Now(),
		})
	}

	// Open threads are a natural source for a resumable proposal.
	for _, t := range s.OpenThreads {
		if len(out) >= max || t.Status == "completed" || t.Status == "abandoned" {
			continue
		}
		if strings.TrimSpace(t.Topic) == "" {
			continue
		}
		out = append(out, ProposalCandidate{
			Purpose:         ProposalPurposeResume,
			Title:           "Retomar hilo",
			Proposal:        fmt.Sprintf("Si te parece, podríamos retomar %s y ver qué quedó pendiente.", strings.TrimSpace(t.Topic)),
			Rationale:       "an open conversational thread can be resumed without deleting other threads",
			ExpectedBenefit: "evitar que una conversación útil se pierda entre interrupciones",
			Priority:        0.55, Confidence: 0.78, Topic: strings.TrimSpace(t.Topic),
			Objective: strings.TrimSpace(t.Objective), RequiresConsent: true, CreatedAt: time.Now(),
		})
		break
	}

	// A user-model proposal is deliberately about collaboration style, not a
	// fabricated personal fact. It lets personalization emerge from evidence.
	if ctx.Profile != nil && len(out) < max {
		if _, ok := ctx.Profile.Recall("communication", "preferred_response_style"); ok {
			// Already known: do not repeatedly propose learning the same thing.
		} else if strings.TrimSpace(req.Raw) != "" &&
			(acts.Has(ActSocial) || acts.Has(ActInform) || acts.Has(ActProposal) || acts.Primary == ActUnknown) {
			out = append(out, ProposalCandidate{
				Purpose:         ProposalPurposePersonalize,
				Title:           "Ajustar cómo trabajamos",
				Proposal:        "Podríamos ir ajustando la forma en que te acompaño según lo que vaya aprendiendo de nuestras conversaciones.",
				Rationale:       "the user model can improve collaboration gradually instead of requiring a fixed preference questionnaire",
				ExpectedBenefit: "hacer futuras interacciones más naturales y útiles",
				Priority:        0.48, Confidence: 0.68, Topic: topic,
				Objective: objective, RequiresConsent: true, CreatedAt: time.Now(),
			})
		}
	}

	// If there is no durable objective, a proposal may still open a
	// collaborative direction, but only after the user has contributed content.
	if len(out) == 0 && strings.TrimSpace(req.Raw) != "" &&
		(acts.Has(ActSocial) || acts.Has(ActInform) || acts.Primary == ActUnknown) {
		out = append(out, ProposalCandidate{
			Purpose:         ProposalPurposeExplore,
			Title:           "Explorar",
			Proposal:        "Podríamos profundizar un poco en esto y ver si aparece una dirección que merezca la pena seguir.",
			Rationale:       "the latest turn contains conversational material without a defined work objective",
			ExpectedBenefit: "convertir una conversación abierta en una dirección compartida solo si resulta útil",
			Priority:        0.44, Confidence: 0.64, Topic: topic,
			Objective: objective, RequiresConsent: true, CreatedAt: time.Now(),
		})
	}

	return rankProposals(out, max)
}

func rankProposals(in []ProposalCandidate, max int) []ProposalCandidate {
	for i := 1; i < len(in); i++ {
		v := in[i]
		j := i - 1
		for ; j >= 0 && in[j].Priority < v.Priority; j-- {
			in[j+1] = in[j]
		}
		in[j+1] = v
	}
	if len(in) > max {
		in = in[:max]
	}
	return in
}

// GenerateProposal is the compact single-proposal API for initiative and
// response-planning layers.
func GenerateProposal(ctx ProposalGenerationContext) (ProposalCandidate, bool) {
	candidates := DefaultProposalGenerator().Generate(ctx)
	if len(candidates) == 0 {
		return ProposalCandidate{}, false
	}
	return candidates[0], true
}
