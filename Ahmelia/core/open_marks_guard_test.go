package core

// open_marks_guard_test.go — guardas de la firma B1/B2 (consumo en core).
//
// R11: las frases exactas del propietario solo viven en tests/evidencia.
// Esta tabla es de MECANISMOS: verifica que la guarda solo puede prohibir la
// captura llana con las marcas firmadas, y que sin marcas el piso cerrado
// queda intacto (cero regresión).

import (
	"os"
	"strings"
	"testing"

	"ahmelia/interaction"
	"ahmelia/linguistics"
)

func TestOpenMarksGuardBlocksPlainCapture(t *testing.T) {
	cases := []struct {
		name        string
		marks       linguistics.OpenMarks
		wantBlocked bool
		wantReason  string
	}{
		{"sin marcas", linguistics.OpenMarks{}, false, ""},
		{"negación", linguistics.OpenMarks{Negation: &linguistics.OpenNegation{Scope: "accion", Target: "abrir el archivo"}}, true, "negation"},
		{"condición", linguistics.OpenMarks{Condition: &linguistics.OpenCondition{Antecedent: "termina la descarga", Consequent: "abre el documento"}}, true, "condition"},
		{"incompatibilidad", linguistics.OpenMarks{Incompatibility: []linguistics.OpenIncompatibility{{A: "nunca he estado allí", B: "fui ayer", Reason: "contradicción"}}}, true, "incompatibility"},
		{"modality=hypothesis", linguistics.OpenMarks{Modality: linguistics.ModalityHypothesis}, true, "modality=hypothesis"},
		{"modality=reported", linguistics.OpenMarks{Modality: linguistics.ModalityReported}, true, "modality=reported"},
		{"modality=observed", linguistics.OpenMarks{Modality: linguistics.ModalityObserved}, true, "modality=observed"},
		{"modality=historical", linguistics.OpenMarks{Modality: linguistics.ModalityHistorical}, true, "modality=historical"},
		{"modality=unknown", linguistics.OpenMarks{Modality: linguistics.ModalityUnknown}, true, "modality=unknown"},
		{"modality=affirmed", linguistics.OpenMarks{Modality: linguistics.ModalityAffirmed}, false, ""},
		{"modality vacía + provenance user (piso cerrado)", linguistics.OpenMarks{Provenance: linguistics.ProvenanceUser}, false, ""},
		{"provenance llm (no inhibe por esta guarda)", linguistics.OpenMarks{Provenance: linguistics.ProvenanceLLM}, false, ""},
		{"excepción sola (fuera de la firma de consumo)", linguistics.OpenMarks{Exception: &linguistics.OpenException{Set: "todos", Excluded: "visitantes"}}, false, ""},
		{"negación gana a modality afirmada", linguistics.OpenMarks{Negation: &linguistics.OpenNegation{Scope: "estado", Target: "gaseosa de piña"}, Modality: linguistics.ModalityAffirmed}, true, "negation"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, reason := openMarksBlockPlainCapture(c.marks)
			if got != c.wantBlocked {
				t.Fatalf("blocked=%v, esperaba %v (reason=%q)", got, c.wantBlocked, reason)
			}
			if reason != c.wantReason {
				t.Fatalf("reason=%q, esperaba %q", reason, c.wantReason)
			}
		})
	}
}

func TestOpenMarksGuardOnlyCurrentUtterance(t *testing.T) {
	negated := linguistics.OpenMeaning{
		Purpose:     linguistics.OpenPurposeDeclaration,
		OwnerOfData: linguistics.OpenOwnerUser,
		Mentions:    []string{"gaseosa de piña"},
		Marks:       linguistics.OpenMarks{Negation: &linguistics.OpenNegation{Scope: "estado", Target: "gaseosa de piña"}},
	}
	e := &Engine{}
	e.lastOpenMeaning = &openMeaningEvidence{Raw: "no me gusta la gaseosa de piña", Meaning: negated}

	turnOther := &processTurn{input: "otra frase", semanticReq: interaction.SemanticRequest{Raw: "otra frase"}}
	if blocked, why := e.plainCaptureBlockedByOpenMarks(turnOther); blocked {
		t.Fatalf("evidencia de otro enunciado no debe bloquear (reason=%q)", why)
	}

	turnSame := &processTurn{input: "no me gusta la gaseosa de piña", semanticReq: interaction.SemanticRequest{Raw: "no me gusta la gaseosa de piña"}}
	if blocked, why := e.plainCaptureBlockedByOpenMarks(turnSame); !blocked || why != "negation" {
		t.Fatalf("el enunciado con negación debe bloquear: blocked=%v reason=%q", blocked, why)
	}

	turnTrim := &processTurn{input: "  no me gusta la gaseosa de piña  ", semanticReq: interaction.SemanticRequest{Raw: " no me gusta la gaseosa de piña "}}
	if blocked, _ := e.plainCaptureBlockedByOpenMarks(turnTrim); !blocked {
		t.Fatal("el match por Raw debe tolerar espacios")
	}

	empty := &Engine{}
	if blocked, _ := empty.plainCaptureBlockedByOpenMarks(turnSame); blocked {
		t.Fatal("sin evidencia abierta no hay bloqueo")
	}
}

