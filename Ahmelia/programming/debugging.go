package programming

import (
	"context"
	"fmt"
	"strings"
)

// B96.12 — Debugging
//
// Debugging is the programming-domain diagnosis/repair coordinator. It does
// not own recovery, retry, authorization, execution, or verification policy;
// those remain B95 authorities. Programming only supplies the code-specific
// diagnosis and patch operations needed by a recovery decision.

type DebugRequest struct {
	ProjectID   string
	Diagnostics []Diagnostic
	Source      SourceFile
	Before      string
	Expected    string
	Replacement string
}

func (r DebugRequest) Validate() error {
	if strings.TrimSpace(r.ProjectID) == "" {
		return fmt.Errorf("programming debug: ProjectID vacío")
	}
	if err := r.Source.Validate(); err != nil {
		return err
	}
	if r.Source.ProjectID != r.ProjectID {
		return fmt.Errorf("programming debug: project mismatch")
	}
	if len(r.Diagnostics) == 0 {
		return fmt.Errorf("programming debug: no diagnostics")
	}
	if strings.TrimSpace(r.Before) == "" {
		return fmt.Errorf("programming debug: source vacío")
	}
	if r.Expected == "" {
		return fmt.Errorf("programming debug: Expected vacío")
	}
	return nil
}

type DebugResult struct {
	ProjectID   string
	Diagnostics []Diagnostic
	Patch       Patch
	Resolved    bool
}

func (r DebugResult) Validate() error {
	if strings.TrimSpace(r.ProjectID) == "" {
		return fmt.Errorf("programming debug result: ProjectID vacío")
	}
	if len(r.Diagnostics) == 0 {
		return fmt.Errorf("programming debug result: no diagnostics")
	}
	if r.Patch.ProjectID != r.ProjectID {
		return fmt.Errorf("programming debug result: patch project mismatch")
	}
	if err := r.Patch.Validate(); err != nil {
		return err
	}
	return nil
}

type Debugger interface {
	DiagnoseAndPatch(context.Context, DebugRequest) (DebugResult, error)
}

type PreciseDebugger struct{ Editor CodeEditor }

func NewPreciseDebugger(editor CodeEditor) *PreciseDebugger {
	if editor == nil {
		editor = NewPreciseCodeEditor()
	}
	return &PreciseDebugger{Editor: editor}
}

func (d *PreciseDebugger) DiagnoseAndPatch(ctx context.Context, req DebugRequest) (DebugResult, error) {
	if d == nil || d.Editor == nil {
		return DebugResult{}, fmt.Errorf("programming debug: debugger unavailable")
	}
	if err := req.Validate(); err != nil {
		return DebugResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return DebugResult{}, err
	}

	// A debugger must never invent a diagnosis: it requires an existing
	// diagnostic that points at the requested source file when file metadata
	// is available.
	relevant := false
	for _, diag := range req.Diagnostics {
		if err := diag.Validate(); err != nil {
			return DebugResult{}, err
		}
		if diag.ProjectID != req.ProjectID {
			return DebugResult{}, fmt.Errorf("programming debug: diagnostic project mismatch")
		}
		if diag.File == "" || diag.File == req.Source.Path {
			relevant = true
		}
	}
	if !relevant {
		return DebugResult{}, fmt.Errorf("programming debug: no diagnostic matches source file")
	}

	if err := ctx.Err(); err != nil {
		return DebugResult{}, err
	}
	spec := CodeEditSpec{ID: "debug-patch-" + req.Source.Path, ProjectID: req.ProjectID, File: req.Source, Expected: req.Expected, Replace: req.Replacement}
	edited, err := d.Editor.CreatePatch(spec, req.Before)
	if err != nil {
		return DebugResult{}, err
	}
	result := DebugResult{ProjectID: req.ProjectID, Diagnostics: append([]Diagnostic(nil), req.Diagnostics...), Patch: edited.Patch, Resolved: true}
	if err := result.Validate(); err != nil {
		return DebugResult{}, err
	}
	return result, nil
}

// DebugAssessment is the analysis-only B96.12 entry point used when a
// failure has reached the production loop but no safe patch specification is
// available yet. It deliberately produces no mutation. A concrete
// DiagnoseAndPatch call is used when a caller can supply an exact source
// edit; otherwise this assessment prevents the failure from disappearing
// into the generic string "tests failed".
type DebugAssessment struct {
	ProjectID   string
	Diagnostics []Diagnostic
	Action      string
	Resolved    bool
	Reason      string
}

func AssessFailureForDebugging(ctx context.Context, projectID string, diagnostics []Diagnostic) (DebugAssessment, error) {
	if err := ctx.Err(); err != nil {
		return DebugAssessment{}, err
	}
	if strings.TrimSpace(projectID) == "" {
		return DebugAssessment{}, fmt.Errorf("programming debug assessment: ProjectID vacío")
	}
	if len(diagnostics) == 0 {
		return DebugAssessment{}, fmt.Errorf("programming debug assessment: no diagnostics")
	}
	for _, d := range diagnostics {
		if err := d.Validate(); err != nil {
			return DebugAssessment{}, err
		}
		if d.ProjectID != projectID {
			return DebugAssessment{}, fmt.Errorf("programming debug assessment: project mismatch")
		}
	}
	return DebugAssessment{
		ProjectID:   projectID,
		Diagnostics: append([]Diagnostic(nil), diagnostics...),
		Action:      "requires_precise_patch",
		Resolved:    false,
		Reason:      "diagnóstico listo para B96.12; se requiere una especificación de edición precisa antes de mutar código",
	}, nil
}
