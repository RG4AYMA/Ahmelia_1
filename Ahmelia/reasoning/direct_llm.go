package reasoning

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"ahmelia/linguistics"
)

// DirectLLMStructuredClient is the ONLY generation boundary the bypass may
// use. It takes a caller-owned schema. The bypass must never fall back to
// Generate/GenerateContext: on OllamaClient both force
// interpretationJSONSchema(), which re-introduces the linguistic
// interpretation contract this experiment exists to exclude.
type DirectLLMStructuredClient interface {
	GenerateStructuredContext(ctx context.Context, prompt string, schema json.RawMessage, numPredict int) (string, error)
}

const directLLMNumPredict = 200

// DirectLLMAction is the deliberately thin action proposal used only by the
// direct-LLM bypass experiment. It is a proposal, not authorization.
type DirectLLMAction struct {
	CapabilityID string            `json:"capability_id"`
	Operation    string            `json:"operation"`
	Target       string            `json:"target"`
	Parameters   map[string]string `json:"parameters"`
}

// DirectLLMDecision is the model's bounded workflow decision. "plan" contains
// short externally observable subtasks, not hidden chain-of-thought.
type DirectLLMDecision struct {
	// DoneSoFar / Remaining are an observable task ledger the model must fill
	// BEFORE choosing an action. They are data, not hidden reasoning.
	DoneSoFar  string           `json:"done_so_far"`
	Remaining  string           `json:"remaining"`
	Status     string           `json:"status"`
	Plan       []string         `json:"plan"`
	Action     *DirectLLMAction `json:"action"`
	Final      string           `json:"final"`
	Done       bool             `json:"done"`
	Confidence float64          `json:"confidence"`
	// Inconsistent is set when the model chose a capability but did not
	// return status="act". Diagnostic only; never used to execute.
	Inconsistent bool `json:"-"`
}

// DirectLLMTool describes a capability made available to the experimental
// model. Go remains the owner of the physical execution boundary.
type DirectLLMTool struct {
	ID          string
	Operation   string
	Description string
	Parameters  []string
}

// DirectLLMReasoner is intentionally independent of interaction, Situation,
// UserModel, memory and Dialogue. It exists to answer one experimental
// question: what happens if the model receives the raw user request and a
// minimal tool vocabulary and is allowed to choose the next step directly?
type DirectLLMReasoner struct {
	client linguistics.LanguageClient
	// LastPrompt / LastRaw are kept for instrumentation only.
	LastPrompt string
	LastRaw    string
}

func NewDirectLLMReasoner(client linguistics.LanguageClient) *DirectLLMReasoner {
	return &DirectLLMReasoner{client: client}
}

func (r *DirectLLMReasoner) Decide(ctx context.Context, input string, observation string, tools []DirectLLMTool) (DirectLLMDecision, error) {
	if r == nil || r.client == nil {
		return DirectLLMDecision{}, fmt.Errorf("direct llm reasoner: client unavailable")
	}
	sc, ok := r.client.(DirectLLMStructuredClient)
	if !ok {
		return DirectLLMDecision{}, fmt.Errorf("direct llm reasoner: client has no structured boundary; refusing to fall back to the interpretation schema")
	}
	prompt := buildDirectLLMPrompt(input, observation, tools)
	r.LastPrompt = prompt
	raw, err := sc.GenerateStructuredContext(ctx, prompt, DirectLLMDecisionSchemaFor(tools), directLLMNumPredict)
	r.LastRaw = raw
	if err != nil {
		return DirectLLMDecision{}, err
	}
	var decision DirectLLMDecision
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &decision); err != nil {
		return DirectLLMDecision{}, fmt.Errorf("direct llm reasoner: invalid JSON: %w", err)
	}
	if decision.Confidence < 0 {
		decision.Confidence = 0
	}
	if decision.Confidence > 1 {
		decision.Confidence = 1
	}
	if err := ValidateDirectLLMDecision(&decision, tools); err != nil {
		return decision, err
	}
	return decision, nil
}

