package programming

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// B96.16 — Refactoring.
//
// Refactoring describes behaviour-preserving source transformations. This
// layer proposes explicit patches; B96.3 applies them and B95 remains the
// authority for authorization, safety, recovery and final verification.

type RefactorRequest struct {
	ID          string
	Project     CodeProject
	Kind        RefactorKind
	Description string
	Target      string
	Replacement string
	Files       map[string]string
}

func (r RefactorRequest) Validate() error {
	if strings.TrimSpace(r.ID) == "" {
		return fmt.Errorf("programming refactor request: ID vacío")
	}
	if err := r.Project.Validate(); err != nil {
		return err
	}
	switch r.Kind {
	case RefactorRename, RefactorExtract, RefactorMove, RefactorSimplify, RefactorDeduplicate, RefactorReorganize:
	default:
		return fmt.Errorf("programming refactor request %q: kind inválido", r.ID)
	}
	if strings.TrimSpace(r.Description) == "" {
		return fmt.Errorf("programming refactor request %q: description vacío", r.ID)
	}
	if r.Kind == RefactorRename {
		if strings.TrimSpace(r.Target) == "" {
			return fmt.Errorf("programming refactor request %q: target vacío", r.ID)
		}
		if strings.TrimSpace(r.Replacement) == "" {
			return fmt.Errorf("programming refactor request %q: replacement vacío", r.ID)
		}
		if r.Target == r.Replacement {
			return fmt.Errorf("programming refactor request %q: target y replacement iguales", r.ID)
		}
	}
	if len(r.Files) == 0 {
		return fmt.Errorf("programming refactor request %q: files vacíos", r.ID)
	}
	return nil
}

type RefactorResult struct {
	Refactor     Refactor
	Patches      []Patch
	FilesChanged int
	Occurrences  int
	Supported    bool
	Reason       string
	CreatedAt    time.Time
}

func (r RefactorResult) Validate() error {
	if err := r.Refactor.Validate(); err != nil {
		return err
	}
	if r.FilesChanged < 0 || r.Occurrences < 0 {
		return fmt.Errorf("programming refactor %q: métricas negativas", r.Refactor.ID)
	}
	for _, p := range r.Patches {
		if err := p.Validate(); err != nil {
			return err
		}
		if p.ProjectID != r.Refactor.ProjectID {
			return fmt.Errorf("programming refactor %q: patch project mismatch", r.Refactor.ID)
		}
	}
	return nil
}

type RefactorEngine interface {
	Propose(RefactorRequest) (RefactorResult, error)
}

type DeterministicRefactorEngine struct{}

func NewRefactorEngine() *DeterministicRefactorEngine { return &DeterministicRefactorEngine{} }

func (e *DeterministicRefactorEngine) Propose(req RefactorRequest) (RefactorResult, error) {
	if e == nil {
		return RefactorResult{}, fmt.Errorf("programming: refactor engine unavailable")
	}
	if err := req.Validate(); err != nil {
		return RefactorResult{}, err
	}
	ref := Refactor{ID: req.ID, ProjectID: req.Project.ID, Kind: req.Kind, Description: req.Description}
	result := RefactorResult{Refactor: ref, CreatedAt: time.Now().UTC()}
	if req.Kind != RefactorRename {
		result.Supported = false
		result.Reason = "esta implementación requiere una estrategia especializada antes de proponer patches"
		if err := result.Validate(); err != nil {
			return RefactorResult{}, err
		}
		return result, nil
	}

	paths := make([]string, 0, len(req.Files))
	for path := range req.Files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	pattern := regexp.MustCompile(`(?m)\b` + regexp.QuoteMeta(req.Target) + `\b`)
	for _, path := range paths {
		before := req.Files[path]
		resolved, err := filepath.Abs(filepath.Join(req.Project.Root, path))
		if err != nil || !pathWithin(resolved, req.Project.Root) {
			return RefactorResult{}, fmt.Errorf("programming refactor %q: archivo fuera del proyecto: %s", req.ID, path)
		}
		count := len(pattern.FindAllStringIndex(before, -1))
		if count == 0 {
			continue
		}
		after := pattern.ReplaceAllString(before, req.Replacement)
		patch := Patch{ID: fmt.Sprintf("%s:%s", req.ID, path), ProjectID: req.Project.ID, File: path, Before: before, After: after, UnifiedDiff: "", CreatedAt: result.CreatedAt}
		if err := patch.Validate(); err != nil {
			return RefactorResult{}, err
		}
		result.Patches = append(result.Patches, patch)
		result.FilesChanged++
		result.Occurrences += count
	}
	if len(result.Patches) == 0 {
		result.Supported = false
		result.Reason = fmt.Sprintf("target %q no encontrado en los archivos proporcionados", req.Target)
	} else {
		result.Supported = true
		result.Reason = fmt.Sprintf("rename propuesto en %d archivo(s), %d ocurrencia(s)", result.FilesChanged, result.Occurrences)
	}
	result.Refactor.Patches = append([]Patch(nil), result.Patches...)
	if err := result.Validate(); err != nil {
		return RefactorResult{}, err
	}
	return result, nil
}
