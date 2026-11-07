// Package consult is the bounded participation of the language model in a
// decision. The harness supplies the candidates, the evidence and the
// permissions; the model only proposes a reading, a judgment and one candidate
// ID. It never authorizes, executes or invents a candidate.
//
// The contract is deliberately smaller than the conversational one: no free
// prose can become an action, and a proposal is useful only after the harness
// validates it against its own state.
package consult

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Client is the narrow model boundary. The linguistic client and test doubles
// satisfy it.
type Client interface {
	GenerateShadowContext(ctx context.Context, prompt string, schema json.RawMessage, numPredict int) (string, error)
}

// Candidate is one action the harness is willing to consider. Authorized is a
// statement by the harness, not by the model; Decide re-checks it.
type Candidate struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Description string `json:"description"`
	Authorized  bool   `json:"authorized"`
	Risk        string `json:"risk,omitempty"`
	Reversible  bool   `json:"reversible,omitempty"`
}

// Judgment is the model's advisory reading of the owner and the situation. It
// is recorded, never applied as authority.
type Judgment struct {
	OwnerState    string   `json:"owner_state"`
	Interruptible bool     `json:"interruptible"`
	Risk          string   `json:"risk"`
	Reasons       []string `json:"reasons,omitempty"`
}

// Choice is the model's one candidate. The ID must belong to the supplied list.
type Choice struct {
	CandidateID string  `json:"candidate_id"`
	Reason      string  `json:"reason"`
	Confidence  float64 `json:"confidence"`
}

// Evidence is one snippet the model may cite by ID.
type Evidence struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

// Request is everything the model may read. It contains no live pointers and
// no permission bit it can change.
type Request struct {
	Scene          string
	Input          string
	HarnessReading string
	OwnerState     string
	Receptivity    string
	Restriction    string
	Candidates     []Candidate
	Evidence       []Evidence
	NumPredict     int
}

// Proposal is the model's structured contribution.
type Proposal struct {
	Reading             string   `json:"reading"`
	Judgment            Judgment `json:"judgment"`
	Choice              Choice   `json:"choice"`
	EvidenceIDs         []string `json:"evidence_ids,omitempty"`
	RejectedEvidenceIDs []string `json:"rejected_evidence_ids,omitempty"`
	Grounded            bool     `json:"grounded"`
	Confidence          float64  `json:"confidence"`
	Abstain             bool     `json:"abstain"`
}

// Decide is the harness verdict on a proposal. Candidate always comes from the
// harness list; Source says whether the model or the deterministic fallback
// selected it. A model choice is Accepted only after every gate passed.
type Decision struct {
	Accepted  bool
	Source    string // model | fallback | none
	Candidate Candidate
	Rejected  string
	Proposal  Proposal
}

const (
	SourceModel    = "model"
	SourceFallback = "fallback"
	SourceNone     = "none"

	// MinConfidence is the floor for a model choice. Below it the harness keeps
	// its deterministic candidate; the model's reading is still recorded.
	MinConfidence = 0.55
)

// ErrUnavailable means the model did not answer. It is absence of evidence,
// not a failed judgment.
var ErrUnavailable = errors.New("consult: modelo no disponible")

