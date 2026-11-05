package competence

import (
	"strings"
	"testing"
	"time"
)

// ledgerWithAttempts construye un Ledger en memoria con intentos cronológicos.
func ledgerWithAttempts(t *testing.T, attempts ...Attempt) *Ledger {
	t.Helper()
	l, err := Load("") // path vacío: en memoria, sin escrituras
	if err != nil {
		t.Fatalf("in-memory ledger: %v", err)
	}
	for _, a := range attempts {
		if err := l.Record(a); err != nil {
			t.Fatalf("record attempt: %v", err)
		}
	}
	return l
}

func attempt(capability, operation, context string, verified bool, minute int) Attempt {
	return Attempt{
		Capability: capability,
		Operation:  operation,
		ContextKey: context,
		Strategy:   "proc:" + capability + "@1",
		Verified:   verified,
		At:         time.Unix(1700000000+int64(minute), 0),
	}
}

type mapCatalog map[string]bool // clave "cap/op" → conocida

func (m mapCatalog) Known(capability, operation string) bool {
	return m[capability+"/"+operation]
}

type mapOracle map[string]bool

func (m mapOracle) DeclaresOracle(capability, operation string) bool {
	return m[capability+"/"+operation]
}

// TestOverlayDerivesStandingFromLedgerEvidence es la pieza E2 de
// aceptación del plan (T4): "¿Qué sabes hacer?" con estados derivados.
func TestOverlayDerivesStandingFromLedgerEvidence(t *testing.T) {
	cases := []struct {
		name           string
		attempts       []Attempt
		catalog        Catalog
		oracle         OracleDeclarer
		regressed      bool
		expectClaimed  bool
		expectStanding Standing
	}{
		{
			name:         "sin intentos no se afirma nada de la capacidad",
			expectClaimed: false,
		},
		{
			name:           "sin oráculo declarado máximo afirmable es unverifiable",
			attempts:       []Attempt{attempt("media.dimension", "rename", "projA", true, 0), attempt("media.dimension", "rename", "projA", true, 1)},
			expectClaimed:  true,
			expectStanding: StandingUnverifiable,
		},
		{
			name: "un solo contexto perfecto jamás es demonstrated",
			attempts: []Attempt{
				attempt("media.organize", "classify", "projA", true, 0),
				attempt("media.organize", "classify", "projA", true, 1),
				attempt("media.organize", "classify", "projA", true, 2),
				attempt("media.organize", "classify", "projA", true, 3),
				attempt("media.organize", "classify", "projA", true, 4),
				attempt("media.organize", "classify", "projA", true, 5),
			},
			oracle:         mapOracle{"media.organize/classify": true},
			expectClaimed:  true,
			expectStanding: StandingTrialed,
		},
		{
			name: "dos contextos con cota de Wilson suficiente es demonstrated",
			attempts: []Attempt{
				attempt("deploy.check", "verify", "projA", true, 0),
				attempt("deploy.check", "verify", "projA", true, 1),
				attempt("deploy.check", "verify", "projA", true, 2),
				attempt("deploy.check", "verify", "projA", true, 3),
				attempt("deploy.check", "verify", "projB", true, 4),
				attempt("deploy.check", "verify", "projB", true, 5),
				attempt("deploy.check", "verify", "projB", true, 6),
				attempt("deploy.check", "verify", "projB", true, 7),
				attempt("deploy.check", "verify", "projC", true, 8),
				attempt("deploy.check", "verify", "projC", true, 9),
			},
			oracle:         mapOracle{"deploy.check/verify": true},
			expectClaimed:  true,
			expectStanding: StandingDemonstrated,
		},
		{
			name: "registro con mayoría de fallos no pasa la cota inferior",
			attempts: []Attempt{
				attempt("net.retry", "attempt", "projA", false, 0),
				attempt("net.retry", "attempt", "projA", false, 1),
				attempt("net.retry", "attempt", "projA", true, 2),
				attempt("net.retry", "attempt", "projA", false, 3),
				attempt("net.retry", "attempt", "projA", false, 4),
				attempt("net.retry", "attempt", "projB", false, 5),
			},
			oracle:         mapOracle{"net.retry/attempt": true},
			expectClaimed:  true,
			expectStanding: StandingCompiled,
		},
		{
			name: "regresión declarada prevalece sobre todo lo demás",
			attempts: []Attempt{
				attempt("deploy.check", "verify", "projA", true, 0),
				attempt("deploy.check", "verify", "projB", true, 1),
				attempt("deploy.check", "verify", "projC", true, 2),
				attempt("deploy.check", "verify", "projD", true, 3),
				attempt("deploy.check", "verify", "projE", true, 4),
				attempt("deploy.check", "verify", "projF", true, 5),
				attempt("deploy.check", "verify", "projG", true, 6),
				attempt("deploy.check", "verify", "projH", true, 7),
			},
			oracle:         mapOracle{"deploy.check/verify": true},
			regressed:      true,
			expectClaimed:  true,
			expectStanding: StandingRegressed,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			overlay := NewOverlay(ledgerWithAttempts(t, tc.attempts...), tc.catalog, tc.oracle, DefaultPolicy())
			overlay.SetRegressed(tc.regressed)

			if tc.expectClaimed {
				assessment, ok := overlay.StandingOf(tc.attempts[0].Capability)
				if !ok {
					t.Fatalf("capability with attempts must be claimed")
				}
				if assessment.Standing != tc.expectStanding {
					t.Fatalf("standing = %q (%s), want %q", assessment.Standing, assessment.Reason, tc.expectStanding)
				}
			} else {
				if _, ok := overlay.StandingOf("memory.search"); ok {
					t.Fatal("capability without attempts must not be claimed")
				}
			}
		})
	}
}

