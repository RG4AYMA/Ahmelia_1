package interaction

import "testing"

func TestNeedsLinguisticDepthEscalatesUngroundedShortQuestions(t *testing.T) {
	cases := []struct {
		name string
		text string
		want bool
	}{
		{
			name: "unrecognized ownership paraphrase",
			text: "¿Tú tienes alguna noción de mi bebida favorita?",
			want: true,
		},
		{
			name: "embedded memory paraphrase",
			text: "¿Recuerdas cuál es la bebida que más me gusta?",
			want: true,
		},
		{
			name: "recognized generic attribute query stays fast",
			text: "¿Cuál es mi ciudad?",
			want: false,
		},
		{
			name: "recognized self attribute query stays fast",
			text: "¿Tú tienes color favorito?",
			want: false,
		},
		{
			name: "question with contextual reference keeps existing escalation",
			text: "¿Dónde está Blender?",
			want: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := Analyze(tc.text, Context{Language: "es"})
			if got := NeedsLinguisticDepth(req); got != tc.want {
				t.Fatalf("NeedsLinguisticDepth(%q) = %v, want %v; req=%+v", tc.text, got, tc.want, req)
			}
		})
	}
}

func TestShouldDeferLinguisticDepthDoesNotCompeteWithExecutableCommand(t *testing.T) {
	req := Analyze("Quiero que implementes una pequeña utilidad de programación. Crea una función Go llamada IsEven(n int) bool.", Context{Language: "es"})
	if !CanonicalActs(req).Has(ActCommand) {
		t.Fatalf("expected command act, req=%+v", req)
	}
	if req.Confidence < 0.75 {
		t.Fatalf("expected confident semantic request, confidence=%.2f req=%+v", req.Confidence, req)
	}
	if !ResolveCapabilityInteraction(req).Executable {
		t.Fatalf("expected request to resolve to an executable capability, req=%+v", req)
	}
	if ShouldDeferLinguisticDepth(req) {
		t.Fatalf("executable command must not enqueue optional deferred linguistic work")
	}
}

func TestShouldDeferLinguisticDepthKeepsAmbiguousConversationalInputEligible(t *testing.T) {
	req := Analyze("¿Podrías interpretar esta situación de manera diferente?", Context{Language: "es"})
	if !NeedsLinguisticDepth(req) {
		t.Fatalf("expected linguistic depth for conversational question, req=%+v", req)
	}
	if !ShouldDeferLinguisticDepth(req) {
		t.Fatalf("non-executable conversational input should remain eligible for deferred enrichment")
	}
}
