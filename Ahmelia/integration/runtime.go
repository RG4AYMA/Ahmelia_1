package integration

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"ahmelia/capability"
	"ahmelia/environment"
	"ahmelia/interaction"
	"ahmelia/production"
	"ahmelia/programming"
	"ahmelia/tools"
)

// RegisterRuntime wires domain providers into the generic runtime boundary.
// The caller supplies registries; this package owns domain-specific composition.
// Adding a future domain means adding a provider here, not another branch in
// core.Engine.
func NewProgrammingIntegration(world *environment.Manager) (*programming.ProgrammingIntegration, error) {
	if world == nil {
		return nil, fmt.Errorf("integration: programming world nil")
	}
	source, err := production.NewEnvironmentWorkspaceSource(world)
	if err != nil {
		return nil, err
	}
	ws, err := production.NewWorkspace("programming-runtime", source)
	if err != nil {
		return nil, err
	}
	project := programming.CodeProject{
		ID: "runtime", Name: "runtime", Root: ".", Language: "Go",
		SourceRoots: []string{"."}, TestRoots: []string{"."},
		BuildCommand: "go build ./...", TestCommand: "go test ./...", RuntimeCommand: "go run .",
	}
	pw, err := programming.NewProgrammingWorkspace(ws, project)
	if err != nil {
		return nil, err
	}
	ops, err := programming.NewOSSourceFileOperations(pw)
	if err != nil {
		return nil, err
	}
	editor, err := programming.NewMultiFileEditor(ops)
	if err != nil {
		return nil, err
	}
	loop, err := programming.NewSoftwareProductionLoop(editor, programming.NewOSBuildExecutor(), programming.NewOSTestExecutor(), programming.NewCodeVerifier())
	if err != nil {
		return nil, err
	}
	return programming.NewProgrammingIntegration(loop)
}

func RegisterCapabilities(caps *capability.Registry) error {
	if caps == nil {
		return fmt.Errorf("integration: capability registry nil")
	}
	for _, c := range capability.BuiltinProgrammingCapabilities() {
		if caps.Has(c.ID) {
			continue
		}
		if err := caps.Register(c); err != nil {
			return err
		}
	}
	for _, c := range capability.BuiltinCapabilities() {
		if caps.Has(c.ID) {
			continue
		}
		if err := caps.Register(c); err != nil {
			return err
		}
	}
	return nil
}

func RegisterRuntimeProviders(caps *capability.Registry, providers *production.ProviderRegistry, world *environment.Manager) error {
	return RegisterRuntimeProvidersWithSynthesizer(caps, providers, world, nil)
}

// RegisterRuntimeProvidersWithSynthesizer wires the runtime programming
// provider to an injected code generator. The Engine owns which model service
// handles programming; linguistic interpretation remains separately wired.
func RegisterRuntimeProvidersWithSynthesizer(caps *capability.Registry, providers *production.ProviderRegistry, world *environment.Manager, synthesizer programming.CodeObjectiveSynthesizer) error {
	if caps == nil || providers == nil {
		return fmt.Errorf("integration: registries no configurados")
	}
	if world == nil {
		return fmt.Errorf("integration: environment nil")
	}
	if err := RegisterCapabilities(caps); err != nil {
		return err
	}
	var provider *ProgrammingProvider
	var err error
	if synthesizer == nil {
		return fmt.Errorf("integration: programming synthesizer is required; a code generation provider must be injected by the Engine")
	}
	provider, err = NewProgrammingProviderWithSynthesizer(world, synthesizer)
	if err != nil {
		return err
	}
	return providers.RegisterCapabilities(provider,
		"programming.read_source",
		"programming.create_file",
		"programming.modify_code",
		"programming.build",
		"programming.run_tests",
		"programming.debug",
	)
}

type ProgrammingProvider struct {
	world       *environment.Manager
	synthesizer programming.CodeObjectiveSynthesizer
}

func NewProgrammingProviderWithSynthesizer(world *environment.Manager, synthesizer programming.CodeObjectiveSynthesizer) (*ProgrammingProvider, error) {
	if world == nil {
		return nil, fmt.Errorf("integration: programming provider world nil")
	}
	if synthesizer == nil {
		return nil, fmt.Errorf("integration: programming synthesizer nil")
	}
	return &ProgrammingProvider{world: world, synthesizer: synthesizer}, nil
}

func (p *ProgrammingProvider) ID() string { return "programming" }

