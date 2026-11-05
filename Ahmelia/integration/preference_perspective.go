package integration

import (
	"time"

	"ahmelia/situation"
)

// PreferencePerspective aporta la lectura del POSTERIOR de preferencias
// formado (P4, directiva DeepSeek 19/10 — Paso 2 al patrón de
// CompetencePerspective) al modelo compartido de situación. Es una LECTURA
// independiente: no vota, no muta la situación ni sus dueños, y JAMÁS
// ejecuta, autoriza o modifica trabajo. La autoridad sobre lo recordado
// sigue siendo B96.34; el posterior derivado (personality.PreferenceModel)
// solo aporta stance y fiabilidad.
//
// Recibe stances YA DERIVADOS y FORMADOS (el motor descarta "unformed"
// antes de llamar); aquí no se re-interpreta nada. Cada stance entra como
// Relation tipada ("reflects_user_preference", Epistemic Derived) para que
// situation.Evaluate la convierta en DecisionFinding legible por las
// propuestas (turn_pipeline.buildInteractiveProposal). Sin stances forma-
// dos no hay contribución: la ausencia no se disimula ni se inventa.
type PreferenceStancePort struct {
	Key    string  // objeto normalizado de la preferencia (la key del posterior)
	Stance string  // likes | dislikes | mixed (nunca unformed)
	Mean   float64 // media posterior [0,1]
}

func PreferencePerspective(s situation.Situation, formed []PreferenceStancePort) situation.Contribution {
	c := situation.Contribution{
		ID:          s.ID + ":preference_stance",
		Perspective: "preference_stance",
		CreatedAt:   time.Now(),
		Provenance:  []string{"personality.PreferenceModel", "P4", "B96.34"},
	}
	if len(formed) == 0 {
		return c
	}
	best := 0.0
	for _, v := range formed {
		conf := v.Mean
		if conf < 0 {
			conf = 0
		} else if conf > 1 {
			conf = 1
		}
		if conf > best {
			best = conf
		}
		c.Relations = append(c.Relations, situation.Relation{
			Type:       "reflects_user_preference",
			Source:     "user",
			Target:     v.Key,
			Provenance: "personality.PreferenceModel",
			Relevance:  .9,
			Confidence: conf,
			// Epistemic DERIVED: es una lectura posterior, no una observación
			// canónica de este turno (directiva Paso 2 — semántica honesta).
			Epistemic: situation.EpistemicDerived,
		})
	}
	// Confianza del PACKAGE de la lectura: la media máxima entre stances
	// formados (Evaluate la intersecta con la de cada relación min01 — sin
	// esto las Findings heredarían 0 por defecto).
	c.Confidence = best
	return c
}
