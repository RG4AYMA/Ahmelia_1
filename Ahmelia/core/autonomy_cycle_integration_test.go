package core

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"ahmelia/autonomy"
)

// onceActionPlanner es un CyclePlanner mínimo que ofrece una única
// ActionRequest la primera vez que se le pregunta, y nada después —
// alcanza para probar una iteración de Cycle.RunStep() de punta a
// punta sin necesitar todavía un planner real (Bloque 81, futuro).
type onceActionPlanner struct {
	req    autonomy.ActionRequest
	served bool
}

func (p *onceActionPlanner) GetNextAction(context.Context) (autonomy.ActionRequest, bool, error) {
	if p.served {
		return autonomy.ActionRequest{}, false, nil
	}
	p.served = true
	return p.req, true, nil
}

// TestCycleWithRealAutonomyExecutor_CrearCarpeta es la validación
// empírica real de B79: arma un autonomy.Cycle (B78) de verdad, con
// AutonomyExecutor (B79) como su ExecutorAdapter real — sin mock en el
// punto de ejecución — y confirma que una vuelta de RunStep() crea una
// carpeta de verdad en disco y el estado B72 cierra en Idle.
func TestCycleWithRealAutonomyExecutor_CrearCarpeta(t *testing.T) {
	state := autonomy.New()
	e := newB963334AuditEngine(t)
	dir := t.TempDir()
	target := filepath.Join(dir, "carpeta_b79")

	planner := &onceActionPlanner{
		req: autonomy.ActionRequest{
			ID:     "cycle-1",
			Action: "crear_carpeta",
			Target: target,
		},
	}

	cycle := autonomy.NewCycle(
		state,
		nil, // CycleObserver — no hace falta observación real para esta prueba
		nil, // CycleEvaluator
		planner,
		NewAutonomyExecutor(e), // ExecutorAdapter REAL detrás de la autoridad de Engine
		nil,                    // ActionVerifier — usa el default (NewActionVerifier)
		nil,                    // CycleDiagnoser
		nil,                    // CycleLearner
		nil,                    // CycleUpdater
	)

	if err := cycle.RunStep(context.Background()); err != nil {
		t.Fatalf("RunStep() devolvió error inesperado: %v", err)
	}

	if info, err := os.Stat(target); err != nil || !info.IsDir() {
		t.Fatalf("el ciclo completo no creó la carpeta real en disco: %v", err)
	}

	if got := state.Execution(); got != autonomy.ExecIdle {
		t.Errorf("esperaba que el ciclo cerrara en Idle, quedó en %s", got)
	}
}

// TestCycleWithRealAutonomyExecutor_AccionNoSoportadaQuedaBloqueado
// confirma el camino de fallo real: una Action sin ejecución real
// conectada (abrir_programa) hace que Cycle pase por
// Recovering→Blocked→Idle, no que finja éxito.
func TestCycleWithRealAutonomyExecutor_AccionNoSoportadaQuedaBloqueado(t *testing.T) {
	state := autonomy.New()
	e := newB963334AuditEngine(t)

	planner := &onceActionPlanner{
		req: autonomy.ActionRequest{
			ID:     "cycle-2",
			Action: "abrir_programa",
			Target: "notepad",
		},
	}

	diag := &recordingDiagnoser{}

	cycle := autonomy.NewCycle(
		state, nil, nil, planner,
		NewAutonomyExecutor(e),
		nil,
		diag,
		nil, nil,
	)

	if err := cycle.RunStep(context.Background()); err != nil {
		t.Fatalf("RunStep() devolvió error inesperado: %v", err)
	}

	if diag.calls != 1 {
		t.Errorf("esperaba que el diagnóstico se llamara 1 vez, se llamó %d", diag.calls)
	}
	if got := state.Execution(); got != autonomy.ExecIdle {
		t.Errorf("esperaba que el ciclo cerrara en Idle tras el fallo, quedó en %s", got)
	}
}

type recordingDiagnoser struct{ calls int }

func (d *recordingDiagnoser) DiagnoseFailure(context.Context, autonomy.ActionRequest, autonomy.VerificationResult) error {
	d.calls++
	return nil
}
