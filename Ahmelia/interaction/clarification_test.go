package interaction

import "testing"

func TestClarificationQueuePushAndOldest(t *testing.T) {
	q := NewClarificationQueue()
	if q.HasOpen() {
		t.Fatal("cola vacía no debería tener nada abierto")
	}
	q.Push("avanzar escena", "¿Priorizamos iluminación o interacción?", []string{"iluminación", "interacción"})
	pc := q.Oldest()
	if pc == nil {
		t.Fatal("se esperaba una aclaración abierta")
	}
	if pc.OriginalGoal != "avanzar escena" {
		t.Fatalf("OriginalGoal=%q", pc.OriginalGoal)
	}
	if pc.Status != ClarificationOpen {
		t.Fatalf("status=%q", pc.Status)
	}
}

func TestClarificationAnswersRecognizesShortReply(t *testing.T) {
	pc := &PendingClarification{OriginalGoal: "avanzar escena", Assumptions: []string{"iluminación", "interacción"}}
	if !pc.Answers(KindConversation, "la interacción") {
		t.Fatal("una respuesta corta debería contar como respuesta a la aclaración")
	}
	if !pc.Answers(KindUnknown, "la interacción del personaje, por favor") {
		// 6 palabras o menos sigue contando como respuesta directa.
		t.Fatal("una frase corta que confirma la opción debería contar como respuesta")
	}
}

func TestClarificationAnswersRejectsNewProductionOrAction(t *testing.T) {
	pc := &PendingClarification{OriginalGoal: "avanzar escena", Assumptions: []string{"iluminación", "interacción"}}
	if pc.Answers(KindProduction, "crea un programa llamado prueba") {
		t.Fatal("una producción nueva nunca debe tratarse como respuesta a la aclaración pendiente")
	}
	if pc.Answers(KindAction, "abre chrome") {
		t.Fatal("una acción nueva nunca debe tratarse como respuesta a la aclaración pendiente")
	}
}

func TestClarificationQueueSurvivesInterruptionAndResumesInOrder(t *testing.T) {
	q := NewClarificationQueue()
	firstID := q.Push("avanzar escena", "¿Priorizamos iluminación o interacción?", []string{"iluminación", "interacción"})

	// El usuario se desvía: esto no debe descartar la pregunta.
	pc := q.Oldest()
	if pc.Answers(KindAction, "abre chrome") {
		t.Fatal("'abre chrome' no responde la pregunta de la escena")
	}
	if !q.HasOpen() {
		t.Fatal("la aclaración debe seguir abierta tras la interrupción")
	}

	// Se acumula una segunda pregunta mientras la primera sigue pendiente.
	secondID := q.Push("otra cosa", "¿Confirmas la ruta B?", nil)
	if firstID == secondID {
		t.Fatal("cada push debe tener un id distinto")
	}

	// Se retoma la más antigua primero.
	oldest := q.Oldest()
	if oldest.ID() != firstID {
		t.Fatalf("se esperaba retomar la más antigua (id=%d), se obtuvo id=%d", firstID, oldest.ID())
	}

	resolved := q.Resolve(oldest.ID())
	if resolved == nil || resolved.OriginalGoal != "avanzar escena" {
		t.Fatalf("resolve inesperado: %+v", resolved)
	}
	merged := resolved.Merge("la interacción", false)
	if merged != "avanzar escena la interacción" {
		t.Fatalf("merge=%q", merged)
	}

	// Ahora debe quedar la segunda como la abierta.
	next := q.Oldest()
	if next == nil || next.ID() != secondID {
		t.Fatal("tras resolver la primera, la segunda debe quedar como la abierta")
	}
}

func TestClarificationMergeKeepsGoalOnBareConfirmation(t *testing.T) {
	pc := &PendingClarification{OriginalGoal: "avanzar escena"}
	if got := pc.Merge("sí", true); got != "avanzar escena" {
		t.Fatalf("confirmación pura no debería ensuciar el objetivo: %q", got)
	}
	if got := pc.Merge("la interacción", false); got != "avanzar escena la interacción" {
		t.Fatalf("merge=%q", got)
	}
}
