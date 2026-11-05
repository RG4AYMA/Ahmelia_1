package interaction

import (
	"strings"
	"time"
)

// DiagnosticKind identifies a subsystem's metacognitive observation.
// Diagnostics are semantic evidence for Cognition; they are never finished
// response text and never authorize execution.
type DiagnosticKind string

const (
	DiagnosticContext             DiagnosticKind = "context"
	DiagnosticPipeline            DiagnosticKind = "pipeline"
	DiagnosticAttention           DiagnosticKind = "attention"
	DiagnosticWorkingMemory       DiagnosticKind = "working_memory"
	DiagnosticTrust               DiagnosticKind = "trust"
	DiagnosticInternalState       DiagnosticKind = "internal_state"
	DiagnosticMode                DiagnosticKind = "mode"
	DiagnosticRelationship        DiagnosticKind = "relationship"
	DiagnosticConversationSummary DiagnosticKind = "conversation_summary"
	DiagnosticHumor               DiagnosticKind = "humor"
)

// DiagnosticFact is a typed semantic attribute. Value may contain domain data,
// but it must not be treated as a sentence to emit directly.
type DiagnosticFact struct {
	Key        string
	Value      string
	Confidence float64
}

// DiagnosticResult is the semantic boundary between a subsystem and
// Conversational Cognition. Subsystems may describe their current state,
// policy, history, or observation here; they do not generate the response.
type DiagnosticResult struct {
	Kind       DiagnosticKind
	Source     string
	Subject    string
	Facts      []DiagnosticFact
	Confidence float64
	CreatedAt  time.Time
}

func NewDiagnosticResult(kind DiagnosticKind, source, subject string, confidence float64, now time.Time) DiagnosticResult {
	return DiagnosticResult{
		Kind: kind, Source: strings.TrimSpace(source), Subject: strings.TrimSpace(subject),
		Confidence: confidence, CreatedAt: now,
	}
}

func (d *DiagnosticResult) AddFact(key, value string, confidence float64) {
	if d == nil || strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
		return
	}
	d.Facts = append(d.Facts, DiagnosticFact{
		Key: strings.TrimSpace(key), Value: strings.TrimSpace(value), Confidence: confidence,
	})
}

func (d DiagnosticResult) Valid() bool {
	return d.Kind != "" && strings.TrimSpace(d.Source) != "" && !d.CreatedAt.IsZero() && len(d.Facts) > 0
}
