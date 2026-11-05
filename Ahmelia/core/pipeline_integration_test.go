package core

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"ahmelia/integration"
	"ahmelia/memory"
	"ahmelia/programming"
	"ahmelia/selfknowledge"
)

type coreTestCodeSynthesizer struct{}

func registerCoreTestProduction(e *Engine, t *testing.T) {
	t.Helper()
	if err := integration.RegisterRuntimeProvidersWithSynthesizer(e.capabilityRegistry, e.productionProviders, e.world, coreTestCodeSynthesizer{}); err != nil {
		t.Fatalf("register deterministic production fixture: %v", err)
	}
}

func (coreTestCodeSynthesizer) Synthesize(_ context.Context, objective programming.CodeObjective) (*programming.MultiFilePlan, error) {
	project := programming.CodeProject{ID: objective.ProjectID, Name: objective.ProjectID, Root: objective.ProjectRoot, Language: objective.Language, SourceRoots: []string{"."}, TestRoots: []string{"."}, BuildCommand: "go build ./...", TestCommand: "go test ./...", RuntimeCommand: "go run ."}
	main := "package main\n\n// objective: " + strings.ReplaceAll(strings.TrimSpace(objective.Raw), "\n", " ") + "\nfunc main() {}\n"
	if objective.Language == "Go" && strings.Contains(strings.ToLower(objective.Raw), "sum") {
		main = "package main\n\nfunc Add(a, b int) int { return a + b }\nfunc main() {}\n"
	}
	mainOp, moduleOp := programming.SourceCreate, programming.SourceCreate
	if objective.Existing {
		mainOp, moduleOp = programming.SourceWrite, programming.SourceWrite
	}
	return &programming.MultiFilePlan{ID: "core-test-" + objective.ProjectID, Project: project, Changes: []programming.MultiFileChange{
		{ID: "main", Operation: mainOp, File: programming.SourceFile{Path: "main.go", Language: objective.Language, Role: programming.RoleEntrypoint, ProjectID: objective.ProjectID}, Content: main},
		{ID: "module", Operation: moduleOp, File: programming.SourceFile{Path: "go.mod", Language: objective.Language, Role: programming.RoleConfig, ProjectID: objective.ProjectID}, Content: "module " + objective.ProjectID + "\n\ngo 1.20\n"},
	}}, nil
}

// buildTestEngine arma un Engine real, con las mismas dependencias que
// main.go, para poder probar el pipeline de knowledgeEngine de punta a
// punta (no un mock) — exactamente lo que pide el diagnóstico: ¿Identity/
// SelfKnowledge siguen siendo alcanzables después de que se registró
// Memory Intelligence, o Memory Intelligence las está bloqueando?
func buildTestEngine(t *testing.T) *Engine {
	isolateTestWorkspace(t)
	t.Helper()
	// Los tests del core ejercitan el Engine, no el indexador de programas ni
	// TTS. Ambos efectos externos deben quedar fuera de las pruebas unitarias
	// para que -race pueda repetirlas sin recorrer C:\Program Files ni lanzar
	// procesos Python en cada NewEngine/Process.
	t.Setenv("AHMELIA_SKIP_PROGRAM_INDEX_SCAN", "1")
	t.Setenv("AHMELIA_DISABLE_TTS", "1")
	t.Setenv("AHMELIA_WORKSPACE_ROOT", filepath.Join(t.TempDir(), "workspace"))
	sess := memory.NewSession()
	longMem := memory.LoadLongMemory()
	shortMem := &ShortMemory{}
	history := selfknowledge.NewActionHistory(50)
	registry := selfknowledge.NewModuleRegistry()
	selfQuery := selfknowledge.NewSelfQuery(history, registry, "v3.1")
	e := NewEngine(sess, longMem, shortMem, history, selfQuery)
	registerCoreTestProduction(e, t)
	// Core tests exercise the Engine and planning contracts, not a live Ollama
	// service. Keep plan regeneration deterministic; model-backed decomposition
	// has its own adapter tests in integration/ and planning/.
	e.planner.SetGoalTaskDecomposer(nil)
	installF9LanguageStubs(t, e)
	approveTestProductionExecution(t, e)
	return e
}

