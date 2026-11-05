package linguistics

// Pruebas puras del contrato de comprensión abierta — sin HTTP, sin Ollama.
// La prueba viva (build tag ahmelia_live) está en open_meaning_live_test.go.

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestParseOpenMeaning_ValidFull(t *testing.T) {
	raw := `{"purpose":"declaracion","owner_of_data":"user","topic":"bebida favorita",
		"meaning":"Declara que su bebida favorita es el café con leche",
		"mentions":["bebida favorita","café con leche"],"confidence":0.9}`
	m, err := ParseOpenMeaning(raw)
	if err != nil {
		t.Fatalf("parse válido falló: %v", err)
	}
	if m.Purpose != OpenPurposeDeclaration {
		t.Fatalf("purpose = %q", m.Purpose)
	}
	if m.OwnerOfData != OpenOwnerUser {
		t.Fatalf("owner = %q", m.OwnerOfData)
	}
	if m.Topic != "bebida favorita" || !strings.Contains(m.Meaning, "café con leche") {
		t.Fatalf("topic/meaning corruptos: %q / %q", m.Topic, m.Meaning)
	}
	if len(m.Mentions) != 2 || m.Confidence != 0.9 {
		t.Fatalf("mentions/conf: %v / %v", m.Mentions, m.Confidence)
	}
}

func TestParseOpenMeaning_EmptyMeaningRejected(t *testing.T) {
	if _, err := ParseOpenMeaning(`{"purpose":"social","owner_of_data":"user","topic":"","meaning":"   ","mentions":[],"confidence":0.5}`); err == nil {
		t.Fatal("meaning vacío debe rechazarse")
	}
}

func TestParseOpenMeaning_InvalidJSONRejected(t *testing.T) {
	if _, err := ParseOpenMeaning(`no es json`); err == nil {
		t.Fatal("json inválido debe rechazarse")
	}
}

func TestParseOpenMeaning_HygieneNormalizes(t *testing.T) {
	m, err := ParseOpenMeaning(`{"purpose":"  ","owner_of_data":"intruso","topic":" t ","meaning":"algo","mentions":[" a ", "", "b"],"confidence":1.7}`)
	if err != nil {
		t.Fatalf("parse falló: %v", err)
	}
	if m.Purpose != OpenPurposeUnknown {
		t.Fatalf("purpose fuera de dominio debe caer en desconocido, got %q", m.Purpose)
	}
	if m.OwnerOfData != OpenOwnerUndetermined {
		t.Fatalf("owner fuera de dominio debe caer en no_determinado, got %q", m.OwnerOfData)
	}
	if m.Topic != "t" {
		t.Fatalf("topic sin recortar: %q", m.Topic)
	}
	if len(m.Mentions) != 2 || m.Mentions[0] != "a" || m.Mentions[1] != "b" {
		t.Fatalf("mentions sin higiene: %v", m.Mentions)
	}
	if m.Confidence != 1 {
		t.Fatalf("conf debe clampear a 1, got %v", m.Confidence)
	}
}

func TestOpenMeaningSchema_RequiredShape(t *testing.T) {
	var schema map[string]any
	if err := json.Unmarshal(openMeaningJSONSchema(), &schema); err != nil {
		t.Fatalf("schema inválido: %v", err)
	}
	required, _ := schema["required"].([]any)
	if len(required) != 6 {
		t.Fatalf("required debe tener 6 campos, got %d", len(required))
	}
	props, _ := schema["properties"].(map[string]any)
	purpose := props["purpose"].(map[string]any)
	enum, _ := purpose["enum"].([]any)
	if len(enum) != 8 {
		t.Fatalf("purpose debe tener 8 valores en su enum, got %d", len(enum))
	}
}

func TestBuildOpenMeaningPrompt_HoldsNegativesAndQuotesInput(t *testing.T) {
	p := buildOpenMeaningPrompt("mi bebida favorita es el café con leche")
	for _, frag := range []string{"NO eres Ahmelia", "NO inventes hechos", "NO respondas al usuario", `"mi bebida favorita es el café con leche"`} {
		if !strings.Contains(p, frag) {
			t.Fatalf("el prompt pierde el fragmento %q", frag)
		}
	}
}

// fakeStructuredOpenClient() funciona sin red: prueba el cable interno de la puerta.
type fakeStructuredOpenClient struct {
	raw       string
	available bool
}

func (f fakeStructuredOpenClient) GenerateStructuredContext(ctx context.Context, prompt string, schema json.RawMessage, numPredict int) (string, error) {
	return f.raw, nil
}
func (f fakeStructuredOpenClient) Generate(s string) (string, error) { return f.raw, nil }
func (f fakeStructuredOpenClient) Available() bool                   { return f.available }

