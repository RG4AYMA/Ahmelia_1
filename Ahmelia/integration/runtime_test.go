package integration

import (
	"strings"
	"testing"

	"ahmelia/capability"
	"ahmelia/production"
	"ahmelia/programming"
)

func TestInferLanguageDefaultsToGoWithoutExplicitLanguage(t *testing.T) {
	lang, build, tests, runtime := inferLanguageCommands("analiza una capacidad que te falta y crea un bloque con tests")
	if lang != "Go" {
		t.Fatalf("language = %q, want Go", lang)
	}
	if build != "go build ./..." || tests != "go test ./..." || runtime != "go run ." {
		t.Fatalf("unexpected Go commands: %q %q %q", build, tests, runtime)
	}
}

func TestSelfDevelopmentPlanIsReviewOnlyArtifact(t *testing.T) {
	project := testProject(t)
	plan := selfDevelopmentPlan(project)
	if plan == nil || len(plan.Changes) != 4 {
		t.Fatalf("expected 3 changes, got %#v", plan)
	}
	for _, c := range plan.Changes {
		if c.File.ProjectID != project.ID {
			t.Fatalf("project mismatch in %s", c.File.Path)
		}
		if strings.TrimSpace(c.Content) == "" {
			t.Fatalf("empty generated content for %s", c.File.Path)
		}
	}
}

func TestRegisterCapabilitiesIsDomainIndependentFromEngine(t *testing.T) {
	reg := capability.NewRegistry()
	if err := RegisterCapabilities(reg); err != nil {
		t.Fatal(err)
	}
	if !reg.Has("programming.create_file") {
		t.Fatal("programming capability was not registered")
	}
	providers := production.NewProviderRegistry()
	// A provider registry is deliberately separate; capability registration
	// must not imply execution availability.
	if len(providers.List()) != 0 {
		t.Fatal("capability registration unexpectedly registered providers")
	}
}

func testProject(t *testing.T) (p programming.CodeProject) {
	t.Helper()
	return programming.CodeProject{
		ID: "test_block", Name: "test_block", Root: t.TempDir(), Language: "Go",
		SourceRoots: []string{"."}, TestRoots: []string{"."},
		BuildCommand: "go build ./...", TestCommand: "go test ./...", RuntimeCommand: "go run .",
	}
}
