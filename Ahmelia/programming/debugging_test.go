package programming

import (
	"context"
	"strings"
	"testing"
)

func debugFixture() (DebugRequest, SourceFile) {
	f := SourceFile{Path: "main.go", Language: "go", Role: RoleSource, ProjectID: "p1"}
	d := Diagnostic{ID: "d1", ProjectID: "p1", File: "main.go", Line: 10, Column: 2, ErrorType: "undefined_symbol", Message: "undefined: foo", Severity: SeverityError}
	return DebugRequest{ProjectID: "p1", Diagnostics: []Diagnostic{d}, Source: f, Before: "func main(){ foo() }", Expected: "foo()", Replacement: "println()"}, f
}

func TestPreciseDebuggerCreatesPatchFromDiagnostic(t *testing.T) {
	req, _ := debugFixture()
	got, err := NewPreciseDebugger(nil).DiagnoseAndPatch(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Resolved || !strings.Contains(got.Patch.After, "println()") {
		t.Fatalf("unexpected result: %+v", got)
	}
}

func TestPreciseDebuggerRejectsUnrelatedDiagnostic(t *testing.T) {
	req, f := debugFixture()
	req.Diagnostics[0].File = "other.go"
	req.Source = f
	if _, err := NewPreciseDebugger(nil).DiagnoseAndPatch(context.Background(), req); err == nil {
		t.Fatal("expected mismatch error")
	}
}

func TestPreciseDebuggerRejectsAmbiguousEdit(t *testing.T) {
	req, _ := debugFixture()
	req.Before = "foo(); foo()"
	if _, err := NewPreciseDebugger(nil).DiagnoseAndPatch(context.Background(), req); err == nil {
		t.Fatal("expected ambiguity error")
	}
}

func TestPreciseDebuggerHonorsCancellation(t *testing.T) {
	req, _ := debugFixture()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := NewPreciseDebugger(nil).DiagnoseAndPatch(ctx, req); err == nil {
		t.Fatal("expected cancellation")
	}
}

func TestPreciseDebuggerRejectsDiagnosticProjectMismatch(t *testing.T) {
	req, _ := debugFixture()
	req.Diagnostics[0].ProjectID = "other"
	if _, err := NewPreciseDebugger(nil).DiagnoseAndPatch(context.Background(), req); err == nil {
		t.Fatal("expected project mismatch")
	}
}
