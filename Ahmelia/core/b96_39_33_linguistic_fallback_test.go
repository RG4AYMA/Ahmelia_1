package core

import (
	"context"
	"testing"

	"ahmelia/interaction"
	"ahmelia/linguistics"
)

type b963933LanguageClient struct {
	response string
	calls    int
}

func (c *b963933LanguageClient) GenerateContext(_ context.Context, prompt string) (string, error) {
	c.calls++
	return c.response, nil
}

func (c *b963933LanguageClient) Generate(string) (string, error) {
	c.calls++
	return c.response, nil
}

func (c *b963933LanguageClient) Available() bool { return true }

func TestB963933NovelQuestionUsesLinguisticFallback(t *testing.T) {
	// This wording is intentionally not present in the deterministic grammar
	// tests or production regexes. The test proves that the architecture
	// escalates an unseen paraphrase rather than requiring another template.
	client := &b963933LanguageClient{response: `{
		"intent":"question",
		"entities":[],
		"references":[],
		"constraints":[],
		"meaning":"Pregunta por la preferencia del usuario sobre una bebida",
		"speech_act":"question",
		"topic":"bebida favorita",
		"question_type":"general",
		"directness":"question",
		"user_state_signals":[],
		"correction_target":"",
		"relations":[{
			"source":"user",
			"predicate":"asks_about_preference",
			"target":"bebida favorita",
			"value":"",
			"confidence":0.95
		}],
		"confidence":0.95
	}`}
	engine := &Engine{
		interactionManager:    interaction.NewManager(),
		linguisticInterpreter: linguistics.NewInterpreter(client),
	}

	request := "¿Tú tienes alguna noción de mi bebida favorita?"
	req := engine.analyzeSemanticBoundary(request, interaction.Context{Language: "es"}, false, nil)

	if client.calls != 1 {
		t.Fatalf("expected unseen short question to reach linguistic interpreter once, got %d calls", client.calls)
	}

	attribute, ok := interaction.ResolveSemanticAttribute(req)
	if !ok {
		t.Fatalf("linguistic enrichment did not produce a semantic attribute: %+v", req)
	}
	if attribute.Subject != interaction.UserObservationSubjectUser {
		t.Fatalf("expected USER ownership from linguistic relation, got %+v", attribute)
	}
	if attribute.Key != "bebida favorita" {
		t.Fatalf("expected free-form attribute key from linguistic relation, got %+v", attribute)
	}
	if attribute.HasValue {
		t.Fatalf("query relation must not become a declaration: %+v", attribute)
	}

	query, ok := interaction.DetectUserModelQuery(req)
	if !ok {
		t.Fatalf("enriched novel question did not reach User Model query contract: %+v", req)
	}
	if query.Domain != string(interaction.UserModelDomainPreference) || len(query.Topics) != 1 || query.Topics[0] != "bebida favorita" {
		t.Fatalf("unexpected User Model query: %+v", query)
	}
}

func TestB963933DifferentNovelQuestionShapeUsesSameFallback(t *testing.T) {
	client := &b963933LanguageClient{response: `{
		"intent":"question",
		"entities":[],
		"references":[],
		"constraints":[],
		"meaning":"Pregunta por una preferencia del usuario",
		"speech_act":"question",
		"topic":"estilo preferido",
		"question_type":"general",
		"directness":"question",
		"user_state_signals":[],
		"correction_target":"",
		"relations":[{
			"source":"user",
			"predicate":"asks_about_preference",
			"target":"estilo preferido",
			"value":"",
			"confidence":0.94
		}],
		"confidence":0.94
	}`}
	engine := &Engine{
		interactionManager:    interaction.NewManager(),
		linguisticInterpreter: linguistics.NewInterpreter(client),
	}

	request := "¿Recuerdas cuál es el estilo que suelo preferir?"
	req := engine.analyzeSemanticBoundary(request, interaction.Context{Language: "es"}, false, nil)

	if client.calls != 1 {
		t.Fatalf("expected second unseen question shape to reach linguistic interpreter once, got %d calls", client.calls)
	}
	attribute, ok := interaction.ResolveSemanticAttribute(req)
	if !ok || attribute.Subject != interaction.UserObservationSubjectUser || attribute.Key != "estilo preferido" {
		t.Fatalf("unexpected semantic result from novel question shape: %+v ok=%v", attribute, ok)
	}
}

func TestB963933KnownSemanticQuestionStaysOnFastPath(t *testing.T) {
	client := &b963933LanguageClient{response: `{
		"intent":"question",
		"entities":[],
		"references":[],
		"constraints":[],
		"meaning":"Pregunta por una preferencia del usuario",
		"speech_act":"question",
		"topic":"color favorito",
		"question_type":"general",
		"directness":"question",
		"user_state_signals":[],
		"correction_target":"",
		"relations":[],
		"confidence":0.95
	}`}
	engine := &Engine{
		interactionManager:    interaction.NewManager(),
		linguisticInterpreter: linguistics.NewInterpreter(client),
	}

	req := engine.analyzeSemanticBoundary("¿Cuál es mi color favorito?", interaction.Context{Language: "es"}, false, nil)
	if client.calls != 0 {
		t.Fatalf("known deterministic semantic form should stay on the fast path, got %d LLM calls", client.calls)
	}
	attribute, ok := interaction.ResolveSemanticAttribute(req)
	if !ok || attribute.Subject != interaction.UserObservationSubjectUser || attribute.Key != "color favorito" {
		t.Fatalf("fast-path semantic query was not preserved: %+v ok=%v", attribute, ok)
	}
}
