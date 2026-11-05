package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ahmelia/expectation"
	"ahmelia/memory"
	"ahmelia/production"
	"ahmelia/selfknowledge"
)

func newExpectationBaseEngine(t *testing.T) *Engine {
	t.Helper()
	t.Setenv("AHMELIA_DISABLE_TTS", "1")
	t.Setenv("AHMELIA_SKIP_PROGRAM_INDEX_SCAN", "1")
	sess := memory.NewSession()
	shortMem := LoadShortMemory()
	history := selfknowledge.NewActionHistory(16)
	selfQuery := selfknowledge.NewSelfQuery(history, selfknowledge.NewModuleRegistry(), "test")
	return NewEngine(sess, memory.LoadLongMemory(), &shortMem, history, selfQuery)
}

func newExpectationTestEngine(t *testing.T, path string) *Engine {
	t.Helper()
	e := newExpectationBaseEngine(t)
	e.enableExpectationLedgerAt(path)
	if e.LastExpectationError() != "" {
		t.Fatalf("acta no habilitada: %s", e.LastExpectationError())
	}
	return e
}

func TestExpectationLedgerDeclaresBeforeExecutionAndConfirmsFolder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory", "expectations.jsonl")
	e := newExpectationTestEngine(t, path)
	target := filepath.Join(t.TempDir(), "Prueba")

	result := e.ExecuteCapabilityAction(context.Background(), production.ProviderRequest{
		Operation: "crear_carpeta", Target: target,
	})
	if !result.Success || !result.Verified {
		t.Fatalf("la carpeta real no quedó verificada: success=%t verified=%t reason=%s", result.Success, result.Verified, result.VerificationReason)
	}
	if result.ExpectationID == "" || result.ExpectationStatus != string(expectation.StatusConfirmed) {
		t.Fatalf("veredicto publicado incorrecto: id=%q status=%q summary=%q", result.ExpectationID, result.ExpectationStatus, result.ExpectationSummary)
	}
	if !strings.Contains(result.ExpectationSummary, "Veredicto: confirmado") {
		t.Fatalf("resumen sin veredicto: %q", result.ExpectationSummary)
	}

	ledger := expectation.NewLedger(path)
	records, err := ledger.Entries()
	if err != nil {
		t.Fatalf("Entries: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("esperaba declaración + veredicto, obtuve %d registros", len(records))
	}
	if records[0].Kind != expectation.RecordExpectation || records[0].Expectation == nil {
		t.Fatalf("el primer registro no es la declaración previa: %+v", records[0])
	}
	if records[0].Expectation.ID != result.ExpectationID || records[0].Expectation.Action != "crear_carpeta" {
		t.Fatalf("declaración no coincide con el veredicto: %+v", records[0].Expectation)
	}
	if len(records[0].Expectation.Facts) != 1 || records[0].Expectation.Facts[0].Subject != target {
		t.Fatalf("la declaración no fijó el sujeto real: %+v", records[0].Expectation.Facts)
	}
	if records[1].Kind != expectation.RecordVerdict || records[1].Verdict == nil || records[1].Verdict.Status != expectation.StatusConfirmed {
		t.Fatalf("el segundo registro no es el veredicto confirmado: %+v", records[1])
	}
	verdict, ok, err := ledger.LatestVerdict()
	if err != nil || !ok {
		t.Fatalf("veredicto no recuperable: ok=%t err=%v", ok, err)
	}
	if verdict.ExpectationID != result.ExpectationID {
		t.Fatalf("identidad del veredicto perdida: %+v", verdict)
	}
}

func TestExpectationLedgerForcesContradictionWhenEffectDoesNotMaterialize(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory", "expectations.jsonl")
	e := newExpectationTestEngine(t, path)

	// The path exists as a file, so crear_carpeta cannot create a directory.
	target := filepath.Join(t.TempDir(), "ocupado")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	result := e.ExecuteCapabilityAction(context.Background(), production.ProviderRequest{
		Operation: "crear_carpeta", Target: target,
	})
	if result.ExpectationStatus != string(expectation.StatusContradicted) {
		t.Fatalf("esperaba contradicho, obtuve status=%q error=%q summary=%q", result.ExpectationStatus, result.ExpectationError, result.ExpectationSummary)
	}
	if result.Success {
		t.Fatal("una acción que no materializó la carpeta no puede reportar éxito operativo")
	}
	verdict, ok, err := expectation.NewLedger(path).LatestVerdict()
	if err != nil || !ok {
		t.Fatalf("veredicto no persistió: ok=%t err=%v", ok, err)
	}
	if verdict.Status != expectation.StatusContradicted {
		t.Fatalf("el acta no conserva la contradicción: %+v", verdict)
	}
}