func (p *ProgrammingProvider) Execute(ctx context.Context, req production.ProviderRequest) production.ProviderResult {
	if p == nil || p.world == nil {
		return production.ProviderResult{Err: fmt.Errorf("programming provider unavailable")}
	}
	forcedID := strings.TrimSpace(req.Project.ID)
	result, err := runProgrammingRequestWithContract(ctx, p.world, req.Input, forcedID, req.Project, p.synthesizer, req.RecoveryEvidence, req.EffectRecorder, req.EffectEvaluationRecorder, req.RecoveryContext, req.WorkID, req.ObjectiveFrame)
	if err != nil {
		return production.ProviderResult{Err: err}
	}
	projectPath := resultProjectPath(req.Project, result.ProjectID, resolveWorkspaceRoot(p.world))
	return p.providerResultFromProduction(ctx, result, projectPath, req.AttemptNumber, false)
}

// Repair reconciles the existing programming artifact represented by the prior
// ActionResult. A simple project-identifier correction renames the existing
// artifact before rerunning production; other corrections reuse the existing
// project ID so repair cannot silently create a duplicate project.
func (p *ProgrammingProvider) Repair(ctx context.Context, intent production.RepairIntent) production.ProviderResult {
	if p == nil || p.world == nil {
		return production.ProviderResult{Err: fmt.Errorf("programming provider unavailable")}
	}
	previous := intent.PreviousResult
	if !previous.Success || !previous.Verified || !previous.Project.Valid() {
		return production.ProviderResult{Err: fmt.Errorf("programming repair requires a successful verified prior project")}
	}
	replacement := strings.TrimSpace(intent.Replacement)
	if replacement == "" {
		return production.ProviderResult{Err: fmt.Errorf("programming repair replacement is empty")}
	}
	oldID := previous.Project.ID
	forcedID := oldID
	projectPath := previous.Project.Path
	if isProjectIdentifier(replacement) && !strings.EqualFold(replacement, oldID) {
		home, err := os.UserHomeDir()
		if err != nil {
			return production.ProviderResult{Err: err}
		}
		oldPath := projectPath
		if strings.TrimSpace(oldPath) == "" {
			oldPath = filepath.Join(home, "Desktop", "AhmeliaProduced", oldID)
		}
		newPath := filepath.Join(filepath.Dir(oldPath), strings.ToLower(replacement))
		if _, err := os.Stat(newPath); err == nil {
			return production.ProviderResult{Err: fmt.Errorf("repair target project already exists: %s", newPath)}
		} else if !os.IsNotExist(err) {
			return production.ProviderResult{Err: err}
		}
		if err := os.Rename(oldPath, newPath); err != nil {
			return production.ProviderResult{Err: fmt.Errorf("rename existing project for repair: %w", err)}
		}
		forcedID = strings.ToLower(replacement)
		projectPath = newPath
	}
	// If a project-identifier correction moved the artifact directory, synthesis
	// must continue against that new canonical path instead of recreating the
	// old directory from the stale PreviousResult snapshot.
	previous.Project.Path = projectPath
	result, err := runProgrammingRequestForProject(ctx, p.world, replacement, forcedID, previous.Project, p.synthesizer, "", nil, nil, previous.RecoveryContext, "")
	if err != nil {
		return production.ProviderResult{Err: err}
	}
	if result.ProjectID == "" {
		result.ProjectID = forcedID
	}
	if projectPath == "" {
		home, _ := os.UserHomeDir()
		projectPath = filepath.Join(home, "Desktop", "AhmeliaProduced", result.ProjectID)
	}
	return p.providerResultFromProduction(ctx, result, projectPath, 1, true)
}

