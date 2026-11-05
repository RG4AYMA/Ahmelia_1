package initiative

import "strings"

type sourceAdapter struct {
	name    string
	collect func(RuntimeSnapshot) []InitiativeSignal
}

func (a sourceAdapter) Name() string { return a.name }
func (a sourceAdapter) CollectSnapshot(s RuntimeSnapshot) []InitiativeSignal {
	if a.collect == nil {
		return nil
	}
	return a.collect(s)
}
func RuntimeSourceAdapters() []SnapshotSource {
	return []SnapshotSource{conversationContextAdapter(), cognitionAdapter(), authoritativeDisagreementAdapter(), userModelAdapter(), historicalMemoryAdapter(), workAdapter(), commitmentAdapter(), autonomyAdapter(), availabilityAdapter(), privacySafetyAdapter(), channelAdapter(), deferredLinguisticAdapter()}
}
func contextSignal(s RuntimeSnapshot, source Source, reason Reason, typ, value, target string, confidence float64) InitiativeSignal {
	return makeSignal(s.Input, source, reason, typ, value, target, true, confidence)
}
func conversationContextAdapter() SnapshotSource {
	return sourceAdapter{name: "conversational_context", collect: func(s RuntimeSnapshot) []InitiativeSignal {
		in := s.Input
		var out []InitiativeSignal
		if strings.TrimSpace(in.ActiveTopic) != "" || strings.TrimSpace(in.ActiveObjective) != "" {
			t := in.ActiveTopic
			if t == "" {
				t = in.ActiveObjective
			}
			out = append(out, contextSignal(s, SourceConversationalContext, ReasonConversationContinue, "active_context", t, t, .75))
		}
		if in.CandidateExists {
			out = append(out, contextSignal(s, SourceConversationalContext, ReasonConversationContinue, "question_candidate", in.CandidateText, in.CandidateTopic, in.CandidateConfidence))
		}
		return out
	}}
}
func cognitionAdapter() SnapshotSource {
	return sourceAdapter{name: "cognition", collect: func(s RuntimeSnapshot) []InitiativeSignal {
		if s.Input.UserConfusion <= .65 {
			return nil
		}
		return []InitiativeSignal{contextSignal(s, SourceInteractionContinuity, ReasonClarificationNeeded, "cognitive_confusion", "high", s.Input.ActiveTopic, s.Input.UserConfusion)}
	}}
}

