package solving

import (
	"ahmelia/interaction"
	"strings"
	"testing"
)

// TestAdaptiveConfidence_AltaResponde confirma la banda superior:
// Confidence >= 0.85 → TierRespond, sin alternativas.
func TestAdaptiveConfidence_AltaResponde(t *testing.T) {
	d := &Decision{Confidence: 0.9}
	ac := NewAdaptiveConfidence()

	action := ac.Decide(d, nil)

	if action.Tier != TierRespond {
		t.Fatalf("esperaba TierRespond con Confidence=0.9, obtuve %q", action.Tier)
	}
	if len(action.Alternatives) != 0 {
		t.Errorf("TierRespond no debería traer alternativas, obtuve %+v", action.Alternatives)
	}
}

// TestAdaptiveConfidence_BajaPideInformacion confirma la banda
// inferior: Confidence < 0.4 → TierSeekInfo.
func TestAdaptiveConfidence_BajaPideInformacion(t *testing.T) {
	d := &Decision{Confidence: 0.2}
	ac := NewAdaptiveConfidence()

	action := ac.Decide(d, nil)

	if action.Tier != TierSeekInfo {
		t.Fatalf("esperaba TierSeekInfo con Confidence=0.2, obtuve %q", action.Tier)
	}
}

// TestAdaptiveConfidence_MediaExploraConAlternativa confirma el caso
// central del bloque: Confidence en la banda media, con una Selection
// que tiene más de un candidato evaluado, trae la alternativa real
// (Bloque 39) en vez de solo decir "explorá otra cosa" sin nada
// concreto.
func TestAdaptiveConfidence_MediaExploraConAlternativa(t *testing.T) {
	d := &Decision{Confidence: 0.6}
	sel := &Selection{
		TaskID:   "T1",
		TaskDesc: "abrir el programa de diseño",
		Chosen:   Candidate{Action: "abrir_blender", Label: "Abrir Blender", Similarity: 0.7},
		Scores: []CandidateScore{
			{Candidate: Candidate{Action: "abrir_blender", Label: "Abrir Blender", Similarity: 0.7}, Combined: 0.7},
			{Candidate: Candidate{Action: "abrir_unity", Label: "Abrir Unity", Similarity: 0.6}, Combined: 0.6},
		},
	}
	ac := NewAdaptiveConfidence()

	action := ac.Decide(d, []*Selection{sel})

	if action.Tier != TierExplore {
		t.Fatalf("esperaba TierExplore con Confidence=0.6, obtuve %q", action.Tier)
	}
	if len(action.Alternatives) != 1 {
		t.Fatalf("esperaba 1 alternativa real, obtuve %+v", action.Alternatives)
	}
	if action.Alternatives[0].Candidate.Action != "abrir_unity" {
		t.Errorf("esperaba la alternativa 'abrir_unity' (el candidato no elegido), obtuve %q", action.Alternatives[0].Candidate.Action)
	}
}

// TestAdaptiveConfidence_MediaSinAlternativasReales confirma que, en
// la banda media, si ninguna Selection tiene un segundo candidato real,
// Alternatives queda vacío — no se inventa una alternativa donde no la
// hay.
func TestAdaptiveConfidence_MediaSinAlternativasReales(t *testing.T) {
	d := &Decision{Confidence: 0.5}
	sel := &Selection{
		TaskID: "T1",
		Chosen: Candidate{Action: "abrir_blender", Label: "Abrir Blender", Similarity: 0.7},
		Scores: []CandidateScore{
			{Candidate: Candidate{Action: "abrir_blender", Label: "Abrir Blender", Similarity: 0.7}, Combined: 0.7},
		},
	}
	ac := NewAdaptiveConfidence()

	action := ac.Decide(d, []*Selection{sel})

	if action.Tier != TierExplore {
		t.Fatalf("esperaba TierExplore con Confidence=0.5, obtuve %q", action.Tier)
	}
	if len(action.Alternatives) != 0 {
		t.Errorf("sin un segundo candidato real, Alternatives debería quedar vacío, obtuve %+v", action.Alternatives)
	}
}

// TestAdaptiveConfidence_SinDecisionPideInformacion confirma el
// criterio defensivo: Decide(nil, ...) no rompe y cae en la banda más
// cautelosa, no en la más optimista.
func TestAdaptiveConfidence_SinDecisionPideInformacion(t *testing.T) {
	ac := NewAdaptiveConfidence()

	action := ac.Decide(nil, nil)

	if action.Tier != TierSeekInfo {
		t.Fatalf("sin Decision, esperaba TierSeekInfo (el más cauteloso), obtuve %q", action.Tier)
	}
}

// TestAdaptiveConfidence_UmbralesSonPartitionCompleta confirma que los
// dos umbrales (RespondThreshold, ExploreThreshold) no dejan huecos:
// justo en el borde de cada banda, el resultado es el esperado.
func TestAdaptiveConfidence_UmbralesSonPartitionCompleta(t *testing.T) {
	ac := NewAdaptiveConfidence()

	cases := []struct {
		confidence float64
		want       ConfidenceTier
	}{
		{RespondThreshold, TierRespond},
		{RespondThreshold - 0.01, TierExplore},
		{ExploreThreshold, TierExplore},
		{ExploreThreshold - 0.01, TierSeekInfo},
	}

	for _, c := range cases {
		got := ac.Decide(&Decision{Confidence: c.confidence}, nil).Tier
		if got != c.want {
			t.Errorf("con Confidence=%.2f esperaba %q, obtuve %q", c.confidence, c.want, got)
		}
	}
}

// TestAdaptiveAction_Describe_ExploreIncluyeLaAlternativa confirma que
// Describe() menciona la alternativa concreta, no solo el mensaje
// genérico de "explorá otra cosa".
func TestAdaptiveAction_Describe_ExploreIncluyeLaAlternativa(t *testing.T) {
	action := &AdaptiveAction{
		Tier:       TierExplore,
		Confidence: 0.6,
		Reason:     "Confianza 60% — respondo, pero vale la pena tener presente una alternativa.",
		Alternatives: []GapAlternative{
			{TaskID: "T1", Candidate: Candidate{Action: "abrir_unity", Label: "Abrir Unity"}},
		},
	}

	desc := action.Describe()

	if !strings.Contains(desc, "Abrir Unity") {
		t.Errorf("esperaba que Describe() mencionara la alternativa concreta, obtuve: %q", desc)
	}
}

// TestParseAdaptiveConfidenceQuery_DistintoDeDecisionEngine confirma
// que el trigger no se pisa con Decision Engine (Bloque 27), la
// pregunta conceptualmente más parecida.
func TestParseAdaptiveConfidenceQuery_DistintoDeDecisionEngine(t *testing.T) {
	if !ParseAdaptiveConfidenceQuery(interaction.Analyze("que tan seguro estas de esto", interaction.Context{})) {
		t.Error("esperaba detectar el trigger de Adaptive Confidence")
	}
	if ParseAdaptiveConfidenceQuery(interaction.Analyze("que deberia hacer con armar una tienda", interaction.Context{})) {
		t.Error("un pedido de Decision Engine no debería disparar Adaptive Confidence")
	}
}
