package core

// expectation_ledger.go — ACTA DE EXPECTATIVA EN EL MOTOR (PASO 4.1).
//
// El punto de declaración es el mismo adaptador por el que cruzan todas las
// acciones locales: ExecuteCapabilityAction. Antes de e.execute se fija la
// postcondición con solving.ExpectedFacts; después de observar se cierra con
// un veredicto. El libro es append-only y tiene un solo escritor.
//
// Si el libro no se puede escribir, la acción sigue bajo la autoridad
// existente, pero el veredicto publicado es insuficiente: nunca confirmed.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ahmelia/deliberation"
	"ahmelia/expectation"
	"ahmelia/production"
	"ahmelia/solving"
)

const expectationSource = "core.local_action"

// EnableExpectationLedger turns on the durable expectation act in the live
// process. Like the other paso mechanisms it is opt-in so test suites do not
// write runtime state.
func (e *Engine) EnableExpectationLedger() {
	e.enableExpectationLedgerAt(expectation.LedgerPath)
}

func (e *Engine) enableExpectationLedgerAt(path string) {
	if e == nil {
		return
	}
	e.expectationMu.Lock()
	defer e.expectationMu.Unlock()
	e.expectationConfigured = true
	if e.expectationLedger == nil {
		e.expectationLedger = expectation.NewLedger(path)
	} else {
		e.expectationLedger.SetPath(path)
	}
	if err := e.expectationLedger.Ensure(); err != nil {
		e.expectationOn = false
		e.lastExpectationErr = "no se pudo preparar el acta de expectativa: " + err.Error()
		return
	}
	e.expectationOn = true
	verdict, ok, err := e.expectationLedger.LatestVerdict()
	switch {
	case err != nil:
		e.lastExpectationErr = "no se pudo leer el acta de expectativa: " + err.Error()
	case ok:
		e.lastExpectation = verdict
		e.lastExpectationSeen = true
		e.lastExpectationErr = ""
	default:
		e.lastExpectation = expectation.Verdict{}
		e.lastExpectationSeen = false
		e.lastExpectationErr = ""
	}
}

// LastExpectationVerdict returns a read-only copy of the latest durable
// verdict. The model may read it; it cannot write or change it.
func (e *Engine) LastExpectationVerdict() (expectation.Verdict, bool) {
	if e == nil {
		return expectation.Verdict{}, false
	}
	e.expectationMu.Lock()
	defer e.expectationMu.Unlock()
	return e.lastExpectation, e.lastExpectationSeen
}

// LastExpectationError reports the last load/read/write problem, if any.
func (e *Engine) LastExpectationError() string {
	if e == nil {
		return ""
	}
	e.expectationMu.Lock()
	defer e.expectationMu.Unlock()
	return e.lastExpectationErr
}

// ExpectationCounts summarizes the durable act for diagnostics. It never
// changes the act.
func (e *Engine) ExpectationCounts() map[expectation.Status]int {
	if e == nil {
		return nil
	}
	e.expectationMu.Lock()
	ledger := e.expectationLedger
	on := e.expectationOn
	e.expectationMu.Unlock()
	if !on || ledger == nil {
		return nil
	}
	counts, err := ledger.Counts()
	if err != nil {
		e.expectationMu.Lock()
		e.lastExpectationErr = err.Error()
		e.expectationMu.Unlock()
	}
	return counts
}

func (e *Engine) expectationState() (*expectation.Ledger, bool) {
	if e == nil {
		return nil, false
	}
	e.expectationMu.Lock()
	defer e.expectationMu.Unlock()
	return e.expectationLedger, e.expectationOn
}

func (e *Engine) expectationIsConfigured() bool {
	if e == nil {
		return false
	}
	e.expectationMu.Lock()
	defer e.expectationMu.Unlock()
	return e.expectationConfigured
}

