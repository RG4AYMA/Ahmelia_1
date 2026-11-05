package programming

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// B96.4 — Code Understanding
//
// Understanding is analysis only. It does not modify files, execute code, or
// make production decisions. Language-specific analyzers implement this
// contract; Go is the first concrete implementation.

type SymbolKind string

const (
	SymbolPackage   SymbolKind = "package"
	SymbolImport    SymbolKind = "import"
	SymbolType      SymbolKind = "type"
	SymbolStruct    SymbolKind = "struct"
	SymbolInterface SymbolKind = "interface"
	SymbolFunction  SymbolKind = "function"
	SymbolMethod    SymbolKind = "method"
	SymbolVariable  SymbolKind = "variable"
	SymbolConstant  SymbolKind = "constant"
)

type CodeSymbol struct {
	Name   string
	Kind   SymbolKind
	File   string
	Line   int
	Column int
	Parent string
}

type CodeImport struct {
	Path string
	Name string
	File string
}

type CodeFileAnalysis struct {
	File         SourceFile
	Package      string
	Symbols      []CodeSymbol
	Imports      []CodeImport
	Dependencies []string
}

type ProjectAnalysis struct {
	Project      CodeProject
	Files        []CodeFileAnalysis
	Symbols      []CodeSymbol
	Dependencies []string
}

type CodeAnalyzer interface {
	Language() string
	AnalyzeFile(SourceFile, string) (CodeFileAnalysis, error)
}

type CodeUnderstanding struct {
	analyzers map[string]CodeAnalyzer
}

func NewCodeUnderstanding() *CodeUnderstanding {
	return &CodeUnderstanding{analyzers: make(map[string]CodeAnalyzer)}
}

func (u *CodeUnderstanding) RegisterAnalyzer(analyzer CodeAnalyzer) error {
	if u == nil {
		return fmt.Errorf("programming: understanding nil")
	}
	if analyzer == nil || strings.TrimSpace(analyzer.Language()) == "" {
		return fmt.Errorf("programming: analyzer inválido")
	}
	key := strings.ToLower(strings.TrimSpace(analyzer.Language()))
	if _, exists := u.analyzers[key]; exists {
		return fmt.Errorf("programming: analyzer ya registrado: %s", key)
	}
	u.analyzers[key] = analyzer
	return nil
}

func (u *CodeUnderstanding) Analyzer(language string) (CodeAnalyzer, bool) {
	if u == nil {
		return nil, false
	}
	a, ok := u.analyzers[strings.ToLower(strings.TrimSpace(language))]
	return a, ok
}

func (u *CodeUnderstanding) AnalyzeFile(file SourceFile, content string) (CodeFileAnalysis, error) {
	if err := file.Validate(); err != nil {
		return CodeFileAnalysis{}, err
	}
	analyzer, ok := u.Analyzer(file.Language)
	if !ok {
		return CodeFileAnalysis{}, fmt.Errorf("programming: no hay analyzer para lenguaje %q", file.Language)
	}
	return analyzer.AnalyzeFile(file, content)
}

func (u *CodeUnderstanding) AnalyzeProject(project CodeProject, files []SourceFile, contents map[string]string) (ProjectAnalysis, error) {
	if err := project.Validate(); err != nil {
		return ProjectAnalysis{}, err
	}
	result := ProjectAnalysis{Project: project}
	seenDeps := map[string]struct{}{}
	for _, file := range files {
		if file.ProjectID != project.ID {
			return ProjectAnalysis{}, fmt.Errorf("programming: file %q pertenece a otro proyecto", file.Path)
		}
		content, ok := contents[file.Path]
		if !ok {
			return ProjectAnalysis{}, fmt.Errorf("programming: contenido ausente para %q", file.Path)
		}
		analysis, err := u.AnalyzeFile(file, content)
		if err != nil {
			return ProjectAnalysis{}, err
		}
		result.Files = append(result.Files, analysis)
		result.Symbols = append(result.Symbols, analysis.Symbols...)
		for _, dep := range analysis.Dependencies {
			seenDeps[dep] = struct{}{}
		}
	}
	for dep := range seenDeps {
		result.Dependencies = append(result.Dependencies, dep)
	}
	sort.Strings(result.Dependencies)
	sort.Slice(result.Files, func(i, j int) bool { return result.Files[i].File.Path < result.Files[j].File.Path })
	return result, nil
}

