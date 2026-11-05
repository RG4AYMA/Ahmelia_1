package contextintelligence

import (
	"ahmelia/interaction"
	"strings"
	"time"
)

type Result struct {
	Version    string
	References []Reference
	Candidates []Candidate
	Continuity Continuity
	Historical HistoricalContext
	Valid      bool
	Errors     []string
}

type Intelligence struct {
	reference  *Model
	anaphora   *AnaphoraResolver
	context    *ContextModel
	continuity *ContinuityModel
	bridge     *HistoricalBridge
}

func New() *Intelligence {
	return &Intelligence{NewReferenceModel(), NewAnaphoraResolver(), NewContextModel(), NewContinuityModel(), NewHistoricalBridge()}
}
func (i *Intelligence) Resolve(req interaction.SemanticRequest, ctx interaction.ConversationSnapshot) Result {
	out := Result{Version: "B96.36.6"}
	out.References = i.reference.Build(req, ctx)
	out.Continuity = i.continuity.Evaluate(req, ctx)
	if out.Continuity.Kind == ContinuityNewTopic && out.Continuity.Reason != "" && out.Continuity.ThreadID == "" {
		// An explicit boundary invalidates discourse-deictic references from
		// the previous lane. Do not let "eso" survive as a hidden anchor.
		out.References = dropBoundaryReferences(out.References)
	}
	out.References = i.anaphora.Resolve(out.References, req, ctx)
	if out.Continuity.Kind == ContinuityNewTopic {
		out.References = dropBoundaryReferences(out.References)
	}
	out.Candidates = i.context.Select(req, out.References, ctx, now(), out.Continuity)
	out.Historical = i.bridge.PrepareWithContinuity(req, out.References, ctx, out.Continuity)
	out.Valid = true
	for _, r := range out.References {
		if !r.Resolved && r.Expression != "" {
			out.Errors = append(out.Errors, "unresolved reference: "+r.Expression)
		}
	}
	if len(out.Errors) > 0 && req.Action.Name != "" {
		out.Valid = false
	}
	return out
}
func now() (t time.Time) { return time.Now() }

// ApplyToSemanticRequest projects B96.36's derived resolution back onto the
// existing semantic contract. The interaction layer remains the owner of the
// request type; B96.36 only fills context that has already been resolved.
func ApplyToSemanticRequest(req interaction.SemanticRequest, result Result) interaction.SemanticRequest {
	out := req
	if result.Continuity.Kind == ContinuityNewTopic {
		out.BreaksContinuity = true
		out.FollowUp = false
		out.Continues = false
		out.AddressesCurrent = false
	}
	out.References = append([]interaction.Reference(nil), req.References...)
	for i := range out.References {
		for _, r := range result.References {
			if r.Expression == out.References[i].Expression && r.Resolved && r.Value != "" {
				out.References[i].Resolved = true
				out.References[i].Value = r.Value
				break
			}
		}
	}
	if (result.Continuity.Kind == ContinuityContinue || result.Continuity.Kind == ContinuityNewTopic) && out.Topic == "" {
		out.Topic = result.Continuity.Topic
	}
	if len(result.Errors) > 0 && out.Action.Name != "" {
		out.NeedsClarification = true
	}
	return out
}

func dropBoundaryReferences(refs []Reference) []Reference {
	out := make([]Reference, 0, len(refs))
	for _, r := range refs {
		expr := strings.ToLower(strings.TrimSpace(r.Expression))
		if containsAny(expr, "current referent", "eso", "that", "aquello", "previous turn") {
			continue
		}
		out = append(out, r)
	}
	return out
}
