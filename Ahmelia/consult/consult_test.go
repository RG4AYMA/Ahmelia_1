package consult

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

type scriptedClient struct {
	raw   string
	err   error
	calls int
}

func (s *scriptedClient) GenerateShadowContext(ctx context.Context, prompt string, schema json.RawMessage, numPredict int) (string, error) {
	s.calls++
	return s.raw, s.err
}

func testCandidates() []Candidate {
	return []Candidate{
		{ID: "silence", Kind: "silence", Description: "No intervenir", Authorized: true, Risk: "low"},
		{ID: "ask_reason", Kind: "ask", Description: "Preguntar una vez por el motivo", Authorized: true, Risk: "low"},
		{ID: "work_now", Kind: "work", Description: "Retomar trabajo", Authorized: false, Risk: "high"},
	}
}

func testProposal(id string, conf float64) Proposal {
	return Proposal{
		Reading:     "el propietario declaró un día de descanso",
		Judgment:    Judgment{OwnerState: "resting", Interruptible: false, Risk: "low"},
		Choice:      Choice{CandidateID: id, Reason: "la restricción sigue activa", Confidence: conf},
		EvidenceIDs: []string{"e1"},
		Grounded:    true,
		Confidence:  0.8,
	}
}

func TestConsultParsesAndBinds(t *testing.T) {
	client := &scriptedClient{raw: `{
		"reading":"día de descanso",
		"judgment":{"owner_state":"resting","interruptible":false,"risk":"low","reasons":["dijo que no trabaja"]},
		"choice":{"candidate_id":"silence","reason":"no hay trabajo","confidence":0.9},
		"evidence_ids":["e1","made-up"],
		"confidence":0.8,
		"abstain":false
	}`}
	req := Request{
		Scene:      "scene1",
		Input:      "hoy no trabajamos",
		Candidates: testCandidates(),
		Evidence:   []Evidence{{ID: "e1", Text: "hoy no trabajamos"}},
	}
	p, err := Consult(context.Background(), client, req)
	if err != nil {
		t.Fatalf("consult: %v", err)
	}
	if p.Choice.CandidateID != "silence" || p.Judgment.OwnerState != "resting" {
		t.Fatalf("propuesta = %+v", p)
	}
	if !p.Grounded || len(p.EvidenceIDs) != 1 || len(p.RejectedEvidenceIDs) != 1 {
		t.Fatalf("evidencia = %v rechazada=%v grounded=%t", p.EvidenceIDs, p.RejectedEvidenceIDs, p.Grounded)
	}
	if client.calls != 1 {
		t.Fatalf("llamadas = %d, quería 1", client.calls)
	}
}

func TestConsultUnavailable(t *testing.T) {
	client := &scriptedClient{err: errors.New("timeout")}
	if _, err := Consult(context.Background(), client, Request{}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v, quería ErrUnavailable", err)
	}
}

func TestDecideAcceptsOnlyGroundedAuthorizedCandidate(t *testing.T) {
	candidates := testCandidates()

	accepted := Decide(testProposal("silence", 0.9), candidates, nil, "silence")
	if !accepted.Accepted || accepted.Source != SourceModel || accepted.Candidate.ID != "silence" {
		t.Fatalf("decisión = %+v", accepted)
	}

	cases := []struct {
		name     string
		proposal Proposal
		allow    func(Candidate) (bool, string)
		want     string
	}{
		{"unknown", testProposal("invented", 0.9), nil, "unknown_candidate:invented"},
		{"unauthorized", testProposal("work_now", 0.9), nil, "not_authorized:work_now"},
		{"ungrounded", func() Proposal { p := testProposal("silence", 0.9); p.Grounded = false; return p }(), nil, "ungrounded"},
		{"low confidence", testProposal("silence", 0.2), nil, "low_confidence"},
		{"abstain", func() Proposal { p := testProposal("silence", 0.9); p.Abstain = true; return p }(), nil, "abstain"},
		{"blocked by harness", testProposal("ask_reason", 0.9), func(Candidate) (bool, string) { return false, "question_budget_exhausted" }, "question_budget_exhausted"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := Decide(tc.proposal, candidates, tc.allow, "silence")
			if d.Accepted || d.Source != SourceFallback || d.Candidate.ID != "silence" {
				t.Fatalf("decisión = %+v", d)
			}
			if d.Rejected != tc.want {
				t.Fatalf("rechazo = %q, quería %q", d.Rejected, tc.want)
			}
		})
	}
}

func TestDecideWithoutFallbackReportsNone(t *testing.T) {
	d := Decide(testProposal("invented", 0.9), testCandidates(), nil, "silence_missing")
	if d.Accepted || d.Source != SourceNone || d.Candidate.ID != "" {
		t.Fatalf("decisión = %+v", d)
	}
}

func TestPromptStatesAuthorityAndCandidates(t *testing.T) {
	prompt := BuildPrompt(Request{
		Scene:       "scene1",
		Input:       "hoy no trabajamos",
		Restriction: "work_initiative activa hasta 00:00",
		Candidates:  testCandidates(),
		Evidence:    []Evidence{{ID: "e1", Text: "hoy no trabajamos"}},
	})
	for _, want := range []string{
		"El arnés conserva la autoridad",
		"id=work_now",
		"autorizado=false",
		"RESTRICCIÓN ACTIVA",
		"e1",
		"No propongas trabajo",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("el prompt no contiene %q:\n%s", want, prompt)
		}
	}
}
