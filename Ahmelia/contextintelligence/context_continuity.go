package contextintelligence

import (
	"ahmelia/interaction"
	"strings"
)

type ContinuityKind string

const (
	ContinuityNone     ContinuityKind = "none"
	ContinuityContinue ContinuityKind = "continue"
	ContinuityResume   ContinuityKind = "resume"
	ContinuityNewTopic ContinuityKind = "new_topic"
)

type Continuity struct {
	Kind       ContinuityKind
	Topic      string
	ThreadID   string
	Confidence float64
	Reason     string
}
type ContinuityModel struct{}

func NewContinuityModel() *ContinuityModel { return &ContinuityModel{} }
func (m *ContinuityModel) Evaluate(req interaction.SemanticRequest, ctx interaction.ConversationSnapshot) Continuity {
	n := strings.ToLower(strings.TrimSpace(req.Normalized))
	if explicitContextBreak(n) {
		return Continuity{ContinuityNewTopic, explicitTopicTarget(n), "", 0.99, "explicit discourse boundary: previous context is no longer the active referent"}
	}
	if ctx.ActiveThreadID != "" && (req.Continues || req.FollowUp) {
		return Continuity{ContinuityContinue, ctx.ActiveTopic, ctx.ActiveThreadID, 0.94, "semantic follow-up on active thread"}
	}
	if containsAny(n, "volvamos a ", "retomemos ", "sigamos con ", "continuemos con ") {
		return Continuity{ContinuityResume, req.Topic, ctx.ActiveThreadID, 0.91, "explicit resume language"}
	}
	if req.Topic != "" && ctx.ActiveTopic != "" && !strings.Contains(strings.ToLower(req.Topic), strings.ToLower(ctx.ActiveTopic)) && !strings.Contains(strings.ToLower(ctx.ActiveTopic), strings.ToLower(req.Topic)) {
		return Continuity{ContinuityNewTopic, req.Topic, "", 0.82, "topic differs from active focus"}
	}
	if len(ctx.Turns) > 0 {
		return Continuity{ContinuityContinue, ctx.ActiveTopic, ctx.ActiveThreadID, 0.70, "semantic turn history available"}
	}
	return Continuity{Kind: ContinuityNone, Confidence: 0.45, Reason: "no prior conversational anchor"}
}

func explicitContextBreak(n string) bool {
	for _, marker := range []string{
		"cambiemos de tema", "cambiando de tema", "hablemos de otra cosa",
		"dejemos eso por ahora", "dejemos esto por ahora", "eso ya no importa",
		"esto ya no importa", "por otro lado", "ahora hablemos de",
		"switch topics", "let's change the subject", "on another topic",
		"speaking of something else",
	} {
		if strings.Contains(n, marker) {
			return true
		}
	}
	return false
}

func explicitTopicTarget(n string) string {
	for _, marker := range []string{"hablemos de ", "ahora hablemos de ", "cambiando al tema de ", "speaking of ", "on another topic: "} {
		if idx := strings.Index(n, marker); idx >= 0 {
			target := strings.TrimSpace(n[idx+len(marker):])
			if target != "" {
				target = strings.Trim(target, " .!?¿¡,")
				if target == "otra cosa" || target == "something else" {
					return ""
				}
				return target
			}
		}
	}
	return ""
}
