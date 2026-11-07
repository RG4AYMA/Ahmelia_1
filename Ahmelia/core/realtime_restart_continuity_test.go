package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ahmelia/capability"
	"ahmelia/cognition"
	"ahmelia/interaction"
	"ahmelia/memory"
	"ahmelia/selfknowledge"
	"ahmelia/situation"
)

// TestB9644RestartContinuity_E2E_PersistenceToSituation verifies the complete
// lifecycle documented in Fase 14 Bloque M:
//
//	almacenamiento real → terminación/reinicio real → observación/reconciliación →
//	nueva Situation → siguiente decisión
//
// This test:
// 1. Creates an Engine in a temp dir (simulates first process)
// 2. Executes a production action that creates a WorkSession with completed attempt
// 3. Simulates process termination by creating a NEW Engine instance in SAME dir
// 4. Verifies the completed work's evidence is rehydrated into Situation with TemporalHistorical
// 5. Verifies the next cognition/decision cycle uses that Situation
func TestB9644RestartContinuity_E2E_PersistenceToSituation(t *testing.T) {
	t.Setenv("AHMELIA_SKIP_PROGRAM_INDEX_SCAN", "1")
	t.Setenv("AHMELIA_DISABLE_TTS", "1")

	// Directorio temporal compartido - simula el workspace persistente
	tempDir := t.TempDir()
	origDir, _ := os.Getwd()
	_ = os.Chdir(tempDir)
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	// Asegurar que memory/ existe
	_ = os.MkdirAll("memory", 0755)

	// ── 1. Primer Engine: crea Work, ejecuta acción, persiste ─────────────
	e1 := newInteractionE2EEngine(t)

	// Registrar provider de programación que sí verifica
	if err := e1.productionProviders.RegisterCapabilities(b9643VerifiedProductionProvider{}, capability.ID("programming.create_file")); err != nil {
		t.Fatal(err)
	}

	var responses1 []string
	e1.SetResponseCallback(func(msg string) { responses1 = append(responses1, msg) })

	// Turno 1: pide crear un programa (producción)
	e1.Process("Empieza creando un programa base para el juego de supervivencia.")

	// Verificar que se creó Work con intento completado
	work1, ok := e1.workStore.Last()
	if !ok {
		t.Fatal("E2E production request did not create WorkSession")
	}
	if work1.GoalID == "" || work1.PlanID == "" {
		t.Fatalf("E2E work lost canonical Goal/Plan link: %+v", work1)
	}

	// Forzar guardado explícito de estados persistentes
	if err := e1.conversationState.Save(interaction.ConversationStatePath); err != nil {
		t.Fatalf("save conversation state: %v", err)
	}
	if err := e1.PersistAutonomyState(); err != nil {
		t.Fatalf("save autonomy state: %v", err)
	}

	// ── 2. Simular TERMINACIÓN: crear NUEVO Engine en MISMO directorio ─────
	// El directorio de trabajo sigue siendo tempDir, así que las rutas relativas
	// memory/production_work.json, memory/conversation_state.json, etc. son las MISMAS
	e2 := newInteractionE2EEngineNoCleanup(t)

	// ── 3. Verificar que WorkStore cargó el work completado ─────────────────
	work2, ok := e2.workStore.Get(work1.ID)
	if !ok {
		t.Fatalf("Work %q not found after restart", work1.ID)
	}

	// Verificar que el work completado persiste con su evidencia
	if work2.LastActionResult == nil || !work2.LastActionResult.Success || !work2.LastActionResult.Verified {
		t.Fatalf("Work evidence not persisted correctly: %+v", work2.LastActionResult)
	}

	// ── 4. Construir Situation en el nuevo Engine ───────────────────────────
	// Buscar el work desde WorkLinks (historial de vinculación conversación-work)
	req := interaction.Parse("¿qué pasó con mi programa?", interaction.ChannelText)
	snap := e2.conversationState.Snapshot()

	var workContext interaction.WorkContext
	hasWork := false

	// Primero intentar ActiveWorkID (para work operativo activo)
	workID := strings.TrimSpace(snap.ActiveWorkID)

	// Si no hay ActiveWorkID (limpiado por ReconcileObjectiveOwnership para work terminal),
	// buscar en WorkLinks el último work vinculado
	if workID == "" && len(snap.WorkLinks) > 0 {
		lastLink := snap.WorkLinks[len(snap.WorkLinks)-1]
		workID = lastLink.WorkID
	}

	if workID != "" && e2.workStore != nil {
		if work, ok := e2.workStore.Get(workID); ok {
			workContext = workContextFromSession(work)
			hasWork = true
		}
	}

	if !hasWork {
		t.Fatal("Expected to find work via WorkLinks for Situation construction")
	}

	// Obtener observaciones pendientes
	pending := append([]situation.Observation(nil), e2.pendingSituationObservations...)
	e2.pendingSituationObservations = nil

	sit, contributions := e2.buildSharedSituation(req, snap, workContext, hasWork, pending)

	// ── 5. Verificar Situation incluye evidence rehidratada con TemporalHistorical ────
	foundHistorical := false
	foundReconciliation := false
	for _, ev := range sit.Evidence {
		if ev.Temporal == situation.TemporalHistorical {
			foundHistorical = true
		}
		if ev.Predicate == "verification" && ev.Temporal == situation.TemporalHistorical {
			foundReconciliation = true
		}
	}

	if !foundHistorical {
		t.Fatal("Expected TemporalHistorical evidence in Situation after restart")
	}
	if !foundReconciliation {
		t.Fatal("Expected verification evidence (reconciliation) in Situation after restart")
	}

	// ── 6. Verificar siguiente decisión/cognición usa esa Situation ──────────
	iteration := situation.NewIterationContext("interactive_turn_restarted", sit, contributions)
	cognitiveContext := cognition.SituationalCognitionContext{
		Iteration:             iteration,
		Situation:             sit,
		Contributions:         contributions,
		Work:                  workContext,
		HasWork:               hasWork,
		AuthoritativeEvidence: e2.authoritativeUserModelEvidence(req),
	}

	cog := cognition.RunConversationalCognitionE2E(req, snap, cognitiveContext)

	if !cog.Valid {
		t.Fatalf("Cognition invalid after restart: %v", cog.Errors)
	}

	// Verificar que DecisionEvidence incluye findings del work histórico
	foundWorkFinding := false
	for _, f := range cog.DecisionEvidence.Findings {
		if strings.HasPrefix(f.Kind, "result") || strings.HasPrefix(f.Kind, "verification") ||
			strings.HasPrefix(f.Kind, "involves_work") || strings.HasPrefix(f.Kind, "status") ||
			strings.HasPrefix(f.Kind, "pursues") || strings.HasPrefix(f.Kind, "last_") {
			foundWorkFinding = true
			break
		}
	}
	if !foundWorkFinding {
		t.Fatal("Expected DecisionEvidence findings from historical work evidence")
	}

	// ── 7. Verificar que WorkContext proyecta estado epistémico correcto ──
	if workContext.OutcomeStatus == "" {
		t.Fatal("Expected WorkContext.OutcomeStatus to be populated from persisted work")
	}
	if workContext.OutcomeStatus != "succeeded" {
		t.Fatalf("Expected OutcomeStatus=succeeded, got %q", workContext.OutcomeStatus)
	}

	t.Log("✅ Full persistence → restart → Situation → decision cycle verified")
}

// newInteractionE2EEngineNoCleanup crea un Engine igual a newInteractionE2EEngine
// pero SIN eliminar los archivos de persistencia (para simular reinicio real)
func newInteractionE2EEngineNoCleanup(t *testing.T) *Engine {
	isolateTestWorkspace(t)
	t.Helper()
	t.Setenv("AHMELIA_SKIP_PROGRAM_INDEX_SCAN", "1")
	t.Setenv("AHMELIA_DISABLE_TTS", "1")
	t.Setenv("AHMELIA_WORKSPACE_ROOT", filepath.Join(t.TempDir(), "workspace"))

	longMem := memory.LoadLongMemory()
	shortMem := LoadShortMemory()
	sess := memory.NewSession()
	history := selfknowledge.NewActionHistory(4)
	registry := selfknowledge.NewModuleRegistry()
	selfQuery := selfknowledge.NewSelfQuery(history, registry, "interaction-e2e-restart")
	e := NewEngine(sess, longMem, &shortMem, history, selfQuery)
	registerCoreTestProduction(e, t)
	e.planner.SetGoalTaskDecomposer(nil)
	installF9LanguageStubs(t, e)
	approveTestProductionExecution(t, e)
	return e
}
