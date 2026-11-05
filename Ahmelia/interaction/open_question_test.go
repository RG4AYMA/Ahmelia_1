package interaction

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ahmelia/socialinteraction"
)

func sceneOpenQuestion() OpenQuestion {
	return OpenQuestion{
		ID: 1, Kind: OpenQuestionKindPersonalValue, Category: "preference",
		Key: "bebida favorita", Question: "¿Me lo dices?", AskedTurn: 1,
		AskedAt: time.Now(), Status: OpenQuestionOpen,
	}
}

// testSocialDetector mirrors the runtime wiring: verification consumes the
// canonical social layer through its seam (core injects it in production);
// the B96.32.15 import audit deliberately skips _test.go files.
type testSocialDetector struct{}

func (testSocialDetector) IsSocial(text string) bool {
	return socialinteraction.NewIntentDetector().Detect(text, nil) != socialinteraction.UnknownIntent
}

// Frases exactas del propietario (R11): solo viven en tests y evidencia.
func TestVerifyAnswerToOpenQuestion_ScenePhrases(t *testing.T) {
	now := time.Now()
	q := sceneOpenQuestion()

	accept := []struct {
		text    string
		minConf float64
	}{
		{"me gusta la gaseosa de piña", .90},
		{"café", .82},
		{"la gaseosa de piña", .82},
		{"el café", .82},
		{"es la gaseosa de piña", .82},
		{"me gustan las hamburguesas", .86},
		{"prefiero el té", .86},
	}
	for _, c := range accept {
		req := Analyze(c.text, Context{})
		req = InferSemanticRelations(req)
		answer, ok := VerifyAnswerToOpenQuestion(q, req, now, testSocialDetector{})
		if !ok {
			t.Errorf("debió aceptar %q", c.text)
			continue
		}
		if answer.Key != q.Key || answer.Category != q.Category {
			t.Errorf("%q: key=%q category=%q, esperaba key=%q category=%q", c.text, answer.Key, answer.Category, q.Key, q.Category)
		}
		if answer.Value != strings.TrimSpace(c.text) {
			t.Errorf("%q: valor verbatim inesperado %q", c.text, answer.Value)
		}
		if answer.Confidence < c.minConf {
			t.Errorf("%q: confianza %.2f < %.2f", c.text, answer.Confidence, c.minConf)
		}
	}

	reject := []string{
		"que onda",
		"sabes mi bebida favorita?",
		"por qué no?",
		"abre chrome",
		"gracias",
		"jajaja",
		"sí",
		"ok",
		"no sé",
		"no me gusta el café",
		"me gusta",
		"mi bebida favorita es la gaseosa de piña", // declaración directa: la captura el camino de declaraciones
	}
	for _, text := range reject {
		req := Analyze(text, Context{})
		req = InferSemanticRelations(req)
		if _, ok := VerifyAnswerToOpenQuestion(q, req, now, testSocialDetector{}); ok {
			t.Errorf("no debió aceptar %q", text)
		}
	}
}

// El veto social es un contrato del seam: sin capa social la verificación
// falla cerrada; con la capa inyectada, lo social nunca se vuelve valor.
type flagSocialDetector bool

func (f flagSocialDetector) IsSocial(string) bool { return bool(f) }

func TestVerifyAnswerToOpenQuestion_SocialSeam(t *testing.T) {
	now := time.Now()
	q := sceneOpenQuestion()
	req := Analyze("me gusta la gaseosa de piña", Context{})
	req = InferSemanticRelations(req)

	if _, ok := VerifyAnswerToOpenQuestion(q, req, now, nil); ok {
		t.Error("sin capa social inyectada la verificación no puede atestiguar: debió rechazar")
	}
	if _, ok := VerifyAnswerToOpenQuestion(q, req, now, flagSocialDetector(true)); ok {
		t.Error("un enunciado social no es un valor")
	}
	if _, ok := VerifyAnswerToOpenQuestion(q, req, now, testSocialDetector{}); !ok {
		t.Error("con la capa social real y sin veto, el predicado debe aceptarse")
	}
}

func TestVerifyAnswerToOpenQuestion_Guards(t *testing.T) {
	now := time.Now()

	answered := sceneOpenQuestion()
	answered.Status = OpenQuestionAnswered
	if _, ok := VerifyAnswerToOpenQuestion(answered, Analyze("café", Context{}), now, testSocialDetector{}); ok {
		t.Error("una pregunta cerrada no admite respuestas")
	}

	noKey := sceneOpenQuestion()
	noKey.Key = ""
	if _, ok := VerifyAnswerToOpenQuestion(noKey, Analyze("café", Context{}), now, testSocialDetector{}); ok {
		t.Error("una pregunta sin clave no admite respuestas")
	}

	empty := sceneOpenQuestion()
	if _, ok := VerifyAnswerToOpenQuestion(empty, SemanticRequest{Raw: "   "}, now, testSocialDetector{}); ok {
		t.Error("un turno vacío no es respuesta")
	}
}