func (p *ProgrammingProvider) providerResultFromProduction(ctx context.Context, result programming.ProgrammingIntegrationResult, projectPath string, attemptNumber int, repaired bool) production.ProviderResult {
	objectiveContractJSON, _ := json.Marshal(result.Production.Verification.Objective)
	pr := production.ProviderResult{
		GenerationSource:      synthesisGenerationSource(p.synthesizer),
		VerificationProfile:   programmingVerificationProfile(result),
		ObjectiveStatus:       result.Production.Verification.ObjectiveStatus,
		ObjectiveContractJSON: string(objectiveContractJSON),
		Success:               result.Status == programming.SoftwareProductionSucceeded,
		Verified:              result.Status == programming.SoftwareProductionSucceeded,
		Observed:              result.Status == programming.SoftwareProductionSucceeded,
		ObservationDetails:    "el proveedor de programación confirmó el estado de producción mediante su evidencia de construcción/pruebas disponible",
		VerificationSource:    "programming",
		VerificationReason:    "resultado de producción evaluado por el proveedor de programación",
		OutcomeCode:           "production_result",
		ProductionID:          result.ProjectID,
		Producer:              "programming",
		Project:               production.ProductionProject{ID: result.ProjectID, Name: result.ProjectID, Path: projectPath},
	}

	// Stable harness checks describe partial progress even when the production
	// loop exits before final verification. Generated tests are a technical
	// milestone only; they do not imply semantic acceptance of the objective.
	if build := result.Production.Build; build != nil {
		pr.ProgressChecks = append(pr.ProgressChecks, production.ProgressCheck{ID: "programming/build", Passed: build.Status == programming.BuildSuccess && build.ExitCode == 0})
	}
	if tests := result.Production.Tests; tests != nil {
		pr.ProgressChecks = append(pr.ProgressChecks, production.ProgressCheck{ID: "programming/tests", Passed: tests.Status == programming.TestPassed && tests.ExitCode == 0})
	}
	for _, check := range result.Production.Verification.Checks {
		if check.Name == "build_succeeds" || check.Name == "tests_pass" {
			continue
		}
		pr.ProgressChecks = append(pr.ProgressChecks, production.ProgressCheck{ID: "programming/verification/" + check.Name, Passed: check.Success})
	}

	if !pr.Success && result.Production.FailureReason != "" {
		payload, _ := json.Marshal(struct {
			FailedStage   string                               `json:"failed_stage,omitempty"`
			FailureReason string                               `json:"failure_reason,omitempty"`
			ErrorAnalysis *programming.ErrorAnalysis           `json:"error_analysis,omitempty"`
			Verification  programming.CodeVerificationEvidence `json:"verification"`
		}{
			FailedStage:   result.Production.FailedStage,
			FailureReason: result.Production.FailureReason,
			ErrorAnalysis: result.Production.ErrorAnalysis,
			Verification:  result.Production.Verification,
		})
		pr.RecoveryContext = &tools.RecoveryContext{
			AttemptNumber: attemptNumber,
			FailureReason: result.Production.FailureReason,
			Evidence:      []tools.RecoveryEvidence{{Kind: "programming.production", Source: "programming", Stage: result.Production.FailedStage, Summary: result.Production.FailureReason, Payload: payload}},
		}
	}

	// Artifact reporting is derived from the project request rather than a
	// hard-coded list in core.Engine.
	if result.Production.Changes != nil {
		for _, change := range result.Production.Changes.Changes {
			name := change.Change.File.Path
			kind := "source"
			if change.Change.File.Role == programming.RoleTest {
				kind = "test"
			} else if change.Change.File.Role == programming.RoleConfig {
				kind = "config"
			}
			pr.Artifacts = append(pr.Artifacts, production.ProductionArtifact{
				ID: name, Name: filepath.Base(name), Path: filepath.Join(projectPath, filepath.FromSlash(name)),
				Type: kind, Producer: "programming", ProjectID: result.ProjectID,
			})
		}
	}
	if result.Production.Build != nil && result.Production.Build.Status == programming.BuildSuccess {
		pr.Artifacts = append(pr.Artifacts, production.ProductionArtifact{
			ID: "build", Name: "build", Path: projectPath, Type: "build", Producer: "programming", ProjectID: result.ProjectID,
		})
	}

	if pr.Success {
		if repaired {
			pr.Summary = fmt.Sprintf("Corrección materializada: proyecto %s actualizado y verificado en %s.", result.ProjectID, projectPath)
		} else {
			pr.Summary = fmt.Sprintf("Producción completada: proyecto %s creado y verificado en %s.", result.ProjectID, projectPath)
		}
		return pr
	}

	pr.FailedStage = result.Production.FailedStage
	pr.FailureReason = result.Production.FailureReason
	if result.Production.ErrorAnalysis != nil && len(result.Production.ErrorAnalysis.Diagnostics) > 0 {
		assessment, debugErr := programming.AssessFailureForDebugging(ctx, result.ProjectID, result.Production.ErrorAnalysis.Diagnostics)
		if debugErr == nil {
			d := result.Production.ErrorAnalysis.Diagnostics[0]
			pr.ObservationDetails = fmt.Sprintf("programming failure evidence: stage=%s diagnostics=%d", result.Production.FailedStage, len(result.Production.ErrorAnalysis.Diagnostics))
			pr.Summary = fmt.Sprintf("Producción detenida en %s. Diagnóstico: %s:%d:%d %s. %s.",
				result.Production.FailedStage, d.File, d.Line, d.Column, d.Message, assessment.Reason)
		}
	}
	return pr
}

