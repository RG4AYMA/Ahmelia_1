package linguistics

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"ahmelia/interaction"
)

// recordingOpenClient conserva el prompt para verificar que el contexto
// autorizado llega a la puerta abierta (la que usa el proceso vivo).
type recordingOpenClient struct {
	raw     string
	prompts []string
}

func (c *recordingOpenClient) GenerateStructuredContext(ctx context.Context, prompt string, schema json.RawMessage, numPredict int) (string, error) {
	c.prompts = append(c.prompts, prompt)
	return c.raw, nil
}
func (c *recordingOpenClient) Generate(string) (string, error) { return c.raw, nil }
func (c *recordingOpenClient) Available() bool                 { return true }

func TestSituationSnapshotRenderIsBoundedAndStable(t *testing.T) {
	s := &SituationSnapshot{Facts: []SnapshotFact{
		{ID: "f2", Kind: "pregunta_pendiente", Text: "¿qué proyecto?", Source: "clarification_queue", Confidence: 1},
		{ID: "f1", Kind: "turno_usuario", Text: "abre el proyecto de Unity", Source: "conversation_history", Confidence: 1},
	}}
	if s.Empty() {
		t.Fatal("el snapshot con hechos no puede estar vacío")
	}
	out := s.Render()
	if !strings.Contains(out, "[f1]") || !strings.Contains(out, "[f2]") {
		t.Fatalf("el render debe conservar los IDs estables: %q", out)
	}
	if strings.Index(out, "[f1]") > strings.Index(out, "[f2]") {
		t.Fatalf("el render debe ser determinista y ordenado por ID: %q", out)
	}
	if runes := len([]rune(out)); runes > maxSnapshotRunes+2 {
		t.Fatalf("el render excede el presupuesto: %d runas", runes)
	}
}

func TestSituationSnapshotEmpty(t *testing.T) {
	var s *SituationSnapshot
	if !s.Empty() {
		t.Fatal("un snapshot nil debe considerarse vacío")
	}
	if s.Render() != "" {
		t.Fatalf("un snapshot vacío no debe renderizar contexto: %q", s.Render())
	}
}

func TestSanitizeResolutionsDropsUnverifiable(t *testing.T) {
	s := &SituationSnapshot{Facts: []SnapshotFact{
		{ID: "f1", Kind: "turno_usuario", Text: "abre el proyecto de Unity", Confidence: 1},
	}}
	in := []ReferenceResolution{
		{Reference: "eso", Resolved: "el proyecto de Unity", EvidenceIDs: []string{"f1"}, Confidence: 0.9},
		{Reference: "aquello", Resolved: "algo inventado", EvidenceIDs: []string{"f9"}, Confidence: 0.9},
		{Reference: "lo de ayer", Resolved: "sin cita", Confidence: 0.9},
	}
	out := SanitizeResolutions(in, s)
	if len(out) != 1 {
		t.Fatalf("solo la resolución con evidencia válida debe sobrevivir: %+v", out)
	}
	if out[0].Reference != "eso" || out[0].EvidenceIDs[0] != "f1" {
		t.Fatalf("resolución válida alterada: %+v", out[0])
	}
}

func TestSanitizeResolutionsWithoutSnapshotDropsAll(t *testing.T) {
	out := SanitizeResolutions([]ReferenceResolution{
		{Reference: "eso", Resolved: "algo", EvidenceIDs: []string{"f1"}},
	}, nil)
	if len(out) != 0 {
		t.Fatalf("sin contexto autorizado no puede haber resoluciones: %+v", out)
	}
}

func TestInterpretationPromptAuthorizesContext(t *testing.T) {
	s := &SituationSnapshot{Facts: []SnapshotFact{
		{ID: "f1", Kind: "turno_usuario", Text: "abre el proyecto de Unity", Confidence: 1},
	}}
	p := buildInterpretationPrompt("abre eso", s)
	for _, frag := range []string{"CONTEXTO AUTORIZADO", "[f1]", "evidence_ids", "abre el proyecto de Unity"} {
		if !strings.Contains(p, frag) {
			t.Fatalf("el prompt con contexto pierde %q", frag)
		}
	}
}

