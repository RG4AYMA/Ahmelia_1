package cognition

import (
	"testing"

	"ahmelia/comprehension"
	"ahmelia/environment"
	"ahmelia/memory"
	memoryintelligence "ahmelia/memory_intelligence"
	"ahmelia/planning"
	"ahmelia/reasoning_deep"
)

// TestBuild_VacioSinDatos simula la primera sesión: ninguna fuente tiene
// nada que aportar todavía. Build() no debe fallar ni panicar, y
// Narrative() debe decirlo explícito en vez de devolver un string vacío
// (que se leería como que Ahmelia no entendió la pregunta).
func TestBuild_VacioSinDatos(t *testing.T) {
	b := NewBuilder(
		environment.NewManager(),
		memoryintelligence.NewEventStore(&memoryintelligence.EventLog{}),
		&memory.LongMemory{Data: map[string]*memory.Knowledge{}},
		planning.NewAgendaManager(),
		reasoning_deep.NewMentalAgendaV2(),
		comprehension.NewConversationContext(),
		memoryintelligence.NewLearningStore(memoryintelligence.NewLearningState()),
	)

	uc := b.Build()
	if uc == nil {
		t.Fatal("Build() no debería devolver nil")
	}

	narrative := uc.Narrative()
	if narrative == "" {
		t.Fatal("Narrative() no debería devolver string vacío, ni siquiera sin datos")
	}
}

// TestBuild_ConEventLogPobla verifica que, con eventos reales en el
// EventLog, al menos una de las fuentes derivadas (Insights/Reflection)
// deja de estar vacía — prueba que Build() de verdad está consultando
// memory_intelligence, no devolviendo un objeto muerto.
func TestBuild_ConEventLogPobla(t *testing.T) {
	log := &memoryintelligence.EventLog{}
	for i := 0; i < 5; i++ {
		log.Events = append(log.Events, memoryintelligence.RawEvent{
			Action:  "abrir_programa",
			Target:  "chrome",
			Outcome: memoryintelligence.Outcome{Success: true},
		})
	}

	b := NewBuilder(
		environment.NewManager(),
		memoryintelligence.NewEventStore(log),
		&memory.LongMemory{Data: map[string]*memory.Knowledge{}},
		planning.NewAgendaManager(),
		reasoning_deep.NewMentalAgendaV2(),
		comprehension.NewConversationContext(),
		memoryintelligence.NewLearningStore(memoryintelligence.NewLearningState()),
	)

	uc := b.Build()
	if uc.IsEmpty() {
		t.Fatal("con 5 eventos exitosos registrados, el contexto no debería salir completamente vacío")
	}
}

// TestParseContextQuery_DetectaPanoramaCompleto confirma los triggers
// principales sin falsos positivos sobre preguntas puntuales ya
// resueltas por otros Parse*Query (Habits, Timeline, etc).
func TestParseContextQuery_DetectaPanoramaCompleto(t *testing.T) {
	positivos := []string{
		"dame el contexto completo",
		"quiero el panorama general",
		"ponme al dia de todo",
	}
	for _, p := range positivos {
		if !ParseContextQuery(p) {
			t.Errorf("esperaba detectar %q como pedido de contexto completo", p)
		}
	}

	negativos := []string{
		"que hicimos hoy",
		"como te va",
		"cuales son tus habitos",
	}
	for _, n := range negativos {
		if ParseContextQuery(n) {
			t.Errorf("%q no debería disparar el contexto completo (es una pregunta puntual de otro módulo)", n)
		}
	}
}