func programmingVerificationProfile(result programming.ProgrammingIntegrationResult) string {
	checks := make([]string, 0, 2)
	if build := result.Production.Build; build != nil {
		checks = append(checks, "build["+strings.TrimSpace(build.Command)+"]="+string(build.Status))
	} else {
		checks = append(checks, "build=not_run")
	}
	if tests := result.Production.Tests; tests != nil {
		checks = append(checks, "tests["+strings.TrimSpace(tests.Command)+"]="+string(tests.Status))
	} else {
		checks = append(checks, "tests=not_run")
	}
	return strings.Join(checks, ";")
}

func synthesisGenerationSource(synthesizer programming.CodeObjectiveSynthesizer) string {
	if source, ok := synthesizer.(interface{ GenerationSource() string }); ok {
		return strings.TrimSpace(source.GenerationSource())
	}
	return ""
}

func (p *ProgrammingProvider) executeWithProjectID(ctx context.Context, input, forcedID string) production.ProviderResult {
	if p == nil || p.world == nil {
		return production.ProviderResult{Err: fmt.Errorf("programming provider unavailable")}
	}
	result, err := runProgrammingRequestForProject(ctx, p.world, input, forcedID, production.ProductionProject{}, p.synthesizer, "", nil, nil, nil, "")
	if err != nil {
		return production.ProviderResult{Err: err}
	}
	projectPath := resultProjectPath(production.ProductionProject{}, result.ProjectID, resolveWorkspaceRoot(p.world))
	return p.providerResultFromProduction(ctx, result, projectPath, 1, false)
}

func runProgrammingRequest(ctx context.Context, world *environment.Manager, input string, synthesizer programming.CodeObjectiveSynthesizer) (programming.ProgrammingIntegrationResult, error) {
	return runProgrammingRequestForProject(ctx, world, input, "", production.ProductionProject{}, synthesizer, "", nil, nil, nil, "")
}

func runProgrammingRequestWithContract(ctx context.Context, world *environment.Manager, input, forcedProjectID string, existingProject production.ProductionProject, synthesizer programming.CodeObjectiveSynthesizer, recoveryEvidence string, recordEffect func([]byte) error, recordEffectEvaluation func([]byte) error, recoveryContext *tools.RecoveryContext, workID string, frame interaction.ObjectiveFrame) (programming.ProgrammingIntegrationResult, error) {
	return runProgrammingRequestForProjectWithFrame(ctx, world, input, forcedProjectID, existingProject, synthesizer, recoveryEvidence, recordEffect, recordEffectEvaluation, recoveryContext, workID, frame)
}

func runProgrammingRequestWithProjectID(ctx context.Context, world *environment.Manager, input, forcedProjectID string, synthesizer programming.CodeObjectiveSynthesizer) (programming.ProgrammingIntegrationResult, error) {
	return runProgrammingRequestForProject(ctx, world, input, forcedProjectID, production.ProductionProject{}, synthesizer, "", nil, nil, nil, "")
}

// runProgrammingRequestForProject is the domain production adapter. It derives
// project continuity from the generic ProductionProject supplied by the Work
// owner. Synthesis is the only model-backed step; all filesystem/build/test
// effects remain in the existing Programming production loop.
func runProgrammingRequestForProject(ctx context.Context, world *environment.Manager, input, forcedProjectID string, existingProject production.ProductionProject, synthesizer programming.CodeObjectiveSynthesizer, recoveryEvidence string, recordEffect func([]byte) error, recordEffectEvaluation func([]byte) error, recoveryContext *tools.RecoveryContext, workID string) (programming.ProgrammingIntegrationResult, error) {
	return runProgrammingRequestForProjectWithFrame(ctx, world, input, forcedProjectID, existingProject, synthesizer, recoveryEvidence, recordEffect, recordEffectEvaluation, recoveryContext, workID, interaction.ObjectiveFrame{})
}

