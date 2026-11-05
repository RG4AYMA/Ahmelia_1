package expectation

// expectation.go — ACTA DE EXPECTATIVA (PASO 4.1).
//
// El arnés declara qué debería observarse antes de actuar, observa por su
// cuenta después y emite un veredicto. El modelo puede leer el veredicto y
// sugerir una expectativa, pero no puede declararla, verificarla, corregirla
// ni escribir este libro.
//
// La regla central se sostiene en tres estados terminales:
//
//   - confirmed: todos los hechos declarados fueron observados y coinciden.
//   - contradicted: al menos un hecho observado contradice lo declarado.
//   - insufficient: no se pudo observar con certeza, o no hubo acta durable.
//
// "El ejecutor no devolvió error" no es evidencia. "El modelo dijo listo"
// tampoco. Solo una observación independiente puede confirmar.

import (
	"fmt"
	"strings"
	"time"
)

// Status is the state of one expectation or verdict.
type Status string

const (
	// StatusDeclared is the declaration record. It is not a verdict.
	StatusDeclared Status = "declared"
	// StatusConfirmed means every declared fact was observed and matched.
	StatusConfirmed Status = "confirmed"
	// StatusContradicted means at least one observed fact contradicted the
	// declaration. This is knowledge, not uncertainty.
	StatusContradicted Status = "contradicted"
	// StatusInsufficient means the observation could not establish the
	// declaration, or the durable act could not be written. Never a success
	// by default.
	StatusInsufficient Status = "insufficient"
)

func (s Status) Valid() bool {
	switch s {
	case StatusDeclared, StatusConfirmed, StatusContradicted, StatusInsufficient:
		return true
	default:
		return false
	}
}

// Terminal reports whether the status can close an expectation.
func (s Status) Terminal() bool {
	switch s {
	case StatusConfirmed, StatusContradicted, StatusInsufficient:
		return true
	default:
		return false
	}
}

func (s Status) Word() string {
	switch s {
	case StatusConfirmed:
		return "confirmado"
	case StatusContradicted:
		return "contradicho"
	case StatusInsufficient:
		return "insuficiente"
	case StatusDeclared:
		return "declarado"
	default:
		return string(s)
	}
}

// Fact is one checkable postcondition or observation. Expected and Observed
// are deliberately strings: the ledger stores evidence, not live values.
type Fact struct {
	Subject   string `json:"subject"`
	Predicate string `json:"predicate"`
	Expected  string `json:"expected,omitempty"`
	Observed  string `json:"observed,omitempty"`
}

// Expectation is the harness declaration made before acting.
type Expectation struct {
	ID            string    `json:"id"`
	DeclaredAt    time.Time `json:"declared_at"`
	Action        string    `json:"action"`
	Target        string    `json:"target,omitempty"`
	WorkID        string    `json:"work_id,omitempty"`
	AttemptNumber int       `json:"attempt_number,omitempty"`
	Source        string    `json:"source"`
	Facts         []Fact    `json:"facts"`
	Note          string    `json:"note,omitempty"`
}

// Verdict is the harness conclusion after independent observation. It never
// rewrites the declaration.
type Verdict struct {
	ExpectationID string    `json:"expectation_id"`
	Status        Status    `json:"status"`
	ObservedAt    time.Time `json:"observed_at"`
	Source        string    `json:"source,omitempty"`
	Details       string    `json:"details,omitempty"`
	Facts         []Fact    `json:"facts,omitempty"`
	Summary       string    `json:"summary,omitempty"`
	// LedgerError is set when the declaration or verdict could not be
	// persisted. The verdict is then published as insufficient, never as
	// confirmed.
	LedgerError string `json:"ledger_error,omitempty"`
}

// Hypothesis is a model proposal. It is evidence about what the model thought,
// never a criterion: Checkable is always false for this record kind.
type Hypothesis struct {
	ID         string    `json:"id"`
	RecordedAt time.Time `json:"recorded_at"`
	Source     string    `json:"source"`
	Checkable  bool      `json:"checkable"`
	Action     string    `json:"action,omitempty"`
	TurnID     string    `json:"turn_id,omitempty"`
	Text       string    `json:"text"`
}

// Record is one append-only line in the ledger.
type Record struct {
	Kind        string       `json:"kind"`
	Expectation *Expectation `json:"expectation,omitempty"`
	Verdict     *Verdict     `json:"verdict,omitempty"`
	Hypothesis  *Hypothesis  `json:"hypothesis,omitempty"`
}

