package programming

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// B96.17 — Multi-File Development.
//
// Coordinates a set of programming-domain source changes belonging to one
// project. Validation happens before mutation; each operation is delegated to
// B96.3. If a later operation fails, already-applied changes are restored from
// their captured Before state. B95 remains the authority for authorization,
// safety and final verification.

type MultiFileChange struct {
	ID        string
	Operation SourceFileOperation
	File      SourceFile
	NewPath   string
	Content   string
	OldText   string
	NewText   string
}

func (c MultiFileChange) Validate(project CodeProject) error {
	if err := project.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(c.ID) == "" {
		return fmt.Errorf("programming multi-file change: ID vacío")
	}
	if err := c.File.Validate(); err != nil {
		return err
	}
	if c.File.ProjectID != project.ID {
		return fmt.Errorf("programming multi-file change %q: project mismatch", c.ID)
	}
	switch c.Operation {
	case SourceCreate, SourceWrite:
	case SourceEdit:
		if c.OldText == "" {
			return fmt.Errorf("programming multi-file change %q: OldText vacío", c.ID)
		}
	case SourceMove, SourceRename:
		if strings.TrimSpace(c.NewPath) == "" {
			return fmt.Errorf("programming multi-file change %q: NewPath vacío", c.ID)
		}
	case SourceDelete:
	default:
		return fmt.Errorf("programming multi-file change %q: operación inválida", c.ID)
	}
	return nil
}

type MultiFilePlan struct {
	ID        string
	Project   CodeProject
	Changes   []MultiFileChange
	CreatedAt time.Time
}

func (p MultiFilePlan) Validate() error {
	if strings.TrimSpace(p.ID) == "" {
		return fmt.Errorf("programming multi-file plan: ID vacío")
	}
	if err := p.Project.Validate(); err != nil {
		return err
	}
	if len(p.Changes) == 0 {
		return fmt.Errorf("programming multi-file plan %q: changes vacíos", p.ID)
	}
	ids := make(map[string]struct{}, len(p.Changes))
	for _, c := range p.Changes {
		if err := c.Validate(p.Project); err != nil {
			return err
		}
		if _, exists := ids[c.ID]; exists {
			return fmt.Errorf("programming multi-file plan %q: change ID duplicado %q", p.ID, c.ID)
		}
		ids[c.ID] = struct{}{}
	}
	return nil
}

type MultiFileChangeResult struct {
	Change MultiFileChange
	Result SourceFileResult
}

type MultiFileResult struct {
	Plan          MultiFilePlan
	Changes       []MultiFileChangeResult
	Applied       int
	RolledBack    bool
	Success       bool
	FailureReason string
	CreatedAt     time.Time
}

type MultiFileEditor struct {
	Operations SourceFileOperations
	mu         sync.Mutex
}

func NewMultiFileEditor(operations SourceFileOperations) (*MultiFileEditor, error) {
	if operations == nil {
		return nil, fmt.Errorf("programming: source operations nil")
	}
	return &MultiFileEditor{Operations: operations}, nil
}

func (e *MultiFileEditor) Apply(plan MultiFilePlan) (MultiFileResult, error) {
	if e == nil || e.Operations == nil {
		return MultiFileResult{}, fmt.Errorf("programming: multi-file editor unavailable")
	}
	if err := plan.Validate(); err != nil {
		return MultiFileResult{}, err
	}

	// Serialize a complete plan so two plans cannot interleave mutations.
	e.mu.Lock()
	defer e.mu.Unlock()

	changes := append([]MultiFileChange(nil), plan.Changes...)
	sort.SliceStable(changes, func(i, j int) bool {
		return changes[i].ID < changes[j].ID
	})

	result := MultiFileResult{
		Plan:      plan,
		CreatedAt: time.Now().UTC(),
		Success:   true,
	}

	for _, change := range changes {
		r, err := e.apply(change)
		if err != nil {
			result.Success = false
			result.FailureReason = err.Error()
			if rollbackErr := e.rollback(result.Changes); rollbackErr != nil {
				result.FailureReason = result.FailureReason + "; rollback: " + rollbackErr.Error()
			}
			result.RolledBack = true
			return result, nil
		}
		result.Changes = append(result.Changes, MultiFileChangeResult{Change: change, Result: r})
		result.Applied++
	}
	return result, nil
}

func (e *MultiFileEditor) apply(c MultiFileChange) (SourceFileResult, error) {
	switch c.Operation {
	case SourceCreate:
		return e.Operations.Create(c.File, c.Content)
	case SourceWrite:
		return e.Operations.Write(c.File, c.Content)
	case SourceEdit:
		return e.Operations.Edit(c.File, c.OldText, c.NewText)
	case SourceMove:
		return e.Operations.Move(c.File, c.NewPath)
	case SourceRename:
		return e.Operations.Rename(c.File, c.NewPath)
	case SourceDelete:
		return e.Operations.Delete(c.File)
	default:
		return SourceFileResult{}, fmt.Errorf("programming: unsupported multi-file operation %q", c.Operation)
	}
}

// rollback restores the exact pre-change state where possible. It deliberately
// uses the narrow B96.3 contract rather than writing directly to the OS.
func (e *MultiFileEditor) rollback(applied []MultiFileChangeResult) error {
	var errs []string
	for i := len(applied) - 1; i >= 0; i-- {
		c := applied[i].Change
		r := applied[i].Result
		var err error
		switch c.Operation {
		case SourceCreate:
			_, err = e.Operations.Delete(c.File)
		case SourceWrite, SourceEdit:
			_, err = e.Operations.Write(c.File, r.Before)
		case SourceDelete:
			_, err = e.Operations.Create(c.File, r.Before)
		case SourceMove, SourceRename:
			// A move/rename result identifies the original and resulting path.
			if r.After != "" {
				// SourceFileResult does not carry a destination path, so use
				// the inverse operation with the requested NewPath.
				target := c.File
				target.Path = c.NewPath
				if c.Operation == SourceMove {
					_, err = e.Operations.Move(target, c.File.Path)
				} else {
					_, err = e.Operations.Rename(target, filepath.Base(c.File.Path))
				}
			}
		default:
			err = fmt.Errorf("unsupported rollback operation %q", c.Operation)
		}
		if err != nil {
			errs = append(errs, c.ID+": "+err.Error())
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}

// OS helper used by integration tests to assert that the coordinator itself
// does not bypass B96.3. It is intentionally not used by Apply.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
