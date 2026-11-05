package core

import (
	"time"

	conversationalinitiative "ahmelia/initiative"
	"ahmelia/interaction"
)

// prepareInitiativeOpportunity applies the already-existing enrichment/
// admissibility evidence layers to one candidate. None of these layers
// authorizes a conversational turn; B96.37.2 remains the sole authority.
func (e *Engine) prepareInitiativeOpportunity(
	op conversationalinitiative.InitiativeOpportunity,
	input conversationalinitiative.SignalInput,
	snap interaction.ConversationSnapshot,
	now time.Time,
) conversationalinitiative.InitiativeOpportunity {
	if e.initiativePriorityUrgencyPolicy != nil {
		if enriched, _, err := e.initiativePriorityUrgencyPolicy.Enrich(op, input, now); err == nil {
			op = enriched
		}
	}
	if e.initiativePrivacyPolicy != nil {
		privacyCtx := conversationalinitiative.PrivacyContext{
			Classification:      conversationalinitiative.InformationClassificationFromLegacy(op.Privacy),
			Audience:            conversationalinitiative.AudienceUserOnly,
			Channel:             input.Channel,
			Permission:          conversationalinitiative.PermissionAllowed,
			UserPrivate:         op.Source == conversationalinitiative.SourceUserModel,
			HistoricalMemory:    input.HistoricalAvailable || input.HistoricalRequired,
			UserModel:           op.Source == conversationalinitiative.SourceUserModel,
			WorkProject:         op.Source == conversationalinitiative.SourceOperationalWork,
			RequestedDisclosure: true,
			Now:                 now,
		}
		if enriched, _, err := conversationalinitiative.EnrichPrivacyOpportunity(op, privacyCtx, *e.initiativePrivacyPolicy, now); err == nil {
			op = enriched
		}
	}
	if e.initiativeInterruptionPolicy != nil {
		if enriched, _, err := e.initiativeInterruptionPolicy.EnrichOpportunity(op, input, now); err == nil {
			op = enriched
		}
	}
	if e.initiativeConversationLoadEvaluator != nil {
		turns := make([]conversationalinitiative.ConversationTurnObservation, 0, len(snap.Turns))
		for _, t := range snap.Turns {
			turns = append(turns, conversationalinitiative.ConversationTurnObservation{
				At: t.At, UserText: t.UserText, Assistant: t.Assistant, SpeechAct: string(t.SpeechAct),
			})
		}
		if enriched, _, err := e.initiativeConversationLoadEvaluator.EnrichOpportunity(op, turns, input.Now); err == nil {
			op = enriched
		}
	}
	return op
}
