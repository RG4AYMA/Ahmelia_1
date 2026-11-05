package programming

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// B96.3 — Source File Operations
//
// These operations are programming-domain operations over the Code Workspace.
// They deliberately do not perform authorization or safety decisions; those
// remain owned by B95/B94. The implementation only enforces programming
// invariants such as project identity and path confinement.

type SourceFileOperation string

const (
	SourceCreate SourceFileOperation = "create"
	SourceRead   SourceFileOperation = "read"
	SourceWrite  SourceFileOperation = "write"
	SourceEdit   SourceFileOperation = "edit"
	SourceMove   SourceFileOperation = "move"
	SourceRename SourceFileOperation = "rename"
	SourceDelete SourceFileOperation = "delete"
)

type SourceFileRequest struct {
	Operation SourceFileOperation
	File      SourceFile
	Content   string
	NewPath   string
	OldText   string
	NewText   string
}

func (r SourceFileRequest) Validate() error {
	if r.Operation == "" {
		return fmt.Errorf("programming source operation: operation vacío")
	}
	if err := r.File.Validate(); err != nil {
		return err
	}
	switch r.Operation {
	case SourceCreate, SourceWrite:
		// Content may intentionally be empty: an empty source file is valid.
	case SourceEdit:
		if r.OldText == "" {
			return fmt.Errorf("programming source edit: OldText vacío")
		}
	case SourceMove:
		if strings.TrimSpace(r.NewPath) == "" {
			return fmt.Errorf("programming source move: NewPath vacío")
		}
	case SourceRename:
		if strings.TrimSpace(r.NewPath) == "" {
			return fmt.Errorf("programming source rename: NewPath vacío")
		}
		if filepath.Base(r.NewPath) != r.NewPath || r.NewPath == "." || r.NewPath == ".." {
			return fmt.Errorf("programming source rename: nombre inválido")
		}
	case SourceRead, SourceDelete:
	default:
		return fmt.Errorf("programming source operation %q desconocida", r.Operation)
	}
	return nil
}

type SourceFileResult struct {
	File    SourceFile
	Content string
	Before  string
	After   string
}

// SourceFileOperations is the narrow contract used by B96.3. Implementations
// can later be backed by a local filesystem, a remote workspace, or another
// environment adapter without changing the programming model.
type SourceFileOperations interface {
	Create(SourceFile, string) (SourceFileResult, error)
	Read(SourceFile) (SourceFileResult, error)
	Write(SourceFile, string) (SourceFileResult, error)
	Edit(SourceFile, string, string) (SourceFileResult, error)
	Move(SourceFile, string) (SourceFileResult, error)
	Rename(SourceFile, string) (SourceFileResult, error)
	Delete(SourceFile) (SourceFileResult, error)
}

// OSSourceFileOperations performs real source-file operations inside the
// ProgrammingWorkspace project root. It is intentionally small: no shell,
// compiler, VCS, or authorization logic belongs here.
type OSSourceFileOperations struct {
	Workspace *ProgrammingWorkspace
	mu        sync.Mutex
}

func NewOSSourceFileOperations(workspaces ...*ProgrammingWorkspace) (*OSSourceFileOperations, error) {
	var workspace *ProgrammingWorkspace
	if len(workspaces) > 1 {
		return nil, fmt.Errorf("programming: expected at most one workspace")
	}
	if len(workspaces) == 1 {
		workspace = workspaces[0]
		if workspace == nil {
			return nil, fmt.Errorf("programming: workspace nil")
		}
	}
	return &OSSourceFileOperations{Workspace: workspace}, nil
}

func (o *OSSourceFileOperations) resolve(file SourceFile) (string, error) {
	if o == nil || o.Workspace == nil {
		return "", fmt.Errorf("programming: source workspace unavailable")
	}
	if err := file.Validate(); err != nil {
		return "", err
	}
	if file.ProjectID != o.Workspace.Project.ID {
		return "", fmt.Errorf("programming: source project %q no coincide con workspace project %q", file.ProjectID, o.Workspace.Project.ID)
	}
	return o.Workspace.SourcePath(file.Path)
}

func (o *OSSourceFileOperations) resolveNew(relative string) (string, error) {
	if o == nil || o.Workspace == nil {
		return "", fmt.Errorf("programming: source workspace unavailable")
	}
	if strings.TrimSpace(relative) == "" {
		return "", fmt.Errorf("programming: destination vacío")
	}
	return o.Workspace.SourcePath(relative)
}