// E2E — la guarda bloquea la captura de un enunciado capturable cuando la
// lectura de ESE enunciado trae marcas no afirmadas, y la pregunta abierta
// queda abierta (la admisión no ocurre).
func TestOpenMarksGuardBlocksPlainCaptureE2E(t *testing.T) {
	e := newInteractionE2EEngine(t)
	e.SetResponseCallback(func(string) {})

	e.Process("sabes mi bebida favorita?")
	om := linguistics.OpenMeaning{
		Purpose:     linguistics.OpenPurposeDeclaration,
		OwnerOfData: linguistics.OpenOwnerUser,
		Mentions:    []string{"gaseosa de piña"},
		Marks:       linguistics.OpenMarks{Modality: linguistics.ModalityHypothesis},
	}
	e.adoptOpenMeaning("me gusta la gaseosa de piña", om)
	e.Process("me gusta la gaseosa de piña")

	if prefs := scenePrefs(t); len(prefs) != 0 {
		t.Fatalf("la guarda B2 debía impedir la captura llana; prefs=%+v", prefs)
	}
	snap := e.conversationState.Snapshot()
	if len(snap.OpenQuestions) != 1 || snap.OpenQuestions[0].Status != interaction.OpenQuestionOpen {
		t.Fatalf("la pregunta abierta no debe cerrarse cuando la guarda bloquea: %+v", snap.OpenQuestions)
	}
}

// E2E control — con marcas afirmadas (o sin marcas), la captura legítima
// ocurre igual que siempre: la guarda no rompe el camino honesto.
func TestOpenMarksGuardAllowsAffirmedCaptureE2E(t *testing.T) {
	e := newInteractionE2EEngine(t)
	e.SetResponseCallback(func(string) {})

	e.Process("sabes mi bebida favorita?")
	om := linguistics.OpenMeaning{
		Purpose:     linguistics.OpenPurposeDeclaration,
		OwnerOfData: linguistics.OpenOwnerUser,
		Mentions:    []string{"gaseosa de piña"},
		Marks:       linguistics.OpenMarks{Modality: linguistics.ModalityAffirmed, Provenance: linguistics.ProvenanceUser},
	}
	e.adoptOpenMeaning("me gusta la gaseosa de piña", om)
	e.Process("me gusta la gaseosa de piña")

	prefs := scenePrefs(t)
	if len(prefs) != 1 {
		t.Fatalf("marcas afirmadas no deben romper la captura legítima: prefs=%+v", prefs)
	}
	snap := e.conversationState.Snapshot()
	if len(snap.OpenQuestions) != 1 || snap.OpenQuestions[0].Status != interaction.OpenQuestionAnswered {
		t.Fatalf("la pregunta debía quedar respondida: %+v", snap.OpenQuestions)
	}
}

// Guarda de regresión: el consumo debe vivir en la admisión única. Si alguien
// la mueve o la borra, este test cae (patrón de las guardas H7).
func TestOpenMarksGuardWiredAtAdmission(t *testing.T) {
	src, err := os.ReadFile("engine.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	start := strings.Index(body, "func (e *Engine) captureVerifiedPersonalAnswer(")
	if start < 0 {
		t.Fatal("engine.go no contiene captureVerifiedPersonalAnswer")
	}
	body = body[start:]
	if next := strings.Index(body[1:], "\nfunc "); next > 0 {
		body = body[:next]
	}
	if !strings.Contains(body, "plainCaptureBlockedByOpenMarks(") {
		t.Fatal("la admisión única debe consultar la guarda B2 (plainCaptureBlockedByOpenMarks)")
	}

	guardSrc, err := os.ReadFile("open_marks_guard.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"func openMarksBlockPlainCapture(", "func (e *Engine) plainCaptureBlockedByOpenMarks("} {
		if !strings.Contains(string(guardSrc), want) {
			t.Fatalf("open_marks_guard.go no contiene %q", want)
		}
	}
}
