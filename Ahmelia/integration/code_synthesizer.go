package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"ahmelia/programming"
)

// StructuredCodeObjectiveSynthesizer is a bounded synthesis capability. Its
// injected model adapter supplies proposed code only; the surrounding
// production architecture owns filesystem mutation, authorization,
// execution, recovery and verification.
type StructuredCodeGenerator interface {
	GenerateStructuredContext(context.Context, string, json.RawMessage, int) (string, error)
}

type StructuredCodeStreamGenerator interface {
	GenerateStructuredStreamContext(context.Context, string, json.RawMessage, int, func(string)) (string, error)
}

type StructuredCodeObjectiveSynthesizer struct {
	Client StructuredCodeGenerator
}

// GenerationSource reports who proposed the code so downstream experience can
// distinguish Ahmelia's production/verification work from external synthesis.
func (s *StructuredCodeObjectiveSynthesizer) GenerationSource() string {
	if s == nil || s.Client == nil {
		return ""
	}
	if source, ok := s.Client.(interface{ GenerationSource() string }); ok {
		return strings.TrimSpace(source.GenerationSource())
	}
	return "generator:identity_unreported"
}

// OllamaCodeObjectiveSynthesizer remains as a source-compatible name for
// existing callers. Runtime wiring injects OpenCode instead.
type OllamaCodeObjectiveSynthesizer = StructuredCodeObjectiveSynthesizer

func NewOllamaCodeObjectiveSynthesizerWithClient(client StructuredCodeGenerator) *StructuredCodeObjectiveSynthesizer {
	return NewStructuredCodeObjectiveSynthesizerWithClient(client)
}

func NewStructuredCodeObjectiveSynthesizerWithClient(client StructuredCodeGenerator) *StructuredCodeObjectiveSynthesizer {
	return &StructuredCodeObjectiveSynthesizer{Client: client}
}

type synthesizedFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	Role    string `json:"role"`
}

type synthesizedProject struct {
	Files []synthesizedFile `json:"files"`
}

