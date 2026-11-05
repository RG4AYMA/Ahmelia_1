package cognition

import (
	"strings"

	"ahmelia/interaction"
)

// PostExecutionRepairAssessment is cognition's provider-neutral assessment
// that a correction targets work which has already been materially produced.
// It does not authorize or execute anything.
type PostExecutionRepairAssessment struct {
	Required    bool
	WorkID      string
	Target      string
	Replacement string
	Confidence  float64
	Reason      string
}

// DetectPostExecutionRepair activates only when all three evidence conditions
// are present: an active work identity, a materialized prior ActionResult, and
// a correction semantically linked to that same work. The action result is
// intentionally kept downstream in the repair intent, where the provider can
// decide whether reconciliation means rename, mutate, overwrite, or recreate.
func DetectPostExecutionRepair(req interaction.SemanticRequest, conversation interaction.ConversationSnapshot, work interaction.WorkContext, hasWork bool) PostExecutionRepairAssessment {
	if !req.Correction.IsCorrection || strings.TrimSpace(req.Correction.Replacement) == "" {
		return PostExecutionRepairAssessment{Reason: "current turn is not a replacement correction"}
	}
	if !interaction.HasConcreteCorrectionReplacement(req.Correction.Replacement) {
		return PostExecutionRepairAssessment{Reason: "correction rejects prior meaning but provides no concrete replacement"}
	}
	if !hasWork || strings.TrimSpace(work.ID) == "" || strings.TrimSpace(conversation.ActiveWorkID) != strings.TrimSpace(work.ID) {
		return PostExecutionRepairAssessment{Reason: "correction has no active work owner"}
	}
	if !work.HasMaterializedResult || !work.LastActionSuccess || !work.LastActionVerified {
		return PostExecutionRepairAssessment{Reason: "active work has no successful verified materialized result"}
	}
	if !correctionReferencesMaterializedWork(req, conversation, work) {
		return PostExecutionRepairAssessment{Reason: "correction does not reference the active materialized work objective"}
	}

	target := strings.TrimSpace(req.Correction.Target)
	if target == "" {
		target = strings.TrimSpace(work.Objective)
	}
	return PostExecutionRepairAssessment{
		Required:    true,
		WorkID:      work.ID,
		Target:      target,
		Replacement: strings.TrimSpace(req.Correction.Replacement),
		Confidence:  req.Correction.Confidence,
		Reason:      "verified materialized work is the semantic target of the correction",
	}
}

func correctionReferencesMaterializedWork(req interaction.SemanticRequest, conversation interaction.ConversationSnapshot, work interaction.WorkContext) bool {
	if strings.TrimSpace(work.ID) == "" || len(conversation.Turns) == 0 {
		return false
	}
	current := conversation.Turns[len(conversation.Turns)-1]
	if current.Relation != interaction.RelationCorrection && current.Kind != interaction.KindCorrection {
		return false
	}

	// An explicit semantic reference to recent work/artifacts is sufficient
	// evidence even when the immediately preceding turn was conversational.
	if hasReference(req, interaction.ReferenceRecentWork) || hasReference(req, interaction.ReferenceRecentArtifact) {
		return true
	}

	// Otherwise the correction must target a turn that was itself attached to
	// the materialized work. Never use the current correction's own WorkLink:
	// the work bridge intentionally links every correction to active work for
	// continuity, and that alone would make unrelated corrections executable.
	target := strings.TrimSpace(req.Correction.Target)
	if normalizedRepairText(target) != "" && normalizedRepairText(target) == normalizedRepairText(work.Objective) {
		return true
	}

	targetTurnID := current.ParentTurnID
	if targetTurnID == 0 && target != "" {
		for _, turn := range conversation.Turns[:len(conversation.Turns)-1] {
			if strings.EqualFold(strings.TrimSpace(turn.UserText), target) {
				targetTurnID = turn.ID
				break
			}
		}
	}
	if targetTurnID == 0 {
		return false
	}

	for _, turn := range conversation.Turns[:len(conversation.Turns)-1] {
		if turn.ID != targetTurnID {
			continue
		}
		if normalizedRepairText(turn.Objective) != "" && normalizedRepairText(turn.Objective) == normalizedRepairText(work.Objective) {
			return true
		}
		break
	}

	for i := len(conversation.WorkLinks) - 1; i >= 0; i-- {
		link := conversation.WorkLinks[i]
		if link.WorkID != work.ID || link.SourceTurnID != targetTurnID {
			continue
		}
		if link.Relation != interaction.WorkRelationPreserve && link.Relation != interaction.WorkRelationNone {
			return true
		}
	}
	return false
}

func normalizedRepairText(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.Trim(value, " \t\r\n.,;:!?¿¡")
	return strings.Join(strings.Fields(value), " ")
}

func hasReference(req interaction.SemanticRequest, kind interaction.ReferenceKind) bool {
	for _, ref := range req.References {
		if ref.Kind == kind {
			return true
		}
	}
	return false
}
