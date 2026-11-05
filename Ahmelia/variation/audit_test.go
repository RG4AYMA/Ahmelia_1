package variation

import (
	"bufio"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAudit_ForbiddenSceneLiterals recorre los paquetes de decisión listados en T3.1
// y falla si encuentra literales del archivo testdata/forbidden_scene_literals.txt.
func TestAudit_ForbiddenSceneLiterals(t *testing.T) {
	t.Helper()

	// Paquetes de decisión identificados en T3.1 (recon_T3.md)
	decisionPackages := []string{
		"autonomy",
		"initiative",
		"situation",
		"capability",
		"planning",
		"cognition",
		"interaction",
	}

	// Leer lista de literales prohibidos
	forbiddenPath := filepath.Join("testdata", "forbidden_scene_literals.txt")
	forbidden, err := loadForbiddenLiterals(forbiddenPath)
	if err != nil {
		t.Fatalf("load forbidden literals: %v", err)
	}

	var findings []Finding

	for _, pkg := range decisionPackages {
		pkgPath := filepath.Join("..", pkg) // relativo a variation/
		findings = append(findings, auditPackage(t, pkgPath, forbidden)...)
	}

	if len(findings) > 0 {
		t.Errorf("Auditoría anti-escena encontró %d literales prohibidos:\n", len(findings))
		for _, f := range findings {
			t.Errorf("  %s:%d: %s (literal: %q)", f.File, f.Line, f.Code, f.Literal)
		}
	}
}

// Finding representa un hallazgo de la auditoría.
type Finding struct {
	File    string
	Line    int
	Code    string
	Literal string
}

// loadForbiddenLiterals carga la lista de literales prohibidos.
func loadForbiddenLiterals(path string) ([]string, error) {
	// Try multiple locations
	paths := []string{
		path,
		filepath.Join("..", "testdata", "forbidden_scene_literals.txt"),
		filepath.Join("..", "..", "testdata", "forbidden_scene_literals.txt"),
	}

	for _, p := range paths {
		file, err := os.Open(p)
		if err == nil {
			defer file.Close()
			var literals []string
			scanner := bufio.NewScanner(file)
			for scanner.Scan() {
				line := strings.TrimSpace(scanner.Text())
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}
				literal := extractLiteral(line)
				if literal != "" {
					literals = append(literals, literal)
				} else if line != "" {
					literals = append(literals, line)
				}
			}
			return literals, scanner.Err()
		}
	}
	return nil, os.ErrNotExist
}

// extractLiteral extrae el contenido entre comillas dobles o simples.
func extractLiteral(line string) string {
	// Buscar primer par de comillas
	for _, quote := range []string{"\"", "'"} {
		start := strings.Index(line, quote)
		if start >= 0 {
			end := strings.Index(line[start+1:], quote)
			if end >= 0 {
				return line[start+1 : start+1+end]
			}
		}
	}
	return ""
}

// auditPackage audita un paquete buscando literales prohibidos en string literals.
func auditPackage(t *testing.T, pkgPath string, forbidden []string) []Finding {
	t.Helper()

	var findings []Finding

	err := filepath.Walk(pkgPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			// Ignorar errores de parseo (archivos generados, etc.)
			return nil
		}

		ast.Inspect(f, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.BasicLit:
				if x.Kind == token.STRING {
					literal := strings.Trim(x.Value, "`\"")
					for _, forbidden := range forbidden {
						if strings.Contains(strings.ToLower(literal), strings.ToLower(forbidden)) {
							pos := fset.Position(x.Pos())
							findings = append(findings, Finding{
								File:    path,
								Line:    pos.Line,
								Code:    literal,
								Literal: forbidden,
							})
						}
					}
				}
			case *ast.CompositeLit:
				// También revisar claves de mapas, etc.
			}
			return true
		})

		return nil
	})

	if err != nil {
		t.Logf("walk %s: %v", pkgPath, err)
	}
	return findings
}

// TestAudit_ForbiddenSceneLiterals_Planted verifica que la auditoría detecta un literal plantado.
func TestAudit_ForbiddenSceneLiterals_Planted(t *testing.T) {
	t.Helper()

	// Crear archivo temporal con un literal prohibido
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test_audit.go")
	content := `package test

func TestFunc() {
	_ = "buenos días" // literal prohibido
}
`
	if err := os.WriteFile(testFile, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	forbidden := []string{"buenos días"}
	findings := auditPackage(t, tmpDir, forbidden)

	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Literal != "buenos días" {
		t.Fatalf("expected literal 'buenos días', got %q", findings[0].Literal)
	}
}
