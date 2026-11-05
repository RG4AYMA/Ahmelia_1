package core

// linguistic_snapshot.go — ensamblador del CONTEXTO AUTORIZADO que recibe la
// frontera lingüística.
//
// El Engine ya posee el estado; hasta ahora no se lo pasaba al modelo. Este
// archivo construye una proyección acotada, con IDs estables y procedencia, y
// la entrega al Pipeline. No crea verdad nueva: solo selecciona y ordena lo que
// los dueños canónicos ya registraron.
//
// Privacidad: solo se proyecta la conversación del propietario y la evidencia
// de situación ya cargada. Nada de terceros y nada que no esté ya en memoria.

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"ahmelia/linguistics"
	"ahmelia/situation"
)

const (
	// linguisticSnapshotTurns acota cuántos turnos previos ve el modelo.
	linguisticSnapshotTurns = 4
	// linguisticSnapshotEvidence acota la evidencia de situación.
	linguisticSnapshotEvidence = 6
)

// buildLinguisticSituationSnapshot ensambla el contexto autorizado del turno.
// Devuelve nil cuando no hay nada que autorizar, de modo que el intérprete
// conserva exactamente su comportamiento actual.
func (e *Engine) buildLinguisticSituationSnapshot(now time.Time, currentInput string) *linguistics.SituationSnapshot {
	if e == nil {
		return nil
	}
	if now.IsZero() {
		now = time.Now()
	}

	facts := make([]linguistics.SnapshotFact, 0, linguisticSnapshotTurns*2+linguisticSnapshotEvidence+1)

	// 0. La postura durable del propietario. Va primero para que el límite de
	// la proyección no la desplace detrás de evidencia situacional.
	if state, ok := e.persistentModeState(); ok {
		at := state.ChangedAt
		if at.IsZero() {
			at = now
		}
		facts = append(facts, linguistics.SnapshotFact{
			Kind:       "modo_persistente",
			Text:       persistentModeSummary(state),
			Source:     "persistent_mode",
			Confidence: 1,
			At:         at,
		})
	}

	// 1. Turnos recientes. Se miran hasta tres episodios porque el episodio
	// actual puede estar recién abierto y vacío; en ese caso el contexto
	// relevante sigue estando en el anterior.
	if e.conversationHistory != nil {
		turnsSeen := 0
		for _, match := range e.conversationHistory.Latest(now, 3, 0) {
			ep := match.Episode
			if ep == nil {
				continue
			}
			if len(ep.Turns) == 0 {
				if summary := strings.TrimSpace(ep.Summary); summary != "" && turnsSeen == 0 {
					facts = append(facts, linguistics.SnapshotFact{
						Kind:       "resumen_episodio",
						Text:       summary,
						Source:     "conversation_history",
						Confidence: 1,
						At:         ep.Start,
					})
				}
				continue
			}
			remaining := linguisticSnapshotTurns - turnsSeen
			start := len(ep.Turns) - remaining
			if start < 0 {
				start = 0
			}
			for _, t := range ep.Turns[start:] {
				if s := strings.TrimSpace(t.UserText); s != "" && s != strings.TrimSpace(currentInput) {
					facts = append(facts, linguistics.SnapshotFact{
						Kind:       "turno_usuario",
						Text:       s,
						Source:     "conversation_history",
						Confidence: 1,
						At:         t.At,
					})
				}
				if s := strings.TrimSpace(t.Assistant); s != "" {
					facts = append(facts, linguistics.SnapshotFact{
						Kind:       "respuesta_previa",
						Text:       s,
						Source:     "conversation_history",
						Confidence: 1,
						At:         t.At,
					})
				}
				turnsSeen++
			}
			if turnsSeen >= linguisticSnapshotTurns {
				break
			}
		}
	}

	// 2. Pregunta pendiente en la frontera de interacción.
	if e.clarificationQueue != nil {
		if q := e.clarificationQueue.Oldest(); q != nil {
			text := strings.TrimSpace(q.Question)
			if goal := strings.TrimSpace(q.OriginalGoal); goal != "" {
				text = "objetivo: " + goal + "; pregunta: " + text
			}
			if text != "" {
				facts = append(facts, linguistics.SnapshotFact{
					Kind:       "pregunta_pendiente",
					Text:       text,
					Source:     "clarification_queue",
					Confidence: 1,
					At:         q.AskedAt,
				})
			}
		}
	}

	// 3. Evidencia de la última situación derivada, la más fuerte primero.
	if len(e.lastSituation.Evidence) > 0 {
		evidence := append([]situation.Evidence(nil), e.lastSituation.Evidence...)
		sort.SliceStable(evidence, func(i, j int) bool {
			return evidence[i].Confidence > evidence[j].Confidence
		})
		for _, ev := range evidence {
			if len(facts) >= linguisticSnapshotTurns*2+linguisticSnapshotEvidence+1 {
				break
			}
			text := strings.TrimSpace(strings.Join(nonEmptyStrings(ev.Subject, ev.Predicate, ev.Object, ev.Value), " "))
			if text == "" {
				continue
			}
			facts = append(facts, linguistics.SnapshotFact{
				Kind:       "situacion",
				Text:       text,
				Source:     firstNonEmpty(ev.Source, ev.Authority),
				Confidence: ev.Confidence,
				At:         ev.ObservedAt,
			})
		}
	}

	if len(facts) == 0 {
		return nil
	}
	for i := range facts {
		facts[i].ID = fmt.Sprintf("f%d", i+1)
	}
	return &linguistics.SituationSnapshot{Facts: facts}
}

func nonEmptyStrings(values ...string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		if s := strings.TrimSpace(v); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}
