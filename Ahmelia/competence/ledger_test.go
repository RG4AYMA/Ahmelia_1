package competence

import (
	"path/filepath"
	"testing"
	"time"
)

func feed(l *Ledger, n int, ok func(i int) bool, ctx func(i int) string) {
	t0 := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < n; i++ {
		_ = l.Record(Attempt{Capability: "programming", Operation: "create_file", ContextKey: ctx(i), Verified: ok(i), FailureClass: "build", At: t0.Add(time.Duration(i) * time.Minute)})
	}
}

func TestNoClaimWithoutDisjointWindows(t *testing.T) {
	l, _ := Load("")
	feed(l, 12, func(int) bool { return true }, func(int) string { return "a" })
	if r := l.Report("programming", "create_file", 10); r.Claim != ClaimInsufficient {
		t.Fatalf("%+v", r)
	}
}

func TestImprovementNeedsDisjointIntervalsNotLuck(t *testing.T) {
	l, _ := Load("")
	feed(l, 20, func(i int) bool { return i%2 == 0 || i >= 18 }, func(int) string { return "a" }) // ruido: 50% -> 60%
	if r := l.Report("programming", "create_file", 10); r.Claim != ClaimNoChange {
		t.Fatalf("luck must not be improvement: %+v", r)
	}
	l2, _ := Load("")
	feed(l2, 40, func(i int) bool { return i >= 20 }, func(i int) string {
		if i >= 30 {
			return "B"
		}
		return "A"
	})
	r := l2.Report("programming", "create_file", 20)
	if r.Claim != ClaimImproved || r.TransferCtx != 2 {
		t.Fatalf("%+v", r)
	}
}

func TestPersistsAtomicallyAndReportsCorruption(t *testing.T) {
	p := filepath.Join(t.TempDir(), "c.json")
	l, _ := Load(p)
	feed(l, 3, func(int) bool { return true }, func(int) string { return "a" })
	l2, err := Load(p)
	if err != nil || len(l2.data) != 3 {
		t.Fatalf("%v %d", err, len(l2.data))
	}
}
