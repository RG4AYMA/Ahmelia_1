package core

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ahmelia/autonomy"
)

func TestPostAuditAutonomyExecutorHasNoDirectSolvingCall(t *testing.T) {
	path := filepath.Join("autonomy_executor.go")
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, imp := range file.Imports {
		if strings.Trim(imp.Path.Value, `"`) == "ahmelia/solving" {
			t.Fatal("AutonomyExecutor no debe importar solving directamente")
		}
	}
	foundAuthorityAdapter := false
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
			if sel.Sel.Name == "executeAutonomousAction" || sel.Sel.Name == "executeAutonomousActionWithContext" {
				foundAuthorityAdapter = true
			}
			if sel.Sel.Name == "Execute" {
				if ident, ok := sel.X.(*ast.Ident); ok && ident.Name == "solving" {
					t.Fatalf("AutonomyExecutor contiene una llamada directa a solving.Execute")
				}
			}
		}
		return true
	})
	if !foundAuthorityAdapter {
		t.Fatal("AutonomyExecutor no está conectado al handoff Engine.executeAutonomousAction")
	}
}

func TestPostAuditI6_NoDirectSolvingExecutionOutsideCapabilityProvider(t *testing.T) {
	root := "."
	fset := token.NewFileSet()
	count := 0
	var foundFile string
	var foundLine int

	err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") || filepath.Base(path) == "execution_engine.go" {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Execute" {
				if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "solving" {
					count++
					foundFile = path
					foundLine = fset.Position(call.Pos()).Line
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("I6 no permite solving.Execute como ruta física del runtime: hallado=%s:%d", foundFile, foundLine)
	}

	engineSrc, err := os.ReadFile(filepath.Join(root, "engine.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(engineSrc), "e.productionRuntime.ExecuteFromAuthority(") {
		t.Fatal("I6 incumplido: la autoridad no delega en el runtime de providers")
	}
}

func TestPostAuditAutonomyCycleObserveEvaluatePlanExecuteVerifyE2E(t *testing.T) {
	e := newB963334AuditEngine(t)
	e.flowTrace = NewTurnFlowTrace("post-audit-autonomy")

	cycleState := autonomy.New()
	dir := t.TempDir()
	source := filepath.Join(dir, "post_audit_source.txt")
	if err := os.WriteFile(source, []byte("caso no visto"), 0o644); err != nil {
		t.Fatalf("no pude preparar el archivo del caso no visto: %v", err)
	}
	target := filepath.Join(dir, "post_audit_renamed.txt")
	planner := &onceActionPlanner{
		req: autonomy.ActionRequest{
			ID: "post-audit-cycle", Action: "renombrar_archivo", Target: source,
			Parameters: map[string]string{"nombre_nuevo": filepath.Base(target)},
		},
	}

	cycle := autonomy.NewCycle(
		cycleState,
		cycleObserverForTest{},
		cycleEvaluatorForTest{},
		planner,
		NewAutonomyExecutor(e),
		nil,
		nil,
		nil,
		nil,
	)

	if err := cycle.RunStep(context.Background()); err != nil {
		t.Fatalf("Observe→Evaluate→Plan→Execute→Verify falló: %v", err)
	}
	if info, err := os.Stat(target); err != nil || info.IsDir() {
		t.Fatalf("la ejecución autónoma del caso no visto no produjo el archivo renombrado: %v", err)
	}
	if _, err := os.Stat(source); !os.IsNotExist(err) {
		t.Fatalf("el origen del caso no visto todavía existe después del renombrado")
	}
	if got := cycleState.Execution(); got != autonomy.ExecIdle {
		t.Fatalf("el ciclo no cerró en Idle: %s", got)
	}
	trace := strings.Join(e.flowTrace.Snapshot(), "\n")
	if !strings.Contains(trace, "route=action") || !strings.Contains(trace, "source=autonomy.plan") {
		t.Fatalf("la traza no demuestra el cruce por la autoridad/proveedor unificado: %s", trace)
	}

	// Tres preguntas de cierre del Bloque A.
	if !strings.Contains(trace, "EXECUTION AUTHORITY") {
		t.Fatal("la evidencia de quién decidió no quedó visible en la traza")
	}
}

type cycleObserverForTest struct{}

func (cycleObserverForTest) Observe(context.Context) (string, error) {
	return "observación no vista", nil
}

type cycleEvaluatorForTest struct{}

func (cycleEvaluatorForTest) EvaluateRelevance(context.Context, string) (bool, error) {
	return true, nil
}
