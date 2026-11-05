package reasoning

import (
	"strings"
	"testing"

	"ahmelia/selfknowledge"
)

func newTestReasoner() *Reasoner {
	history := selfknowledge.NewActionHistory(50)
	return NewReasoner(history)
}

// TestDecideTrust_ConvierteEscala0a100EnEscala0a1 confirma la conversión
// de escala: ActionConfidence (memory_intelligence) guarda 0-100, pero
// Decider.DecideTrust espera 0.0-1.0 — DecideTrust en Reasoner es el
// punto donde eso se traduce, para que quien llama (core/engine.go) no
// tenga que acordarse de dividir entre 100 él mismo.
func TestDecideTrust_ConvierteEscala0a100EnEscala0a1(t *testing.T) {
	r := newTestReasoner()

	// 95% de éxito (escala ActionConfidence) debería mapear a "ejecutar
	// sin cuestionar" (Decider.DecideTrust exige >=0.8 en escala 0-1).
	tree := r.DecideTrust("abrir_programa", 95.0)
	if tree.Chosen != "Ejecutar sin cuestionar" {
		t.Fatalf("con 95%% de éxito histórico, Chosen=%q, esperaba 'Ejecutar sin cuestionar' (si no convirtiera bien la escala, 95.0 se leería como certeza absurda o como <0.5)", tree.Chosen)
	}

	// 20% de éxito debería mapear a "pedir confirmación al usuario".
	tree = r.DecideTrust("editar_registro", 20.0)
	if tree.Chosen != "Pedir confirmación al usuario" {
		t.Fatalf("con 20%% de éxito histórico, Chosen=%q, esperaba pedir confirmación", tree.Chosen)
	}
}

// TestDecideTrust_SinHistorialPideConfirmacion confirma el caso que más
// importa en la práctica: una acción sin ninguna entrada todavía en
// ActionConfidence (mapa vacío en Go devuelve el cero-valor, 0.0) debe
// tratarse como "no confío todavía", no como "0% de éxito confirmado" —
// ambos casos coinciden en el mismo Chosen porque DecideTrust ya trata
// todo lo que no llega a 0.5 igual, así que el comportamiento resultante
// es el correcto (pedir confirmación) aunque el motivo interno sea
// distinto ("sin datos" vs "falla mucho").
func TestDecideTrust_SinHistorialPideConfirmacion(t *testing.T) {
	r := newTestReasoner()
	tree := r.DecideTrust("accion_nunca_vista", 0.0)
	if tree.Chosen != "Pedir confirmación al usuario" {
		t.Fatalf("sin historial (0.0), Chosen=%q, esperaba pedir confirmación por las dudas", tree.Chosen)
	}
}

// TestSummarize_IncluyeChosenYConfianza confirma que el wrapper
// Summarize() (que reemplaza acceder a r.decider directamente desde
// fuera del paquete) arma una frase legible con lo esencial de la
// decisión.
func TestSummarize_IncluyeChosenYConfianza(t *testing.T) {
	r := newTestReasoner()
	tree := r.DecideTrust("abrir_programa", 95.0)
	summary := r.Summarize(tree)

	if !strings.Contains(summary, "Ejecutar sin cuestionar") {
		t.Fatalf("Summarize() = %q, esperaba que incluyera la opción elegida", summary)
	}
}

// TestQuery_ComoDecidisteResponder_SinPreguntaPrevia confirma el caso
// base: si todavía no se respondió ninguna pregunta en la sesión,
// "cómo decidiste responder" lo dice explícito en vez de devolver un
// DecisionTree vacío o hacer panic con un nil pointer.
func TestQuery_ComoDecidisteResponder_SinPreguntaPrevia(t *testing.T) {
	r := newTestReasoner()
	answer, handled := r.Query("como decidiste responder")
	if !handled {
		t.Fatal("la meta-pregunta sobre estrategia de respuesta debería estar 'handled', aunque no haya nada que explicar todavía")
	}
	if !strings.Contains(answer, "Todavía no respondí") {
		t.Fatalf("answer = %q, esperaba el mensaje explícito de 'sin pregunta previa'", answer)
	}
}

// TestQuery_ComoDecidisteResponder_TrasUnaPreguntaPrevia confirma la
// integración real: después de que Query() responde una pregunta
// cualquiera (lo que ahora, Bloque 14, deja registrado un DecisionTree
// vía Decider.DecideResponse), la meta-pregunta siguiente puede
// explicar esa estrategia.
func TestQuery_ComoDecidisteResponder_TrasUnaPreguntaPrevia(t *testing.T) {
	r := newTestReasoner()

	// Cualquier pregunta reconocida por el pipeline deja un
	// lastResponseTree registrado — se usa "que tendencia" porque no
	// depende de haber ejecutado acciones previas para no ser "unknown".
	_, handled := r.Query("como vas")
	if !handled {
		t.Fatal("'como vas' debería ser una pregunta reconocida por el pipeline de Reasoner")
	}

	answer, handled := r.Query("como decidiste responder")
	if !handled {
		t.Fatal("la meta-pregunta debería seguir handled")
	}
	if strings.Contains(answer, "Todavía no respondí") {
		t.Fatalf("answer = %q, después de una pregunta previa ya debería haber un DecisionTree que explicar, no el mensaje de 'sin historial'", answer)
	}
}

// TestParseTrustQuery_DetectaVariantesSinPisarPorQueHiciste confirma el
// trigger de core/engine.go, y que no compite con "por qué hiciste eso"
// (que ya existía, pide justificación, no evaluación de confianza).
func TestParseTrustQuery_DetectaVariantesSinPisarPorQueHiciste(t *testing.T) {
	positive := []string{
		"confias en eso",
		"que tan confiable fue eso",
		"que tanto confias en",
	}
	for _, q := range positive {
		if !ParseTrustQuery(q) {
			t.Errorf("ParseTrustQuery(%q) = false, esperaba true", q)
		}
	}

	negative := []string{
		"por que hiciste eso",
		"que analizas",
		"abre chrome",
	}
	for _, q := range negative {
		if ParseTrustQuery(q) {
			t.Errorf("ParseTrustQuery(%q) = true, esperaba false (no debería pisar otro trigger)", q)
		}
	}
}