func (s *StructuredCodeObjectiveSynthesizer) Synthesize(ctx context.Context, objective programming.CodeObjective) (*programming.MultiFilePlan, error) {
	if s == nil || s.Client == nil {
		return nil, fmt.Errorf("programming synthesis: code generation provider unavailable")
	}
	if err := objective.Validate(); err != nil {
		return nil, err
	}

	prompt := buildCodeSynthesisPrompt(objective)
	var raw string
	var err error
	if streamer, ok := s.Client.(StructuredCodeStreamGenerator); ok {
		raw, err = streamer.GenerateStructuredStreamContext(ctx, prompt, codeSynthesisSchema(), 2048, nil)
	} else {
		raw, err = s.Client.GenerateStructuredContext(ctx, prompt, codeSynthesisSchema(), 2048)
	}
	if err != nil {
		return nil, fmt.Errorf("programming synthesis: %w", err)
	}

	var response synthesizedProject
	if err := json.Unmarshal([]byte(stripJSONCodeFence(raw)), &response); err != nil {
		return nil, fmt.Errorf("programming synthesis: invalid structured response: %w", err)
	}
	if len(response.Files) == 0 {
		return nil, fmt.Errorf("programming synthesis: model returned no files")
	}

	project := programming.CodeProject{
		ID: objective.ProjectID, Name: objective.ProjectID, Root: objective.ProjectRoot,
		Language: objective.Language, SourceRoots: []string{"."}, TestRoots: []string{"."},
		BuildCommand: "go build ./...", TestCommand: "go test ./...", RuntimeCommand: "go run .",
	}
	if strings.EqualFold(objective.Language, "Python") {
		project.BuildCommand, project.TestCommand, project.RuntimeCommand = "python -m py_compile ./*.py", "python -m unittest", "python main.py"
	} else if strings.EqualFold(objective.Language, "JavaScript") {
		project.BuildCommand, project.TestCommand, project.RuntimeCommand = "node --check main.js", "node --test", "node main.js"
	}

	plan := &programming.MultiFilePlan{ID: "synthesis-" + objective.ProjectID, Project: project}
	seen := map[string]bool{}
	for i, f := range response.Files {
		path := filepath.ToSlash(strings.TrimSpace(f.Path))
		path = normalizeSynthesisPath(path, objective.ProjectID)
		if err := validateSynthesisPath(path); err != nil {
			return nil, err
		}
		if seen[path] {
			return nil, fmt.Errorf("programming synthesis: duplicate file %q", path)
		}
		seen[path] = true
		if strings.TrimSpace(f.Content) == "" {
			return nil, fmt.Errorf("programming synthesis: empty content for %q", path)
		}
		role := synthesisRole(path, f.Role)
		op := programming.SourceCreate
		if objectiveFileExists(objective.Files, path) || objectiveFileExistsOnDisk(objective.ProjectRoot, path) {
			op = programming.SourceWrite
		}
		if op == programming.SourceWrite && protectedExistingProjectFile(path) && !objectiveExplicitlyAllowsFileChange(objective.Raw, path) {
			if existing, ok := objectiveFileContent(objective.Files, path); ok && existing == f.Content {
				// A retry may echo an unchanged manifest while repairing source files.
				// Treat the identical protected file as a no-op, never as permission to mutate it.
				continue
			}
			if objectiveHasRecoveryEvidence(objective) {
				// Recovery is authorized to repair the failed artifact, not to mutate
				// project identity/dependency manifests implicitly. A model may propose
				// a manifest change while diagnosing a source failure; ignore that
				// change and keep processing safe source/test repairs from the same plan.
				continue
			}
			return nil, fmt.Errorf("programming synthesis: refused to overwrite protected existing file %q without explicit objective scope", path)
		}
		if op == programming.SourceWrite && objectiveIsCreation(objective.Raw) && !objectiveHasRecoveryEvidence(objective) && !objectiveExplicitlyAllowsFileChange(objective.Raw, path) {
			return nil, fmt.Errorf("programming synthesis: creation objective cannot overwrite unrelated existing file %q", path)
		}
		plan.Changes = append(plan.Changes, programming.MultiFileChange{
			ID: fmt.Sprintf("synth-%03d", i+1), Operation: op,
			File:    programming.SourceFile{Path: path, Language: objective.Language, Role: role, ProjectID: objective.ProjectID},
			Content: f.Content,
		})
	}

	if strings.EqualFold(objective.Language, "Go") {
		// A new Go project's module manifest is deterministic project metadata,
		// not source-code reasoning. Repair its omission at this boundary rather
		// than failing production and asking the model to regenerate everything.
		// Existing projects remain strict: never invent/replace their module identity.
		if !seen["go.mod"] && !objectiveFileExists(objective.Files, "go.mod") && !objectiveFileExistsOnDisk(objective.ProjectRoot, "go.mod") {
			if objective.Existing {
				return nil, fmt.Errorf("programming synthesis: existing Go project has no go.mod")
			}
			module := canonicalGoModuleName(objective.ProjectID)
			plan.Changes = append(plan.Changes, programming.MultiFileChange{
				ID: "synth-go-mod", Operation: programming.SourceCreate,
				File:    programming.SourceFile{Path: "go.mod", Language: objective.Language, Role: programming.RoleConfig, ProjectID: objective.ProjectID},
				Content: "module " + module + "\n\ngo 1.20\n",
			})
			seen["go.mod"] = true
		}
		hasTest := false
		for p := range seen {
			if strings.HasSuffix(strings.ToLower(p), "_test.go") {
				hasTest = true
				break
			}
		}
		if !hasTest {
			for _, f := range objective.Files {
				if strings.HasSuffix(strings.ToLower(filepath.ToSlash(f.Path)), "_test.go") {
					hasTest = true
					break
				}
			}
		}
		if !hasTest && objectiveFileExistsOnDiskBySuffix(objective.ProjectRoot, "_test.go") {
			hasTest = true
		}
		if !hasTest {
			return nil, fmt.Errorf("programming synthesis: Go project must include or already contain at least one _test.go")
		}
	}
	if err := plan.Validate(); err != nil {
		return nil, err
	}
	return plan, nil
}

func stripJSONCodeFence(raw string) string {
	value := strings.TrimSpace(raw)
	if !strings.HasPrefix(value, "```") {
		return value
	}
	firstNewline := strings.IndexByte(value, '\n')
	if firstNewline < 0 || !strings.HasSuffix(value, "```") {
		return value
	}
	return strings.TrimSpace(strings.TrimSuffix(value[firstNewline+1:], "```"))
}

