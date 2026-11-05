package solving

import "testing"

// TestScoreReasoningNeed_SaludoNoActiva confirma el caso negativo
// central: un saludo corto sin dominio ni requisitos no llega al
// umbral — mismo ejemplo que se usó para diseñar el gate.
func TestScoreReasoningNeed_SaludoNoActiva(t *testing.T) {
	ga := &GoalAnalysis{Request: "hola", Domain: "general"}
	need := ScoreReasoningNeed(ga)

	if need.Score != 0 {
		t.Fatalf("Score = %d, esperaba 0", need.Score)
	}
	if need.ShouldActivate() {
		t.Fatal("ShouldActivate() = true para un saludo, esperaba false")
	}
}

// TestScoreReasoningNeed_DominioReconocidoActiva confirma que un
// dominio específico por sí solo alcanza el umbral (+2 == ReasoningThreshold).
func TestScoreReasoningNeed_DominioReconocidoActiva(t *testing.T) {
	ga := &GoalAnalysis{Request: "crea un juego", Domain: "videojuego"}
	need := ScoreReasoningNeed(ga)

	if need.Score != 2 {
		t.Fatalf("Score = %d, esperaba 2 (solo el dominio)", need.Score)
	}
	if !need.ShouldActivate() {
		t.Fatal("ShouldActivate() = false con dominio reconocido, esperaba true")
	}
}

// TestScoreReasoningNeed_DosRequirementsActivan confirma el ejemplo
// central del diseño: "necesito un sistema rápido y seguro" — dominio
// general, pero dos requisitos implícitos suman 2 igual.
func TestScoreReasoningNeed_DosRequirementsActivan(t *testing.T) {
	ga := &GoalAnalysis{
		Request: "necesito un sistema rapido y seguro",
		Domain:  "general",
		Requirements: []Requirement{
			{Description: "rapidez", Implicit: true},
			{Description: "seguridad", Implicit: true},
		},
	}
	need := ScoreReasoningNeed(ga)

	if need.Score != 2 {
		t.Fatalf("Score = %d, esperaba 2 (dos requisitos)", need.Score)
	}
	if !need.ShouldActivate() {
		t.Fatal("ShouldActivate() = false con dos requisitos, esperaba true")
	}
}

// TestScoreReasoningNeed_UnRequirementSoloNoActiva confirma que un
// solo requisito, sin dominio ni largo, se queda un punto por debajo
// del umbral — el gate no es demasiado sensible.
func TestScoreReasoningNeed_UnRequirementSoloNoActiva(t *testing.T) {
	ga := &GoalAnalysis{
		Request:      "hazlo rapido",
		Domain:       "general",
		Requirements: []Requirement{{Description: "rapidez", Implicit: true}},
	}
	need := ScoreReasoningNeed(ga)

	if need.ShouldActivate() {
		t.Fatalf("ShouldActivate() = true con Score=%d, esperaba false (por debajo del umbral)", need.Score)
	}
}

// TestScoreReasoningNeed_ObjetivoLargoSumaUnPunto confirma el
// reemplazo real de "más de un subobjetivo" (que no funcionaba, ver
// comentario en reasoning_gate.go): el largo del pedido original sí
// varía con el input.
func TestScoreReasoningNeed_ObjetivoLargoSumaUnPunto(t *testing.T) {
	corto := &GoalAnalysis{Request: "que hora es", Domain: "general"}
	largo := &GoalAnalysis{Request: "necesito ayuda para organizar todo el proyecto completo", Domain: "general"}

	scoreCorto := ScoreReasoningNeed(corto)
	scoreLargo := ScoreReasoningNeed(largo)

	if scoreCorto.Score != 0 {
		t.Fatalf("Score del pedido corto = %d, esperaba 0", scoreCorto.Score)
	}
	if scoreLargo.Score != 1 {
		t.Fatalf("Score del pedido largo = %d, esperaba 1 (8+ palabras)", scoreLargo.Score)
	}
}

// TestScoreReasoningNeed_Nil confirma el criterio defensivo estándar
// de Fase 3: ga nil no rompe.
func TestScoreReasoningNeed_Nil(t *testing.T) {
	need := ScoreReasoningNeed(nil)
	if need.Score != 0 || need.ShouldActivate() {
		t.Fatal("ScoreReasoningNeed(nil) debería dar Score=0 y ShouldActivate()=false, no romper")
	}
}

// TestDescribe_ListaLasSeñales confirma que Describe() no es solo el
// número — incluye la razón de cada punto sumado.
func TestDescribe_ListaLasSeñales(t *testing.T) {
	ga := &GoalAnalysis{Request: "crea un juego", Domain: "videojuego"}
	need := ScoreReasoningNeed(ga)

	desc := need.Describe()
	if desc == "" {
		t.Fatal("Describe() vacío")
	}
	found := false
	for _, s := range need.Signals {
		if s.Reason == "dominio reconocido: videojuego" {
			found = true
		}
	}
	if !found {
		t.Fatal("Signals no incluye la razón del dominio reconocido")
	}
}
