package core

import (
	"fmt"
	"strings"
	"time"

	"ahmelia/interaction"
	"ahmelia/situation"
)

// B96.39-G2 — authoritative evidence projection.
// Cognition may compare the current structured semantic relation against
// already-confirmed UserModel beliefs, but it must not read raw user text or
// become a second UserModel authority. This helper performs a read-only
// projection from the canonical UserModel store into Situation-style evidence.
func (e *Engine) authoritativeUserModelEvidence(req interaction.SemanticRequest) []situation.Evidence {
	if e == nil || e.userModelStore == nil || len(req.Relations) == 0 {
		return nil
	}
	view, err := e.userModelStore.Read(interaction.UserModelAccessRequest{Actor: interaction.UserModelActorCognition, Mode: interaction.UserModelAccessRead})
	if err != nil {
		return nil
	}

	wanted := make(map[string]map[string]bool)
	for _, rel := range req.Relations {
		predicate := strings.ToLower(strings.TrimSpace(rel.Predicate))
		if predicate != "declares_attribute" && predicate != "declares_preference" {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(rel.Source), "user") && strings.TrimSpace(rel.Source) != "" {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(rel.Target))
		if key == "" || strings.TrimSpace(rel.Value) == "" {
			continue
		}
		if wanted[predicate] == nil {
			wanted[predicate] = map[string]bool{}
		}
		wanted[predicate][key] = true
	}
	if len(wanted) == 0 {
		return nil
	}

	now := time.Now()
	out := make([]situation.Evidence, 0)
	appendEvidence := func(predicate, key, value, sourceID string, confidence float64, observedAt time.Time, evidenceID string) {
		if confidence <= 0 || confidence > 1 || strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
			return
		}
		if observedAt.IsZero() {
			observedAt = now
		}
		id := strings.TrimSpace(evidenceID)
		if id == "" {
			id = "user_model:" + predicate + ":" + strings.ToLower(strings.TrimSpace(key))
		}
		out = append(out, situation.Evidence{
			ID:            id,
			Authority:     "interaction.UserModel",
			Source:        sourceID,
			Subject:       "user",
			Predicate:     predicate,
			Object:        strings.ToLower(strings.TrimSpace(key)),
			Value:         strings.TrimSpace(value),
			ObservedAt:    observedAt,
			Temporal:      situation.TemporalRecent,
			Relevance:     1,
			Confidence:    confidence,
			Epistemic:     situation.EpistemicObserved,
			Authoritative: true,
		})
	}

	for _, item := range view.Model.Attributes {
		if item.Status != interaction.UserModelStatusActive {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(item.Key))
		if wanted["declares_attribute"][key] {
			appendEvidence("declares_attribute", key, item.Value, "B96.34.UserAttribute", item.Confidence, item.UpdatedAt, item.Provenance.EvidenceID)
		}
	}
	for _, item := range view.Model.Preferences {
		if item.Status != interaction.UserModelStatusActive {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(item.Key))
		if wanted["declares_preference"][key] {
			appendEvidence("declares_preference", key, item.Value, "B96.34.UserPreference", item.Confidence, item.UpdatedAt, item.Provenance.EvidenceID)
		}
	}
	return out
}

// generalUserModelEvidence returns a broad projection of active UserModel
// attributes and preferences for contexts without a specific semantic request
// (e.g., autonomous planning, initiative). It does not filter by request
// relations; it returns all active attributes/preferences as Evidence.
func (e *Engine) generalUserModelEvidence() []situation.Evidence {
	if e == nil || e.userModelStore == nil {
		return nil
	}
	view, err := e.userModelStore.Read(interaction.UserModelAccessRequest{Actor: interaction.UserModelActorCognition, Mode: interaction.UserModelAccessRead})
	if err != nil {
		return nil
	}

	now := time.Now()
	out := make([]situation.Evidence, 0)
	appendEvidence := func(predicate, key, value, sourceID string, confidence float64, observedAt time.Time, evidenceID string) {
		if confidence <= 0 || confidence > 1 || strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
			return
		}
		if observedAt.IsZero() {
			observedAt = now
		}
		id := strings.TrimSpace(evidenceID)
		if id == "" {
			id = "user_model:" + predicate + ":" + strings.ToLower(strings.TrimSpace(key))
		}
		out = append(out, situation.Evidence{
			ID:            id,
			Authority:     "interaction.UserModel",
			Source:        sourceID,
			Subject:       "user",
			Predicate:     predicate,
			Object:        strings.ToLower(strings.TrimSpace(key)),
			Value:         strings.TrimSpace(value),
			ObservedAt:    observedAt,
			Temporal:      situation.TemporalRecent,
			Relevance:     1,
			Confidence:    confidence,
			Epistemic:     situation.EpistemicObserved,
			Authoritative: true,
		})
	}

	for _, item := range view.Model.Attributes {
		if item.Status != interaction.UserModelStatusActive {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(item.Key))
		appendEvidence("declares_attribute", key, item.Value, "B96.34.UserAttribute", item.Confidence, item.UpdatedAt, item.Provenance.EvidenceID)
	}
	for _, item := range view.Model.Preferences {
		if item.Status != interaction.UserModelStatusActive {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(item.Key))
		appendEvidence("declares_preference", key, item.Value, "B96.34.UserPreference", item.Confidence, item.UpdatedAt, item.Provenance.EvidenceID)
	}
	return out
}

// userModelContribution converts UserModel evidence into a Situation Contribution
// so it flows into DecisionEvidence via Evaluate. Used by autonomous planning,
// autonomous execution, and initiative to share the same authoritative user context.
func (e *Engine) userModelContribution(now time.Time) *situation.Contribution {
	evidence := e.generalUserModelEvidence()
	if len(evidence) == 0 {
		return nil
	}
	if now.IsZero() {
		now = time.Now()
	}
	contribution := &situation.Contribution{
		ID:          fmt.Sprintf("user_model-%d", now.UnixNano()),
		Perspective: "user_model",
		Confidence:  0.9,
		CreatedAt:   now,
		Provenance:  []string{"interaction.UserModel", "B96.34"},
	}
	for idx, ev := range evidence {
		id := fmt.Sprintf("user_model:%d", idx)
		contribution.Claims = append(contribution.Claims, fmt.Sprintf("%s: %s", ev.Predicate, ev.Value))
		contribution.Relations = append(contribution.Relations, situation.Relation{
			Source:      "user",
			Target:      "current_context",
			Type:        ev.Predicate,
			EvidenceIDs: []string{id},
			Provenance:  ev.Source,
			Temporal:    ev.Temporal,
			Relevance:   ev.Relevance,
			Confidence:  ev.Confidence,
			Epistemic:   ev.Epistemic,
		})
	}
	return contribution
}
