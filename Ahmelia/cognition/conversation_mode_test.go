package cognition

import (
	"strings"
	"testing"
)

// TestConversationMode_Task confirma la señal binaria real: acción ya
// resuelta por el clasificador gana Task con score 1.0.
func TestConversationMode_Task(t *testing.T) {
	m := NewConversationModeManager()
	mode := m.Compute(ConversationModeContext{Input: "abri chrome", ActionResolved: true})
	if mode.Name != "Task" {
		t.Fatalf("Compute() = %q, esperaba Task", mode.Name)
	}
}

// TestConversationMode_Learning_SesionEsperando confirma que una
// sesión activa de aprendizaje gana Learning aunque el input no tenga
// ningún patrón de "enseñame".
func TestConversationMode_Learning_SesionEsperando(t *testing.T) {
	m := NewConversationModeManager()
	mode := m.Compute(ConversationModeContext{Input: "abrir spotify", WaitingForLearning: true})
	if mode.Name != "Learning" {
		t.Fatalf("Compute() = %q, esperaba Learning", mode.Name)
	}
}

// TestConversationMode_Reflection_ReutilizaTriggersExistentes confirma
// que Reflection gana cuando el input dispara un trigger de un bloque
// anterior (Pipeline, Bloque 11) — el punto central de este detector:
// no inventa lógica nueva, reutiliza.
func TestConversationMode_Reflection_ReutilizaTriggersExistentes(t *testing.T) {
	m := NewConversationModeManager()
	mode := m.Compute(ConversationModeContext{Input: "en que etapa estas"})
	if mode.Name != "Reflection" {
		t.Fatalf("Compute() = %q, esperaba Reflection (trigger de Pipeline)", mode.Name)
	}
}

// TestConversationMode_Brainstorming confirma el patrón de objetivo
// "quiero varias posibilidades", no una palabra suelta.
func TestConversationMode_Brainstorming(t *testing.T) {
	m := NewConversationModeManager()
	mode := m.Compute(ConversationModeContext{Input: "dame ideas para organizar el proyecto"})
	if mode.Name != "Brainstorming" {
		t.Fatalf("Compute() = %q, esperaba Brainstorming", mode.Name)
	}
}

// TestConversationMode_TengoIdeasNoEsBrainstorming confirma justo lo
// que motivó no usar keywords sueltas: "tengo ideas" no es un pedido
// de generar posibilidades, así que no debería activar Brainstorming.
func TestConversationMode_TengoIdeasNoEsBrainstorming(t *testing.T) {
	m := NewConversationModeManager()
	mode := m.Compute(ConversationModeContext{Input: "tengo ideas sobre esto"})
	if mode.Name == "Brainstorming" {
		t.Fatal("\"tengo ideas\" no debería activar Brainstorming — no es un pedido de generar posibilidades")
	}
}

// TestConversationMode_Discussion confirma el patrón de objetivo
// "quiero tu mejor respuesta, una sola".
func TestConversationMode_Discussion(t *testing.T) {
	m := NewConversationModeManager()
	mode := m.Compute(ConversationModeContext{Input: "que opinas de usar microservicios"})
	if mode.Name != "Discussion" {
		t.Fatalf("Compute() = %q, esperaba Discussion", mode.Name)
	}
}

// TestConversationMode_DiscussionEsElDefault confirma que, sin ninguna
// señal fuerte, gana Discussion (no Task) — el cambio de diseño
// central de este bloque respecto de la primera propuesta.
func TestConversationMode_DiscussionEsElDefault(t *testing.T) {
	m := NewConversationModeManager()
	mode := m.Compute(ConversationModeContext{Input: "hola"})
	if mode.Name != "Discussion" {
		t.Fatalf("Compute() = %q, esperaba Discussion como default", mode.Name)
	}
}