func (o *OSSourceFileOperations) Create(file SourceFile, content string) (SourceFileResult, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	path, err := o.resolve(file)
	if err != nil {
		return SourceFileResult{}, err
	}
	if _, err := os.Stat(path); err == nil {
		return SourceFileResult{}, fmt.Errorf("programming: source file ya existe: %s", file.Path)
	} else if !os.IsNotExist(err) {
		return SourceFileResult{}, fmt.Errorf("programming: no se pudo comprobar source file: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return SourceFileResult{}, fmt.Errorf("programming: crear directorio: %w", err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return SourceFileResult{}, fmt.Errorf("programming: crear source file: %w", err)
	}
	return SourceFileResult{File: file, After: content, Content: content}, nil
}

func (o *OSSourceFileOperations) Read(file SourceFile) (SourceFileResult, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	path, err := o.resolve(file)
	if err != nil {
		return SourceFileResult{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return SourceFileResult{}, fmt.Errorf("programming: leer source file: %w", err)
	}
	content := string(data)
	return SourceFileResult{File: file, Content: content, Before: content, After: content}, nil
}

func (o *OSSourceFileOperations) Write(file SourceFile, content string) (SourceFileResult, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	path, err := o.resolve(file)
	if err != nil {
		return SourceFileResult{}, err
	}
	old, err := os.ReadFile(path)
	if err != nil {
		return SourceFileResult{}, fmt.Errorf("programming: escribir source file: %w", err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return SourceFileResult{}, fmt.Errorf("programming: escribir source file: %w", err)
	}
	return SourceFileResult{File: file, Content: content, Before: string(old), After: content}, nil
}

func (o *OSSourceFileOperations) Edit(file SourceFile, oldText, newText string) (SourceFileResult, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	path, err := o.resolve(file)
	if err != nil {
		return SourceFileResult{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return SourceFileResult{}, fmt.Errorf("programming: editar source file: %w", err)
	}
	before := string(data)
	if !strings.Contains(before, oldText) {
		return SourceFileResult{}, fmt.Errorf("programming: texto objetivo no encontrado en %s", file.Path)
	}
	after := strings.Replace(before, oldText, newText, 1)
	if err := os.WriteFile(path, []byte(after), 0644); err != nil {
		return SourceFileResult{}, fmt.Errorf("programming: guardar edición: %w", err)
	}
	return SourceFileResult{File: file, Content: after, Before: before, After: after}, nil
}

func (o *OSSourceFileOperations) Move(file SourceFile, newPath string) (SourceFileResult, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	source, err := o.resolve(file)
	if err != nil {
		return SourceFileResult{}, err
	}
	destination, err := o.resolveNew(newPath)
	if err != nil {
		return SourceFileResult{}, err
	}
	if _, err := os.Stat(source); err != nil {
		return SourceFileResult{}, fmt.Errorf("programming: source file no existe: %w", err)
	}
	if _, err := os.Stat(destination); err == nil {
		return SourceFileResult{}, fmt.Errorf("programming: destination ya existe: %s", newPath)
	} else if !os.IsNotExist(err) {
		return SourceFileResult{}, err
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
		return SourceFileResult{}, fmt.Errorf("programming: crear destino: %w", err)
	}
	if err := os.Rename(source, destination); err != nil {
		return SourceFileResult{}, fmt.Errorf("programming: mover source file: %w", err)
	}
	moved := file
	moved.Path = newPath
	return SourceFileResult{File: moved}, nil
}

func (o *OSSourceFileOperations) Rename(file SourceFile, newName string) (SourceFileResult, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if filepath.Base(newName) != newName || newName == "." || newName == ".." || strings.TrimSpace(newName) == "" {
		return SourceFileResult{}, fmt.Errorf("programming: nombre de archivo inválido")
	}
	source, err := o.resolve(file)
	if err != nil {
		return SourceFileResult{}, err
	}
	destination := filepath.Join(filepath.Dir(source), newName)
	if _, err := os.Stat(destination); err == nil {
		return SourceFileResult{}, fmt.Errorf("programming: destination ya existe: %s", newName)
	} else if !os.IsNotExist(err) {
		return SourceFileResult{}, err
	}
	if err := os.Rename(source, destination); err != nil {
		return SourceFileResult{}, fmt.Errorf("programming: renombrar source file: %w", err)
	}
	renamed := file
	renamed.Path = filepath.Join(filepath.Dir(file.Path), newName)
	return SourceFileResult{File: renamed}, nil
}

func (o *OSSourceFileOperations) Delete(file SourceFile) (SourceFileResult, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	path, err := o.resolve(file)
	if err != nil {
		return SourceFileResult{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return SourceFileResult{}, fmt.Errorf("programming: leer antes de eliminar: %w", err)
	}
	if err := os.Remove(path); err != nil {
		return SourceFileResult{}, fmt.Errorf("programming: eliminar source file: %w", err)
	}
	return SourceFileResult{File: file, Before: string(data)}, nil
}
