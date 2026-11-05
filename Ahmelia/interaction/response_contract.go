package interaction

import "strings"

// ResponseContract is the canonical hand-off between behavioral decision
// layers and language rendering. It describes what response work is allowed,
// not the sentence itself. Domain providers may contribute evidence/results,
// but they cannot use this contract to become conversational authority.
type ResponseContract struct {
	Intent      ResponseIntent
	Move        ResponseMove
	Subject     UserObservationSubject
	Content     string
	Evidence    []ResponseEvidence
	Source      string
	Reason      string
	CanMention  bool
	NeedsInput  bool
	ShouldSpeak bool
	ShouldAct   bool
	Confidence  float64
	// RenderScene, when set, marks this turn as a closed Fase-2 scene: the
	// spoken surface must be produced by the deterministic renderer with
	// exactly these slots. The language model never rewrites these scenes;
	// core is the only layer that translates them (core/fase2_render.go).
	RenderScene *RenderScene
}

// RenderScene is the closed-scene descriptor carried by a ResponseContract
// (Fase 2). Scene holds the renderer's movement id; Subject uses the
// ownership vocabulary and is translated by core into the renderer's
// "user"|"ahmelia" slots. A scene with incomplete slots is refused by the
// renderer (ok=false): the turn falls back to the existing paths instead of
// inventing a surface.
type RenderScene struct {
	Scene     string
	Subject   UserObservationSubject
	Attribute string
	Value     string
}

type ResponseEvidence struct {
	Text       string
	Source     string
	Relevant   bool
	Confidence float64
}

// NewResponseContract creates the single response boundary for a turn.
func NewResponseContract(req SemanticRequest, intent ResponseIntent, move ResponseMove) ResponseContract {
	acts := CanonicalActs(req)
	return ResponseContract{
		Intent:      intent,
		Move:        move,
		Subject:     req.Subject,
		NeedsInput:  intent.NeedsUserInput,
		ShouldSpeak: intent.ShouldSpeak,
		ShouldAct:   intent.ShouldAct || acts.Has(ActCommand),
		Confidence:  intent.Confidence,
	}
}

func (c ResponseContract) Valid() bool {
	return c.Intent.Kind != ResponseIntentNone && ResponseMoveIsValid(c.Move) && c.ShouldSpeak
}

// AddEvidence applies the relevance boundary before evidence reaches the
// language layer. Unrelated memory/user-model facts must never leak into a
// response merely because they are available.
func (c *ResponseContract) AddEvidence(text, source string, relevant bool, confidence float64) {
	if c == nil || !relevant || strings.TrimSpace(text) == "" {
		return
	}
	c.Evidence = append(c.Evidence, ResponseEvidence{
		Text: strings.TrimSpace(text), Source: strings.TrimSpace(source),
		Relevant: true, Confidence: confidence,
	})
}
