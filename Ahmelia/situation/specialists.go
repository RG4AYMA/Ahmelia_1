package situation

import (
	"strings"
	"time"
)

// ConversationalPerspective reads only the already-reconciled Situation.
// It does not reinterpret raw text and does not decide a response.
func ConversationalPerspective(s Situation) Contribution {
	c := Contribution{ID: s.ID + ":conversation", Perspective: "conversation", CreatedAt: time.Now(), Confidence: s.Confidence, Provenance: []string{"interaction.SemanticRequest", "interaction.ConversationState", "B96.36"}}
	for _, r := range s.Relations {
		// Conversation owns linguistic/social/intentional relations. Keep the
		// whole non-operational semantic relation set so a novel relation is not
		// silently discarded merely because no specialist case existed yet.
		if !strings.HasPrefix(r.Provenance, "production.") && !strings.HasPrefix(r.Provenance, "environment.") {
			c.Relations = append(c.Relations, r)
		}
	}
	for _, r := range c.Relations {
		switch r.Type {
		case "addresses", "gives_feedback_to", "asks_about_state", "asks_about_opinion":
			c.Claims = append(c.Claims, "el turno contiene una relación social o intencional explícita con Ahmelia")
		case "proposes", "anticipates", "commits_to":
			c.Claims = append(c.Claims, "el turno contiene una relación orientada a una posibilidad o estado futuro")
		case "supports", "enables", "affects", "depends_on":
			c.Claims = append(c.Claims, "el turno contiene una relación entre elementos que puede afectar su interpretación")
		}
	}
	if len(c.Claims) == 0 {
		c.Claims = append(c.Claims, "la situación conversacional está descrita por el turno actual y su estado conversacional")
	}
	return c
}

// OperationalPerspective reads only work/world relations. It never creates,
// mutates, or authorizes work.
func OperationalPerspective(s Situation) Contribution {
	c := Contribution{ID: s.ID + ":operational", Perspective: "operational", CreatedAt: time.Now(), Confidence: s.Confidence, Provenance: []string{"production.WorkStore", "environment.WorldState"}}
	for _, r := range s.Relations {
		if strings.HasPrefix(r.Provenance, "production.") || strings.HasPrefix(r.Provenance, "environment.") {
			c.Relations = append(c.Relations, r)
		}
	}
	if len(c.Relations) == 0 {
		c.Claims = append(c.Claims, "no hay evidencia operacional suficiente en esta situación")
	} else {
		c.Claims = append(c.Claims, "existe evidencia operacional relevante para contextualizar el turno")
	}
	return c
}
