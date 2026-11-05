package programming

import (
	"strings"
	"testing"
)

func TestTemplateCodeGenerator_GeneratesWithoutFilesystemSideEffects(t *testing.T) {
	generator := NewTemplateCodeGenerator()
	result, err := generator.Generate(CodeGenerationSpec{
		ID: "artifact-1", ProjectID: "project-1", Path: "main.go", Language: "go", Role: RoleEntrypoint,
		Template:  "package {{.Package}}\n\nfunc {{.Function}}() {}\n",
		Variables: map[string]string{"Package": "main", "Function": "Run"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := result.Validate(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Content, "package main") || !strings.Contains(result.Content, "func Run") {
		t.Fatalf("contenido inesperado: %q", result.Content)
	}
}

func TestTemplateCodeGenerator_RejectsMissingVariable(t *testing.T) {
	_, err := NewTemplateCodeGenerator().Generate(CodeGenerationSpec{
		ID: "artifact-2", ProjectID: "project-1", Path: "main.go", Language: "go",
		Template: "package {{.Missing}}", Variables: map[string]string{},
	})
	if err == nil {
		t.Fatal("expected missing variable error")
	}
}

func TestTemplateCodeGenerator_RejectsInvalidSpec(t *testing.T) {
	_, err := NewTemplateCodeGenerator().Generate(CodeGenerationSpec{ID: "only-id"})
	if err == nil {
		t.Fatal("expected validation error")
	}
}
