package programming

import (
	"fmt"
	"time"

	"ahmelia/capability"
	"ahmelia/planning"
	"ahmelia/production"
)

// B96.1 — Programming Model
//
// Programming is a domain layer over Production. It does not create a second
// Goal/Plan/Task/Capability system. Generic lifecycle and authority remain in
// Production/B94; this package only adds the programming-specific vocabulary
// and references the existing entities.

type ProgrammingTask struct {
	ProductionID string
	Task         *planning.Task
	ProjectID    string
}

func NewProgrammingTask(productionID string, task *planning.Task, projectID string) (*ProgrammingTask, error) {
	if productionID == "" {
		return nil, fmt.Errorf("programming: production ID vacío")
	}
	if task == nil {
		return nil, fmt.Errorf("programming: task nil")
	}
	if projectID == "" {
		return nil, fmt.Errorf("programming: project ID vacío")
	}
	return &ProgrammingTask{ProductionID: productionID, Task: task, ProjectID: projectID}, nil
}

// CodeProject describes the programming context without owning Production's
// workspace or duplicating its state.
type CodeProject struct {
	ID             string
	Name           string
	Root           string
	Language       string
	SourceRoots    []string
	TestRoots      []string
	BuildCommand   string
	TestCommand    string
	RuntimeCommand string
}

func (p CodeProject) Validate() error {
	if p.ID == "" {
		return fmt.Errorf("programming project: ID vacío")
	}
	if p.Root == "" {
		return fmt.Errorf("programming project %q: Root vacío", p.ID)
	}
	return nil
}

type SourceFileRole string

const (
	RoleUnknown    SourceFileRole = "unknown"
	RoleEntrypoint SourceFileRole = "entrypoint"
	RoleSource     SourceFileRole = "source"
	RoleTest       SourceFileRole = "test"
	RoleConfig     SourceFileRole = "config"
	RoleGenerated  SourceFileRole = "generated"
)

type SourceFile struct {
	Path      string
	Language  string
	Role      SourceFileRole
	ProjectID string
}

func (f SourceFile) Validate() error {
	if f.Path == "" {
		return fmt.Errorf("programming source file: Path vacío")
	}
	if f.Language == "" {
		return fmt.Errorf("programming source file %q: Language vacío", f.Path)
	}
	if f.ProjectID == "" {
		return fmt.Errorf("programming source file %q: ProjectID vacío", f.Path)
	}
	return nil
}

type CodeArtifactType string

const (
	ArtifactSource     CodeArtifactType = "source"
	ArtifactPatch      CodeArtifactType = "patch"
	ArtifactBuild      CodeArtifactType = "build"
	ArtifactTest       CodeArtifactType = "test"
	ArtifactExecutable CodeArtifactType = "executable"
	ArtifactDiagnostic CodeArtifactType = "diagnostic"
)

type CodeArtifact struct {
	ID        string
	Type      CodeArtifactType
	ProjectID string
	Path      string
	CreatedAt time.Time
}

func (a CodeArtifact) Validate() error {
	if a.ID == "" {
		return fmt.Errorf("programming artifact: ID vacío")
	}
	if a.Type == "" {
		return fmt.Errorf("programming artifact %q: Type vacío", a.ID)
	}
	if a.ProjectID == "" {
		return fmt.Errorf("programming artifact %q: ProjectID vacío", a.ID)
	}
	return nil
}

type BuildStatus string

const (
	BuildUnknown BuildStatus = "unknown"
	BuildSuccess BuildStatus = "success"
	BuildFailed  BuildStatus = "failed"
)

type Build struct {
	ID         string
	ProjectID  string
	Command    string
	Status     BuildStatus
	ExitCode   int
	Output     string
	Errors     []string
	Warnings   []string
	StartedAt  time.Time
	FinishedAt time.Time
}

func (b Build) Validate() error {
	if b.ID == "" {
		return fmt.Errorf("programming build: ID vacío")
	}
	if b.ProjectID == "" {
		return fmt.Errorf("programming build %q: ProjectID vacío", b.ID)
	}
	if b.Command == "" {
		return fmt.Errorf("programming build %q: Command vacío", b.ID)
	}
	return nil
}

type TestStatus string

const (
	TestUnknown TestStatus = "unknown"
	TestPassed  TestStatus = "passed"
	TestFailed  TestStatus = "failed"
)

type TestRun struct {
	ID         string
	ProjectID  string
	Command    string
	Status     TestStatus
	ExitCode   int
	Output     string
	Failures   []string
	StartedAt  time.Time
	FinishedAt time.Time
}

func (t TestRun) Validate() error {
	if t.ID == "" {
		return fmt.Errorf("programming test: ID vacío")
	}
	if t.ProjectID == "" {
		return fmt.Errorf("programming test %q: ProjectID vacío", t.ID)
	}
	if t.Command == "" {
		return fmt.Errorf("programming test %q: Command vacío", t.ID)
	}
	return nil
}

type RuntimeStatus string

const (
	RuntimeStopped RuntimeStatus = "stopped"
	RuntimeRunning RuntimeStatus = "running"
	RuntimeFailed  RuntimeStatus = "failed"
)