// TestPipelineIdentitySuite reproduce la "Suite Identity" propuesta:
// preguntas de identidad deben seguir llegando a SelfKnowledgeSource
// aunque Memory Intelligence (MemorySource, prioridad más alta) esté
// registrada por delante en el Knowledge Engine.
func TestPipelineIdentitySuite(t *testing.T) {
	e := buildTestEngine(t)
	cases := []string{
		"preséntate",
		"quién eres",
		"quien te creo",
		"cuál es tu propósito",
		"qué puedes hacer",
	}
	for _, q := range cases {
		r := e.knowledgeEngine.Query(q)
		t.Logf("Q=%q -> source=%s kind=%s content=%q", q, r.Source, r.Kind, r.Content)
		if r.Content == "" {
			t.Errorf("Identity suite: %q no obtuvo respuesta (source=%s) — posible bloqueo del pipeline", q, r.Source)
		}
	}
}

// TestPipelineMemorySuite confirma que Memory Intelligence sigue
// respondiendo lo suyo sin robarse preguntas que no son de memoria.
func TestPipelineMemorySuite(t *testing.T) {
	e := buildTestEngine(t)
	cases := []string{
		"que hicimos",
		"que hice ayer",
	}
	for _, q := range cases {
		r := e.knowledgeEngine.Query(q)
		t.Logf("Q=%q -> source=%s kind=%s content=%q", q, r.Source, r.Kind, r.Content)
		if r.Source != "memory" {
			t.Errorf("Memory suite: %q fue respondida por %q, se esperaba 'memory'", q, r.Source)
		}
	}
}

// TestPipelineEnvironmentSuite confirma que las preguntas de entorno
// (qué tengo abierto, qué navegador, qué ventanas) llegan a
// EnvironmentSource y no se quedan atrapadas antes.
func TestPipelineEnvironmentSuite(t *testing.T) {
	e := buildTestEngine(t)
	cases := []string{
		"que tengo abierto",
		"que ventanas hay abiertas",
		"que aplicaciones hay abiertas",
		"que estas haciendo",
		"cual es tu contexto de trabajo",
	}
	for _, q := range cases {
		r := e.knowledgeEngine.Query(q)
		t.Logf("Q=%q -> source=%s kind=%s content=%q", q, r.Source, r.Kind, r.Content)
		if r.Content == "" {
			t.Errorf("Environment suite: %q no obtuvo respuesta (source=%s)", q, r.Source)
		}
		if r.Source != "environment" {
			t.Errorf("Environment suite: %q fue respondida por %q, se esperaba 'environment'", q, r.Source)
		}
	}
}

// TestPipelineMentalStateSuite confirma que el estado cognitivo vivo
// (Capa 5.5 — MentalAgendaV2) es consultable. Antes se actualizaba con
// eventos reales (Wonder/Observe/Doubt/Worry) pero era de solo
// escritura: nada lo exponía a una pregunta por voz.
func TestPipelineMentalStateSuite(t *testing.T) {
	e := buildTestEngine(t)
	// Generar algo de estado real antes de preguntar, igual que pasaría
	// en una sesión de verdad tras un par de preguntas.
	e.mentalAgenda.SetFocus("Prueba", "Suite de integración")
	e.mentalAgenda.Worry("Preocupación de prueba", 2)

	cases := []string{
		"como te sientes",
		"que te preocupa",
		"estado mental",
		"reporte mental",
	}
	for _, q := range cases {
		r := e.knowledgeEngine.Query(q)
		t.Logf("Q=%q -> source=%s kind=%s content=%q", q, r.Source, r.Kind, r.Content)
		if r.Source != "mental_state" {
			t.Errorf("MentalState suite: %q fue respondida por %q, se esperaba 'mental_state'", q, r.Source)
		}
	}
}

// TestPipelineNoCrossContamination verifica que preguntas de identidad
// no se cuelen por memory/environment antes de llegar a self-knowledge,
// y viceversa — el síntoma exacto que se reportó ("Ahmelia dejó de
// responder capacidades que ya tenía" tras integrar Memory Intelligence).
func TestPipelineNoCrossContamination(t *testing.T) {
	e := buildTestEngine(t)
	want := map[string]string{
		"preséntate":        "self",
		"quién eres":        "self",
		"que hicimos":       "memory",
		"que tengo abierto": "environment",
	}
	for q, wantSource := range want {
		r := e.knowledgeEngine.Query(q)
		if r.Source != wantSource {
			t.Errorf("%q -> source=%q, se esperaba %q (posible fuga entre módulos)", q, r.Source, wantSource)
		}
	}
}

func TestProcessConcurrentTurnsSerializeEngineOwnership(t *testing.T) {
	e := buildTestEngine(t)
	var wg sync.WaitGroup
	for _, input := range []string{"hola como estas", "¿cuál es mi color favorito?"} {
		wg.Add(1)
		go func(in string) {
			defer wg.Done()
			e.Process(in)
		}(input)
	}
	wg.Wait()
}
