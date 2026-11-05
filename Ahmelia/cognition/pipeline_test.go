package cognition

import (
	"strings"
	"testing"
)

// TestPipeline_CurrentSigueLaUltimaEtapa confirma lo básico: Current()
// devuelve la última etapa marcada, no la primera ni un promedio.
func TestPipeline_CurrentSigueLaUltimaEtapa(t *testing.T) {
	p := NewPipeline()
	if p.Current() != "" {
		t.Fatalf("pipeline recién creado debería no tener etapa actual, obtuve %q", p.Current())
	}

	p.Enter(StageListen, "abrí chrome")
	p.Enter(StageComprehend, "clasificando")
	if p.Current() != StageComprehend {
		t.Fatalf("Current() = %q, esperaba %q", p.Current(), StageComprehend)
	}
}

// TestPipeline_LastPassCortaEnElUltimoListen confirma que LastPass() no
// devuelve TODO el historial acumulado de la sesión — solo la pasada más
// reciente, arrancando en el StageListen más reciente.
func TestPipeline_LastPassCortaEnElUltimoListen(t *testing.T) {
	p := NewPipeline()

	// Pasada 1 (input distinto, se resuelve en 3 etapas — como una
	// corrección o un saludo, que no llegan hasta Aprender/Experiencia).
	p.Enter(StageListen, "hola")
	p.Enter(StageComprehend, "hola")
	p.Enter(StageExecute, "saludo")

	// Pasada 2 (la que debería devolver LastPass).
	p.Enter(StageListen, "abre chrome")
	p.Enter(StageComprehend, "abre chrome")
	p.Enter(StageStrategy, "AdaptStrategy(abrir_programa)")
	p.Enter(StageExecute, "abrir_programa chrome")

	pass := p.LastPass()
	if len(pass) != 4 {
		t.Fatalf("LastPass() devolvió %d eventos, esperaba 4 (solo la segunda pasada)", len(pass))
	}
	if pass[0].Stage != StageListen || pass[0].Detail != "abre chrome" {
		t.Fatalf("LastPass()[0] = %+v, esperaba que arrancara en el StageListen de la segunda pasada", pass[0])
	}
	if pass[len(pass)-1].Stage != StageExecute {
		t.Fatalf("LastPass() debería terminar en StageExecute, terminó en %q", pass[len(pass)-1].Stage)
	}
}

// TestPipeline_TraceRecortaAlMaxHistory confirma que el trace no crece
// sin límite en sesiones largas.
func TestPipeline_TraceRecortaAlMaxHistory(t *testing.T) {
	p := NewPipeline()
	p.maxHistory = 5

	for i := 0; i < 20; i++ {
		p.Enter(StageListen, "input")
	}

	full := p.Trace(-1)
	if len(full) != 5 {
		t.Fatalf("Trace(-1) devolvió %d eventos, esperaba que el historial estuviera recortado a maxHistory=5, no a 20", len(full))
	}
}

// TestPipeline_NarrativeSinHistorial confirma el mensaje explícito
// cuando todavía no se procesó nada — mismo criterio que
// UnifiedContext.Narrative() con IsEmpty().
func TestPipeline_NarrativeSinHistorial(t *testing.T) {
	p := NewPipeline()
	n := p.Narrative()
	if !strings.Contains(n, "Todavía no procesé") {
		t.Fatalf("Narrative() sin historial = %q, esperaba que dijera explícitamente que no hay pasadas", n)
	}
}

// TestPipeline_NarrativeIncluyeLaCadenaDeEtapas confirma que Narrative()
// arma la cadena legible con flechas, no solo el nombre de la última
// etapa.
func TestPipeline_NarrativeIncluyeLaCadenaDeEtapas(t *testing.T) {
	p := NewPipeline()
	p.Enter(StageListen, "abre chrome")
	p.Enter(StageComprehend, "abre chrome")
	p.Enter(StageStrategy, "AdaptStrategy(abrir_programa)")
	p.Enter(StageExecute, "abrir_programa chrome")

	n := p.Narrative()
	for _, want := range []string{string(StageListen), string(StageComprehend), string(StageStrategy), string(StageExecute), "→"} {
		if !strings.Contains(n, want) {
			t.Fatalf("Narrative() = %q, esperaba que incluyera %q", n, want)
		}
	}
}

// TestParsePipelineQuery_DetectaVariantes cubre algunas de las frases que
// deberían disparar el trigger, y confirma que texto no relacionado no
// lo dispara por accidente (evitar falsos positivos, mismo criterio que
// ParseContextQuery).
func TestParsePipelineQuery_DetectaVariantes(t *testing.T) {
	positive := []string{
		"en que etapa estas",
		"muestrame el pipeline",
		"como procesas lo que digo",
	}
	for _, q := range positive {
		if !ParsePipelineQuery(q) {
			t.Errorf("ParsePipelineQuery(%q) = false, esperaba true", q)
		}
	}

	negative := []string{
		"abre chrome",
		"que hicimos ayer",
		"contexto completo",
	}
	for _, q := range negative {
		if ParsePipelineQuery(q) {
			t.Errorf("ParsePipelineQuery(%q) = true, esperaba false (falso positivo)", q)
		}
	}
}
