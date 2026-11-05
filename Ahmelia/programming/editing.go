package programming

import (
	"fmt"
	"strings"
	"time"
)

// B96.6 — Code Editing
//
// Editing is a programming-domain transformation over an existing source
// artifact. It produces an explicit Patch first; applying that patch is
// delegated to B96.3. This keeps editing separate from filesystem mutation
// and preserves Before/After evidence for B95 verification and rollback.

type CodeEditSpec struct {
	ID        string
	ProjectID string
	File      SourceFile
	Expected  string
	Replace   string
}

func (s CodeEditSpec) Validate() error {
	if strings.TrimSpace(s.ID) == "" {
		return fmt.Errorf("programming edit: ID vacío")
	}
	if strings.TrimSpace(s.ProjectID) == "" {
		return fmt.Errorf("programming edit %q: ProjectID vacío", s.ID)
	}
	if err := s.File.Validate(); err != nil {
		return err
	}
	if s.File.ProjectID != s.ProjectID {
		return fmt.Errorf("programming edit %q: file project mismatch", s.ID)
	}
	if s.Expected == "" {
		return fmt.Errorf("programming edit %q: Expected vacío", s.ID)
	}
	return nil
}

type CodeEditResult struct {
	Spec  CodeEditSpec
	Patch Patch
}

func (r CodeEditResult) Validate() error {
	if err := r.Spec.Validate(); err != nil {
		return err
	}
	if err := r.Patch.Validate(); err != nil {
		return err
	}
	if r.Patch.ProjectID != r.Spec.ProjectID || r.Patch.File != r.Spec.File.Path {
		return fmt.Errorf("programming edit %q: patch identity mismatch", r.Spec.ID)
	}
	return nil
}

type CodeEditor interface {
	CreatePatch(CodeEditSpec, string) (CodeEditResult, error)
}

type PreciseCodeEditor struct{}

func NewPreciseCodeEditor() *PreciseCodeEditor { return &PreciseCodeEditor{} }

func (e *PreciseCodeEditor) CreatePatch(spec CodeEditSpec, before string) (CodeEditResult, error) {
	if e == nil {
		return CodeEditResult{}, fmt.Errorf("programming: code editor unavailable")
	}
	if err := spec.Validate(); err != nil {
		return CodeEditResult{}, err
	}
	count := strings.Count(before, spec.Expected)
	if count == 0 {
		return CodeEditResult{}, fmt.Errorf("programming edit %q: texto objetivo no encontrado", spec.ID)
	}
	if count > 1 {
		return CodeEditResult{}, fmt.Errorf("programming edit %q: texto objetivo ambiguo (%d coincidencias)", spec.ID, count)
	}
	after := strings.Replace(before, spec.Expected, spec.Replace, 1)
	patch := Patch{ID: spec.ID, ProjectID: spec.ProjectID, File: spec.File.Path, Before: before, After: after, CreatedAt: time.Now().UTC()}
	if err := patch.Validate(); err != nil {
		return CodeEditResult{}, err
	}
	result := CodeEditResult{Spec: spec, Patch: patch}
	if err := result.Validate(); err != nil {
		return CodeEditResult{}, err
	}
	return result, nil
}

// ApplyPatch applies a previously-created patch through the existing B96.3
// source-file operation contract. The expected content is checked first so a
// stale patch cannot silently overwrite newer work.
func ApplyPatch(ops SourceFileOperations, patch Patch) (SourceFileResult, error) {
	if ops == nil {
		return SourceFileResult{}, fmt.Errorf("programming: source operations unavailable")
	}
	if err := patch.Validate(); err != nil {
		return SourceFileResult{}, err
	}
	file := SourceFile{Path: patch.File, ProjectID: patch.ProjectID}
	current, err := ops.Read(file)
	if err != nil {
		return SourceFileResult{}, err
	}
	if current.Content != patch.Before {
		return SourceFileResult{}, fmt.Errorf("programming: patch %q is stale; current content differs", patch.ID)
	}
	return ops.Write(file, patch.After)
}