const (
	RecordExpectation = "expectation"
	RecordVerdict     = "verdict"
	RecordHypothesis  = "hypothesis"
)

// Compare classifies observed facts against the declared expectation.
//
// It is intentionally conservative:
//   - a contradiction wins over an unknown (a known failure is knowledge);
//   - any unknown without contradiction is insufficient;
//   - only full agreement is confirmed.
func Compare(expected, observed []Fact) (Status, string) {
	if len(expected) == 0 {
		return StatusInsufficient, "no hay hechos esperados que comparar"
	}
	byKey := make(map[string]Fact, len(observed))
	for _, fact := range observed {
		byKey[factKey(fact)] = fact
	}
	var contradictions []string
	var unknowns []string
	for _, want := range expected {
		got, ok := byKey[factKey(want)]
		if !ok {
			unknowns = append(unknowns, fmt.Sprintf("%s: no se observó", factKey(want)))
			continue
		}
		value := strings.TrimSpace(got.Observed)
		if value == "" || strings.EqualFold(value, "unknown") {
			unknowns = append(unknowns, fmt.Sprintf("%s: observación desconocida", factKey(want)))
			continue
		}
		if !strings.EqualFold(value, strings.TrimSpace(want.Expected)) {
			contradictions = append(contradictions,
				fmt.Sprintf("%s: esperaba %q, observé %q", factKey(want), want.Expected, got.Observed))
		}
	}
	if len(contradictions) > 0 {
		return StatusContradicted, strings.Join(contradictions, "; ")
	}
	if len(unknowns) > 0 {
		return StatusInsufficient, strings.Join(unknowns, "; ")
	}
	return StatusConfirmed, ""
}

func factKey(f Fact) string {
	return strings.TrimSpace(f.Subject) + "|" + strings.TrimSpace(f.Predicate)
}

// DescribeFact renders one fact as a human-readable Spanish clause. The
// wording distinguishes the expected side from the observed side; it never
// turns an observation into a promise.
func DescribeFact(f Fact) string {
	subject := strings.TrimSpace(f.Subject)
	predicate := strings.TrimSpace(f.Predicate)
	expected := strings.TrimSpace(f.Expected)
	observed := strings.TrimSpace(f.Observed)

	switch predicate {
	case "filesystem.object_type":
		expect := "que «" + subject + "» fuera un directorio"
		if expected != "" && expected != "directory" {
			expect = "que «" + subject + "» tuviera tipo «" + expected + "»"
		}
		return expect + "; observé que " + describeObjectType(observed)
	case "filesystem.exists":
		if expected == "false" {
			return "que «" + subject + "» ya no existiera; observé que " + describeExists(observed)
		}
		return "que «" + subject + "» existiera; observé que " + describeExists(observed)
	default:
		expect := "que «" + subject + "» cumpliera «" + predicate + "»"
		if expected != "" {
			expect += " = «" + expected + "»"
		}
		if observed == "" {
			return expect + "; no se observó"
		}
		return expect + "; observé «" + observed + "»"
	}
}

func describeObjectType(observed string) string {
	switch strings.TrimSpace(observed) {
	case "directory":
		return "es un directorio"
	case "absent":
		return "no existe"
	case "non_directory":
		return "existe, pero no es un directorio"
	default:
		return "no se pudo determinar su tipo"
	}
}

func describeExists(observed string) string {
	switch strings.TrimSpace(observed) {
	case "true":
		return "existe"
	case "false":
		return "no existe"
	default:
		return "no se pudo determinar"
	}
}

// DescribeVerdict builds the deterministic sentence the harness may publish.
// It is deliberately plain: expected, observed, verdict.
func DescribeVerdict(v Verdict) string {
	parts := make([]string, 0, len(v.Facts))
	seen := make(map[string]bool, len(v.Facts))
	for _, fact := range v.Facts {
		key := factKey(fact)
		if seen[key] {
			continue
		}
		seen[key] = true
		parts = append(parts, DescribeFact(fact))
	}
	text := ""
	if len(parts) > 0 {
		text = "Esperaba " + strings.Join(parts, "; ")
	} else if strings.TrimSpace(v.Details) != "" {
		text = strings.TrimSpace(v.Details)
	}
	if text == "" {
		text = "no hay hechos observables registrados"
	}
	if !strings.HasSuffix(text, ".") {
		text += "."
	}
	text += " Veredicto: " + v.Status.Word() + "."
	if strings.TrimSpace(v.LedgerError) != "" {
		text += " El acta no pudo escribirse: " + strings.TrimSpace(v.LedgerError) + "."
	}
	return text
}
