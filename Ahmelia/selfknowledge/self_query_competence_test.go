package selfknowledge

import (
	"strings"
	"testing"
	"time"

	"ahmelia/competence"
)

func attemptRef(capability, context string, verified bool, minute int) competence.Attempt {
	return competence.Attempt{
		Capability: capability,
		Operation:  "op",
		ContextKey: context,
		Strategy:   "proc:" + capability + "@1",
		Verified:   verified,
		At:         time.Unix(1700000000+int64(minute), 0),
	}
}

func inMemoryLedger(t *testing.T, attempts ...competence.Attempt) *competence.Ledger {
	t.Helper()
	l, err := competence.Load("") // en memoria, sin escrituras en disco
	if err != nil {
		t.Fatalf("in-memory ledger: %v", err)
	}
	for _, a := range attempts {
		if err := l.Record(a); err != nil {
			t.Fatalf("record: %v", err)
		}
	}
	return l
}

// TestCapabilitiesQuestionAnswersWithDerivedStanding es la línea de
// aceptación T4 del plan: "¿Qué sabes hacer?" presenta Standing derivado de
// evidencia Y el piso híbrido (directiva DeepSeek Paso 1.1, 19/10): la lista
// estática de identidad NO desaparece en silencio — sube explícitamente
// rotulada como declaración, distinta de lo verificado.
func TestCapabilitiesQuestionAnswersWithDerivedStanding(t *testing.T) {
	ledger := inMemoryLedger(t, attemptRef("media.organize", "projectA", true, 0))
	overlay := competence.NewOverlay(ledger, nil, nil, competence.DefaultPolicy())

	sq := NewSelfQuery(NewActionHistory(50), NewModuleRegistry(), "v3.1")
	sq.AttachCompetence(overlay)

	answer, ok := sq.Query("¿Qué sabes hacer?")
	if !ok {
		t.Fatal("capabilities question must be understood")
	}
	if !strings.Contains(answer, "unverifiable") {
		t.Fatalf("answer must present the derived standing (no oracle declared): %q", answer)
	}
	if strings.Contains(strings.ToLower(answer), "puedo afirmar") {
		t.Fatalf("unverifiable must not be phrased as an affirmed capability: %q", answer)
	}
	if !strings.Contains(answer, "media.organize") && !strings.Contains(answer, "evidencia") {
		t.Fatalf("answer must reference the derived capability assessment: %q", answer)
	}
	if !strings.Contains(answer, "Puedo (declarado en mi identidad)") {
		t.Fatalf("hybrid floor: the declared identity list must stay visible as a declaration: %q", answer)
	}
	if !strings.Contains(answer, "sin evidencia de práctica que lo respalde") {
		t.Fatalf("hybrid floor: the declared part must not read as verified evidence: %q", answer)
	}
}

// TestCapabilitiesQuestionWithoutEvidenceIsHonest: contrato T4 con piso
// híbrido (directiva DeepSeek Paso 1.1, 19/10) — sin evidencia la respuesta
// CONSERVA la lista declarada de identidad con aviso claro de que es
// declaración, no evidencia; nunca inventa capacidades ni las presenta como
// verificadas.
func TestCapabilitiesQuestionWithoutEvidenceIsHonest(t *testing.T) {
	overlay := competence.NewOverlay(inMemoryLedger(t), nil, nil, competence.DefaultPolicy())

	sq := NewSelfQuery(NewActionHistory(50), NewModuleRegistry(), "v3.1")
	sq.AttachCompetence(overlay)

	answer, ok := sq.Query("¿Qué sabes hacer?")
	if !ok {
		t.Fatal("capabilities question must be understood even without evidence")
	}
	if !strings.Contains(answer, "Puedo (declarado en mi identidad)") {
		t.Fatalf("hybrid floor: the static identity list must stay visible as declared: %q", answer)
	}
	if !strings.Contains(answer, "no tengo práctica verificada que lo respalde") {
		t.Fatalf("honest answer must separate the declaration from verified practice: %q", answer)
	}
	if !strings.Contains(strings.ToLower(answer), "declarado") {
		t.Fatalf("honest floor must label the list as declared, not verified: %q", answer)
	}
}

// TestAttachedOverlayNeverTouchesOtherSelfAnswers: el overlay solo cambia la
// respuesta de capacidades; el resto de las consultas de self quedan intactas.
func TestAttachedOverlayNeverTouchesOtherSelfAnswers(t *testing.T) {
	overlay := competence.NewOverlay(inMemoryLedger(t, attemptRef("media.organize", "projectA", true, 0)), nil, nil, competence.DefaultPolicy())

	sq := NewSelfQuery(NewActionHistory(50), NewModuleRegistry(), "v3.1")
	sq.AttachCompetence(overlay)

	// "quién eres" no es de competencia: sigue por identidad/atributos.
	who, ok := sq.Query("¿Quién eres?")
	if !ok {
		t.Fatal("identity question must still be understood with an attached overlay")
	}
	if strings.Contains(who, "unverifiable") {
		t.Fatalf("overlay must not leak into identity answers: %q", who)
	}

	// Pregunta de atributo propio (contrato T2) sigue idéntica.
	color, ok := sq.Query("¿Tú tienes color favorito?")
	if !ok || color != "No tengo un color favorito propio registrado todavía." {
		t.Fatalf("T2 self attribute contract must stay untouched: ok=%v %q", ok, color)
	}
}

// TestUnattachedSelfQueryKeepsStaticBehavior: guard de compatibilidad — sin
// overlay adjunto, la respuesta estática previa continúa (cambio inerte).
func TestUnattachedSelfQueryKeepsStaticBehavior(t *testing.T) {
	sq := NewSelfQuery(NewActionHistory(50), NewModuleRegistry(), "v3.1")

	answer, ok := sq.Query("¿Qué sabes hacer?")
	if !ok {
		t.Fatal("capabilities question must be understood")
	}
	if !strings.HasPrefix(answer, "Puedo: ") {
		t.Fatalf("static behavior must remain until integration: %q", answer)
	}
}
