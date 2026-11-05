package reasoning

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// structuredFake implements ONLY the structured boundary plus the base
// interface. Generate deliberately fails: reaching it means the bypass fell
// back to the interpretation path.
type structuredFake struct {
	response string
	prompt   string
	schema   json.RawMessage
	called   bool
}

func (f *structuredFake) Generate(string) (string, error) {
	panic("bypass must not use Generate/GenerateContext")
}
func (f *structuredFake) Available() bool { return true }
func (f *structuredFake) GenerateStructuredContext(_ context.Context, p string, s json.RawMessage, _ int) (string, error) {
	f.called, f.prompt, f.schema = true, p, s
	return f.response, nil
}

type legacyOnly struct{}

func (legacyOnly) Generate(string) (string, error) { return "{}", nil }
func (legacyOnly) Available() bool                 { return true }

var readTool = []DirectLLMTool{{ID: "computeruse", Operation: "read_file", Description: "Read text from a file", Parameters: []string{"path", "lines"}}}

func TestDecideUsesOwnSchemaNotInterpretation(t *testing.T) {
	f := &structuredFake{response: `{"status":"act","plan":["read"],"action":{"capability_id":"computeruse","operation":"read_file","target":"a.csv","parameters":{"path":"a.csv","lines":"5"}},"final":"","done":false,"confidence":0.9}`}
	got, err := NewDirectLLMReasoner(f).Decide(context.Background(), "lee a.csv", "", readTool)
	if err != nil || !f.called {
		t.Fatalf("err=%v called=%t", err, f.called)
	}
	if got.Status != "act" || got.Action.Operation != "read_file" {
		t.Fatalf("bad decision %+v", got)
	}
	if !strings.Contains(string(f.schema), `"finish"`) || strings.Contains(string(f.schema), "interpretation") || !strings.Contains(string(f.schema), `"read_file"`) {
		t.Fatalf("wrong schema: %s", f.schema)
	}
}

func TestDecideRefusesClientWithoutStructuredBoundary(t *testing.T) {
	if _, err := NewDirectLLMReasoner(legacyOnly{}).Decide(context.Background(), "x", "", readTool); err == nil {
		t.Fatal("must not fall back to Generate")
	}
}

func TestPromptContainsOnlyFourSections(t *testing.T) {
	p := BuildDirectLLMPromptForTest("lee a.csv", "", readTool)
	for _, want := range []string{"USER INPUT:", "AVAILABLE CAPABILITIES:", "capability_id=computeruse", "LATEST OBSERVATION:\n<none>", "Return ONLY JSON", "done_so_far", "remaining"} {
		if !strings.Contains(p, want) {
			t.Fatalf("missing %q in prompt:\n%s", want, p)
		}
	}
	for _, banned := range []string{"Ahmelia", "memory", "UserModel", "Situation", "Cognition"} {
		if strings.Contains(p, banned) {
			t.Fatalf("prompt leaks %q", banned)
		}
	}
}

func TestValidationRejectsInventedCapabilityAndBadStatus(t *testing.T) {
	bad := DirectLLMDecision{Status: "act", Action: &DirectLLMAction{CapabilityID: "x", Operation: "y"}}
	if ValidateDirectLLMDecision(&bad, readTool) == nil {
		t.Fatal("invented capability accepted")
	}
	if ValidateDirectLLMDecision(&DirectLLMDecision{Status: "maybe"}, readTool) == nil {
		t.Fatal("bad status accepted")
	}
	fin := DirectLLMDecision{Status: "finish", Action: &DirectLLMAction{}}
	if err := ValidateDirectLLMDecision(&fin, readTool); err != nil || !fin.Done || fin.Action != nil {
		t.Fatalf("finish not normalized: %+v %v", fin, err)
	}
}

func TestSchemaFieldOrderActionStatusFinal(t *testing.T) {
	s := string(DirectLLMDecisionSchemaFor(readTool))
	var v map[string]any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatalf("schema is not valid JSON: %v", err)
	}
	idx := func(k string) int { return strings.Index(s, k) }
	d, r, a, st, f := idx(`"done_so_far":{`), idx(`"remaining":{`), idx(`"action":{"type"`), idx(`"status":{"type"`), idx(`"final":{"type"`)
	if !(d >= 0 && d < r && r < a && a < st && st < f) {
		t.Fatalf("order must be done_so_far<remaining<action<status<final, got %d %d %d %d %d", d, r, a, st, f)
	}
}

func TestInconsistentRespondWithCapabilityIsFlagged(t *testing.T) {
	d := DirectLLMDecision{Status: "respond", Action: &DirectLLMAction{CapabilityID: "computeruse", Operation: "read_file"}, Final: "hecho"}
	if err := ValidateDirectLLMDecision(&d, readTool); err != nil || !d.Inconsistent || d.Action != nil {
		t.Fatalf("not flagged: %+v %v", d, err)
	}
}