func TestOpenQuestionRegistryLifecycle(t *testing.T) {
	s := NewConversationState()
	now := time.Now()

	id := s.RegisterOpenQuestion(OpenQuestionKindPersonalValue, "preference", "bebida favorita", "¿Me lo dices?", 1)
	if id == 0 {
		t.Fatal("no se registró la pregunta abierta")
	}
	q, ok := s.ActiveOpenQuestion(now, 2)
	if !ok || q.Key != "bebida favorita" || q.Status != OpenQuestionOpen || q.Category != "preference" {
		t.Fatalf("pregunta activa inesperada: %+v (ok=%v)", q, ok)
	}

	// Re-preguntar cancela la anterior y solo la última admite respuesta.
	id2 := s.RegisterOpenQuestion(OpenQuestionKindPersonalValue, "preference", "bebida favorita", "¿Me lo dices?", 4)
	if id2 <= id {
		t.Fatalf("id2=%d debe ser mayor que id=%d", id2, id)
	}
	if s.OpenQuestions[0].Status != OpenQuestionCancelled {
		t.Errorf("la anterior debió quedar cancelada, estado=%q", s.OpenQuestions[0].Status)
	}
	active, ok := s.ActiveOpenQuestion(now, 5)
	if !ok || active.ID != id2 {
		t.Fatalf("activa=%+v, esperaba la última (id=%d)", active, id2)
	}

	// Responder cierra.
	s.CloseOpenQuestion(id2, OpenQuestionAnswered, 5)
	if _, ok := s.ActiveOpenQuestion(now, 6); ok {
		t.Error("una pregunta respondida no debe seguir activa")
	}
}

func TestOpenQuestionExpiry(t *testing.T) {
	now := time.Now()

	// Ventana de turnos.
	s := NewConversationState()
	s.RegisterOpenQuestion(OpenQuestionKindPersonalValue, "preference", "bebida favorita", "q", 1)
	if _, ok := s.ActiveOpenQuestion(now, 1+OpenQuestionMaxTurnDistance); !ok {
		t.Error("debe seguir activa dentro de la ventana de turnos")
	}
	if _, ok := s.ActiveOpenQuestion(now, 1+OpenQuestionMaxTurnDistance+1); ok {
		t.Error("debe vencer fuera de la ventana de turnos")
	}
	if s.OpenQuestions[0].Status != OpenQuestionExpired {
		t.Errorf("estado=%q, esperaba expired", s.OpenQuestions[0].Status)
	}

	// Edad.
	s2 := NewConversationState()
	s2.RegisterOpenQuestion(OpenQuestionKindPersonalValue, "preference", "bebida favorita", "q", 5)
	s2.OpenQuestions[0].AskedAt = now.Add(-OpenQuestionMaxAge - time.Minute)
	if _, ok := s2.ActiveOpenQuestion(now, 6); ok {
		t.Error("debe vencer por edad")
	}

	// Otra sesión.
	s3 := NewConversationState()
	s3.RegisterOpenQuestion(OpenQuestionKindPersonalValue, "preference", "bebida favorita", "q", 5)
	s3.SessionID = "otra-sesion"
	if _, ok := s3.ActiveOpenQuestion(now, 6); ok {
		t.Error("una pregunta de otra sesión no debe seguir activa")
	}
}

func TestOpenQuestionSaveLoadRoundtrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "conversation_state.json")
	s := NewConversationState()
	s.RegisterOpenQuestion(OpenQuestionKindPersonalValue, "preference", "bebida favorita", "¿Me lo dices?", 3)
	if err := s.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded := LoadConversationState(path)
	if len(loaded.OpenQuestions) != 1 {
		t.Fatalf("preguntas cargadas=%d, esperaba 1", len(loaded.OpenQuestions))
	}
	q := loaded.OpenQuestions[0]
	if q.Key != "bebida favorita" || q.Status != OpenQuestionOpen || q.AskedTurn != 3 {
		t.Fatalf("pregunta cargada inesperada: %+v", q)
	}
	if loaded.NextOpenQuestionID != s.NextOpenQuestionID {
		t.Errorf("contador=%d, esperaba %d", loaded.NextOpenQuestionID, s.NextOpenQuestionID)
	}
	if _, ok := loaded.ActiveOpenQuestion(time.Now(), 4); !ok {
		t.Error("la pregunta cargada debe seguir activa")
	}
}

func TestPersonalAnswerToUserObservationCarriesEvidenceTurn(t *testing.T) {
	obs := PersonalAnswerToUserObservation(
		PersonalAnswer{OK: true, Category: "preference", Key: "bebida favorita", Value: "café", Confidence: .82},
		time.Now(), "turn-2", "turn-1",
	)
	if obs.Domain != UserModelDomainPreference || obs.Subject != UserObservationSubjectUser {
		t.Fatalf("observación inesperada: %+v", obs)
	}
	if obs.Provenance.SourceID != "turn-2" || obs.Provenance.EvidenceID != "turn-1" {
		t.Fatalf("procedencia inesperada: %+v", obs.Provenance)
	}

	attr := PersonalAnswerToUserObservation(
		PersonalAnswer{OK: true, Category: "attribute", Key: "ciudad", Value: "Medellín", Confidence: .82},
		time.Now(), "turn-4", "turn-3",
	)
	if attr.Domain != UserModelDomainAttribute || attr.Key != "ciudad" || attr.Value != "Medellín" {
		t.Fatalf("atributo inesperado: %+v", attr)
	}
}
