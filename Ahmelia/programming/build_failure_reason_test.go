package programming

import (
	"strings"
	"testing"
)

func TestBuildFailureReasonKeepsGoErrorBelowPackageHeader(t *testing.T) {
	build := Build{
		Status: BuildFailed,
		Output: "# ahmelia.local/project-x\nruntime.main_main·f: function main is undeclared in the main package\n",
		Errors: []string{
			"# ahmelia.local/project-x",
			"runtime.main_main·f: function main is undeclared in the main package",
		},
	}
	reason := buildFailureReason(build, nil)
	if !strings.Contains(reason, "function main is undeclared") {
		t.Fatalf("reason lost the real diagnostic: %q", reason)
	}
	if strings.Contains(reason, "# ahmelia.local/project-x") {
		t.Fatalf("reason kept the package header: %q", reason)
	}
}

func TestBuildFailureReasonPrefersStructuredDiagnostics(t *testing.T) {
	build := Build{Errors: []string{"# module", "file.go:1:2: undefined: x"}}
	analysis := &ErrorAnalysis{Diagnostics: []Diagnostic{{
		File: "file.go", Line: 1, Column: 2, Message: "undefined: x",
	}}}
	reason := buildFailureReason(build, analysis)
	if !strings.Contains(reason, "file.go:1:2: undefined: x") {
		t.Fatalf("unexpected reason: %q", reason)
	}
}