func TestInterpretationPromptWithoutSnapshotKeepsProhibition(t *testing.T) {
	p := buildInterpretationPrompt("abre eso", nil)
	if strings.Contains(p, "CONTEXTO AUTORIZADO (hechos verificados") {
		t.Fatal("sin snapshot no debe inyectarse contexto autorizado")
	}
	if !strings.Contains(p, "NO resuelves referencias con información que no aparezca en el texto") {
		t.Fatal("sin snapshot debe conservarse la prohibición original")
	}
}

func TestInterpretWithSituationSanitizesResolutions(t *testing.T) {
	raw := `{"intent":"command","entities":[],"references":["eso"],"resolutions":[{"reference":"eso","resolved":"el proyecto de Unity","evidence_ids":["f1"],"confidence":0.9},{"reference":"aquello","resolved":"inventado","evidence_ids":["f9"],"confidence":0.9}],"constraints":[],"meaning":"abrir eso","speech_act":"command","topic":"Unity","question_type":"","directness":"direct_command","user_state_signals":[],"correction_target":"","confidence":0.9}`
	client := &testLanguageClient{response: raw, available: true}
	interpreter := NewInterpreter(client)
	snapshot := &SituationSnapshot{Facts: []SnapshotFact{
		{ID: "f1", Kind: "turno_usuario", Text: "abre el proyecto de Unity", Confidence: 1},
	}}

	got, err := interpreter.InterpretContextWithSituation(context.Background(), "abre eso", snapshot)
	if err != nil {
		t.Fatalf("InterpretContextWithSituation devolvió error: %v", err)
	}
	if len(got.Resolutions) != 1 || got.Resolutions[0].Reference != "eso" {
		t.Fatalf("la resolución sin evidencia válida debe descartarse: %+v", got.Resolutions)
	}
	if len(client.prompts) != 1 || !strings.Contains(client.prompts[0], "[f1]") {
		t.Fatal("el prompt enviado al modelo debe incluir el contexto autorizado")
	}
}

func TestOpenPipelineWithSituationPassesSnapshotAndSanitizes(t *testing.T) {
	raw := `{"purpose":"pedido","owner_of_data":"user","topic":"Unity","meaning":"Abrir el proyecto de Unity","mentions":["Unity"],"resolutions":[{"reference":"eso","resolved":"el proyecto de Unity","evidence_ids":["f1"],"confidence":0.9},{"reference":"aquello","resolved":"algo inventado","evidence_ids":["f9"],"confidence":0.9}],"confidence":0.9}`
	client := &recordingOpenClient{raw: raw}
	pipeline := NewPipeline(NewRouter(), NewInterpreter(client), NewValidator())
	pipeline.SetOpenComprehension(true)
	input := "¿y eso qué es?"
	request := interaction.Analyze(input, interaction.Context{})
	snapshot := &SituationSnapshot{Facts: []SnapshotFact{
		{ID: "f1", Kind: "turno_usuario", Text: "abre el proyecto de Unity", Confidence: 1},
	}}

	if !NewRouter().DecideRequest(request).UseLLM {
		t.Fatal("la entrada de prueba debe requerir profundidad lingüística")
	}

	result, err := pipeline.ProcessRequestWithSituation(context.Background(), input, request, snapshot)
	if err != nil {
		t.Fatalf("pipeline abierto con situación falló: %v", err)
	}
	if result.OpenMeaning == nil {
		t.Fatal("modo abierto debe producir OpenMeaning")
	}
	if len(result.OpenMeaning.Resolutions) != 1 || result.OpenMeaning.Resolutions[0].Reference != "eso" {
		t.Fatalf("la resolución sin evidencia válida debe descartarse: %+v", result.OpenMeaning.Resolutions)
	}
	if len(client.prompts) != 1 || !strings.Contains(client.prompts[0], "CONTEXTO AUTORIZADO") || !strings.Contains(client.prompts[0], "[f1]") {
		t.Fatal("el contexto autorizado debe llegar al prompt de la puerta abierta")
	}
}