// declareLocalActionExpectation fixes the postcondition before execution. If
// the action has no harness oracle, it returns no expectation: the fail-safe is
// "no verdict", never a manufactured check.
func (e *Engine) declareLocalActionExpectation(action string, req production.ProviderRequest, params map[string]string) (*expectation.Expectation, []solving.PostconditionFact, string) {
	if !e.normalizeLocalActionExpectationParams(action, req.Target, params) {
		return nil, nil, ""
	}
	expected, _ := solving.ExpectedFacts(action, params)
	if len(expected) == 0 {
		return nil, nil, ""
	}
	ledger, on := e.expectationState()
	if !on || ledger == nil {
		// Sin acta habilitada no hay veredicto. Las suites que nunca configuran
		// el mecanismo no escriben estado vivo ni publican una confirmación que
		// no pueden registrar. Si el proceso vivo SÍ intentó habilitarla y
		// falló, el fallo no puede desaparecer: se publica insuficiente con una
		// identidad no durable y el error visible.
		if !e.expectationIsConfigured() {
			return nil, nil, ""
		}
		return &expectation.Expectation{
			ID: expectation.NewID("exp-unwritten"), Action: action,
			Target: strings.TrimSpace(req.Target), WorkID: strings.TrimSpace(req.WorkID),
			AttemptNumber: req.AttemptNumber, Source: expectationSource,
			Facts: expectationFactsFromPostconditions(expected),
			Note:  "el acta no está disponible; declaración no durable",
		}, expected, "el acta de expectativa no está habilitada"
	}
	declared, err := ledger.Declare(expectation.Expectation{
		Action:        action,
		Target:        strings.TrimSpace(req.Target),
		WorkID:        strings.TrimSpace(req.WorkID),
		AttemptNumber: req.AttemptNumber,
		Source:        expectationSource,
		Facts:         expectationFactsFromPostconditions(expected),
		Note:          "declaración del arnés antes de ejecutar",
	})
	if err != nil {
		return &declared, expected, err.Error()
	}
	return &declared, expected, ""
}

// concludeLocalActionExpectation observes the declared facts independently of
// the executor's self-report and appends one terminal verdict. A failed act
// write is surfaced as insufficient; it never becomes confirmed.
func (e *Engine) concludeLocalActionExpectation(action string, params map[string]string, result production.ProviderResult, declared *expectation.Expectation, expected []solving.PostconditionFact, declareErr string) production.ProviderResult {
	if declared == nil || len(expected) == 0 {
		return result
	}

	observed := solving.ObserveExpected(action, params, expected)
	status, detail := expectationStatus(observed)
	published := status
	ledgerErr := strings.TrimSpace(declareErr)

	// A failed act write is itself a reason not to publish confirmed: the
	// declaration or verdict has no durable evidence.
	if ledgerErr != "" {
		published = expectation.StatusInsufficient
	}

	// A provider that failed before an observable completion cannot confirm
	// the postcondition even if the desired state happens to hold already.
	if published == expectation.StatusConfirmed && (!result.Success || !result.Observed) {
		published = expectation.StatusInsufficient
		if ledgerErr == "" {
			ledgerErr = "la acción no completó una ejecución observable"
		}
	}

	verdict := expectation.Verdict{
		ExpectationID: declared.ID,
		Status:        published,
		ObservedAt:    time.Now().UTC(),
		Source:        "solving.verification_engine",
		Details:       detail,
		Facts:         expectationFactsFromPostconditions(observed.Facts),
		LedgerError:   ledgerErr,
	}
	verdict.Summary = expectation.DescribeVerdict(verdict)

	ledger, on := e.expectationState()
	if ledgerErr == "" {
		if !on || ledger == nil {
			// The declaration was durable but the writer disappeared. Do not
			// publish a confirmed verdict without a durable closing record.
			published = expectation.StatusInsufficient
			verdict.Status = published
			verdict.LedgerError = "el acta de expectativa no está disponible para cerrar el veredicto"
			verdict.Summary = expectation.DescribeVerdict(verdict)
		} else if err := ledger.Conclude(verdict); err != nil {
			published = expectation.StatusInsufficient
			verdict.Status = published
			verdict.LedgerError = err.Error()
			verdict.Summary = expectation.DescribeVerdict(verdict)
		}
	}

	result.ExpectationID = declared.ID
	result.ExpectationStatus = string(published)
	result.ExpectationSummary = verdict.Summary
	result.ExpectationError = verdict.LedgerError
	result.ObservationFacts = observedPostconditionFacts(observed.Facts)
	e.recordLastExpectation(verdict)
	return result
}

func expectationStatus(result *solving.VerificationResult) (expectation.Status, string) {
	if result == nil {
		return expectation.StatusInsufficient, "no se produjo una observación"
	}
	switch result.Outcome {
	case solving.VerificationConfirmed:
		return expectation.StatusConfirmed, result.Detail
	case solving.VerificationNotConfirmed:
		return expectation.StatusContradicted, result.Detail
	default:
		return expectation.StatusInsufficient, result.Detail
	}
}

func (e *Engine) recordLastExpectation(verdict expectation.Verdict) {
	if e == nil {
		return
	}
	e.expectationMu.Lock()
	e.lastExpectation = verdict
	e.lastExpectationSeen = true
	if strings.TrimSpace(verdict.LedgerError) != "" {
		e.lastExpectationErr = verdict.LedgerError
	} else {
		e.lastExpectationErr = ""
	}
	e.expectationMu.Unlock()
}

