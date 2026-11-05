package integration

import (
	"errors"
	"fmt"
	"strings"

	"ahmelia/competence"
	"ahmelia/production"
)

// ProjectAttempts deriva intentos de competencia de las WorkSessions ya
// persistidas. Un intento cuenta como verificado SOLO si Result.Verified.
// ContextKey = ProjectID del primer artefacto; sin él, "unknown" (la
// transferencia se subestima; nunca se infla con IDs de Work).
func ProjectAttempts(works []production.WorkSession, skillRef string) []competence.Attempt {
	var out []competence.Attempt
	for _, w := range works {
		ctx := "unknown"
		for _, a := range w.Artifacts {
			if strings.TrimSpace(a.ProjectID) != "" {
				ctx = a.ProjectID
				break
			}
		}
		for _, at := range w.Attempts {
			if at.SkillID != skillRef || at.FinishedAt.IsZero() {
				continue
			}
			fc := ""
			verified := at.Result != nil && at.Result.Verified
			if !verified {
				fc = "unclassified"
				if at.Result != nil && at.Result.FailureReason != "" {
					fc = at.Result.FailureReason
				}
			}
			out = append(out, competence.Attempt{
				Capability: at.CapabilityID, Operation: at.Operation, ContextKey: ctx,
				Strategy: skillRef, Verified: verified, FailureClass: fc, At: at.FinishedAt,
			})
		}
	}
	return out
}

// AttemptsForLedger proyecta TODOS los intentos finalizados de trabajo REAL
// (WorkSessions persistidas en memory/production_work.json) hacia el ledger
// de competencia — feed de producción del Paso 1.2 (directiva DeepSeek
// 19/10). Difiere de ProjectAttempts en que no filtra por skill: el Standing
// se reconstruye sobre toda la práctica real registrada.
//
// Token de deduplicación ESTABLE entre boots: ExecutionAttemptID si existe;
// en su defecto workSessionID#índice#unix(FinishedAt) — determinista porque
// el mismo work store persistido produce la misma secuencia en cada boot,
// así EnsureAll no duplica (criterio de aceptación "dos boots no duplican").
// ContextKey = ProjectID del primer artefacto, "unknown" si no hay: la
// transferencia se subestima, nunca se infla. Sin CapabilityID el intento no
// es medible y se omite (no se inventa)._skillRef pasa como Strategy.
func AttemptsForLedger(works []production.WorkSession) []competence.Attempt {
	out := make([]competence.Attempt, 0, len(works))
	for _, w := range works {
		ctx := "unknown"
		for _, a := range w.Artifacts {
			if strings.TrimSpace(a.ProjectID) != "" {
				ctx = a.ProjectID
				break
			}
		}
		for i, at := range w.Attempts {
			if strings.TrimSpace(at.CapabilityID) == "" || at.FinishedAt.IsZero() {
				continue // sin capability u sin fin no hay intento evaluable
			}
			verified := at.Result != nil && at.Result.Verified
			fc := ""
			if !verified {
				fc = "unclassified"
				if at.Result != nil && at.Result.FailureReason != "" {
					fc = at.Result.FailureReason
				}
			}
			token := strings.TrimSpace(at.ExecutionAttemptID)
			if token == "" {
				token = fmt.Sprintf("%s#%d#%d", w.ID, i, at.FinishedAt.Unix())
			}
			out = append(out, competence.Attempt{
				Capability: at.CapabilityID, Operation: at.Operation, ContextKey: ctx,
				Strategy: at.SkillID, Verified: verified, FailureClass: fc,
				At: at.FinishedAt, Token: token,
			})
		}
	}
	return out
}

// RebuildCompetenceAttempts reconstruye la ledger desde trabajo real
// persistido (caller de producción: main tras crear el engine). Idempotente
// vía Ledger.EnsureAll — el mismo work store en dos boots no duplica.
func RebuildCompetenceAttempts(ledger *competence.Ledger, works []production.WorkSession) (int, error) {
	if ledger == nil {
		return 0, errors.New("integration: ledger de competencia nil")
	}
	return ledger.EnsureAll(AttemptsForLedger(works))
}