func canonicalGoModuleName(projectID string) string {
	var segment strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(projectID)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' || r == '.' {
			segment.WriteRune(r)
		} else {
			segment.WriteByte('-')
		}
	}
	value := strings.Trim(segment.String(), "-_.")
	if value == "" {
		value = "generated"
	}
	return "ahmelia.local/project-" + value
}

func normalizeSynthesisPath(path, projectID string) string {
	path = filepath.ToSlash(strings.TrimSpace(path))
	projectID = filepath.ToSlash(strings.Trim(strings.TrimSpace(projectID), "/"))
	if projectID == "" || path == projectID {
		return path
	}
	prefix := projectID + "/"
	if strings.HasPrefix(path, prefix) {
		return strings.TrimPrefix(path, prefix)
	}
	return path
}

func objectiveFileContent(files []programming.CodeObjectiveFile, path string) (string, bool) {
	for _, f := range files {
		if filepath.ToSlash(f.Path) == path {
			return f.Content, true
		}
	}
	return "", false
}

func objectiveHasRecoveryEvidence(objective programming.CodeObjective) bool {
	return objective.RecoveryContext != nil || strings.TrimSpace(objective.RecoveryEvidence) != "" || strings.Contains(strings.ToLower(objective.Raw), "[recovery evidence]")
}

func objectiveFileExists(files []programming.CodeObjectiveFile, path string) bool {
	for _, f := range files {
		if filepath.ToSlash(f.Path) == path {
			return true
		}
	}
	return false
}

func objectiveFileExistsOnDisk(root, path string) bool {
	if strings.TrimSpace(root) == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(root, filepath.FromSlash(path)))
	return err == nil
}

func objectiveFileExistsOnDiskBySuffix(root, suffix string) bool {
	if strings.TrimSpace(root) == "" {
		return false
	}
	found := false
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		if strings.HasSuffix(strings.ToLower(info.Name()), strings.ToLower(suffix)) {
			found = true
			return filepath.SkipDir
		}
		return nil
	})
	return found
}

func objectiveIsCreation(raw string) bool {
	if strings.Contains(strings.ToLower(raw), "[recovery evidence]") {
		return false
	}
	lower := strings.ToLower(raw)
	return strings.Contains(lower, "crea ") || strings.Contains(lower, "crear ") || strings.Contains(lower, "implementa ") || strings.Contains(lower, "implementar ") || strings.Contains(lower, "genera ") || strings.Contains(lower, "generar ") || strings.Contains(lower, "create ") || strings.Contains(lower, "implement ") || strings.Contains(lower, "generate ")
}

func protectedExistingProjectFile(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	switch base {
	case "go.mod", "go.sum", "package.json", "package-lock.json", "pnpm-lock.yaml", "yarn.lock", "pyproject.toml", "cargo.toml", "cargo.lock":
		return true
	default:
		return false
	}
}

func objectiveExplicitlyAllowsFileChange(raw, path string) bool {
	lower := strings.ToLower(raw)
	base := strings.ToLower(filepath.Base(path))
	return strings.Contains(lower, base) || strings.Contains(lower, "dependencia") || strings.Contains(lower, "dependencies") || strings.Contains(lower, "módulo") || strings.Contains(lower, "module") || strings.Contains(lower, "build")
}

func validateSynthesisPath(path string) error {
	if path == "" || path == "." || filepath.IsAbs(path) || path == ".." || strings.HasPrefix(path, "../") || strings.Contains(path, "/../") {
		return fmt.Errorf("programming synthesis: unsafe relative path %q", path)
	}
	return nil
}

func synthesisRole(path, declared string) programming.SourceFileRole {
	switch strings.ToLower(strings.TrimSpace(declared)) {
	case "test":
		return programming.RoleTest
	case "config":
		return programming.RoleConfig
	case "entrypoint":
		return programming.RoleEntrypoint
	case "source":
		return programming.RoleSource
	}
	if strings.HasSuffix(strings.ToLower(path), "_test.go") {
		return programming.RoleTest
	}
	if filepath.Base(path) == "go.mod" || filepath.Ext(path) == ".json" {
		return programming.RoleConfig
	}
	if filepath.Base(path) == "main.go" || filepath.Base(path) == "main.py" || filepath.Base(path) == "main.js" {
		return programming.RoleEntrypoint
	}
	return programming.RoleSource
}

