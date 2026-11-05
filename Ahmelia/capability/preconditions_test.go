package capability

import (
	"context"
	"testing"
)

func TestPreconditionsFailClosedAndUseFacts(t *testing.T) {
	c := NewPreconditionChecker()
	c.Register("workspace_exists", func(_ context.Context, f Facts) (bool, string) {
		return f["workspace.exists"] == "true", "workspace.exists=" + f["workspace.exists"]
	})
	cp := Capability{Preconditions: []string{"workspace_exists", "toolchain_present"}}
	fails := c.Check(context.Background(), cp, Facts{"workspace.exists": "true"})
	if len(fails) != 1 || fails[0].Name != "toolchain_present" || fails[0].Evidence != "no_predicate_registered" {
		t.Fatalf("%+v", fails)
	}
	fails = c.Check(context.Background(), Capability{Preconditions: []string{"workspace_exists"}}, Facts{})
	if len(fails) != 1 {
		t.Fatalf("%+v", fails)
	}
}

func TestPredicatePanicDoesNotCrashAndBlocks(t *testing.T) {
	c := NewPreconditionChecker()
	c.Register("boom", func(context.Context, Facts) (bool, string) { panic("x") })
	if f := c.Check(context.Background(), Capability{Preconditions: []string{"boom"}}, nil); len(f) != 1 {
		t.Fatalf("%+v", f)
	}
}