func runProgrammingRequestForProjectWithFrame(ctx context.Context, world *environment.Manager, input, forcedProjectID string, existingProject production.ProductionProject, synthesizer programming.CodeObjectiveSynthesizer, recoveryEvidence string, recordEffect func([]byte) error, recordEffectEvaluation func([]byte) error, recoveryContext *tools.RecoveryContext, workID string, frame interaction.ObjectiveFrame) (programming.ProgrammingIntegrationResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if world == nil {
		return programming.ProgrammingIntegrationResult{}, fmt.Errorf("integration: programming world nil")
	}
	lower := strings.ToLower(input)
	projectID := strings.TrimSpace(forcedProjectID)
	root := strings.TrimSpace(existingProject.Path)
	if projectID == "" {
		projectID = inferProjectID(input)
	}
	if root == "" {
		root = resolveWorkspaceRoot(world)
	}
	if projectID == "" {
		projectID = workspaceProjectID(root, input)
	}
	if err := os.MkdirAll(root, 0755); err != nil {
		return programming.ProgrammingIntegrationResult{}, err
	}
	language, build, tests, runtime := inferLanguageCommands(lower)
	project := programming.CodeProject{ID: projectID, Name: projectID, Root: root, Language: language, SourceRoots: []string{"."}, TestRoots: []string{"."}, BuildCommand: build, TestCommand: tests, RuntimeCommand: runtime}
	source, err := production.NewEnvironmentWorkspaceSource(world)
	if err != nil {
		return programming.ProgrammingIntegrationResult{}, err
	}
	prodWS, err := production.NewWorkspace("programming-"+project.ID, source)
	if err != nil {
		return programming.ProgrammingIntegrationResult{}, err
	}
	pw, err := programming.NewProgrammingWorkspace(prodWS, project)
	if err != nil {
		return programming.ProgrammingIntegrationResult{}, err
	}
	ops, err := programming.NewOSSourceFileOperations(pw)
	if err != nil {
		return programming.ProgrammingIntegrationResult{}, err
	}
	editor, err := programming.NewMultiFileEditor(ops)
	if err != nil {
		return programming.ProgrammingIntegrationResult{}, err
	}
	loop, err := programming.NewSoftwareProductionLoop(editor, programming.NewOSBuildExecutor(), programming.NewOSTestExecutor(), programming.NewCodeVerifier())
	if err != nil {
		return programming.ProgrammingIntegrationResult{}, err
	}
	integration, err := programming.NewProgrammingIntegration(loop)
	if err != nil {
		return programming.ProgrammingIntegrationResult{}, err
	}

	files, err := snapshotProjectFiles(root, language, input)
	if err != nil {
		return programming.ProgrammingIntegrationResult{}, err
	}
	contract := programming.ObjectiveContract{PrimaryGoal: frame.PrimaryGoal, PrimaryOperation: frame.PrimaryOperation, Subject: frame.Subject, Workspace: frame.Workspace, Constraints: append([]string(nil), frame.Constraints...), RequiredActions: append([]string(nil), frame.RequiredActions...), Verification: append([]string(nil), frame.Verification...), Recovery: append([]string(nil), frame.Recovery...), Confidence: frame.Confidence, Provenance: frame.Provenance}
	objective := programming.CodeObjective{Raw: input, Contract: contract, RecoveryEvidence: recoveryEvidence, RecoveryContext: recoveryContext, Language: language, ProjectID: projectID, ProjectRoot: root, Existing: existingProject.Valid() || len(files) > 0, Files: files}
	if synthesizer == nil {
		return programming.ProgrammingIntegrationResult{}, fmt.Errorf("integration: programming objective synthesizer unavailable")
	}
	changes, err := synthesizer.Synthesize(ctx, objective)
	if err != nil {
		return programming.ProgrammingIntegrationResult{}, err
	}
	if recordEffect != nil {
		spec, err := json.Marshal(changes)
		if err != nil {
			return programming.ProgrammingIntegrationResult{}, fmt.Errorf("integration: serialize programming effect specification: %w", err)
		}
		if err := recordEffect(spec); err != nil {
			return programming.ProgrammingIntegrationResult{}, fmt.Errorf("integration: persist programming effect specification: %w", err)
		}
	}
	if recoveryEvidence != "" && !planHasEffectiveRepairChanges(changes, root, input) {
		// A repeated synthesis that leaves every proposed file unchanged is not a
		// new production attempt. Reject it before editor/build/test execution so
		// the recovery loop cannot spend minutes proving the same failure again.
		failure := "no_effective_repair: synthesized plan does not change any existing project file"
		return programming.ProgrammingIntegrationResult{
			ProjectID: project.ID,
			Status:    programming.SoftwareProductionFailed,
			Production: programming.SoftwareProductionResult{
				ID: "req-" + project.ID, ProjectID: project.ID,
				Status:      programming.SoftwareProductionFailed,
				FailedStage: "recovery", FailureReason: failure,
			},
		}, nil
	}
	req := programming.SoftwareProductionRequest{ID: "req-" + project.ID, AttemptNumber: attemptNumberFromContext(recoveryContext), WorkID: workID, Project: project, Changes: changes, Build: &programming.BuildRequest{ID: "build-" + project.ID, Project: project, Command: build, WorkingDir: root, Timeout: 2 * time.Minute}, Tests: &programming.TestRequest{ID: "test-" + project.ID, Project: project, Command: tests, WorkingDir: root, Timeout: 2 * time.Minute}, Verification: programming.CodeVerificationRequest{Project: project, Objective: contract.Clone()}, EffectEvaluationRecorder: recordEffectEvaluation}
	return integration.Run(ctx, req)
}