// recordModelExpectation stores the model's prose as a non-checkable
// hypothesis. It is never compared with the observation and never changes the
// harness verdict.
func (e *Engine) recordModelExpectation(rec deliberation.Record, turnID string) {
	if e == nil {
		return
	}
	text := strings.TrimSpace(rec.Proposal.Expectation)
	if text == "" {
		return
	}
	ledger, on := e.expectationState()
	if !on || ledger == nil {
		return
	}
	if _, err := ledger.Hypothesize(expectation.Hypothesis{
		Text: text, TurnID: strings.TrimSpace(turnID), Action: strings.TrimSpace(rec.Harness.Action),
	}); err != nil {
		e.expectationMu.Lock()
		if e.lastExpectationErr == "" {
			e.lastExpectationErr = "no se pudo registrar la hipótesis del modelo: " + err.Error()
		}
		e.expectationMu.Unlock()
	}
}

func (e *Engine) expectationRuntimeSummary() string {
	verdict, ok := e.LastExpectationVerdict()
	if !ok {
		return ""
	}
	if strings.TrimSpace(verdict.Summary) != "" {
		return verdict.Summary
	}
	return expectation.DescribeVerdict(verdict)
}

func expectationFactsFromPostconditions(facts []solving.PostconditionFact) []expectation.Fact {
	out := make([]expectation.Fact, 0, len(facts))
	for _, fact := range facts {
		out = append(out, expectation.Fact{
			Subject: fact.Subject, Predicate: fact.Predicate, Expected: fact.Expected, Observed: fact.Observed,
		})
	}
	return out
}

func observedPostconditionFacts(facts []solving.PostconditionFact) []production.ObservedPostconditionFact {
	out := make([]production.ObservedPostconditionFact, 0, len(facts))
	for _, fact := range facts {
		out = append(out, production.ObservedPostconditionFact{
			Subject: fact.Subject, Predicate: fact.Predicate, Expected: fact.Expected, Observed: fact.Observed,
		})
	}
	return out
}

// normalizeLocalActionExpectationParams mirrors the path resolution the legacy
// executor performs before touching the filesystem. The declaration must name
// the same target the action will actually use; declaring a relative path while
// the executor writes under %USERPROFILE%\Desktop would verify the wrong place.
func (e *Engine) normalizeLocalActionExpectationParams(action, target string, params map[string]string) bool {
	if e == nil || params == nil {
		return false
	}
	switch action {
	case "crear_carpeta":
		path := strings.TrimSpace(params["ruta"])
		if path == "" {
			path = strings.TrimSpace(target)
		}
		if path == "" {
			return false
		}
		if !filepath.IsAbs(path) {
			home, err := os.UserHomeDir()
			if err != nil {
				return false
			}
			path = filepath.Join(home, "Desktop", path)
		}
		params["ruta"] = path
		return true
	case "mover_archivo":
		source := strings.TrimSpace(params["origen"])
		if source == "" {
			source = strings.TrimSpace(params["source"])
		}
		if source == "" && e.sess != nil && e.sess.HasFile() {
			source = e.sess.LastSelectedFile
		}
		if source == "" {
			return false
		}
		dest := strings.TrimSpace(params["destino"])
		if dest == "" {
			dest = strings.TrimSpace(params["destination"])
		}
		if dest == "" {
			dest = strings.TrimSpace(target)
		}
		if dest == "" {
			return false
		}
		if !filepath.IsAbs(dest) {
			key := e.extractFolderTarget(dest)
			if key == "" || e.folderDst[key] == "" {
				return false
			}
			dest = e.folderDst[key]
		}
		params["origen"] = source
		params["destino"] = dest
		return true
	case "renombrar_archivo":
		source := strings.TrimSpace(params["origen"])
		if source == "" {
			source = strings.TrimSpace(params["source"])
		}
		if source == "" && e.sess != nil && e.sess.HasFile() {
			source = e.sess.LastSelectedFile
		}
		if source == "" {
			return false
		}
		newName := strings.TrimSpace(params["new_name"])
		if newName == "" {
			newName = strings.TrimSpace(params["nombre_nuevo"])
		}
		if newName == "" {
			dest := strings.TrimSpace(params["destino"])
			if dest == "" {
				dest = strings.TrimSpace(params["destination"])
			}
			if dest != "" {
				newName = filepath.Base(dest)
			}
		}
		if newName == "" {
			return false
		}
		params["ruta"] = source
		params["nombre_nuevo"] = newName
		return true
	default:
		return false
	}
}

func expectationDiagnosticLine(counts map[expectation.Status]int) string {
	if counts == nil {
		return "acta de expectativa: sin datos"
	}
	return fmt.Sprintf("acta de expectativa: confirmado=%d contradicho=%d insuficiente=%d",
		counts[expectation.StatusConfirmed], counts[expectation.StatusContradicted], counts[expectation.StatusInsufficient])
}