func authoritativeDisagreementAdapter() SnapshotSource {
	return sourceAdapter{name: "cognition_authoritative_disagreement", collect: func(s RuntimeSnapshot) []InitiativeSignal {
		var out []InitiativeSignal
		for _, finding := range s.Input.DecisionEvidence.Findings {
			if strings.TrimSpace(finding.Kind) != "contradicts_authoritative_evidence" || finding.Confidence < .70 || len(finding.EvidenceIDs) < 2 {
				continue
			}
			sig := contextSignal(s, SourceCognitionConflict, ReasonContradictsAuthoritativeEvidence, "authoritative_disagreement", finding.Target, finding.Target, finding.Confidence)
			sig.Priority = PriorityMedium
			sig.Urgency = UrgencyMedium
			sig.EvidenceIDs = append([]string(nil), finding.EvidenceIDs...)
			sig.Provenance.Component = "cognition.B96.33.8"
			sig.Provenance.Subsystem = "B96.33.8"
			sig.Provenance.OriginID = strings.Join(finding.EvidenceIDs, ",")
			out = append(out, sig)
		}
		return out
	}}
}
func userModelAdapter() SnapshotSource {
	return sourceAdapter{name: "user_model", collect: func(s RuntimeSnapshot) []InitiativeSignal {
		if s.Input.UserFrustration <= .65 {
			return nil
		}
		return []InitiativeSignal{contextSignal(s, SourceUserModel, ReasonPotentialProblem, "user_frustration", "high", s.Input.ActiveTopic, s.Input.UserFrustration)}
	}}
}
func historicalMemoryAdapter() SnapshotSource {
	return sourceAdapter{name: "historical_memory", collect: func(s RuntimeSnapshot) []InitiativeSignal {
		in := s.Input
		if !in.HistoricalAvailable && !in.HistoricalRequired {
			return nil
		}
		value, typ := "available", "historical_context_available"
		if in.HistoricalRequired && !in.HistoricalAvailable {
			value, typ = "required", "historical_context_required"
		}
		return []InitiativeSignal{contextSignal(s, SourceConversationalContext, ReasonUsefulInformation, typ, value, in.ActiveTopic, .85)}
	}}
}
func workAdapter() SnapshotSource {
	return sourceAdapter{name: "work", collect: func(s RuntimeSnapshot) []InitiativeSignal {
		if strings.TrimSpace(s.Input.ActiveWorkID) == "" {
			return nil
		}
		return []InitiativeSignal{contextSignal(s, SourceOperationalWork, ReasonNextStepAvailable, "active_work", s.Input.ActiveWorkID, s.Input.ActiveTopic, .75)}
	}}
}
func commitmentAdapter() SnapshotSource {
	return sourceAdapter{name: "commitment", collect: func(s RuntimeSnapshot) []InitiativeSignal {
		if s.Input.CommitmentCount <= 0 {
			return nil
		}
		return []InitiativeSignal{contextSignal(s, SourceOpenCommitment, ReasonUnfinishedCommitment, "open_commitment", "unfinished_commitment", s.Input.ActiveTopic, .85)}
	}}
}
func autonomyAdapter() SnapshotSource {
	return sourceAdapter{name: "autonomy", collect: func(s RuntimeSnapshot) []InitiativeSignal {
		if s.Input.UserUrgency < .9 {
			return nil
		}
		x := contextSignal(s, SourceAutonomousSignal, ReasonWarning, "autonomous_signal", "critical_attention", s.Input.ActiveTopic, s.Input.UserUrgency)
		x.Urgency = UrgencyCritical
		x.Priority = PriorityCritical
		return []InitiativeSignal{x}
	}}
}
func availabilityAdapter() SnapshotSource {
	return sourceAdapter{name: "availability", collect: func(s RuntimeSnapshot) []InitiativeSignal {
		if s.Input.UserAvailability == AvailabilityUnknown {
			return nil
		}
		return []InitiativeSignal{contextSignal(s, SourceInteractionContinuity, ReasonConversationContinue, "user_availability", string(s.Input.UserAvailability), "", .95)}
	}}
}
func privacySafetyAdapter() SnapshotSource {
	return sourceAdapter{name: "privacy_safety", collect: func(s RuntimeSnapshot) []InitiativeSignal {
		in := s.Input
		if in.Privacy == InformationAllowed && in.Interruptibility == InterruptSafe {
			return nil
		}
		var out []InitiativeSignal
		if in.Privacy != InformationAllowed {
			out = append(out, contextSignal(s, SourceUserModel, ReasonPotentialProblem, "information_policy", string(in.Privacy), "", .95))
		}
		if in.Interruptibility != InterruptSafe {
			out = append(out, contextSignal(s, SourceInteractionContinuity, ReasonPotentialProblem, "interruptibility", string(in.Interruptibility), "", .9))
		}
		return out
	}}
}
func channelAdapter() SnapshotSource {
	return sourceAdapter{name: "channel", collect: func(s RuntimeSnapshot) []InitiativeSignal {
		if strings.TrimSpace(s.Input.Channel) == "" {
			return nil
		}
		return []InitiativeSignal{contextSignal(s, SourceInteractionContinuity, ReasonConversationContinue, "channel", s.Input.Channel, "", .8)}
	}}
}

func deferredLinguisticAdapter() SnapshotSource {
	return sourceAdapter{name: "deferred_linguistic", collect: func(s RuntimeSnapshot) []InitiativeSignal {
		return (DeferredLinguisticSignalSource{}).Collect(s.Input)
	}}
}