func TestExpectationLedgerWriteFailureNeverPublishesConfirmed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory", "expectations.jsonl")
	e := newExpectationTestEngine(t, path)

	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	e.expectationMu.Lock()
	e.expectationLedger.SetPath(filepath.Join(blocker, "expectations.jsonl"))
	e.expectationMu.Unlock()

	target := filepath.Join(t.TempDir(), "Prueba")
	result := e.ExecuteCapabilityAction(context.Background(), production.ProviderRequest{
		Operation: "crear_carpeta", Target: target,
	})
	if result.ExpectationStatus == string(expectation.StatusConfirmed) {
		t.Fatal("sin acta durable no puede publicarse un veredicto confirmado")
	}
	if result.ExpectationStatus != string(expectation.StatusInsufficient) {
		t.Fatalf("esperaba insuficiente, obtuve %q", result.ExpectationStatus)
	}
	if strings.TrimSpace(result.ExpectationError) == "" {
		t.Fatal("el fallo del acta debe quedar visible")
	}
	if !strings.Contains(result.ExpectationSummary, "El acta no pudo escribirse") {
		t.Fatalf("el resumen debe declarar el fallo del acta: %q", result.ExpectationSummary)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("la acción debía continuar bajo la autoridad existente: %v", err)
	}
}

func TestExpectationLedgerUnavailableAtStartupIsVisible(t *testing.T) {
	e := newExpectationBaseEngine(t)
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	e.enableExpectationLedgerAt(filepath.Join(blocker, "expectations.jsonl"))
	if e.LastExpectationError() == "" {
		t.Fatal("el fallo de apertura del acta debe quedar visible")
	}

	target := filepath.Join(t.TempDir(), "PruebaSinActa")
	result := e.ExecuteCapabilityAction(context.Background(), production.ProviderRequest{
		Operation: "crear_carpeta", Target: target,
	})
	if result.ExpectationStatus != string(expectation.StatusInsufficient) {
		t.Fatalf("sin acta durable el veredicto debe ser insuficiente, obtuve %q", result.ExpectationStatus)
	}
	if result.ExpectationID == "" {
		t.Fatal("el fallo debe conservar una identidad visible aunque no sea durable")
	}
	if !strings.Contains(result.ExpectationSummary, "El acta no pudo escribirse") {
		t.Fatalf("el resumen debe declarar el fallo del acta: %q", result.ExpectationSummary)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("la acción debía continuar bajo la autoridad existente: %v", err)
	}
}

func TestExpectationVerdictSurvivesEngineRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory", "expectations.jsonl")
	e := newExpectationTestEngine(t, path)
	target := filepath.Join(t.TempDir(), "Persistente")
	result := e.ExecuteCapabilityAction(context.Background(), production.ProviderRequest{
		Operation: "crear_carpeta", Target: target,
	})
	if result.ExpectationStatus != string(expectation.StatusConfirmed) {
		t.Fatalf("setup: veredicto no confirmado: %q (%s)", result.ExpectationStatus, result.ExpectationError)
	}

	restarted := newExpectationTestEngine(t, path)
	verdict, ok := restarted.LastExpectationVerdict()
	if !ok {
		t.Fatal("el veredicto no se recuperó tras reiniciar el motor")
	}
	if verdict.ExpectationID != result.ExpectationID || verdict.Status != expectation.StatusConfirmed {
		t.Fatalf("veredicto recuperado distinto: %+v", verdict)
	}
	if !strings.Contains(restarted.expectationRuntimeSummary(), "Veredicto: confirmado") {
		t.Fatalf("el resumen recuperado no es legible: %q", restarted.expectationRuntimeSummary())
	}
}

func TestModelHypothesisDoesNotChangeHarnessVerdict(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory", "expectations.jsonl")
	e := newExpectationTestEngine(t, path)
	target := filepath.Join(t.TempDir(), "Prueba")
	result := e.ExecuteCapabilityAction(context.Background(), production.ProviderRequest{
		Operation: "crear_carpeta", Target: target,
	})
	if result.ExpectationStatus != string(expectation.StatusConfirmed) {
		t.Fatalf("setup: %q", result.ExpectationStatus)
	}

	ledger := expectation.NewLedger(path)
	if _, err := ledger.Hypothesize(expectation.Hypothesis{Text: "la carpeta no se creó", TurnID: "9"}); err != nil {
		t.Fatal(err)
	}
	verdict, ok, err := ledger.LatestVerdict()
	if err != nil || !ok {
		t.Fatalf("LatestVerdict: ok=%t err=%v", ok, err)
	}
	if verdict.Status != expectation.StatusConfirmed || verdict.ExpectationID != result.ExpectationID {
		t.Fatalf("una hipótesis del modelo alteró el veredicto: %+v", verdict)
	}
}