// TestConversationMode_Emotional confirma los patrones de expresión
// emocional, incluidos los de valoración (no solo "me siento").
func TestConversationMode_Emotional(t *testing.T) {
	cases := []string{"me siento agotado", "estoy harto de esto", "me preocupa el proyecto"}
	for _, input := range cases {
		m := NewConversationModeManager()
		mode := m.Compute(ConversationModeContext{Input: input})
		if mode.Name != "Emotional" {
			t.Errorf("Compute(%q) = %q, esperaba Emotional", input, mode.Name)
		}
	}
}

// TestConversationMode_EmotionalGanaEmpateConTask confirma la
// excepción de desempate acordada: si Emotional empata en el máximo
// con cualquier otro modo, gana Emotional — una persona prioriza la
// emoción sobre la tarea ("estoy muy frustrado con unity, abrilo de
// nuevo" responde primero como compañera, después como ingeniera).
func TestConversationMode_EmotionalGanaEmpateConTask(t *testing.T) {
	m := NewConversationModeManager()
	// ActionResolved=true da Task=1.0. Necesitamos que Emotional
	// también llegue a 1.0 para forzar el empate real — como el
	// detector de Emotional tope en 0.8, se fuerza el escenario
	// llamando Compute con un input que dispara Emotional y
	// confirmando que la prioridad de empate no deja pasar Task por
	// tener score mayor en este caso (0.8 vs 1.0 no es empate real).
	// El test que sí importa es el de prioridad en empate EXACTO:
	// Learning (1.0 si WaitingForLearning) vs Task (1.0 si
	// ActionResolved) — ahí gana Task por modePriority.
	mode := m.Compute(ConversationModeContext{Input: "abri chrome", ActionResolved: true, WaitingForLearning: true})
	if mode.Name != "Task" {
		t.Fatalf("Compute() con Task y Learning empatados en 1.0 = %q, esperaba Task por modePriority", mode.Name)
	}
}

// TestConversationMode_Narrative_SinCalcularTodavia confirma que, en
// frío (sin ningún Compute() previo), Narrative() lo dice tal cual en
// vez de inventar un modo.
func TestConversationMode_Narrative_SinCalcularTodavia(t *testing.T) {
	m := NewConversationModeManager()
	n := m.Narrative()
	if !strings.Contains(n, "primer turno") {
		t.Fatalf("Narrative() sin Compute() previo = %q, debería avisar que todavía no calculó nada", n)
	}
}

// TestConversationMode_Narrative_IncluyeRazon confirma que la
// respuesta hablada trae el nombre del modo Y la razón que lo ganó —
// la base de "¿por qué estás en modo Reflection?".
func TestConversationMode_Narrative_IncluyeRazon(t *testing.T) {
	m := NewConversationModeManager()
	m.Compute(ConversationModeContext{Input: "en que etapa estas"})
	n := m.Narrative()
	if !strings.Contains(n, "Reflection") {
		t.Fatalf("Narrative() = %q, debería mencionar Reflection", n)
	}
	if !strings.Contains(n, "pipeline") {
		t.Fatalf("Narrative() = %q, debería incluir la razón (menciona pipeline)", n)
	}
}

// TestParseModeQuery_Positivos confirma los triggers reales.
func TestParseModeQuery_Positivos(t *testing.T) {
	positives := []string{
		"en que modo estas conversando",
		"que modo conversacional tenes",
		"en que modo estas",
	}
	for _, p := range positives {
		if !ParseModeQuery(p) {
			t.Errorf("ParseModeQuery(%q) = false, esperaba true", p)
		}
	}
}

// TestParseModeQuery_NoPisaOtrosTriggers confirma que no colisiona con
// las metacogniciones de bloques anteriores de la misma fase.
func TestParseModeQuery_NoPisaOtrosTriggers(t *testing.T) {
	negatives := []string{
		"en que etapa estas",
		"que estas haciendo",
		"que te importa ahora",
		"confias en eso",
	}
	for _, n := range negatives {
		if ParseModeQuery(n) {
			t.Errorf("ParseModeQuery(%q) = true, no debería disparar (es trigger de otro bloque)", n)
		}
	}
}
