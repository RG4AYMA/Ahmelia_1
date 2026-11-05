package procedure

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type cat map[string]bool

func (c cat) Known(capability, op string) bool { return c[capability+"/"+op] }

var known = cat{"programming/build": true, "programming/test": true}

func goodProc() Procedure {
	return Procedure{ID: "verify_go", Name: "verificar proyecto Go", Slots: []Slot{{"project", true}},
		Steps: []Step{
			{ID: "t", Capability: "programming", Operation: "test", Inputs: map[string]string{"project": "{project}"}, After: []string{"b"}},
			{ID: "b", Capability: "programming", Operation: "build", Inputs: map[string]string{"project": "{project}"}},
		}, Origin: Origin{Kind: OriginAuthored}}
}

func TestBindOrdersByDependencyAndResolvesSlotsFromContextOnly(t *testing.T) {
	bs, err := goodProc().Bind(map[string]string{"project": "C:/w/app"})
	if err != nil || len(bs) != 2 || bs[0].StepID != "b" || bs[1].Inputs["project"] != "C:/w/app" {
		t.Fatalf("%v %+v", err, bs)
	}
	if _, err := goodProc().Bind(map[string]string{}); err == nil {
		t.Fatal("missing required slot accepted")
	}
	if _, err := goodProc().Bind(map[string]string{"project": "x", "extra": "y"}); err == nil {
		t.Fatal("unknown slot accepted")
	}
}

func TestCheckRejectsUnknownCapabilityCycleAndExternalLiterals(t *testing.T) {
	p := goodProc()
	p.Steps[0].Capability = "shell"
	if is := p.Check(known); len(is) == 0 || is[0].Kind != IssueCapabilityGap {
		t.Fatalf("%+v", is)
	}
	c := goodProc()
	c.Steps[1].After = []string{"t"}
	if is := c.Check(known); len(is) == 0 || is[0].Kind != IssueCycle {
		t.Fatalf("%+v", is)
	}
	e := goodProc()
	e.Origin.Kind = OriginFromClaims
	e.Steps[1].Inputs["project"] = "; rm -rf /"
	found := false
	for _, i := range e.Check(known) {
		found = found || i.Kind == IssueExternalLiteral
	}
	if !found {
		t.Fatal("web-derived literal input accepted")
	}
}

func TestRegistryVersionsActivationRollbackAndRestart(t *testing.T) {
	p := filepath.Join(t.TempDir(), "proc.json")
	r, _ := Open(p)
	v1, err := r.Propose(goodProc(), known)
	if err != nil || v1.Version != 1 {
		t.Fatal(err)
	}
	if _, ok := r.Active("verify_go"); ok {
		t.Fatal("proposed must not be active")
	}
	_ = r.Activate("verify_go", 1)
	v2, _ := r.Propose(goodProc(), known)
	_ = r.Activate("verify_go", v2.Version)
	r2, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	if a, _ := r2.Active("verify_go"); a.Version != 2 {
		t.Fatalf("restart lost active: %d", a.Version)
	}
	_ = r2.Rollback("verify_go")
	if a, _ := r2.Active("verify_go"); a.Version != 1 {
		t.Fatal("rollback failed")
	}
	var ce *CheckError
	bad := goodProc()
	bad.Steps = nil
	if _, err := r2.Propose(bad, known); !errors.As(err, &ce) {
		t.Fatal("invalid proposal stored")
	}
}

func TestCorruptRegistryIsReportedNotOverwritten(t *testing.T) {
	p := filepath.Join(t.TempDir(), "proc.json")
	_ = os.WriteFile(p, []byte("{rota"), 0o644)
	if _, err := Open(p); err == nil {
		t.Fatal("corrupt registry loaded silently")
	}
}