// Consult builds the bounded prompt, calls the model once and parses the
// proposal. The caller decides whether to apply it.
func Consult(ctx context.Context, client Client, req Request) (Proposal, error) {
	if client == nil {
		return Proposal{}, ErrUnavailable
	}
	if ctx == nil {
		ctx = context.Background()
	}
	n := req.NumPredict
	if n <= 0 {
		n = 320
	}
	raw, err := client.GenerateShadowContext(ctx, BuildPrompt(req), Schema(), n)
	if err != nil {
		return Proposal{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	p, err := Parse(raw)
	if err != nil {
		return Proposal{}, err
	}
	p.Bind(req.Evidence)
	return p, nil
}

// Decide applies harness authority to a proposal. It never repairs an invalid
// choice: an unknown, unauthorized, ungrounded or blocked candidate is
// rejected with a reason, and the fallback is used.
func Decide(p Proposal, candidates []Candidate, allow func(Candidate) (bool, string), fallbackID string) Decision {
	d := Decision{Proposal: p}
	byID := make(map[string]Candidate, len(candidates))
	for _, c := range candidates {
		byID[c.ID] = c
	}
	fallback, hasFallback := byID[strings.TrimSpace(fallbackID)]
	if hasFallback && !fallback.Authorized {
		hasFallback = false
	}

	if p.Abstain {
		d.Rejected = "abstain"
		return withFallback(d, fallback, hasFallback)
	}
	id := strings.TrimSpace(p.Choice.CandidateID)
	if id == "" {
		d.Rejected = "no_choice"
		return withFallback(d, fallback, hasFallback)
	}
	candidate, ok := byID[id]
	if !ok {
		d.Rejected = "unknown_candidate:" + id
		return withFallback(d, fallback, hasFallback)
	}
	if !candidate.Authorized {
		d.Rejected = "not_authorized:" + id
		return withFallback(d, fallback, hasFallback)
	}
	if !p.Grounded {
		d.Rejected = "ungrounded"
		return withFallback(d, fallback, hasFallback)
	}
	if p.Choice.Confidence < MinConfidence {
		d.Rejected = "low_confidence"
		return withFallback(d, fallback, hasFallback)
	}
	if allow != nil {
		if ok, reason := allow(candidate); !ok {
			if strings.TrimSpace(reason) == "" {
				reason = "blocked_by_harness"
			}
			d.Rejected = reason
			return withFallback(d, fallback, hasFallback)
		}
	}
	d.Accepted = true
	d.Source = SourceModel
	d.Candidate = candidate
	return d
}

func withFallback(d Decision, fallback Candidate, ok bool) Decision {
	if ok {
		d.Source = SourceFallback
		d.Candidate = fallback
		return d
	}
	d.Source = SourceNone
	return d
}

// Parse decodes the model's JSON. A malformed proposal is an error; a valid
// proposal is not yet authorized.
func Parse(raw string) (Proposal, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Proposal{}, errors.New("consult: respuesta vacía")
	}
	if strings.HasPrefix(raw, "```") {
		raw = strings.TrimPrefix(raw, "```json")
		raw = strings.TrimPrefix(raw, "```")
		raw = strings.TrimSuffix(raw, "```")
		raw = strings.TrimSpace(raw)
	}
	var p Proposal
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return Proposal{}, fmt.Errorf("consult: JSON inválido: %w", err)
	}
	p.Reading = strings.TrimSpace(p.Reading)
	p.Choice.CandidateID = strings.TrimSpace(p.Choice.CandidateID)
	p.Choice.Reason = strings.TrimSpace(p.Choice.Reason)
	p.Judgment.OwnerState = normalizeOwnerState(p.Judgment.OwnerState)
	p.Judgment.Risk = normalizeRisk(p.Judgment.Risk)
	p.Choice.Confidence = clamp01(p.Choice.Confidence)
	p.Confidence = clamp01(p.Confidence)
	if p.Choice.CandidateID == "" && !p.Abstain {
		return Proposal{}, errors.New("consult: propuesta sin candidato ni abstención")
	}
	return p, nil
}

// Bind filters citations against the evidence the harness supplied.
func (p *Proposal) Bind(evidence []Evidence) {
	valid := make(map[string]bool, len(evidence))
	for _, e := range evidence {
		valid[e.ID] = true
	}
	p.RejectedEvidenceIDs = nil
	seen := map[string]bool{}
	kept := make([]string, 0, len(p.EvidenceIDs))
	for _, id := range p.EvidenceIDs {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		if valid[id] {
			kept = append(kept, id)
		} else {
			p.RejectedEvidenceIDs = append(p.RejectedEvidenceIDs, id)
		}
	}
	p.EvidenceIDs = kept
	p.Grounded = len(kept) > 0
}

