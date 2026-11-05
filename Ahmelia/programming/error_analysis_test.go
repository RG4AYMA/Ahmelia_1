package programming

import (
	"strings"
	"testing"
)

func TestAnalyzeErrors_GoDiagnostic(t *testing.T) {
	result, err := AnalyzeErrors(ErrorAnalysisRequest{
		ProjectID: "p",
		Language:  "Go",
		Output:    "main.go:12:7: undefined: foo\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 1 {
		t.Fatalf("diagnostics=%d", len(result.Diagnostics))
	}
	d := result.Diagnostics[0]
	if d.File != "main.go" || d.Line != 12 || d.Column != 7 {
		t.Fatalf("location=%+v", d)
	}
	if d.ErrorType != "undefined_symbol" {
		t.Fatalf("type=%q", d.ErrorType)
	}
	if d.Severity != SeverityError {
		t.Fatalf("severity=%q", d.Severity)
	}
}

func TestAnalyzeErrors_DeduplicatesDiagnostics(t *testing.T) {
	result, err := AnalyzeErrors(ErrorAnalysisRequest{
		ProjectID: "p", Language: "Go",
		Output: "main.go:2:3: undefined: x\nmain.go:2:3: undefined: x\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 1 {
		t.Fatalf("diagnostics=%d", len(result.Diagnostics))
	}
}

func TestAnalyzeErrors_Warning(t *testing.T) {
	result, err := AnalyzeErrors(ErrorAnalysisRequest{
		ProjectID: "p", Language: "Go",
		Output: "main.go:4:2: warning: example\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Severity != SeverityWarning {
		t.Fatalf("diagnostics=%+v", result.Diagnostics)
	}
}

func TestAnalyzeErrors_GenericFallback(t *testing.T) {
	result, err := AnalyzeErrors(ErrorAnalysisRequest{
		ProjectID: "p", Language: "Python",
		Output: "app.py:9:4: syntax error\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 1 {
		t.Fatalf("diagnostics=%d", len(result.Diagnostics))
	}
	if result.Diagnostics[0].ErrorType != "syntax_error" {
		t.Fatalf("type=%q", result.Diagnostics[0].ErrorType)
	}
}

func TestAnalyzeErrors_RejectsInvalidRequest(t *testing.T) {
	if _, err := AnalyzeErrors(ErrorAnalysisRequest{ProjectID: "p"}); err == nil {
		t.Fatal("empty output accepted")
	}
}

func TestAnalyzeErrors_DistinguishesUnusedImportFromMissingDependency(t *testing.T) {
	tests := []struct {
		name string
		line string
		want string
	}{
		{name: "unused source import", line: `main.go:3:8: "example.com/lib" imported and not used`, want: "unused_import"},
		{name: "missing module metadata", line: `main.go:3:8: no required module provides package example.com/lib`, want: "dependency_error"},
		{name: "package path is not in standard library", line: `f10_repair_test.go:6: package f10_repair is not in std (/usr/local/go/src/f10_repair)`, want: "dependency_error"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := AnalyzeErrors(ErrorAnalysisRequest{ProjectID: "p", Language: "Go", Output: tc.line})
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Diagnostics) != 1 || result.Diagnostics[0].ErrorType != tc.want {
				t.Fatalf("diagnostics=%+v, want error type %q", result.Diagnostics, tc.want)
			}
		})
	}
}

func TestFormatDiagnosticFailurePreservesEveryDiagnostic(t *testing.T) {
	diagnostics := []Diagnostic{
		{File: "alpha.go", Line: 3, Column: 25, Message: "undefined: X"},
		{File: "beta.go", Line: 4, Column: 20, Message: "undefined: Y"},
	}
	got := formatDiagnosticFailure("build failed", diagnostics)
	for _, want := range []string{"alpha.go:3:25: undefined: X", "beta.go:4:20: undefined: Y"} {
		if !strings.Contains(got, want) {
			t.Errorf("failure summary missing %q: %s", want, got)
		}
	}
}

func TestClassifyErrorType_MissingChecksumIsDependencyError(t *testing.T) {
	if got := classifyErrorType("example.com/lib@v1.2.3: missing go.sum entry for go.mod file"); got != "dependency_error" {
		t.Fatalf("type=%q, want dependency_error", got)
	}
}
