package deliberation

// workspace.go — ESPACIO DE TRABAJO COGNITIVO.
//
// El espacio de trabajo no es memoria, ni verdad, ni autoridad. Es una
// proyección acotada y de solo lectura de lo que el arnés ya posee en este
// turno: la entrada, su interpretación, la situación derivada, el trabajo
// activo y la pregunta pendiente. Cada elemento recibe un ID estable (w1…wn)
// que el modelo puede citar.
//
// Regla de oro: el modelo puede leer el espacio y opinar sobre él; no puede
// ampliarlo ni inventar elementos. Toda evidencia que cite debe existir aquí.

import (
	"fmt"
	"sort"
	"strings"

	"ahmelia/agency"
)

const (
	// maxItemRunes acota un elemento. Un turno largo no debe consumir el
	// presupuesto del modelo que comparte la ranura con el resto del sistema.
	maxItemRunes = 320
	// maxWorkspaceRunes acota el bloque completo que se envía al modelo.
	maxWorkspaceRunes = 3200
	// maxWorkspaceItems impide que el workspace crezca sin límite.
	maxWorkspaceItems = 14
)

// Item is one harness-owned fact placed in the cognitive workspace.
type Item struct {
	ID         string
	Kind       string
	Text       string
	Source     string
	Confidence float64
}

// Workspace is the bounded, read-only projection offered to deliberation.
type Workspace struct {
	TurnID string
	Items  []Item
	// Impulses are the structured harness pressures rendered into this
	// workspace. They are not extra model text: they let the core match a
	// cited impulse back to its source without parsing prose.
	Impulses []agency.Impulse
}

// Empty reports whether the model has nothing authorized to read.
func (w *Workspace) Empty() bool {
	return w == nil || len(w.Items) == 0
}

// HasID reports whether id is an item the harness actually provided.
func (w *Workspace) HasID(id string) bool {
	if w == nil {
		return false
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	for _, it := range w.Items {
		if it.ID == id {
			return true
		}
	}
	return false
}

// Add appends one item with a stable sequential ID. Items beyond the cap are
// ignored: the workspace is a bounded window, not an archive.
func (w *Workspace) Add(kind, text, source string, confidence float64) {
	if w == nil {
		return
	}
	text = strings.TrimSpace(text)
	if text == "" || len(w.Items) >= maxWorkspaceItems {
		return
	}
	if confidence < 0 {
		confidence = 0
	}
	if confidence > 1 {
		confidence = 1
	}
	w.Items = append(w.Items, Item{
		ID:         fmt.Sprintf("w%d", len(w.Items)+1),
		Kind:       strings.TrimSpace(kind),
		Text:       text,
		Source:     strings.TrimSpace(source),
		Confidence: confidence,
	})
}

// Render returns the deterministic, bounded block injected into the prompt.
// It returns "" when there is nothing authorized to read.
func (w *Workspace) Render() string {
	if w.Empty() {
		return ""
	}
	items := append([]Item(nil), w.Items...)
	sort.SliceStable(items, func(i, j int) bool { return items[i].ID < items[j].ID })

	var b strings.Builder
	budget := maxWorkspaceRunes
	for _, it := range items {
		if budget <= 0 {
			break
		}
		text := strings.Join(strings.Fields(it.Text), " ")
		if text == "" {
			continue
		}
		if r := []rune(text); len(r) > maxItemRunes {
			text = string(r[:maxItemRunes]) + "…"
		}
		header := "[" + it.ID + "] (" + it.Kind
		if it.Confidence > 0 {
			header += fmt.Sprintf(", %.2f", it.Confidence)
		}
		if it.Source != "" {
			header += ", " + it.Source
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

// IDs returns the IDs in render order. Useful for evidence validation and
// for deterministic tests.
func (w *Workspace) IDs() []string {
	if w == nil {
		return nil
	}
	out := make([]string, 0, len(w.Items))
	for _, it := range w.Items {
		out = append(out, it.ID)
	}
	sort.Strings(out)
	return out
}