type Runtime struct {
	ID        string
	ProjectID string
	Command   string
	Status    RuntimeStatus
	ExitCode  *int
	Output    string
	StartedAt time.Time
	StoppedAt time.Time
}

func (r Runtime) Validate() error {
	if r.ID == "" {
		return fmt.Errorf("programming runtime: ID vacío")
	}
	if r.ProjectID == "" {
		return fmt.Errorf("programming runtime %q: ProjectID vacío", r.ID)
	}
	if r.Command == "" {
		return fmt.Errorf("programming runtime %q: Command vacío", r.ID)
	}
	return nil
}

type Dependency struct {
	Name      string
	Version   string
	Manager   string
	ProjectID string
}

func (d Dependency) Validate() error {
	if d.Name == "" {
		return fmt.Errorf("programming dependency: Name vacío")
	}
	if d.Manager == "" {
		return fmt.Errorf("programming dependency %q: Manager vacío", d.Name)
	}
	if d.ProjectID == "" {
		return fmt.Errorf("programming dependency %q: ProjectID vacío", d.Name)
	}
	return nil
}

type DiagnosticSeverity string

const (
	SeverityInfo    DiagnosticSeverity = "info"
	SeverityWarning DiagnosticSeverity = "warning"
	SeverityError   DiagnosticSeverity = "error"
)

type Diagnostic struct {
	ID        string
	ProjectID string
	File      string
	Line      int
	Column    int
	ErrorType string
	Message   string
	Severity  DiagnosticSeverity
	Context   string
}

func (d Diagnostic) Validate() error {
	if d.ID == "" {
		return fmt.Errorf("programming diagnostic: ID vacío")
	}
	if d.ProjectID == "" {
		return fmt.Errorf("programming diagnostic %q: ProjectID vacío", d.ID)
	}
	if d.Message == "" {
		return fmt.Errorf("programming diagnostic %q: Message vacío", d.ID)
	}
	return nil
}

type Patch struct {
	ID          string
	ProjectID   string
	File        string
	Before      string
	After       string
	UnifiedDiff string
	CreatedAt   time.Time
}

func (p Patch) Validate() error {
	if p.ID == "" {
		return fmt.Errorf("programming patch: ID vacío")
	}
	if p.ProjectID == "" {
		return fmt.Errorf("programming patch %q: ProjectID vacío", p.ID)
	}
	if p.File == "" {
		return fmt.Errorf("programming patch %q: File vacío", p.ID)
	}
	return nil
}

type RefactorKind string

const (
	RefactorRename      RefactorKind = "rename"
	RefactorExtract     RefactorKind = "extract"
	RefactorMove        RefactorKind = "move"
	RefactorSimplify    RefactorKind = "simplify"
	RefactorDeduplicate RefactorKind = "deduplicate"
	RefactorReorganize  RefactorKind = "reorganize"
)

type Refactor struct {
	ID          string
	ProjectID   string
	Kind        RefactorKind
	Description string
	Patches     []Patch
}

func (r Refactor) Validate() error {
	if r.ID == "" {
		return fmt.Errorf("programming refactor: ID vacío")
	}
	if r.ProjectID == "" {
		return fmt.Errorf("programming refactor %q: ProjectID vacío", r.ID)
	}
	if r.Kind == "" {
		return fmt.Errorf("programming refactor %q: Kind vacío", r.ID)
	}
	return nil
}

// Capability IDs are deliberately constants only for vocabulary, not
// registration. B96 later registers implementations through B94's registry.
const (
	CapabilityReadProject    capability.ID = "programming.read_project"
	CapabilityCreateFile     capability.ID = "programming.create_file"
	CapabilityReadFile       capability.ID = "programming.read_file"
	CapabilityWriteFile      capability.ID = "programming.write_file"
	CapabilityEditFile       capability.ID = "programming.edit_file"
	CapabilityBuild          capability.ID = "programming.build"
	CapabilityRunTests       capability.ID = "programming.run_tests"
	CapabilityRunRuntime     capability.ID = "programming.run_runtime"
	CapabilityAnalyzeError   capability.ID = "programming.analyze_error"
	CapabilityDebug          capability.ID = "programming.debug"
	CapabilityVerifyCode     capability.ID = "programming.verify_code"
	CapabilityManageDeps     capability.ID = "programming.manage_dependencies"
	CapabilityVersionControl capability.ID = "programming.version_control"
)

// ProductionBinding makes the domain boundary explicit without introducing a
// second production model.
type ProductionBinding struct {
	Production *production.Production
	Task       *planning.Task
	Project    CodeProject
}

func NewProductionBinding(p *production.Production, project CodeProject) (*ProductionBinding, error) {
	if p == nil {
		return nil, fmt.Errorf("programming: production nil")
	}
	if p.Goal == nil {
		return nil, fmt.Errorf("programming: production goal nil")
	}
	if err := project.Validate(); err != nil {
		return nil, err
	}
	return &ProductionBinding{Production: p, Task: p.CurrentTask, Project: project}, nil
}
