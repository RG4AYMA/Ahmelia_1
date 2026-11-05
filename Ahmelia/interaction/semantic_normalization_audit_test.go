package interaction

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
	"testing"
	"unicode"
)

func TestSemanticClassifiersDoNotCarryAccentVariants(t *testing.T) {
	path := "semantic_request.go"
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	f, err := parser.ParseFile(token.NewFileSet(), path, src, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		name := fn.Name.Name
		if !(strings.HasPrefix(name, "is") || strings.HasPrefix(name, "has") || strings.HasPrefix(name, "infer") || name == "requestedActionFor" || name == "expectedResponseFor" || name == "extractEntities" || name == "detectUrgency") {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			value, err := strconv.Unquote(lit.Value)
			if err != nil {
				return true
			}
			for _, r := range value {
				if unicode.Is(unicode.Mn, r) || strings.ContainsRune("áéíóúÁÉÍÓÚüÜ", r) {
					t.Errorf("classifier %s contains accent-bearing literal %q; classifiers must consume shared normalized surfaces", name, value)
					break
				}
			}
			return true
		})
	}
}
