package core

// open_marks_guard_integration_test.go — integración del cableado completo
// del consumo B1/B2, sin red: el pipeline en modo abierto (cliente stub local)
// produce la lectura del enunciado en el camino sync de analyzeSemanticBoundary,
// el engine la adota con su Raw y la admisión consulta la guarda.
//
// Contraste interno en el mismo test: mismas condiciones SIN marcas capturan
// (control); CON marcas hipotéticas no capturan. Las frases del fixture son
// hipótesis conjeturales (familia de generadores), no las frases fijas del
// propietario (R11).

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"ahmelia/interaction"
	"ahmelia/linguistics"
)

// stubOpenLLM implementa LanguageClient + structuredOpenClient para el camino
// abierto sin tocar Ollama: devuelve la lectura canónica del stub, idéntica
// para toda llamada (el enunciado del turno es lo único que importa aquí).
type stubOpenLLM struct {
	raw string
}

func (s stubOpenLLM) Available() bool { return true }

func (s stubOpenLLM) Generate(prompt string) (string, error) { return s.raw, nil }

func (s stubOpenLLM) GenerateStructuredContext(ctx context.Context, prompt string, schema json.RawMessage, numPredict int) (string, error) {
	return s.raw, nil
}

func openMarksIntegrationStubRaw(withMarks bool) string {
	base := `{"purpose":"declaracion","owner_of_data":"user","topic":"bebida favorita",` +
		`"meaning":"El usuario conjetura que le gusta la gaseosa de piña (no es afirmación firme)",` +
		`"mentions":["gaseosa de piña"],"confidence":0.8`
	if withMarks {
		base += `,"marks":{"modality":"hypothesis"}`
	}
	return base + `}`
}

func TestOpenMarksGuardIntegrationSyncPath(t *testing.T) {
	// Candidatos conjeturales: el fixture elige el primero que cruza AMBAS
	// puertas — profundidad lingüística (camino sync) y admisión (respuesta
	// verificable). Si el clasificador cambia, el test lo dice con claridad.
	candidates := []string{
		"me gusta la gaseosa de piña, creo",
		"creo que me gusta la gaseosa de piña",
		"diría que me gusta la gaseosa de piña",
		"la gaseosa de piña me gusta, aunque no estoy seguro",
	}
	chosen := ""
	var chosenReq interaction.SemanticRequest
	for _, text := range candidates {
		r := interaction.Analyze(text, interaction.Context{})
		open := interaction.OpenQuestion{ID: 1, Status: interaction.OpenQuestionOpen, Category: "preferencia", Key: "bebida favorita", Question: "¿Me lo dices?", AskedTurn: 1}
		if _, accepted := interaction.VerifyAnswerToOpenQuestion(open, r, time.Now(), answerSocialDetector); accepted && interaction.NeedsLinguisticDepth(r) {
			chosen, chosenReq = text, r
			break
		}
	}
	if chosen == "" {
		t.Fatal("ningún candidato cruza profundidad lingüística + admisión: revisar el fixture")
	}
	t.Logf("candidato elegido: %q (conf=%.2f acts=%v refs=%v)", chosen, chosenReq.Confidence, chosenReq.CommunicativeActs, chosenReq.References)

	run := func(withMarks bool) (prefs int, openStatus interaction.OpenQuestionStatus, adoptedRaw string) {
		t.Helper()
		e := newInteractionE2EEngine(t)
		e.SetResponseCallback(func(string) {})
		interp := linguistics.NewInterpreter(stubOpenLLM{raw: openMarksIntegrationStubRaw(withMarks)})
		e.linguisticInterpreter = interp
		e.linguisticPipeline = linguistics.NewPipeline(linguistics.NewRouter(), interp, linguistics.NewValidator())
		e.EnableOpenComprehension()

		e.Process("sabes mi bebida favorita?")
		e.Process(chosen)

		if e.lastOpenMeaning != nil {
			adoptedRaw = strings.TrimSpace(e.lastOpenMeaning.Raw)
		}
		snap := e.conversationState.Snapshot()
		if len(snap.OpenQuestions) == 0 {
			t.Fatal("el fixture no registró pregunta abierta")
		}
		return len(scenePrefs(t)), snap.OpenQuestions[0].Status, adoptedRaw
	}

	// Con marcas PRIMERO: el almacén del test está limpio y la misma lectura
	// con modality=hypothesis bloquea la captura llana; la pregunta queda
	// abierta (la admisión no ocurre). Nada se escribe con la guarda activa.
	prefsM, statusM, adoptedM := run(true)
	if adoptedM != chosen {
		t.Fatalf("con marcas: el camino sync debió adotar %q; adoptó %q", chosen, adoptedM)
	}
	if prefsM != 0 {
		t.Fatalf("con marcas: la guarda debía impedir la captura; prefs=%d", prefsM)
	}
	if statusM != interaction.OpenQuestionOpen {
		t.Fatalf("con marcas: la pregunta debía seguir abierta, quedó %s", statusM)
	}

	// Control después: SIN marcas — el camino sync adota la lectura y la
	// admisión captura la respuesta como siempre (cero regresión por el
	// cableado). El almacén sigue limpio: la corrida con marcas no escribió.
	prefsC, statusC, adoptedC := run(false)
	if adoptedC != chosen {
		t.Fatalf("control: el camino sync debió adotar %q; adoptó %q", chosen, adoptedC)
	}
	if prefsC != 1 {
		t.Fatalf("control: sin marcas la captura legítima debe ocurrir (prefs=%d, status=%s)", prefsC, statusC)
	}
	if statusC != interaction.OpenQuestionAnswered {
		t.Fatalf("control: la pregunta debía quedar respondida, quedó %s", statusC)
	}
}
