package programming

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"go/parser"
	"go/token"
)

// GoAcceptanceContract defines behavioral checks owned by the harness, not by
// the generated project's own tests. The contract is evaluated on a throwaway
// copy of the project so the model cannot weaken or delete the checks.
type GoAcceptanceContract struct {
	ID                          string
	Description                 string
	AcceptanceTestName          string
	AcceptanceTestSource        string
	ForbidExternalModules       bool
	ForbidTestFunctionsInSource bool
	RunVet                      bool
}

// AcceptanceCheck is one observable result of the contract.
type AcceptanceCheck struct {
	Name    string
	Success bool
	Detail  string
}

// AcceptanceResult is the evidence produced by evaluating a contract.
type AcceptanceResult struct {
	ContractID  string
	Description string
	Success     bool
	Checks      []AcceptanceCheck
}

var goTestFunctionPattern = regexp.MustCompile(`(?m)^\s*func\s+Test[A-Z_]`)
var goRequirePattern = regexp.MustCompile(`(?m)^\s*require\b`)

// NewGoSumadorAcceptance is the first task contract: a small Go project must
// provide Add(a, b int) int, pass its own tests, and satisfy an independent
// behavioral test that the model never sees.
func NewGoSumadorAcceptance() GoAcceptanceContract {
	return GoAcceptanceContract{
		ID:                 "go-sumador-add",
		Description:        "Add(a, b int) int must return 5 for Add(2, 3); the project must build, vet, and keep tests separate from sources.",
		AcceptanceTestName: "TestAhmeliaAcceptanceAdd",
		AcceptanceTestSource: `package {{PACKAGE}}

import "testing"

func TestAhmeliaAcceptanceAdd(t *testing.T) {
	if got := Add(2, 3); got != 5 {
		t.Fatalf("Add(2, 3) = %d; want 5", got)
	}
}
`,
		ForbidExternalModules:       true,
		ForbidTestFunctionsInSource: true,
		RunVet:                      true,
	}
}

// Verify evaluates the contract against root. It copies the project to a
// temporary directory, injects the harness-owned acceptance test there, and
// never mutates the original project.
func (c GoAcceptanceContract) Verify(ctx context.Context, root string) (AcceptanceResult, error) {
	if strings.TrimSpace(c.ID) == "" {
		return AcceptanceResult{}, fmt.Errorf("acceptance: contract ID is empty")
	}
	if strings.TrimSpace(c.AcceptanceTestName) == "" || strings.TrimSpace(c.AcceptanceTestSource) == "" {
		return AcceptanceResult{}, fmt.Errorf("acceptance: acceptance test is not defined")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return AcceptanceResult{}, fmt.Errorf("acceptance: resolve project root: %w", err)
	}
	info, err := os.Stat(absRoot)
	if err != nil {
		return AcceptanceResult{}, fmt.Errorf("acceptance: project root: %w", err)
	}
	if !info.IsDir() {
		return AcceptanceResult{}, fmt.Errorf("acceptance: project root is not a directory")
	}

	result := AcceptanceResult{ContractID: c.ID, Description: c.Description, Success: true}
	add := func(name string, ok bool, detail string) {
		result.Checks = append(result.Checks, AcceptanceCheck{Name: name, Success: ok, Detail: truncateAcceptanceDetail(detail)})
		if !ok {
			result.Success = false
		}
	}

	goModPath := filepath.Join(absRoot, "go.mod")
	if _, err := os.Stat(goModPath); err != nil {
		add("go_mod", false, "go.mod is missing")
	} else {
		add("go_mod", true, "go.mod present")
	}

	sourceFiles, testFiles, err := rootGoFiles(absRoot)
	if err != nil {
		return AcceptanceResult{}, err
	}
	add("source_file", len(sourceFiles) > 0, fmt.Sprintf("%d source files at project root", len(sourceFiles)))
	add("test_file", len(testFiles) > 0, fmt.Sprintf("%d test files at project root", len(testFiles)))

	if c.ForbidTestFunctionsInSource {
		leaks, err := testFunctionsInSources(absRoot, sourceFiles)
		if err != nil {
			return AcceptanceResult{}, err
		}
		if len(leaks) == 0 {
			add("source_test_separation", true, "no Test functions in source files")
		} else {
			add("source_test_separation", false, "Test functions found in source files: "+strings.Join(leaks, ", "))
		}
	}

	if c.ForbidExternalModules {
		content, err := os.ReadFile(goModPath)
		if err != nil {
			add("external_dependencies", false, "go.mod is not readable")
			return result, nil
		}
		if goRequirePattern.Match(content) {
			add("external_dependencies", false, "go.mod declares external modules")
			return result, nil
		}
		add("external_dependencies", true, "go.mod has no require directives")
	}

	pkg, pkgErr := detectRootPackage(absRoot, sourceFiles)
	if pkgErr != nil {
		add("root_package", false, pkgErr.Error())
		return result, nil
	}
	add("root_package", true, "package "+pkg)

	copyRoot, err := copyAcceptanceProject(absRoot)
	if err != nil {
		return AcceptanceResult{}, fmt.Errorf("acceptance: copy project: %w", err)
	}
	defer os.RemoveAll(copyRoot)

	buildOutput, buildErr := runAcceptanceCommand(ctx, copyRoot, "go", "build", "./...")
	add("build", buildErr == nil, commandDetail(buildOutput, buildErr))

	if c.RunVet {
		vetOutput, vetErr := runAcceptanceCommand(ctx, copyRoot, "go", "vet", "./...")
		add("vet", vetErr == nil, commandDetail(vetOutput, vetErr))
	}

	projectTestOutput, projectTestErr := runAcceptanceCommand(ctx, copyRoot, "go", "test", "-count=1", "./...")
	add("project_tests", projectTestErr == nil, commandDetail(projectTestOutput, projectTestErr))

	acceptanceSource := strings.ReplaceAll(c.AcceptanceTestSource, "{{PACKAGE}}", pkg)
	acceptancePath := filepath.Join(copyRoot, "zz_ahmelia_acceptance_test.go")
	if err := os.WriteFile(acceptancePath, []byte(acceptanceSource), 0o600); err != nil {
		return AcceptanceResult{}, fmt.Errorf("acceptance: write acceptance test: %w", err)
	}
	add("acceptance_test_written", true, "harness-owned acceptance test injected into a throwaway copy")

	acceptanceOutput, acceptanceErr := runAcceptanceCommand(ctx, copyRoot, "go", "test", "-run", "^"+regexp.QuoteMeta(c.AcceptanceTestName)+"$", "-count=1", "./...")
	add("acceptance_test", acceptanceErr == nil, commandDetail(acceptanceOutput, acceptanceErr))

	return result, nil
}

