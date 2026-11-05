package expectation

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLedgerDeclareThenConcludePersistsAcrossReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory", "expectations.jsonl")
	ledger := NewLedger(path)

	declared, err := ledger.Declare(Expectation{
		Action: "crear_carpeta",
		Target: `C:\tmp\Prueba`,
		WorkID: "work-1",
		Facts: []Fact{{
			Subject: `C:\tmp\Prueba`, Predicate: "filesystem.object_type", Expected: "directory",
		}},
	})
	if err != nil {
		t.Fatalf("Declare: %v", err)
	}
	if declared.ID == "" || declared.DeclaredAt.IsZero() {
		t.Fatalf("declaración sin identidad durable: %+v", declared)
	}

	if err := ledger.Conclude(Verdict{
		ExpectationID: declared.ID,
		Status:        StatusConfirmed,
		Source:        "solving.verification_engine",
		Details:       "carpeta confirmada",
		Facts: []Fact{{
			Subject: `C:\tmp\Prueba`, Predicate: "filesystem.object_type", Expected: "directory", Observed: "directory",
		}},
		Summary: "Esperaba que existiera la carpeta; observé que existe. Veredicto: confirmado.",
	}); err != nil {
		t.Fatalf("Conclude: %v", err)
	}

	reopened := NewLedger(path)
	verdict, ok, err := reopened.LatestVerdict()
	if err != nil {
		t.Fatalf("LatestVerdict: %v", err)
	}
	if !ok {
		t.Fatal("el veredicto no sobrevivió a la reapertura")
	}
	if verdict.ExpectationID != declared.ID || verdict.Status != StatusConfirmed {
		t.Fatalf("veredicto recuperado incompleto: %+v", verdict)
	}
	if len(verdict.Facts) != 1 || verdict.Facts[0].Observed != "directory" {
		t.Fatalf("hechos observados no persistieron: %+v", verdict.Facts)
	}
	if !strings.Contains(verdict.Summary, "Veredicto: confirmado") {
		t.Fatalf("resumen no persistió: %q", verdict.Summary)
	}
}

func TestLedgerIsAppendOnlyAndOrdered(t *testing.T) {
	path := filepath.Join(t.TempDir(), "expectations.jsonl")
	ledger := NewLedger(path)

	declared, err := ledger.Declare(Expectation{
		Action: "renombrar_archivo",
		Facts:  []Fact{{Subject: "a", Predicate: "filesystem.exists", Expected: "false"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := ledger.Conclude(Verdict{ExpectationID: declared.ID, Status: StatusContradicted}); err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Hypothesize(Hypothesis{Text: "el archivo debería estar en la papelera", TurnID: "7"}); err != nil {
		t.Fatal(err)
	}

	records, err := ledger.Entries()
	if err != nil {
		t.Fatalf("Entries: %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("esperaba 3 registros, obtuve %d", len(records))
	}
	if records[0].Kind != RecordExpectation || records[1].Kind != RecordVerdict || records[2].Kind != RecordHypothesis {
		t.Fatalf("orden incorrecto: %s, %s, %s", records[0].Kind, records[1].Kind, records[2].Kind)
	}
	if records[0].Expectation.Action != "renombrar_archivo" {
		t.Fatalf("la declaración cambió: %+v", records[0].Expectation)
	}
}

func TestLedgerHypothesisIsNeverCheckable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "expectations.jsonl")
	ledger := NewLedger(path)
	hyp, err := ledger.Hypothesize(Hypothesis{Text: "debería existir la carpeta", Checkable: true, Source: "harness"})
	if err != nil {
		t.Fatal(err)
	}
	if hyp.Checkable {
		t.Fatal("una hipótesis del modelo no puede ser checkable")
	}
	if hyp.Source != "model" {
		t.Fatalf("la hipótesis debe quedar como source=model, obtuve %q", hyp.Source)
	}
	verdict, ok, err := ledger.LatestVerdict()
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatalf("una hipótesis no puede producir veredicto: %+v", verdict)
	}
	counts, err := ledger.Counts()
	if err != nil {
		t.Fatal(err)
	}
	if counts[StatusConfirmed] != 0 || counts[StatusContradicted] != 0 || counts[StatusInsufficient] != 0 {
		t.Fatalf("una hipótesis alteró los conteos: %+v", counts)
	}
}

func TestLedgerRejectsDeclaredAsVerdict(t *testing.T) {
	ledger := NewLedger(filepath.Join(t.TempDir(), "expectations.jsonl"))
	err := ledger.Conclude(Verdict{ExpectationID: "exp-1", Status: StatusDeclared})
	if err == nil {
		t.Fatal("un veredicto declarado no puede cerrar la expectativa")
	}
}

func TestLedgerWriteFailureIsVisible(t *testing.T) {
	base := t.TempDir()
	blocker := filepath.Join(base, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	ledger := NewLedger(filepath.Join(blocker, "expectations.jsonl"))
	if err := ledger.Ensure(); err == nil {
		t.Fatal("Ensure debía fallar cuando el padre es un archivo")
	}
	if _, err := ledger.Declare(Expectation{
		Action: "crear_carpeta",
		Facts:  []Fact{{Subject: "x", Predicate: "filesystem.object_type", Expected: "directory"}},
	}); err == nil {
		t.Fatal("Declare debía fallar visiblemente")
	}
}

func TestCompareConservativeStates(t *testing.T) {
	expected := []Fact{{
		Subject: "x", Predicate: "filesystem.object_type", Expected: "directory",
	}}
	cases := []struct {
		name     string
		observed []Fact
		want     Status
	}{
		{name: "confirmed", observed: []Fact{{Subject: "x", Predicate: "filesystem.object_type", Observed: "directory"}}, want: StatusConfirmed},
		{name: "contradicted", observed: []Fact{{Subject: "x", Predicate: "filesystem.object_type", Observed: "absent"}}, want: StatusContradicted},
		{name: "unknown", observed: []Fact{{Subject: "x", Predicate: "filesystem.object_type", Observed: "unknown"}}, want: StatusInsufficient},
		{name: "missing", observed: nil, want: StatusInsufficient},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, detail := Compare(expected, tc.observed)
			if got != tc.want {
				t.Fatalf("Compare = %q (%s), esperaba %q", got, detail, tc.want)
			}
		})
	}
}

func TestDescribeVerdictIsDeterministic(t *testing.T) {
	verdict := Verdict{
		Status: StatusContradicted,
		Facts: []Fact{{
			Subject: `C:\tmp\Prueba`, Predicate: "filesystem.object_type", Expected: "directory", Observed: "absent",
		}},
	}
	got := DescribeVerdict(verdict)
	if !strings.Contains(got, "Esperaba") || !strings.Contains(got, "observé") || !strings.Contains(got, "Veredicto: contradicho") {
		t.Fatalf("frase incompleta: %q", got)
	}
}
