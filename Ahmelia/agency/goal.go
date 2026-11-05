package agency

// goal.go — CONTRATO DE META PROPIA.
//
// Una meta propia es una propuesta, no una orden. El modelo articula qué
// haría, por qué y con qué evidencia; el arnés decide si la registra y el
// propietario decide si la autoriza. Ninguna meta de este contrato se ejecuta
// por sí sola.
//
// El contrato es cerrado a propósito:
//
//   - el riesgo y el modo pertenecen a vocabularios fijos;
//   - el costo esperado está acotado;
//   - toda cita debe existir en el espacio de trabajo del turno;
//   - sin evidencia válida, la meta se descarta (no se "repara").

import (
	"errors"
	"fmt"
	"strings"
)

const (
	// MaxGoalTitleRunes acota el título legible.
	MaxGoalTitleRunes = 120
	// MaxGoalTextRunes acota motivo y siguiente paso.
	MaxGoalTextRunes = 280
	// MaxExpectedMinutes acota el costo declarado de una meta propia.
	MaxExpectedMinutes = 240
)

// Risk is the model's declared risk for a self-proposed goal. The harness may
// still refuse or downgrade it; the label is not an authorization.
type Risk string

const (
	RiskUnknown Risk = ""
	RiskLow     Risk = "low"
	RiskMedium  Risk = "medium"
	RiskHigh    Risk = "high"
)

// Mode is the operating mode in which the goal would be pursued. It is a
// proposal, not a switch: the owner remains the only one who changes modes.
type Mode string

const (
	ModeUnknown   Mode = ""
	ModeFocus     Mode = "focus"
	ModeExplore   Mode = "explore"
	ModeRest      Mode = "rest"
	ModeCompanion Mode = "companion"
)

// Valid reports whether the mode belongs to the closed vocabulary.
func (m Mode) Valid() bool {
	switch m {
	case ModeFocus, ModeExplore, ModeRest, ModeCompanion:
		return true
	default:
		return false
	}
}

// NormalizeMode canonicalizes one raw mode string. Unknown values become
// ModeUnknown; callers must fail closed instead of guessing.
func NormalizeMode(raw string) Mode {
	candidate := Mode(strings.ToLower(strings.TrimSpace(raw)))
	if candidate.Valid() {
		return candidate
	}
	return ModeUnknown
}

// GoalProposal is the bounded shape of a self-proposed goal.
type GoalProposal struct {
	Title           string   `json:"title"`
	Motive          string   `json:"motive"`
	NextStep        string   `json:"next_step"`
	ExpectedMinutes int      `json:"expected_minutes"`
	Risk            Risk     `json:"risk"`
	Mode            Mode     `json:"mode"`
	EvidenceIDs     []string `json:"evidence_ids,omitempty"`
	// RejectedEvidenceIDs records citations the harness refused. Kept for
	// diagnostics; they never count as grounding.
	RejectedEvidenceIDs []string `json:"rejected_evidence_ids,omitempty"`
	Confidence          float64  `json:"confidence"`
}

// Normalize trims text, canonicalizes the closed vocabularies and clamps the
// numeric fields. It does not decide whether the proposal is acceptable; that
// is Validate's job.
func (g *GoalProposal) Normalize() {
	if g == nil {
		return
	}
	g.Title = strings.TrimSpace(g.Title)
	g.Motive = strings.TrimSpace(g.Motive)
	g.NextStep = strings.TrimSpace(g.NextStep)
	g.Risk = Risk(strings.ToLower(strings.TrimSpace(string(g.Risk))))
	g.Mode = NormalizeMode(string(g.Mode))
	if g.Confidence < 0 {
		g.Confidence = 0
	}
	if g.Confidence > 1 {
		g.Confidence = 1
	}
	if g.ExpectedMinutes < 0 {
		g.ExpectedMinutes = 0
	}
	if g.ExpectedMinutes > MaxExpectedMinutes {
		g.ExpectedMinutes = MaxExpectedMinutes
	}
}

// Validate checks the closed contract after Normalize. The error names the
// first violated field; the caller decides whether to discard the whole
// proposal or only the self-goal part.
func (g *GoalProposal) Validate() error {
	if g == nil {
		return errors.New("agency: meta vacía")
	}
	g.Normalize()
	switch {
	case g.Title == "":
		return errors.New("agency: la meta no tiene título")
	case len([]rune(g.Title)) > MaxGoalTitleRunes:
		return fmt.Errorf("agency: título demasiado largo (%d runas)", len([]rune(g.Title)))
	case g.Motive == "":
		return errors.New("agency: la meta no tiene motivo")
	case len([]rune(g.Motive)) > MaxGoalTextRunes:
		return fmt.Errorf("agency: motivo demasiado largo (%d runas)", len([]rune(g.Motive)))
	case g.NextStep == "":
		return errors.New("agency: la meta no tiene siguiente paso")
	case len([]rune(g.NextStep)) > MaxGoalTextRunes:
		return fmt.Errorf("agency: siguiente paso demasiado largo (%d runas)", len([]rune(g.NextStep)))
	case g.ExpectedMinutes <= 0:
		return errors.New("agency: el costo esperado debe ser mayor que cero")
	case g.ExpectedMinutes > MaxExpectedMinutes:
		return fmt.Errorf("agency: costo esperado fuera de rango (%d min)", g.ExpectedMinutes)
	}
	switch g.Risk {
	case RiskLow, RiskMedium, RiskHigh:
	default:
		return fmt.Errorf("agency: riesgo desconocido %q", g.Risk)
	}
	switch g.Mode {
	case ModeFocus, ModeExplore, ModeRest, ModeCompanion:
	default:
		return fmt.Errorf("agency: modo desconocido %q", g.Mode)
	}
	return nil
}

// Bind filters evidence citations against the workspace. It returns true when
// at least one citation is valid; a self-goal without grounding must not enter
// the ledger. The proposal is never repaired: invalid citations are recorded
// and removed, text stays as the model wrote it.
func (g *GoalProposal) Bind(valid func(id string) bool) bool {
	if g == nil {
		return false
	}
	g.RejectedEvidenceIDs = nil
	seen := make(map[string]bool, len(g.EvidenceIDs))
	kept := make([]string, 0, len(g.EvidenceIDs))
	for _, id := range g.EvidenceIDs {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		if valid != nil && valid(id) {
			kept = append(kept, id)
		} else {
			g.RejectedEvidenceIDs = append(g.RejectedEvidenceIDs, id)
		}
	}
	g.EvidenceIDs = kept
	return len(kept) > 0
}

// Summary is a one-line description for logs and records.
func (g GoalProposal) Summary() string {
	if g.Title == "" {
		return "(meta vacía)"
	}
	return fmt.Sprintf("%s [%s/%s, %d min]", g.Title, g.Risk, g.Mode, g.ExpectedMinutes)
}
