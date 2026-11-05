package programming

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// B96.7 — Project Structure
//
// Project structure is a read-only description of how a CodeProject is
// organized. It does not create a second workspace or filesystem model.
// The project root remains the source of truth and the existing B96.2/B96.3
// workspace contracts are reused for path confinement.

type ProjectDirectoryRole string

const (
	DirectoryUnknown       ProjectDirectoryRole = "unknown"
	DirectorySource        ProjectDirectoryRole = "source"
	DirectoryTests         ProjectDirectoryRole = "tests"
	DirectoryAssets        ProjectDirectoryRole = "assets"
	DirectoryConfig        ProjectDirectoryRole = "config"
	DirectoryDependencies  ProjectDirectoryRole = "dependencies"
	DirectoryBuild         ProjectDirectoryRole = "build"
	DirectoryDocumentation ProjectDirectoryRole = "documentation"
)

type ProjectDirectory struct {
	Path      string
	Role      ProjectDirectoryRole
	Exists    bool
	FileCount int
}

type ProjectStructure struct {
	ProjectID   string
	Root        string
	Directories []ProjectDirectory
	Files       []SourceFile
	ObservedAt  time.Time
}

func (s ProjectStructure) Validate() error {
	if strings.TrimSpace(s.ProjectID) == "" {
		return fmt.Errorf("programming project structure: ProjectID vacío")
	}
	if strings.TrimSpace(s.Root) == "" {
		return fmt.Errorf("programming project structure %q: Root vacío", s.ProjectID)
	}
	for _, d := range s.Directories {
		if strings.TrimSpace(d.Path) == "" {
			return fmt.Errorf("programming project structure %q: directory path vacío", s.ProjectID)
		}
	}
	for _, f := range s.Files {
		if err := f.Validate(); err != nil {
			return err
		}
		if f.ProjectID != s.ProjectID {
			return fmt.Errorf("programming project structure %q: file project mismatch", s.ProjectID)
		}
	}
	return nil
}

type ProjectStructureAnalyzer interface {
	AnalyzeProjectStructure(CodeProject) (ProjectStructure, error)
}

type OSProjectStructureAnalyzer struct{}

func NewOSProjectStructureAnalyzer() *OSProjectStructureAnalyzer {
	return &OSProjectStructureAnalyzer{}
}

func (a *OSProjectStructureAnalyzer) AnalyzeProjectStructure(project CodeProject) (ProjectStructure, error) {
	if a == nil {
		return ProjectStructure{}, fmt.Errorf("programming: project structure analyzer unavailable")
	}
	if err := project.Validate(); err != nil {
		return ProjectStructure{}, err
	}
	info, err := os.Stat(project.Root)
	if err != nil {
		return ProjectStructure{}, fmt.Errorf("programming: inspect project root: %w", err)
	}
	if !info.IsDir() {
		return ProjectStructure{}, fmt.Errorf("programming: project root is not a directory")
	}

	result := ProjectStructure{ProjectID: project.ID, Root: project.Root, ObservedAt: time.Now().UTC()}
	counts := make(map[string]int)
	err = filepath.Walk(project.Root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == project.Root {
			return nil
		}
		rel, err := filepath.Rel(project.Root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if info.IsDir() {
			role := classifyDirectory(rel, project)
			result.Directories = append(result.Directories, ProjectDirectory{Path: rel, Role: role, Exists: true})
			return nil
		}
		counts[filepath.ToSlash(filepath.Dir(rel))]++
		lang := languageFromExtension(filepath.Ext(path))
		if lang != "" {
			result.Files = append(result.Files, SourceFile{Path: rel, Language: lang, Role: roleForPath(rel, project), ProjectID: project.ID})
		}
		return nil
	})
	if err != nil {
		return ProjectStructure{}, fmt.Errorf("programming: inspect project structure: %w", err)
	}
	for i := range result.Directories {
		result.Directories[i].FileCount = counts[result.Directories[i].Path]
	}
	sort.Slice(result.Directories, func(i, j int) bool { return result.Directories[i].Path < result.Directories[j].Path })
	sort.Slice(result.Files, func(i, j int) bool { return result.Files[i].Path < result.Files[j].Path })
	if err := result.Validate(); err != nil {
		return ProjectStructure{}, err
	}
	return result, nil
}

func classifyDirectory(rel string, project CodeProject) ProjectDirectoryRole {
	clean := filepath.ToSlash(filepath.Clean(rel))
	top := strings.Split(clean, "/")[0]
	for _, p := range project.SourceRoots {
		if normalizeTop(p) == top {
			return DirectorySource
		}
	}
	for _, p := range project.TestRoots {
		if normalizeTop(p) == top {
			return DirectoryTests
		}
	}
	switch strings.ToLower(top) {
	case "assets", "asset":
		return DirectoryAssets
	case "config", "configs", "configuration":
		return DirectoryConfig
	case "build", "bin", "dist", "out":
		return DirectoryBuild
	case "docs", "doc", "documentation":
		return DirectoryDocumentation
	case "vendor", "node_modules", "packages":
		return DirectoryDependencies
	default:
		return DirectoryUnknown
	}
}

func normalizeTop(p string) string {
	p = filepath.ToSlash(filepath.Clean(p))
	return strings.Split(p, "/")[0]
}

func roleForPath(rel string, project CodeProject) SourceFileRole {
	dir := filepath.ToSlash(filepath.Dir(rel))
	top := strings.Split(dir, "/")[0]
	for _, p := range project.TestRoots {
		if normalizeTop(p) == top {
			return RoleTest
		}
	}
	for _, p := range project.SourceRoots {
		if normalizeTop(p) == top {
			return RoleSource
		}
	}
	return RoleUnknown
}

func languageFromExtension(ext string) string {
	switch strings.ToLower(ext) {
	case ".go":
		return "go"
	case ".cs":
		return "csharp"
	case ".py":
		return "python"
	case ".js":
		return "javascript"
	case ".ts":
		return "typescript"
	case ".cpp", ".cc", ".cxx":
		return "cpp"
	case ".c":
		return "c"
	case ".rs":
		return "rust"
	case ".java":
		return "java"
	case ".h", ".hpp":
		return "c-header"
	default:
		return ""
	}
}
