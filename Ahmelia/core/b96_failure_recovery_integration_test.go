package core

import (
	"context"
	"strings"
	"testing"

	"ahmelia/production"
	"ahmelia/programming"
)

func TestB96FailureReachesAnalysisDebuggingAndB95Recovery(t *testing.T) {
	output := `# hello_ahmelia
./main_test.go:5:6: helloMessage redeclared in this block
	./main.go:5:6: other declaration of helloMessage
FAIL	hello_ahmelia [build failed]`

	analysis, err := programming.AnalyzeErrors(programming.ErrorAnalysisRequest{
		ProjectID: "hello_ahmelia",
		Language:  "Go",
		Output:    output,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(analysis.Diagnostics) == 0 {
		t.Fatal("B96.11 no produjo diagnóstico")
	}
	if !strings.Contains(analysis.Diagnostics[0].Message, "helloMessage redeclared") {
		t.Fatalf("diagnóstico inesperado: %+v", analysis.Diagnostics[0])
	}

	assessment, err := programming.AssessFailureForDebugging(context.Background(), "hello_ahmelia", analysis.Diagnostics)
	if err != nil {
		t.Fatal(err)
	}
	if assessment.Resolved {
		t.Fatal("B96.12 no debe mutar/resolver sin patch preciso")
	}

	recovery := production.NewTechnicalRecoveryService()
	decision, err := recovery.Recover(context.Background(), "programming.hello_ahmelia", assessment.Reason, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !decision.Decision.Action.Valid() {
		t.Fatalf("B95 Recovery devolvió decisión inválida: %+v", decision)
	}
	if decision.Decision.Action != "retry" {
		t.Fatalf("esperaba decisión de retry para fallo técnico inicial, got %q", decision.Decision.Action)
	}
}
