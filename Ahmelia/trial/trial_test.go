package trial

import (
	"context"
	"errors"
	"testing"

	"ahmelia/procedure"
)

type cat map[string]bool

func (c cat) Known(a, b string) bool { return c[a+"/"+b] }

var known = cat{"programming/build": true}

func proc() procedure.Procedure {
	return procedure.Procedure{ID: "p", Slots: []procedure.Slot{{Name: "project", Required: true}},
		Steps: []procedure.Step{{ID: "b", Capability: "programming", Operation: "build", Inputs: map[string]string{"project": "{project}"}}}}
}

// fake: resultado por (versión, escenario)
type fake map[string]Outcome

func (f fake) Run(_ context.Context, p procedure.Procedure, s Scenario) (Outcome, error) {
	o, ok := f[p.Ref()+"/"+s.ID]
	if !ok {
		return Outcome{}, errors.New("sin fixture")
	}
	return o, nil
}

var suite = Suite{Scenarios: []Scenario{{ID: "s1"}, {ID: "s2"}, {ID: "s3"}}}

func ok() Outcome  { return Outcome{Verified: true} }
func bad() Outcome { return Outcome{FailureClass: "build"} }

func TestFirstVersionNeedsAllGreenAndBecomesActive(t *testing.T) {
	reg, _ := procedure.Open("")
	v1, _ := reg.Propose(proc(), known)
	r := fake{"proc:p@1/s1": ok(), "proc:p@1/s2": ok(), "proc:p@1/s3": ok()}
	d, err := Release(context.Background(), reg, r, suite, "p", v1.Version)
	if err != nil || !d.Promote {
		t.Fatalf("%v %+v", err, d)
	}
	if a, _ := reg.Active("p"); a.Version != 1 {
		t.Fatal("not active")
	}
}

func TestCandidateThatImprovesOneButBreaksAnotherIsRejectedAndActiveStays(t *testing.T) {
	reg, _ := procedure.Open("")
	v1, _ := reg.Propose(proc(), known)
	_ = reg.Activate("p", v1.Version)
	v2, _ := reg.Propose(proc(), known)
	r := fake{
		"proc:p@1/s1": ok(), "proc:p@1/s2": ok(), "proc:p@1/s3": bad(),
		"proc:p@2/s1": bad(), "proc:p@2/s2": ok(), "proc:p@2/s3": ok(), // mejora s3, rompe s1
	}
	d, _ := Release(context.Background(), reg, r, suite, "p", v2.Version)
	if d.Promote || len(d.Regressions) != 1 || d.Regressions[0] != "s1" {
		t.Fatalf("%+v", d)
	}
	if a, _ := reg.Active("p"); a.Version != 1 {
		t.Fatal("regression was activated")
	}
}

func TestStrictImprovementPromotesAndNoChangeDoesNot(t *testing.T) {
	reg, _ := procedure.Open("")
	v1, _ := reg.Propose(proc(), known)
	_ = reg.Activate("p", v1.Version)
	v2, _ := reg.Propose(proc(), known)
	r := fake{
		"proc:p@1/s1": ok(), "proc:p@1/s2": ok(), "proc:p@1/s3": bad(),
		"proc:p@2/s1": ok(), "proc:p@2/s2": ok(), "proc:p@2/s3": ok(),
	}
	if d, _ := Release(context.Background(), reg, r, suite, "p", v2.Version); !d.Promote {
		t.Fatalf("%+v", d)
	}
	v3, _ := reg.Propose(proc(), known)
	r["proc:p@3/s1"], r["proc:p@3/s2"], r["proc:p@3/s3"] = ok(), ok(), ok()
	if d, _ := Release(context.Background(), reg, r, suite, "p", v3.Version); d.Promote {
		t.Fatal("churn without net improvement promoted")
	}
}

func TestUnknownOutcomesBlockCertification(t *testing.T) {
	reg, _ := procedure.Open("")
	v1, _ := reg.Propose(proc(), known)
	r := fake{"proc:p@1/s1": ok(), "proc:p@1/s2": ok()} // s3 sin fixture => runner error => unknown
	d, _ := Release(context.Background(), reg, r, suite, "p", v1.Version)
	if d.Promote {
		t.Fatalf("certified with unknown outcomes: %+v", d)
	}
}
