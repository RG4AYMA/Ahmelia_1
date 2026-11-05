package programming

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"
	"time"
)

// B96.5 — Code Generation
//
// Generation is a planning/product-domain operation: it produces a code
// artifact in memory. It deliberately does not write files, execute tools,
// authorize actions, or invoke an LLM. Production/B95 remains responsible for
// deciding when and whether the generated artifact is applied.

type CodeGenerationSpec struct {
	ID        string
	ProjectID string
	Path      string
	Language  string
	Role      SourceFileRole
	Template  string
	Variables map[string]string
}

func (s CodeGenerationSpec) Validate() error {
	if strings.TrimSpace(s.ID) == "" {
		return fmt.Errorf("programming generation: ID vacío")
	}
	if strings.TrimSpace(s.ProjectID) == "" {
		return fmt.Errorf("programming generation %q: ProjectID vacío", s.ID)
	}
	if strings.TrimSpace(s.Path) == "" {
		return fmt.Errorf("programming generation %q: Path vacío", s.ID)
	}
	if strings.TrimSpace(s.Language) == "" {
		return fmt.Errorf("programming generation %q: Language vacío", s.ID)
	}
	if s.Template == "" {
		return fmt.Errorf("programming generation %q: Template vacío", s.ID)
	}
	return nil
}

type GeneratedCode struct {
	Artifact CodeArtifact
	File     SourceFile
	Content  string
}

func (g GeneratedCode) Validate() error {
	if err := g.Artifact.Validate(); err != nil {
		return err
	}
	if err := g.File.Validate(); err != nil {
		return err
	}
	if g.Content == "" {
		return fmt.Errorf("programming generated code %q: Content vacío", g.Artifact.ID)
	}
	return nil
}

// CodeGenerator is the narrow domain contract. Implementations can later be
// backed by templates, language-aware generators, or an external model.
type CodeGenerator interface {
	Generate(CodeGenerationSpec) (GeneratedCode, error)
}

// TemplateCodeGenerator performs deterministic generation using Go's standard
// text/template package. It is intentionally side-effect free.
type TemplateCodeGenerator struct{}

func NewTemplateCodeGenerator() *TemplateCodeGenerator { return &TemplateCodeGenerator{} }

func (g *TemplateCodeGenerator) Generate(spec CodeGenerationSpec) (GeneratedCode, error) {
	if err := spec.Validate(); err != nil {
		return GeneratedCode{}, err
	}
	if g == nil {
		return GeneratedCode{}, fmt.Errorf("programming: code generator unavailable")
	}

	tmpl, err := template.New(spec.ID).Option("missingkey=error").Parse(spec.Template)
	if err != nil {
		return GeneratedCode{}, fmt.Errorf("programming: parse generation template: %w", err)
	}

	var out bytes.Buffer
	if err := tmpl.Execute(&out, spec.Variables); err != nil {
		return GeneratedCode{}, fmt.Errorf("programming: execute generation template: %w", err)
	}
	content := out.String()
	if strings.TrimSpace(content) == "" {
		return GeneratedCode{}, fmt.Errorf("programming: generated content vacío")
	}

	file := SourceFile{Path: spec.Path, Language: spec.Language, Role: spec.Role, ProjectID: spec.ProjectID}
	artifact := CodeArtifact{ID: spec.ID, Type: ArtifactSource, ProjectID: spec.ProjectID, Path: spec.Path, CreatedAt: time.Now().UTC()}
	result := GeneratedCode{Artifact: artifact, File: file, Content: content}
	if err := result.Validate(); err != nil {
		return GeneratedCode{}, err
	}
	return result, nil
}
