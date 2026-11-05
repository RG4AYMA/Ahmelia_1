package integration

import (
	"os"
	"strings"
	"testing"

	"ahmelia/environment"
	"ahmelia/linguistics"
)

func TestD35ProgrammingBoundaryRequiresInjectedInfrastructure(t *testing.T) {
	world := &environment.Manager{}
	if _, err := NewProgrammingProviderWithSynthesizer(world, nil); err == nil {
		t.Fatal("programming provider must reject missing synthesizer")
	}
	shared := linguistics.NewOllamaClient()
	synth := NewOllamaCodeObjectiveSynthesizerWithClient(shared)
	provider, err := NewProgrammingProviderWithSynthesizer(world, synth)
	if err != nil {
		t.Fatal(err)
	}
	if provider.synthesizer != synth {
		t.Fatal("programming provider did not retain injected synthesizer")
	}
	if synth.Client != shared {
		t.Fatal("synthesizer does not retain the exact shared Ollama infrastructure pointer")
	}
}

func TestD35SourceHasNoImplicitOllamaConstructionInIntegration(t *testing.T) {
	for _, path := range []string{"runtime.go", "code_synthesizer.go"} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "linguistics.NewOllamaClient()") {
			t.Fatalf("%s creates Ollama implicitly", path)
		}
	}
}