// DirectLLMDecisionSchemaFor is the bypass-owned output contract. It is not
// the linguistic Interpretation schema. capability_id and operation are
// constrained to the supplied vocabulary.
//
// PROPERTY ORDER MATTERS: constrained decoding emits fields in schema order.
// The schema is written as an ordered string on purpose (json.Marshal of a Go
// map would sort keys alphabetically: action, confidence, final, status,
// which made the model write its "final" claim BEFORE deciding status).
// Order here: done_so_far -> remaining -> action -> status -> final -> confidence.
// The ledger comes first so the action is chosen AFTER the model has stated
// what is already done and what is left (in the previous contract the action
// was committed before any assessment, and the model wrote the correct next
// step only afterwards, in "final").
func DirectLLMDecisionSchemaFor(tools []DirectLLMTool) json.RawMessage {
	ids := []string{""}
	ops := []string{""}
	seenID, seenOp := map[string]bool{}, map[string]bool{}
	for _, t := range tools {
		if !seenID[t.ID] {
			seenID[t.ID] = true
			ids = append(ids, t.ID)
		}
		if !seenOp[t.Operation] {
			seenOp[t.Operation] = true
			ops = append(ops, t.Operation)
		}
	}
	idsJSON, _ := json.Marshal(ids)
	opsJSON, _ := json.Marshal(ops)
	schema := `{"type":"object","properties":{` +
		`"done_so_far":{"type":"string"},` +
		`"remaining":{"type":"string"},` +
		`"action":{"type":"object","properties":{` +
		`"capability_id":{"type":"string","enum":` + string(idsJSON) + `},` +
		`"operation":{"type":"string","enum":` + string(opsJSON) + `},` +
		`"target":{"type":"string"}},` +
		`"required":["capability_id","operation","target"]},` +
		`"status":{"type":"string","enum":["respond","act","finish"]},` +
		`"final":{"type":"string"},` +
		`"confidence":{"type":"number"}},` +
		`"required":["done_so_far","remaining","action","status","final","confidence"]}`
	return json.RawMessage(schema)
}

// ValidateDirectLLMDecision is structural only: enum, completeness and
// membership in the supplied vocabulary. Authorization stays with Go's
// ExecutionGuard. It normalizes Action to nil when status != "act".
func ValidateDirectLLMDecision(d *DirectLLMDecision, tools []DirectLLMTool) error {
	switch d.Status {
	case "respond", "finish":
		if d.Action != nil && strings.TrimSpace(d.Action.CapabilityID) != "" {
			d.Inconsistent = true
		}
		d.Action = nil
		if d.Status == "finish" {
			d.Done = true
		}
		return nil
	case "act":
		if d.Action == nil || strings.TrimSpace(d.Action.CapabilityID) == "" || strings.TrimSpace(d.Action.Operation) == "" {
			return fmt.Errorf("direct llm reasoner: status=act with incomplete action")
		}
		d.Done = false
		for _, t := range tools {
			if t.ID == d.Action.CapabilityID && t.Operation == d.Action.Operation {
				if d.Action.Parameters == nil {
					d.Action.Parameters = map[string]string{}
				}
				return nil
			}
		}
		return fmt.Errorf("direct llm reasoner: action %s/%s is not in the supplied capabilities", d.Action.CapabilityID, d.Action.Operation)
	default:
		return fmt.Errorf("direct llm reasoner: invalid status %q", d.Status)
	}
}

func buildDirectLLMPrompt(input, observation string, tools []DirectLLMTool) string {
	var b strings.Builder
	b.WriteString(`You are the direct decision model.

Choose the next useful step.

If the request requires an available capability:
status = "act"
and provide exactly one action.

If the request can be answered directly:
status = "respond"

If the task is complete:
status = "finish"

You may only use capabilities explicitly supplied.
Never claim execution succeeded unless the observation says so.
Fill the fields in this order:
1. "done_so_far": what LATEST OBSERVATION shows is already completed ("nothing" if none). A failed action is not completed.
2. "remaining": what the user asked that is still not done ("nothing" if all done). Max 12 words.
3. "action": the capability for the FIRST remaining item; leave capability_id, operation and target empty if remaining is "nothing" or no capability is needed.
4. "status": "act" if you chose an action; "finish" if remaining is "nothing"; otherwise "respond".
If an action failed and repeating it cannot help, use status = "respond" and explain the failure.
Put the answer for the user in "final", at most two short sentences.
If status is "act", leave "final" empty: you have not executed anything yet.

Return ONLY JSON.

USER INPUT:
`)
	b.WriteString(input)
	b.WriteString("\n\nAVAILABLE CAPABILITIES:\n")
	if len(tools) == 0 {
		b.WriteString("<none>\n")
	}
	for _, t := range tools {
		b.WriteString("\ncapability_id=" + t.ID + "\noperation=" + t.Operation + "\ndescription=" + t.Description + "\nparameters=" + strings.Join(t.Parameters, ",") + "\n")
	}
	b.WriteString("\nLATEST OBSERVATION:\n")
	if strings.TrimSpace(observation) == "" {
		b.WriteString("<none>")
	} else {
		b.WriteString(observation)
	}
	b.WriteString("\n")
	return b.String()
}

// BuildDirectLLMPromptForTest exposes the exact prompt for contract tests.
func BuildDirectLLMPromptForTest(input, observation string, tools []DirectLLMTool) string {
	return buildDirectLLMPrompt(input, observation, tools)
}