// GoAnalyzer extracts structural information using the standard library AST.
type GoAnalyzer struct{}

func (GoAnalyzer) Language() string { return "go" }

func (GoAnalyzer) AnalyzeFile(file SourceFile, content string) (CodeFileAnalysis, error) {
	if err := file.Validate(); err != nil {
		return CodeFileAnalysis{}, err
	}
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, file.Path, content, parser.ParseComments)
	if err != nil {
		return CodeFileAnalysis{}, fmt.Errorf("programming: parse go %s: %w", file.Path, err)
	}
	result := CodeFileAnalysis{File: file, Package: node.Name.Name}
	result.Symbols = append(result.Symbols, CodeSymbol{Name: node.Name.Name, Kind: SymbolPackage, File: file.Path, Line: fset.Position(node.Name.Pos()).Line, Column: fset.Position(node.Name.Pos()).Column})
	for _, spec := range node.Imports {
		path := strings.Trim(spec.Path.Value, "\"")
		name := ""
		if spec.Name != nil {
			name = spec.Name.Name
		}
		result.Imports = append(result.Imports, CodeImport{Path: path, Name: name, File: file.Path})
		result.Dependencies = append(result.Dependencies, path)
		p := fset.Position(spec.Pos())
		result.Symbols = append(result.Symbols, CodeSymbol{Name: path, Kind: SymbolImport, File: file.Path, Line: p.Line, Column: p.Column})
	}
	for _, decl := range node.Decls {
		switch d := decl.(type) {
		case *ast.GenDecl:
			kind := SymbolVariable
			if d.Tok.String() == "const" {
				kind = SymbolConstant
			}
			if d.Tok.String() == "type" {
				for _, spec := range d.Specs {
					ts := spec.(*ast.TypeSpec)
					k := SymbolType
					switch ts.Type.(type) {
					case *ast.StructType:
						k = SymbolStruct
					case *ast.InterfaceType:
						k = SymbolInterface
					}
					p := fset.Position(ts.Pos())
					result.Symbols = append(result.Symbols, CodeSymbol{Name: ts.Name.Name, Kind: k, File: file.Path, Line: p.Line, Column: p.Column})
				}
				continue
			}
			for _, spec := range d.Specs {
				if vs, ok := spec.(*ast.ValueSpec); ok {
					for _, n := range vs.Names {
						p := fset.Position(n.Pos())
						result.Symbols = append(result.Symbols, CodeSymbol{Name: n.Name, Kind: kind, File: file.Path, Line: p.Line, Column: p.Column})
					}
				}
			}
		case *ast.FuncDecl:
			kind := SymbolFunction
			parent := ""
			if d.Recv != nil && len(d.Recv.List) > 0 {
				kind = SymbolMethod
				parent = receiverName(d.Recv.List[0].Type)
			}
			p := fset.Position(d.Pos())
			result.Symbols = append(result.Symbols, CodeSymbol{Name: d.Name.Name, Kind: kind, File: file.Path, Line: p.Line, Column: p.Column, Parent: parent})
		}
	}
	return result, nil
}

func receiverName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return receiverName(t.X)
	case *ast.IndexExpr:
		return receiverName(t.X)
	case *ast.IndexListExpr:
		return receiverName(t.X)
	}
	return ""
}

// AnalyzeProjectFromWorkspace reads files only; it never writes or executes.
func (u *CodeUnderstanding) AnalyzeProjectFromWorkspace(project CodeProject, workspace *ProgrammingWorkspace) (ProjectAnalysis, error) {
	if workspace == nil {
		return ProjectAnalysis{}, fmt.Errorf("programming: workspace nil")
	}
	if workspace.Project.ID != project.ID {
		return ProjectAnalysis{}, fmt.Errorf("programming: workspace/project mismatch")
	}
	var files []SourceFile
	contents := map[string]string{}
	err := filepath.Walk(project.Root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(project.Root, path)
		if err != nil {
			return err
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sf := SourceFile{Path: filepath.ToSlash(rel), Language: "go", Role: RoleSource, ProjectID: project.ID}
		files = append(files, sf)
		contents[sf.Path] = string(data)
		return nil
	})
	if err != nil {
		return ProjectAnalysis{}, fmt.Errorf("programming: analizar workspace: %w", err)
	}
	return u.AnalyzeProject(project, files, contents)
}
