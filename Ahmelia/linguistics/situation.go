package linguistics

// situation.go — CONTEXTO AUTORIZADO para la frontera lingüística.
//
// El intérprete deja de estar ciego: recibe un snapshot acotado y tipado del
// estado que el arnés ya posee (turnos recientes, pregunta pendiente, evidencia
// de situación). El snapshot NO es memoria, NO es autoridad y NO es verdad: es
// una proyección de solo lectura, con IDs estables, que el modelo puede citar.
//
// Regla de oro: el modelo puede describir el contexto; no puede inventarlo.
// Toda resolución de referencia debe citar IDs que existan en este snapshot.
// Las que no citan, o citan IDs inexistentes, se descartan en el arnés.

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// SnapshotFact es un hecho verificado que el arnés autoriza a usar durante la
// interpretación del turno actual.
type SnapshotFact struct {
	ID         string
	Kind       string
	Text       string
	Source     string
	Confidence float64
	At         time.Time
}

// SituationSnapshot es una proyección acotada y de solo lectura del estado.
type SituationSnapshot struct {
	Facts []SnapshotFact
}

const (
	// maxFactRunes acota cada hecho para que un turno largo no consuma el
	// presupuesto del modelo.
	maxFactRunes = 280
	// maxSnapshotRunes acota el bloque completo. El modelo local comparte una
	// sola ranura; el contexto debe caber sin desplazar la tarea.
	maxSnapshotRunes = 2400
)

// Empty reports whether there is nothing the model is allowed to use.
func (s *SituationSnapshot) Empty() bool {
	return s == nil || len(s.Facts) == 0
}

// HasID reports whether id is a fact the harness actually provided.
func (s *SituationSnapshot) HasID(id string) bool {
	if s == nil {
		return false
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	for _, f := range s.Facts {
		if f.ID == id {
			return true
		}
	}
	return false
}

// Render returns the bounded, deterministic block injected into the prompt.
// It returns "" when there is nothing to authorize, so the interpreter keeps
// today's behaviour exactly.
func (s *SituationSnapshot) Render() string {
	if s.Empty() {
		return ""
	}
	facts := append([]SnapshotFact(nil), s.Facts...)
	sort.SliceStable(facts, func(i, j int) bool { return facts[i].ID < facts[j].ID })

	var b strings.Builder
	budget := maxSnapshotRunes
	for _, f := range facts {
		if budget <= 0 {
			break
		}
		text := strings.Join(strings.Fields(f.Text), " ")
		if text == "" {
			continue
		}
		if r := []rune(text); len(r) > maxFactRunes {
			text = string(r[:maxFactRunes]) + "…"
		}
		header := "[" + f.ID + "] (" + f.Kind
		if f.Confidence > 0 {
			header += fmt.Sprintf(", %.2f", f.Confidence)
		}
		if f.Source != "" {
			header += ", " + f.Source
		}
		header += ")"
		line := header + " " + text
		if r := []rune(line); len(r) > budget {
			line = string(r[:budget]) + "…"
		}
		b.WriteString(line)
		b.WriteString("\n")
		budget -= len([]rune(line)) + 1
	}
	return strings.TrimSpace(b.String())
}

// ReferenceResolution is the model's proposal for what a contextual expression
// refers to. It is evidence-bound: every resolution must cite snapshot IDs.
type ReferenceResolution struct {
	Reference   string   `json:"reference"`
	Resolved    string   `json:"resolved"`
	EvidenceIDs []string `json:"evidence_ids,omitempty"`
	Confidence  float64  `json:"confidence,omitempty"`
}

// SanitizeResolutions drops every resolution the harness cannot verify. A
// model may describe context; it may not invent it. Invalid resolutions are
// removed, never repaired: the reference stays unresolved for the rest of the
// pipeline, which is the honest outcome.
func SanitizeResolutions(in []ReferenceResolution, s *SituationSnapshot) []ReferenceResolution {
	if len(in) == 0 || s.Empty() {
		return nil
	}
	seen := make(map[string]bool, len(in))
	out := make([]ReferenceResolution, 0, len(in))
	for _, r := range in {
		ref := strings.TrimSpace(r.Reference)
		resolved := strings.TrimSpace(r.Resolved)
		if ref == "" || resolved == "" || len(r.EvidenceIDs) == 0 {
			continue
		}
		valid := true
		for _, id := range r.EvidenceIDs {
			if !s.HasID(id) {
				valid = false
				break
			}
		}
		if !valid {
			continue
		}
		key := strings.ToLower(ref)
		if seen[key] {
			continue
		}
		seen[key] = true
		if r.Confidence < 0 {
			r.Confidence = 0
		}
		if r.Confidence > 1 {
			r.Confidence = 1
		}
		r.Reference = ref
		r.Resolved = resolved
		out = append(out, r)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