func rootGoFiles(root string) (sourceFiles, testFiles []string, err error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, nil, fmt.Errorf("acceptance: read project root: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".go") {
			continue
		}
		if strings.HasSuffix(strings.ToLower(entry.Name()), "_test.go") {
			testFiles = append(testFiles, entry.Name())
		} else {
			sourceFiles = append(sourceFiles, entry.Name())
		}
	}
	return sourceFiles, testFiles, nil
}

func testFunctionsInSources(root string, sourceFiles []string) ([]string, error) {
	var leaks []string
	for _, name := range sourceFiles {
		content, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			return nil, fmt.Errorf("acceptance: read %s: %w", name, err)
		}
		if goTestFunctionPattern.Match(content) {
			leaks = append(leaks, name)
		}
	}
	return leaks, nil
}

func detectRootPackage(root string, sourceFiles []string) (string, error) {
	pkg := ""
	fset := token.NewFileSet()
	for _, name := range sourceFiles {
		file, err := parser.ParseFile(fset, filepath.Join(root, name), nil, parser.PackageClauseOnly)
		if err != nil {
			return "", fmt.Errorf("acceptance: parse package in %s: %w", name, err)
		}
		pkgName := file.Name.Name
		if pkg == "" {
			pkg = pkgName
			continue
		}
		if pkg != pkgName {
			return "", fmt.Errorf("acceptance: multiple root packages %q and %q", pkg, pkgName)
		}
	}
	if pkg == "" {
		return "", fmt.Errorf("acceptance: no source package at project root")
	}
	return pkg, nil
}

func copyAcceptanceProject(src string) (string, error) {
	dst, err := os.MkdirTemp("", "ahmelia-acceptance-")
	if err != nil {
		return "", err
	}
	cleanup := func(err error) (string, error) {
		_ = os.RemoveAll(dst)
		return "", err
	}
	err = filepath.WalkDir(src, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "memory", "workspaces", ".gotmp-validation", "node_modules", "vendor":
				return filepath.SkipDir
			}
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dst, rel), data, 0o600)
	})
	if err != nil {
		return cleanup(fmt.Errorf("acceptance: copy project tree: %w", err))
	}
	return dst, nil
}

func runAcceptanceCommand(ctx context.Context, dir, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func commandDetail(output string, err error) string {
	text := strings.TrimSpace(output)
	if err != nil {
		if text == "" {
			return err.Error()
		}
		return err.Error() + ": " + text
	}
	if text == "" {
		return "ok"
	}
	return text
}

func truncateAcceptanceDetail(value string) string {
	const limit = 4000
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "…"
}