func codeSynthesisSchema() []byte {
	return []byte(`{"type":"object","properties":{"files":{"type":"array","items":{"type":"object","properties":{"path":{"type":"string"},"content":{"type":"string"},"role":{"type":"string","enum":["source","test","config","entrypoint"]}},"required":["path","content","role"]}}},"required":["files"]}`)
}

func buildCodeSynthesisPrompt(o programming.CodeObjective) string {
	var b strings.Builder
	b.WriteString("You are a bounded code synthesis tool inside an autonomous software production system.\n")
	b.WriteString("Return ONLY the requested JSON object. Do not use Markdown.\n")
	b.WriteString("The system will apply the returned files, build them, run tests, and verify them.\n")
	b.WriteString("You must produce a complete coherent project, not a demonstration scaffold.\n")
	b.WriteString("Every test must exercise the actual production code, not duplicate the implementation inside the test.\n")
	b.WriteString("For Go, always include go.mod, compilable source, and at least one _test.go that calls production functions.\n")
	b.WriteString("For a single-directory Go project, source and test files must use the same package declaration (usually package main) and tests must call production functions directly. Never import the project's own module from its root test file; self-imports such as the project ID or module path are invalid for package main and can create import cycles. Do not define duplicate function names or create a custom TestMain unless the test runner genuinely requires one.\n")
	b.WriteString("All file paths must be relative to PROJECT_ROOT. Do not prefix paths with PROJECT_ID or create a nested directory named PROJECT_ID unless explicitly requested.\n")
	b.WriteString("For an existing project, preserve its identity and modify the existing files needed to satisfy the new objective; do not create a replacement project.\n")
	b.WriteString("When recovery evidence is present, treat the structured recovery context as a concrete failed verification contract, not as optional context. Preserve its causal relationships; do not reinterpret it as generic prose. Identify every reported diagnostic and repair its actual cause in the current project. The prior attempt is part of the same work: inspect its exact effect specification and every available effect evaluation before proposing the next change. Do not reason as if this were a fresh objective.\n")
	b.WriteString("For compiler/build failures, prefer the smallest source-level correction that resolves the reported diagnostic; do not repeat the same defect, regenerate a placeholder implementation, or change unrelated files.\n")
	b.WriteString("For Go, an 'imported and not used' diagnostic means remove that import if unnecessary, or use it meaningfully if the objective requires it; do not treat it as a missing module or add dependencies to hide it.\n")
	b.WriteString("Do not introduce external dependencies when the requested behavior can be implemented with the language standard library. Every import must be used, and production code must implement the requested behavior rather than return a placeholder success.\n")
	b.WriteString("Preserve protected manifests; if a manifest is unchanged, omit it. If existing files are supplied, treat them as the current artifact state and return complete replacement contents only for files that must change.\n\n")
	fmt.Fprintf(&b, "OBJECTIVE: %s\nLANGUAGE: %s\nPROJECT_ID: %s\nEXISTING_PROJECT: %t\n", o.Raw, o.Language, o.ProjectID, o.Existing)
	if o.RecoveryContext != nil {
		if payload, err := json.Marshal(o.RecoveryContext); err == nil {
			fmt.Fprintf(&b, "[Recovery context] %s\n", string(payload))
		}
	} else if strings.TrimSpace(o.RecoveryEvidence) != "" {
		fmt.Fprintf(&b, "[Recovery evidence] Previous attempt failed: %s. Repair the current project, preserve its identity, and do not alter protected manifests unless explicitly authorized.\n", o.RecoveryEvidence)
	}
	if len(o.Files) > 0 {
		b.WriteString("CURRENT PROJECT FILES:\n")
		for _, f := range o.Files {
			fmt.Fprintf(&b, "--- %s ---\n%s\n", f.Path, f.Content)
		}
	}
	b.WriteString("\nReturn files as {\"files\":[{\"path\":\"...\",\"content\":\"...\",\"role\":\"source|test|config|entrypoint\"}]}.")
	return b.String()
}