// planHasEffectiveRepairChanges is an admission check, not a proof that a
// proposed repair is correct. When the observed failure names files, at least
// one of those files must materially change; editing an unrelated file does
// not count as progress. If no failing file can be identified, fall back to
// checking whether any proposed file changes at all.
func planHasEffectiveRepairChanges(plan *programming.MultiFilePlan, root, objective string) bool {
	if plan == nil || len(plan.Changes) == 0 {
		return false
	}
	objective = strings.ReplaceAll(objective, "\\", "/")
	// Parse file references from the failure evidence, not from the proposed
	// plan. Otherwise a model could omit the diagnosed file, edit an unrelated
	// file, and incorrectly pass the progress gate.
	fileReference := regexp.MustCompile(`(?:^|[\s:])([A-Za-z0-9_./-]+\.(?:go|py|js|json|toml))(?:[:][0-9]+(?::[0-9]+)?)?`)
	referenced := map[string]bool{}
	for _, match := range fileReference.FindAllStringSubmatch(objective, -1) {
		if len(match) > 1 {
			ref := filepath.FromSlash(match[1])
			if filepath.IsAbs(ref) {
				if relative, err := filepath.Rel(root, ref); err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
					ref = relative
				} else {
					ref = filepath.Base(ref)
				}
			}
			normalized := filepath.ToSlash(filepath.Clean(ref))
			normalized = strings.TrimPrefix(normalized, "./")
			referenced[normalized] = true
		}
	}
	var diagnosed []programming.MultiFileChange
	for _, change := range plan.Changes {
		path := filepath.ToSlash(change.File.Path)
		if referenced[path] || (len(referenced) == 0 && strings.Contains(objective, path)) {
			diagnosed = append(diagnosed, change)
		}
	}
	changesToCheck := plan.Changes
	if len(referenced) > 0 {
		changesToCheck = diagnosed
	}
	if len(changesToCheck) == 0 {
		return false
	}
	for _, change := range changesToCheck {
		if change.Operation != programming.SourceCreate && change.Operation != programming.SourceWrite {
			return true
		}
		path := filepath.Join(root, filepath.FromSlash(change.File.Path))
		current, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				return true
			}
			// An unreadable file is not safely classifiable as a no-op. Let the
			// canonical editor report the concrete filesystem error.
			return true
		}
		if normalizeRepairContent(string(current)) != normalizeRepairContent(change.Content) {
			return true
		}
	}
	return false
}

func normalizeRepairContent(content string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	lines := strings.Split(content, "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " \t\r")
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n")
}

func snapshotProjectFiles(root, language, objective string) ([]programming.CodeObjectiveFile, error) {
	var files []programming.CodeObjectiveFile
	maxFiles, maxBytes := 24, 80000
	used := 0
	terms := objectiveTerms(objective)
	candidates := make([]string, 0, maxFiles*2)
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			name := info.Name()
			if name == ".git" || name == "vendor" || name == "node_modules" || strings.HasPrefix(name, ".") {
				if path != root {
					return filepath.SkipDir
				}
			}
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if !isSynthesisSourceFile(rel, language) {
			return nil
		}
		candidates = append(candidates, rel)
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Always expose the project's build/module contract. Other source files are
	// included only when the objective gives us a reason to believe they are
	// relevant. This prevents a large unrelated workspace from overwhelming the
	// model and, more importantly, prevents it from treating arbitrary old files
	// as the requested artifact.
	sort.Slice(candidates, func(i, j int) bool {
		ci := synthesisFilePriority(candidates[i], terms)
		cj := synthesisFilePriority(candidates[j], terms)
		if ci != cj {
			return ci > cj
		}
		return candidates[i] < candidates[j]
	})

	for _, rel := range candidates {
		priority := synthesisFilePriority(rel, terms)
		base := strings.ToLower(filepath.Base(rel))
		if priority <= 0 && base != "go.mod" && base != "package.json" && base != "pyproject.toml" {
			continue
		}
		b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			return nil, err
		}
		if used+len(b) > maxBytes {
			continue
		}
		files = append(files, programming.CodeObjectiveFile{Path: rel, Content: string(b)})
		used += len(b)
		if len(files) >= maxFiles {
			break
		}
	}
	return files, nil
}