func TestInterpretOpenContext_WiresAndParses(t *testing.T) {
	interp := NewInterpreter(fakeStructuredOpenClient{
		raw:       `{"purpose":"pregunta","owner_of_data":"no_determinado","topic":"capacidades","meaning":"Pregunta qué sabe hacer","mentions":[],"confidence":0.8}`,
		available: true,
	})
	m, err := interp.InterpretOpenContext(context.Background(), "¿qué sabes hacer?")
	if err != nil {
		t.Fatalf("la puerta falló: %v", err)
	}
	if m.Purpose != OpenPurposeQuestion || m.Confidence != 0.8 || m.Meaning == "" {
		t.Fatalf("resultado incompleto: %+v", m)
	}
}

// flapSequenceClient simula la disponibilidad del cliente como secuencia:
// cada llamada a Available() consume el siguiente valor; cuenta llamadas a
// generación para verificar el fail fast (DeepSeek 17:25).
type flapSequenceClient struct {
	sequence []bool
	idx      int
	genCalls int
	raw      string
}

func (f *flapSequenceClient) GenerateStructuredContext(ctx context.Context, prompt string, schema json.RawMessage, numPredict int) (string, error) {
	f.genCalls++
	return f.raw, nil
}
func (f *flapSequenceClient) Generate(s string) (string, error) { return f.raw, nil }
func (f *flapSequenceClient) Available() bool {
	if f.idx < len(f.sequence) {
		v := f.sequence[f.idx]
		f.idx++
		return v
	}
	return f.sequence[len(f.sequence)-1]
}

func TestInterpretOpenContext_ProbeFlapRetryRecover(t *testing.T) {
	client := &flapSequenceClient{
		sequence: []bool{false, true},
		raw:      `{"purpose":"declaracion","owner_of_data":"user","topic":"t","meaning":"algo","mentions":[],"confidence":0.5}`,
	}
	interp := NewInterpreter(client)
	m, err := interp.InterpretOpenContext(context.Background(), "algo")
	if err != nil {
		t.Fatalf("el retry único debía cruzar el flap: %v", err)
	}
	if !m.ProbeFlapRetry {
		t.Fatal("ProbeFlapRetry debe quedar marcado tras el retry")
	}
	if m.Purpose != OpenPurposeDeclaration {
		t.Fatalf("purpose = %q", m.Purpose)
	}
	if client.genCalls != 1 {
		t.Fatalf("generación debía llamarse 1 vez, got %d", client.genCalls)
	}
}

func TestInterpretOpenContext_ProbeFlapFailFast(t *testing.T) {
	client := &flapSequenceClient{sequence: []bool{false, false}, raw: "{}"}
	interp := NewInterpreter(client)
	_, err := interp.InterpretOpenContext(context.Background(), "algo")
	if !errors.Is(err, ErrOpenMeaningProbeFlap) {
		t.Fatalf("double flap debe exponer ErrOpenMeaningProbeFlap, got: %v", err)
	}
	if client.genCalls != 0 {
		t.Fatalf("el fail fast no debe llamar a generación, got %d llamadas", client.genCalls)
	}
}

func TestInterpretOpenContext_ShortBudgetFailsFast(t *testing.T) {
	client := &flapSequenceClient{sequence: []bool{false, false}, raw: "{}"}
	interp := NewInterpreter(client)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if _, err := interp.InterpretOpenContext(ctx, "algo"); !errors.Is(err, ErrOpenMeaningProbeFlap) {
		t.Fatalf("presupuesto corto + flap debe fallar rápido con el mismo error, got: %v", err)
	}
	if client.genCalls != 0 {
		t.Fatalf("no debe llamar a generación con presupuesto corto, got %d", client.genCalls)
	}
}

func TestInterpretOpenContext_NormalPathNoFlapFlag(t *testing.T) {
	client := &flapSequenceClient{
		sequence: []bool{true},
		raw:      `{"purpose":"pregunta","owner_of_data":"no_determinado","topic":"capacidades","meaning":"Pregunta qué sabe hacer","mentions":[],"confidence":0.8}`,
	}
	interp := NewInterpreter(client)
	m, err := interp.InterpretOpenContext(context.Background(), "¿qué sabes hacer?")
	if err != nil {
		t.Fatalf("camino normal falló: %v", err)
	}
	if m.ProbeFlapRetry {
		t.Fatal("camino normal debe dejar ProbeFlapRetry=false")
	}
}

func TestInterpretOpenContext_Guards(t *testing.T) {
	interp := NewInterpreter(fakeStructuredOpenClient{raw: "{}", available: false})
	if _, err := interp.InterpretOpenContext(context.Background(), "algo"); err == nil {
		t.Fatal("cliente indisponible debe rechazar")
	}
	interp2 := NewInterpreter(fakeStructuredOpenClient{raw: `{"meaning":""}`, available: true})
	if _, err := interp2.InterpretOpenContext(context.Background(), "algo"); err == nil {
		t.Fatal("meaning vacío debe rechazar")
	}
	if _, err := interp2.InterpretOpenContext(context.Background(), "   "); err == nil {
		t.Fatal("entrada vacía debe rechazar")
	}
}
