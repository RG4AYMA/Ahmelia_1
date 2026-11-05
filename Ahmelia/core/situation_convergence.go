package core

import (
	"strings"
	"time"

	"ahmelia/integration"
	"ahmelia/interaction"
	"ahmelia/situation"
)

// buildSharedSituation is the common projection boundary for interactive and
// autonomous cognition. Domain stores remain the owners; this function only
// assembles their current projections into the derived Situation contract.
func (e *Engine) buildSharedSituation(
	req interaction.SemanticRequest,
	conversation interaction.ConversationSnapshot,
	work interaction.WorkContext,
	hasWork bool,
	observations []situation.Observation,
) (situation.Situation, []situation.Contribution) {
	worldEvidence := situation.WorldEvidence{Source: "environment.WorldSnapshot"}
	if e != nil && e.world != nil {
		if snapshot, ok := e.world.WorldSnapshot(); ok && snapshot != nil {
			worldEvidence.Available = true
			worldEvidence.App, worldEvidence.Activity = snapshot.AppName, string(snapshot.Activity)
			worldEvidence.Project, worldEvidence.File, worldEvidence.Workspace = snapshot.Project, snapshot.File, snapshot.Workspace
			worldEvidence.Unsaved = snapshot.HasUnsavedChanges
			worldEvidence.ObservedAt = time.Now()
		}
	}

	// Durable feedback follows the explicit conversation-to-work link, or the
	// canonical Work projection supplied by the autonomous planner. Never infer
	// ownership from WorkStore.Last, which could leak an unrelated old result.
	workID := ""
	if hasWork {
		workID = strings.TrimSpace(work.ID)
	}
	if workID == "" {
		workID = strings.TrimSpace(conversation.ActiveWorkID)
	}
	if e != nil && e.workStore != nil && workID != "" {
		if persistedWork, ok := e.workStore.Get(workID); ok {
			observations = append(observations, persistedWorkOutcomeObservations(persistedWork)...)
		}
	}

	sit := situation.Build(situation.Input{
		Request: req, Conversation: conversation, Work: work, HasWork: hasWork,
		World: worldEvidence, Observations: observations, Now: time.Now(),
	})
	contributions := []situation.Contribution{
		situation.ConversationalPerspective(sit),
		situation.OperationalPerspective(sit),
	}
	// P2.3/T4-M5 (19/10): la lectura de competencia entra como perspectiva
	// independiente del mismo contrato (ES una lectura, no vota, no muta la
	// situación, JAMÁS ejecuta). Sin overlay adjunto el bloque es inerte:
	// la lista de contribuciones queda exactamente como antes.
	if e != nil && e.competenceOverlay != nil {
		contributions = append(contributions, integration.CompetencePerspective(sit, e.competenceOverlay.Capabilities()))
	}
	// P4 (directiva Paso 2, 19/10): las preferencias FORMADAS del posterior
	// entran como contribución del mismo contrato (patrón
	// CompetencePerspective) para que situation.Evaluate las convierta en
	// DecisionFindings legibles por las propuestas. Sin preferencias formadas
	// no hay contribución — la ausencia no se disimula. JAMÁS ejecuta o
	// autoriza; la autoridad sigue siendo B96.34.
	if pc := e.preferenceContribution(sit); len(pc) > 0 {
		contributions = append(contributions, attachContributionsToSituation(sit, pc...)...)
	}
	return sit, contributions
}

// attachContributionsToSituation normalizes specialist contribution IDs to the
// Situation they read (sit-...:<provenance>), the same convention the
// situation-built perspectives (conversation/operational) already use. Each
// producer composes its own provenance ID; attachment happens once, where the
// contribution set is incorporated, so every contribution in the shared set is
// traceable to the single Situation it describes.
func attachContributionsToSituation(sit situation.Situation, contributions ...situation.Contribution) []situation.Contribution {
	if strings.TrimSpace(sit.ID) == "" {
		return contributions
	}
	prefix := sit.ID + ":"
	for i := range contributions {
		id := strings.TrimSpace(contributions[i].ID)
		if id == "" || strings.HasPrefix(id, prefix) {
			continue
		}
		contributions[i].ID = prefix + id
	}
	return contributions
}