func objectiveTerms(objective string) []string {
	clean := strings.NewReplacer("`", " ", "'", " ", "\"", " ", "(", " ", ")", " ", "/", " ", "\\", " ", ".", " ", ":", " ", "-", " ", "_", " ").Replace(strings.ToLower(objective))
	seen := map[string]bool{}
	var terms []string
	for _, token := range strings.Fields(clean) {
		if len(token) < 3 || seen[token] {
			continue
		}
		seen[token] = true
		terms = append(terms, token)
	}
	return terms
}

func synthesisFilePriority(path string, terms []string) int {
	lower := strings.ToLower(filepath.ToSlash(path))
	base := strings.ToLower(filepath.Base(path))
	if base == "go.mod" || base == "package.json" || base == "pyproject.toml" {
		return 100
	}
	priority := 0
	for _, term := range terms {
		if strings.Contains(lower, term) {
			priority += 10
		}
	}
	if strings.HasSuffix(base, "_test.go") {
		priority += 1
	}
	return priority
}

func isSynthesisSourceFile(path, language string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	base := strings.ToLower(filepath.Base(path))
	switch strings.ToLower(language) {
	case "go":
		return ext == ".go" || base == "go.mod"
	case "python":
		return ext == ".py" || base == "pyproject.toml"
	case "javascript":
		return ext == ".js" || ext == ".json" || base == "package.json"
	default:
		return false
	}
}

// B96.22.5 — the "llamado/llamada/de nombre" group used to be optional,
// so a request with no explicit name (e.g. "crea un programa que muestre
// 'Hola mundo'") captured the next word after "programa" regardless of
// whether it was an actual name or a stopword like "que". That silently
// produced projects named "que" and, worse, collided distinct productions
// into the same folder. The name is now only trusted when the user
// explicitly gave one; otherwise inferProjectID returns "" and the caller's
// existing hash-based fallback (ahmelia_capability_block_<hash>) applies.
// resolveWorkspaceRoot establishes the filesystem authority for a new
// programming work session. A provider must never silently redirect a request
// for the current workspace into Desktop\AhmeliaProduced: that creates a second
// project that is invisible to the workspace the user actually named.
//
// Priority is explicit runtime authority, then the process working directory's
// nearest recognizable project root. This keeps `go run main.go` behavior
// deterministic while allowing deployments to pin a workspace explicitly.
func resolveWorkspaceRoot(world *environment.Manager) string {
	if explicit := strings.TrimSpace(os.Getenv("AHMELIA_WORKSPACE_ROOT")); explicit != "" {
		if abs, err := filepath.Abs(explicit); err == nil {
			return abs
		}
	}
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	if root := nearestProjectRoot(cwd); root != "" {
		return root
	}
	if world != nil {
		if project, _, ok := world.CurrentWorkspace(); ok && strings.TrimSpace(project) != "" {
			if root := locateWorkspaceByName(cwd, project); root != "" {
				return root
			}
		}
	}
	return cwd
}

func nearestProjectRoot(start string) string {
	current, err := filepath.Abs(start)
	if err != nil {
		return ""
	}
	for {
		for _, marker := range []string{"go.mod", ".git", "package.json", "pyproject.toml", "Cargo.toml", "ProjectSettings"} {
			if _, err := os.Stat(filepath.Join(current, marker)); err == nil {
				return current
			}
		}
		parent := filepath.Dir(current)
		if parent == current {
			return ""
		}
		current = parent
	}
}

func locateWorkspaceByName(start, project string) string {
	project = strings.TrimSpace(project)
	if project == "" {
		return ""
	}
	current, err := filepath.Abs(start)
	if err != nil {
		return ""
	}
	for {
		if strings.EqualFold(filepath.Base(current), project) {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			return ""
		}
		current = parent
	}
}

func workspaceProjectID(root, input string) string {
	if root != "" {
		if base := strings.TrimSpace(filepath.Base(root)); base != "" && base != "." && base != string(filepath.Separator) {
			if isProjectIdentifier(base) {
				return strings.ToLower(base)
			}
		}
	}
	return "ahmelia_workspace_" + shortHash(input)
}

func resultProjectPath(project production.ProductionProject, projectID, resolvedRoot string) string {
	if path := strings.TrimSpace(project.Path); path != "" {
		return path
	}
	if path := strings.TrimSpace(resolvedRoot); path != "" {
		return path
	}
	return filepath.Join("AhmeliaProduced", projectID)
}

