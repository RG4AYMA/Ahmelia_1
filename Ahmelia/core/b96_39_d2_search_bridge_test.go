package core

import (
	"os"
	"strings"
	"testing"

	"ahmelia/capability"
	"ahmelia/interaction"
	"ahmelia/knowledge"
	"ahmelia/situation"
)

func TestD2IntentSearchProducesObservableKnowledgeEffect(t *testing.T) {
	t.Setenv("AHMELIA_FLOW_TRACE", "1")
	e := newInteractionE2EEngine(t)
	e.conversationGenerator = nil // force the canonical response fallback to expose the test knowledge result
	var responses []string
	e.SetResponseCallback(func(msg string) { responses = append(responses, msg) })

	e.knowledgeEngine.Register(knowledge.NewInternetSourceWithConfig(func(req interaction.SemanticRequest, _ situation.DecisionEvidence) (string, float64, bool) {
		return "D2_SEARCH_E2E_OK:" + req.Topic, 0.93, true
	}, knowledge.InternetSourceConfig{Enabled: true}))

	req := interaction.Analyze("busca información sobre Go", interaction.Context{})
	decision := interaction.ResolveCapabilityInteraction(req)
	route := e.capabilityResolver.Resolve(decision, req)
	if route.Kind != capability.IntentSearch {
		t.Fatalf("search route kind = %q, want %q", route.Kind, capability.IntentSearch)
	}

	e.Process(req.Raw)

	joined := strings.Join(responses, "\n")
	if !strings.Contains(joined, "D2_SEARCH_E2E_OK:go") {
		t.Fatalf("search result did not reach the response boundary: %q", joined)
	}
	if e.lastFlowTrace == nil {
		t.Fatal("expected flow trace")
	}
	trace := strings.Join(e.lastFlowTrace.Snapshot(), "\n")
	for _, marker := range []string{"SEARCH ROUTE", "intent=search"} {
		if !strings.Contains(trace, marker) {
			t.Fatalf("missing search trace marker %q in:\n%s", marker, trace)
		}
	}
}

func TestD2SearchDoesNotFallThroughToLocalFileCapability(t *testing.T) {
	t.Setenv("AHMELIA_FLOW_TRACE", "1")
	e := newInteractionE2EEngine(t)
	e.conversationGenerator = nil
	var responses []string
	e.SetResponseCallback(func(msg string) { responses = append(responses, msg) })

	// Keep Internet disabled: the route must fail closed as search, not mutate
	// the request into the legacy local-file action "buscar_archivo".
	e.Process("busca información sobre Go")

	joined := strings.Join(responses, "\n")
	if strings.Contains(joined, "No encontré el archivo") || strings.Contains(joined, "Archivo no encontrado") {
		t.Fatalf("search request fell through to local file search: %q", joined)
	}
	trace := strings.Join(e.lastFlowTrace.Snapshot(), "\n")
	if !strings.Contains(trace, "SEARCH ROUTE") {
		t.Fatalf("search route was not consumed before local capability execution:\n%s", trace)
	}
}

func TestD2InternetSourceRegistrationUsesRuntimeFlag(t *testing.T) {
	t.Setenv(knowledge.InternetSourceEnv, "true")
	e := newInteractionE2EEngine(t)
	found := false
	for _, name := range e.knowledgeEngine.RegisteredSources() {
		if name == "internet" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("enabled runtime flag did not register the internet knowledge source: %v", e.knowledgeEngine.RegisteredSources())
	}
}

func TestD2CapabilityRegistrationAuthorityRemainsRuntimeOnly(t *testing.T) {
	files := []string{
		"search_route.go",
		"../knowledge/sources.go",
		"../knowledge/internet_config.go",
	}
	for _, name := range files {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		src := string(data)
		if strings.Contains(src, "capability.Registry") || strings.Contains(src, ".Register(") || strings.Contains(src, "RegisterRuntimeProviders") {
			t.Fatalf("D2 search/knowledge boundary %s contains a capability registration mechanism", name)
		}
	}

	runtime, err := os.ReadFile("../integration/runtime.go")
	if err != nil {
		t.Fatal(err)
	}
	runtimeSrc := string(runtime)
	if !strings.Contains(runtimeSrc, "func RegisterRuntimeProviders") || !strings.Contains(runtimeSrc, "providers.RegisterCapabilities") {
		t.Fatal("canonical runtime capability registration authority is missing")
	}
	if strings.Contains(runtimeSrc, "RegisterLearnedCapability") || strings.Contains(runtimeSrc, "RegisterKnowledgeCapability") {
		t.Fatal("a second learned-knowledge capability registrar was introduced")
	}
}
