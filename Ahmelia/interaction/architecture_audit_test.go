package interaction

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// B96.22.31 — Architecture Audit. Interaction is the semantic boundary and
// must not acquire dependencies on domain implementations, providers or
// execution systems. This test makes that architectural rule executable.
func TestB9631InteractionHasNoDomainImplementationImports(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	forbiddenPrefixes := []string{
		"ahmelia/capability", "ahmelia/programming", "ahmelia/production",
		"ahmelia/actions", "ahmelia/tools", "ahmelia/knowledge",
		"ahmelia/linguistics", "ahmelia/autonomy", "ahmelia/tts",
	}
	fset := token.NewFileSet()
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, imp := range f.Imports {
			p := strings.Trim(imp.Path.Value, `"`)
			for _, prefix := range forbiddenPrefixes {
				if p == prefix || strings.HasPrefix(p, prefix+"/") {
					t.Fatalf("interaction/%s imports forbidden domain/provider package %q", name, p)
				}
			}
		}
	}
}
