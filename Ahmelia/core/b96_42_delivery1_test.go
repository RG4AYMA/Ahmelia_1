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
	"ahmelia/planning"
)

func TestB9642_1_OnlyExecutionAuthorityCallsProductionRuntime(t *testing.T) {
	var callers []string
	fset := token.NewFileSet()
	err := filepath.Walk(".", func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || fn.Name == nil {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "ExecuteFromAuthority" {
					if recv, ok := sel.X.(*ast.SelectorExpr); ok && recv.Sel.Name == "productionRuntime" {
						callers = append(callers, path+":"+fn.Name.Name)
					}
				}
				return true
			})
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(callers) != 1 || !strings.HasSuffix(callers[0], "engine.go:authorityExecuteWithOptions") {
		t.Fatalf("ProductionRuntime.ExecuteFromAuthority tiene rutas físicas fuera de la autoridad: %v", callers)
	}
}

func TestB9642_1_NoPhysicalEngineExecuteOutsideLocalProvider(t *testing.T) {
	var owners []string
	fset := token.NewFileSet()
	err := filepath.Walk(".", func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || fn.Name == nil {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "execute" {
					if recv, ok := sel.X.(*ast.Ident); ok && recv.Name == "e" {
						owners = append(owners, path+":"+fn.Name.Name)
					}
				}
				return true
			})
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(owners) != 1 || !strings.HasSuffix(owners[0], "local_action_executor.go:ExecuteCapabilityAction") {
		t.Fatalf("Engine.execute mantiene más de un propietario físico: %v", owners)
	}
}

func TestB9642_1_AutonomousRenamingUsesGenericProviderBoundary(t *testing.T) {
	e := newB963334AuditEngine(t)
	exec := NewAutonomyExecutor(e)
	dir := t.TempDir()
	source := filepath.Join(dir, "source.txt")
	target := filepath.Join(dir, "target.txt")
	if err := os.WriteFile(source, []byte("provider boundary"), 0o644); err != nil {
		t.Fatal(err)
	}

	resp := exec.ExecuteAction(context.Background(), autonomy.ActionRequest{
		ID:     "b96.42.1-renaming",
		Action: "renombrar_archivo",
		Target: source,
		Parameters: map[string]string{
			"nombre_nuevo": filepath.Base(target),
		},
	})
	if !resp.Executed {
		t.Fatalf("el provider unificado rechazó el renombrado: %v", resp.Error)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("el provider unificado no produjo el objetivo: %v", err)
	}
	if _, err := os.Stat(source); !os.IsNotExist(err) {
		t.Fatalf("el origen sigue existiendo después del renombrado")
	}
}

func TestB9642_2_GoalStoreOwnsCurrentAndPendingState(t *testing.T) {
	planner := planning.NewPlanner()
	state := autonomy.New()
	monitor := autonomy.NewGoalMonitor(state, 0)
	adapter, err := autonomy.NewGoalPlanningAdapter(
		state,
		monitor,
		planner,
		autonomy.NewCapabilityResolver(nil),
		nil,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	persistCalls := 0
	store, err := NewGoalStore(planner, state, adapter)
	if err != nil {
		t.Fatal(err)
	}
	store.SetPersister(func() error {
		persistCalls++
		return nil
	})

	goalA := planning.NewGoal("B96.42.2 A", "objetivo activo", planning.PriorityHigh, "user")
	goalB := planning.NewGoal("B96.42.2 B", "objetivo pendiente", planning.PriorityMedium, "user")
	if err := store.Register(goalA); err != nil {
		t.Fatal(err)
	}
	if err := store.Register(goalB); err != nil {
		t.Fatal(err)
	}
	if err := store.ActivateOrQueue(goalA); err != nil {
		t.Fatal(err)
	}
	if err := store.ActivateOrQueue(goalB); err != nil {
		t.Fatal(err)
	}

	if got := store.Current(); got != goalA {
		t.Fatalf("Current no conserva la identidad única del Goal: got=%p want=%p", got, goalA)
	}
	pending := store.Pending()
	if len(pending) != 1 || pending[0] != goalB {
		t.Fatalf("Pending no conserva la identidad única del Goal: %+v", pending)
	}
	registered, ok := store.Get(goalA.ID)
	if !ok || registered != goalA {
		t.Fatalf("Planner y GoalStore no comparten el mismo Goal registrado: ok=%v got=%p want=%p", ok, registered, goalA)
	}
	if persistCalls != 2 {
		t.Fatalf("cada transición de current/pending debe persistirse por el Core owner; llamadas=%d", persistCalls)
	}
}

func TestB9642_2_GoalStorePersistenceFailureIsReported(t *testing.T) {
	planner := planning.NewPlanner()
	state := autonomy.New()
	monitor := autonomy.NewGoalMonitor(state, 0)
	adapter, err := autonomy.NewGoalPlanningAdapter(state, monitor, planner, autonomy.NewCapabilityResolver(nil), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewGoalStore(planner, state, adapter)
	if err != nil {
		t.Fatal(err)
	}
	store.SetPersister(func() error { return os.ErrPermission })

	goal := planning.NewGoal("B96.42.2 persist error", "", planning.PriorityMedium, "user")
	if err := store.SetCurrent(goal); err == nil {
		t.Fatal("GoalStore ocultó el error de persistencia")
	}
	if got := store.Current(); got != goal {
		t.Fatalf("la mutación en memoria no quedó aplicada antes del error de persistencia: %p", got)
	}
}

func TestB9642_2_ProductionWorkIsNotCreatedForLocalAction(t *testing.T) {
	e := newB963334AuditEngine(t)
	_, hadWork := e.workStore.Last()
	e.SetResponseCallback(func(string) {})
	e.Process("cambia a chrome")
	_, hasWorkAfter := e.workStore.Last()
	if hadWork != hasWorkAfter {
		t.Fatalf("una acción local cruzó indebidamente el estado persistente de Work: before=%v after=%v", hadWork, hasWorkAfter)
	}
}

func TestB9642_1_LegacyExecutionPathsHaveNoLiveCoreConsumers(t *testing.T) {
	for _, marker := range []string{"NewProductionLoop(", "NewExecutionGateway(", "NewCapabilityE2E("} {
		var uses []string
		err := filepath.Walk(".", func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if info.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if strings.Contains(string(data), marker) {
				uses = append(uses, path)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		// The declaration itself is the only allowed non-test occurrence.
		allowed := 0
		for _, path := range uses {
			base := filepath.ToSlash(path)
			if strings.HasSuffix(base, "/production/production_loop.go") && marker == "NewProductionLoop(" {
				allowed++
			}
			if strings.HasSuffix(base, "/production/execution_gateway.go") && marker == "NewExecutionGateway(" {
				allowed++
			}
			if strings.HasSuffix(base, "/autonomy/capability_e2e.go") && marker == "NewCapabilityE2E(" {
				allowed++
			}
		}
		if len(uses) != allowed {
			t.Fatalf("legacy execution marker %q todavía tiene consumidores no-test: %v", marker, uses)
		}
	}
}