var projectNameRE = regexp.MustCompile(`(?i)(?:programa|proyecto)\s+(?:go\s+)?(?:llamado|llamada|de nombre)\s+([a-zA-Z0-9_-]+)`)

// projectNameStopwords is a defense-in-depth filter: even if the regex above
// is loosened again in the future, a captured "name" that is actually a
// grammatical filler word must never be trusted as a project identifier.
var projectNameStopwords = map[string]bool{
	"que": true, "el": true, "la": true, "los": true, "las": true,
	"un": true, "una": true, "unos": true, "unas": true,
	"y": true, "o": true, "para": true, "con": true, "sin": true,
	"de": true, "del": true, "en": true, "a": true, "al": true,
}

func isProjectIdentifier(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			continue
		}
		return false
	}
	return true
}

func inferProjectID(input string) string {
	m := projectNameRE.FindStringSubmatch(input)
	if len(m) > 1 {
		name := strings.ToLower(m[1])
		if !projectNameStopwords[name] {
			return name
		}
	}
	if strings.Contains(strings.ToLower(input), "hello_ahmelia") {
		return "hello_ahmelia"
	}
	return ""
}

func inferLanguageCommands(lower string) (string, string, string, string) {
	switch {
	case strings.Contains(lower, "python"):
		return "Python", "python -m py_compile ./*.py", "python -m unittest", "python main.py"
	case strings.Contains(lower, "javascript") || strings.Contains(lower, "node"):
		return "JavaScript", "node --check main.js", "node --test", "node main.js"
	default:
		return "Go", "go build ./...", "go test ./...", "go run ."
	}
}

func selfDevelopmentPlan(project programming.CodeProject) *programming.MultiFilePlan {
	doc := `# Capability block proposal

This scaffold was produced in a sandboxed AhmeliaProduced project.
It is a review artifact, not an automatic modification of Ahmelia itself.

The block demonstrates the contract boundary for a future capability:
- declarative identity
- description
- inputs and outputs
- validation
- tests

Integration into Ahmelia remains a human-reviewed step.
`
	src := `package capabilityblock

import "fmt"

type Block struct {
    ID          string
    Description string
    Inputs      []string
    Outputs     []string
}

func New(id, description string, inputs, outputs []string) (Block, error) {
    if id == "" {
        return Block{}, fmt.Errorf("id vacío")
    }
    if description == "" {
        return Block{}, fmt.Errorf("description vacía")
    }
    return Block{ID:id, Description:description, Inputs:append([]string(nil),inputs...), Outputs:append([]string(nil),outputs...)}, nil
}

func (b Block) Validate() error {
    if b.ID == "" { return fmt.Errorf("id vacío") }
    if b.Description == "" { return fmt.Errorf("description vacía") }
    return nil
}
`
	test := `package capabilityblock

import "testing"

func TestNewAndValidate(t *testing.T) {
    b, err := New("review_block", "candidate capability block", []string{"goal"}, []string{"result"})
    if err != nil { t.Fatal(err) }
    if err := b.Validate(); err != nil { t.Fatal(err) }
    if b.ID != "review_block" { t.Fatalf("ID = %q", b.ID) }
}
`
	module := "module " + project.ID + "\n\ngo 1.20\n"
	return &programming.MultiFilePlan{ID: "self-development-" + project.ID, Project: project, Changes: []programming.MultiFileChange{
		planChange(project, "go.mod", programming.RoleConfig, module, "00-go-mod"),
		planChange(project, "README.md", programming.RoleConfig, doc, "01-readme"),
		planChange(project, "capability_block.go", programming.RoleSource, src, "02-source"),
		planChange(project, "capability_block_test.go", programming.RoleTest, test, "03-test"),
	}}
}

func planChange(project programming.CodeProject, path string, role programming.SourceFileRole, content, id string) programming.MultiFileChange {
	op := programming.SourceCreate
	if _, err := os.Stat(filepath.Join(project.Root, path)); err == nil {
		op = programming.SourceWrite
	}
	return programming.MultiFileChange{ID: id, Operation: op, File: programming.SourceFile{Path: path, Language: project.Language, Role: role, ProjectID: project.ID}, Content: content}
}

func shortHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return fmt.Sprintf("%x", sum[:4])
}

func attemptNumberFromContext(c *tools.RecoveryContext) int {
	if c == nil || c.AttemptNumber < 1 {
		return 1
	}
	return c.AttemptNumber
}