func TestOverlayCapabilitiesListDeterministicAndHonest(t *testing.T) {
	ledger := ledgerWithAttempts(t,
		attempt("b.cap", "op1", "project1", true, 0),
		attempt("a.cap", "op1", "project0", true, 1),
	)
	overlay := NewOverlay(ledger, nil, nil, DefaultPolicy())

	caps := overlay.Capabilities()
	if len(caps) != 2 {
		t.Fatalf("capabilities = %d, want 2", len(caps))
	}
	// Orden alfabético determinista: a.cap antes que b.cap.
	if caps[0].Capability != "a.cap" || caps[1].Capability != "b.cap" {
		t.Fatalf("order not deterministic: %q, %q", caps[0].Capability, caps[1].Capability)
	}
	// Ambas sin oráculo ⇒ unverifiable (cap de aceptación T4).
	for _, a := range caps {
		if a.Standing != StandingUnverifiable {
			t.Fatalf("no-oracle capability %s must cap at unverifiable, got %q", a.Capability, a.Standing)
		}
	}
	if caps := NewOverlay(nil, nil, nil, DefaultPolicy()).Capabilities(); len(caps) != 0 {
		t.Fatalf("overlay without ledger must claim nothing, got %v", caps)
	}
}

func TestOverlayEmptyLedgerAnswersNothingAndNeverInvents(t *testing.T) {
	overlay := NewOverlay(ledgerWithAttempts(t), mapCatalog{"anything/action": true}, nil, DefaultPolicy())

	if _, ok := overlay.StandingOf("anything/action"); ok {
		t.Fatal("catalog knowledge alone does not create attempts: nothing to claim")
	}
	if caps := overlay.Capabilities(); len(caps) != 0 {
		t.Fatalf("empty ledger must yield no capability claims, got %d", len(caps))
	}
}

// TestOverlayDerivationIsPure_RuntimeHonesty: mismos datos ⇒ mismo veredicto
// textual, en cualquier cantidad de consultas. Esto blinda la propiedad T4 de
// que la respuesta es derivada y estable (mismas conclusiones, distinta
// formulación superficial queda a cargo del contrato de lenguaje).
func TestOverlayDerivationIsPureAndRepeatable(t *testing.T) {
	ledger := ledgerWithAttempts(t,
		attempt("media.organize", "classify", "projA", true, 0),
		attempt("media.organize", "classify", "projA", true, 1),
	)
	overlay := NewOverlay(ledger, nil, nil, DefaultPolicy())

	first, ok1 := overlay.StandingOf("media.organize")
	second, ok2 := overlay.StandingOf("media.organize")
	if !ok1 || !ok2 {
		t.Fatal("attempts must be claimable on repeated queries")
	}
	if first.Standing != second.Standing || first.Reason != second.Reason {
		t.Fatalf("derivation drifted: %q/%q vs %q/%q", first.Standing, first.Reason, second.Standing, second.Reason)
	}
	if first.Standing != StandingUnverifiable || !strings.Contains(first.Reason, "oráculo") {
		t.Fatalf("expected unverifiable with no oracle declared, got %q reason %q", first.Standing, first.Reason)
	}
}
