package memory_intelligence

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNoPhraseGateMemoryIntelligenceParsersTyped(t *testing.T) {
	fset := token.NewFileSet()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Clean(entry.Name())
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			fn, ok := n.(*ast.FuncDecl)
			if !ok || !strings.HasPrefix(fn.Name.Name, "Parse") || !strings.HasSuffix(fn.Name.Name, "Query") || fn.Type.Params == nil || len(fn.Type.Params.List) == 0 {
				return true
			}
			field := fn.Type.Params.List[0]
			got := ""
			switch typ := field.Type.(type) {
			case *ast.Ident:
				got = typ.Name
			case *ast.SelectorExpr:
				if pkg, ok := typ.X.(*ast.Ident); ok {
					got = pkg.Name + "." + typ.Sel.Name
				}
			}
			if got != "interaction.SemanticRequest" {
				t.Errorf("%s: parser still accepts %s instead of interaction.SemanticRequest", fn.Name.Name, got)
			}
			return true
		})
	}
}
