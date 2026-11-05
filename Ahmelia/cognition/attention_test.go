package cognition

import (
	"testing"
	"time"
)

// TestAttend_DescartaLoQueEstaPorDebajoDelUmbral confirma el caso
// central: una fuente con salience base baja (Aprendizajes, 0.35) y sin
// relación con el tema activo queda en discarded, no en attended.
func TestAttend_DescartaLoQueEstaPorDebajoDelUmbral(t *testing.T) {
	uc := &UnifiedContext{
		BuiltAt:  time.Now(),
		Learning: "adaptó la estrategia de abrir_programa",
	}
	m := NewManager()

	attended, discarded := m.Attend(uc, "")

	if len(attended) != 0 {
		t.Fatalf("Aprendizajes (salience 0.35, sin tema) debería quedar descartado, pero apareció en attended: %+v", attended)
	}
	if len(discarded) != 1 {
		t.Fatalf("esperaba exactamente 1 señal descartada, obtuve %d", len(discarded))
	}
}

// TestAttend_ConversacionYAhoraMismoSiempresPasan confirma que las
// fuentes con salience base alta (por encima del umbral por sí solas)
// pasan aunque no tengan relación con ningún tema activo.
func TestAttend_ConversacionYAhoraMismoSiemprePasan(t *testing.T) {
	uc := &UnifiedContext{
		BuiltAt:      time.Now(),
		World:        "Unity abierto, escena Map_Desert",
		Conversation: "hablando de optimizar el juego",
	}
	m := NewManager()

	attended, _ := m.Attend(uc, "")

	labels := map[string]bool{}
	for _, s := range attended {
		labels[s.Label] = true
	}
	if !labels["Ahora mismo"] || !labels["Conversación"] {
		t.Fatalf("Ahora mismo y Conversación deberían pasar el umbral por salience base sola, attended=%+v", attended)
	}
}

// TestAttend_TemaActivoSubeUnaSenalDebajoDelUmbral confirma el bonus de
// relevancia: una fuente que por salience base sola no pasaría el
// umbral, sí debería pasar si su contenido toca el tema activo de la
// conversación.
func TestAttend_TemaActivoSubeUnaSenalDebajoDelUmbral(t *testing.T) {
	uc := &UnifiedContext{
		BuiltAt: time.Now(),
		Habits:  "sueles abrir Unity antes de las 9am", // salience base 0.40, por debajo del umbral 0.45
	}
	m := NewManager()

	_, discardedSinTema := m.Attend(uc, "el clima de hoy")
	if len(discardedSinTema) != 1 {
		t.Fatalf("sin tema relacionado, Hábitos debería quedar descartado; discarded=%+v", discardedSinTema)
	}

	attendedConTema, _ := m.Attend(uc, "quiero hablar de Unity")
	if len(attendedConTema) != 1 || attendedConTema[0].Label != "Hábitos" {
		t.Fatalf("con tema activo relacionado (Unity), Hábitos debería subir de score y pasar el umbral; attended=%+v", attendedConTema)
	}
}

// TestAttend_RespetaMaxSignals confirma que aunque muchas señales pasen
// el umbral, Attend() no devuelve más de MaxSignals — y que lo que no
// entra pasa a discarded en vez de perderse silenciosamente.
func TestAttend_RespetaMaxSignals(t *testing.T) {
	uc := &UnifiedContext{
		BuiltAt:      time.Now(),
		World:        "Chrome y Unity abiertos",
		Conversation: "charla sobre el proyecto",
		Goals:        "terminar proyecto Unity",
		Reflection:   "va mejorando en compilaciones",
		Insights:     "trabaja más en Unity que en Python",
	}
	m := &Manager{Threshold: DefaultAttentionThreshold, MaxSignals: 2}

	attended, discarded := m.Attend(uc, "")

	if len(attended) != 2 {
		t.Fatalf("MaxSignals=2, esperaba exactamente 2 señales atendidas, obtuve %d: %+v", len(attended), attended)
	}
	if len(discarded) == 0 {
		t.Fatal("con 5 fuentes de salience alta y tope de 2, esperaba señales de sobrecupo en discarded")
	}
}

// TestNarrative_VacioYConContenido cubre los dos casos de salida de
// Narrative(): nada atendido, y una lista real.
func TestNarrative_VacioYConContenido(t *testing.T) {
	if n := Narrative(nil); n == "" {
		t.Fatal("Narrative(nil) no debería devolver string vacío — hay que decir explícitamente que no hay nada")
	}

	n := Narrative([]Signal{{Label: "Conversación", Content: "hablando de Unity", Score: 0.7}})
	if n != "Conversación: hablando de Unity" {
		t.Fatalf("Narrative() = %q, formato inesperado", n)
	}
}

// TestParseAttentionQuery_DetectaVariantes confirma el trigger, y que no
// se pisa con el de Context Query (Bloque 9) ni Pipeline Query (Bloque 11).
func TestParseAttentionQuery_DetectaVariantes(t *testing.T) {
	positive := []string{
		"que es importante ahora",
		"en que te estas enfocando",
		"a que le prestas atencion",
	}
	for _, q := range positive {
		if !ParseAttentionQuery(q) {
			t.Errorf("ParseAttentionQuery(%q) = false, esperaba true", q)
		}
	}

	negative := []string{
		"contexto completo",
		"en que etapa estas",
		"abre chrome",
	}
	for _, q := range negative {
		if ParseAttentionQuery(q) {
			t.Errorf("ParseAttentionQuery(%q) = true, esperaba false (choca con otro trigger)", q)
		}
	}
}