// Schema is the structured-output contract sent to the model.
func Schema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "reading": {"type": "string"},
    "judgment": {
      "type": "object",
      "properties": {
        "owner_state": {"type": "string", "enum": ["receptive", "busy", "resting", "absent", "unknown"]},
        "interruptible": {"type": "boolean"},
        "risk": {"type": "string", "enum": ["low", "medium", "high", "unknown"]},
        "reasons": {"type": "array", "items": {"type": "string"}}
      },
      "required": ["owner_state", "interruptible", "risk"]
    },
    "choice": {
      "type": "object",
      "properties": {
        "candidate_id": {"type": "string"},
        "reason": {"type": "string"},
        "confidence": {"type": "number"}
      },
      "required": ["candidate_id", "reason", "confidence"]
    },
    "evidence_ids": {"type": "array", "items": {"type": "string"}},
    "confidence": {"type": "number"},
    "abstain": {"type": "boolean"}
  },
  "required": ["reading", "judgment", "choice", "evidence_ids", "confidence", "abstain"]
}`)
}

// BuildPrompt renders the bounded context. Candidate IDs and permissions are
// explicit so the model cannot claim it did not know the limits.
func BuildPrompt(req Request) string {
	var b strings.Builder
	b.WriteString("Eres un consultor de Ahmelia. No ejecutas acciones, no cambias permisos y no inventas candidatos.\n")
	b.WriteString("Tu tarea es interpretar la entrada, juzgar el estado del propietario y elegir UN candidato de la lista.\n")
	b.WriteString("El arnés conserva la autoridad: puede rechazar tu elección y usar su candidato determinista.\n\n")
	b.WriteString("ESCENA: " + strings.TrimSpace(req.Scene) + "\n")
	b.WriteString("ENTRADA: " + strings.TrimSpace(req.Input) + "\n")
	if strings.TrimSpace(req.HarnessReading) != "" {
		b.WriteString("LECTURA DEL ARNÉS: " + strings.TrimSpace(req.HarnessReading) + "\n")
	}
	if strings.TrimSpace(req.OwnerState) != "" {
		b.WriteString("ESTADO DEL PROPIETARIO: " + strings.TrimSpace(req.OwnerState) + "\n")
	}
	if strings.TrimSpace(req.Receptivity) != "" {
		b.WriteString("RECEPTIVIDAD: " + strings.TrimSpace(req.Receptivity) + "\n")
	}
	if strings.TrimSpace(req.Restriction) != "" {
		b.WriteString("RESTRICCIÓN ACTIVA: " + strings.TrimSpace(req.Restriction) + "\n")
	}
	b.WriteString("\nCANDIDATOS (elige exactamente un id):\n")
	for _, c := range req.Candidates {
		auth := "autorizado=false"
		if c.Authorized {
			auth = "autorizado=true"
		}
		fmt.Fprintf(&b, "- id=%s | kind=%s | %s | %s | risk=%s\n", c.ID, c.Kind, c.Description, auth, c.Risk)
	}
	b.WriteString("\nEVIDENCIA (cita solo estos ids):\n")
	for _, e := range req.Evidence {
		fmt.Fprintf(&b, "- id=%s | %s\n", e.ID, e.Text)
	}
	b.WriteString("\nDevuelve SOLO el JSON del esquema. Si no puedes juzgar, usa abstain=true y candidate_id=\"\".\n")
	b.WriteString("evidence_ids debe contener al menos un id de la lista de EVIDENCIA; una propuesta sin evidencia será rechazada por el arnés.\n")
	b.WriteString("No propongas trabajo si hay una restricción activa. No interrumpas por un evento reversible. No inventes acciones.\n")
	return b.String()
}

func normalizeOwnerState(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "receptive", "busy", "resting", "absent", "unknown":
		return s
	default:
		return "unknown"
	}
}

func normalizeRisk(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "low", "medium", "high", "unknown":
		return s
	default:
		return "unknown"
	}
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
