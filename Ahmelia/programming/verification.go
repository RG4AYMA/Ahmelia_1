package programming

import (
	"fmt"
	"strings"

	"ahmelia/tools"
)

// B96.13 — Code Verification
//
// Programming contributes domain-specific evidence about software state.
// B95 remains the authority for the final production verification decision.

type CodeVerificationRequest struct {
	Project         CodeProject
	Objective       ObjectiveContract
	Build           *Build
	Tests           *TestRun
	Runtime         *Runtime
	ExpectedFiles   []string
	ExpectedSymbols []string
	ExpectedOutput  string
}

type CodeVerificationEvidence struct {
	ProjectID string
	Objective ObjectiveContract
	// ObjectiveStatus is unassessed until independent evidence evaluates the
	// semantic objective. Build/test success alone cannot change this status.
	ObjectiveStatus string
	Checks          []CodeVerificationCheck
	Success         bool
}

type CodeVerificationCheck struct {
	Name    string
	Success bool
	Reason  string
}

type CodeVerifier interface {
	VerifyCode(CodeVerificationRequest) CodeVerificationEvidence
}

type DefaultCodeVerifier struct{}

func NewCodeVerifier() *DefaultCodeVerifier { return &DefaultCodeVerifier{} }

func (v *DefaultCodeVerifier) VerifyCode(req CodeVerificationRequest) CodeVerificationEvidence {
	evidence := CodeVerificationEvidence{ProjectID: req.Project.ID, Objective: req.Objective.Clone(), ObjectiveStatus: "unassessed", Success: true}
	if err := req.Project.Validate(); err != nil {
		return failCodeVerification(evidence, "project", err.Error())
	}
	if req.Build != nil {
		ok := req.Build.Status == BuildSuccess && req.Build.ExitCode == 0
		reason := "build successful"
		if !ok {
			reason = "build did not succeed"
		}
		evidence.Checks = append(evidence.Checks, CodeVerificationCheck{Name: "build_succeeds", Success: ok, Reason: reason})
		if !ok {
			evidence.Success = false
		}
	}
	if req.Tests != nil {
		ok := req.Tests.Status == TestPassed && req.Tests.ExitCode == 0
		reason := "tests passed"
		if !ok {
			reason = "tests did not pass"
		}
		evidence.Checks = append(evidence.Checks, CodeVerificationCheck{Name: "tests_pass", Success: ok, Reason: reason})
		if !ok {
			evidence.Success = false
		}
	}
	if req.Runtime != nil {
		ok := req.Runtime.Status != RuntimeFailed
		reason := "runtime not failed"
		if !ok {
			reason = "runtime failed"
		}
		evidence.Checks = append(evidence.Checks, CodeVerificationCheck{Name: "runtime_healthy", Success: ok, Reason: reason})
		if !ok {
			evidence.Success = false
		}
	}
	// File/symbol/output checks are evidence contracts. Concrete environment
	// observers can supply the facts; this verifier does not read or mutate the filesystem.
	for _, path := range req.ExpectedFiles {
		if strings.TrimSpace(path) == "" {
			evidence = failCodeVerification(evidence, "expected_file", "expected file path vacío")
			continue
		}
		evidence.Checks = append(evidence.Checks, CodeVerificationCheck{Name: "expected_file:" + path, Success: false, Reason: "requiere evidencia del entorno"})
		evidence.Success = false
	}
	for _, symbol := range req.ExpectedSymbols {
		if strings.TrimSpace(symbol) == "" {
			evidence = failCodeVerification(evidence, "expected_symbol", "expected symbol vacío")
			continue
		}
		evidence.Checks = append(evidence.Checks, CodeVerificationCheck{Name: "expected_symbol:" + symbol, Success: false, Reason: "requiere evidencia del analizador"})
		evidence.Success = false
	}
	if req.ExpectedOutput != "" {
		evidence.Checks = append(evidence.Checks, CodeVerificationCheck{Name: "expected_output", Success: false, Reason: "requiere evidencia del runtime"})
		evidence.Success = false
	}
	return evidence
}

func failCodeVerification(e CodeVerificationEvidence, name, reason string) CodeVerificationEvidence {
	e.Checks = append(e.Checks, CodeVerificationCheck{Name: name, Success: false, Reason: reason})
	e.Success = false
	return e
}

// ToVerification adapts programming evidence to the authoritative B95 tools
// verification model without creating a second verification authority.
func (e CodeVerificationEvidence) ToVerification(observation tools.Observation) tools.Verification {
	reason := "code verification passed"
	if !e.Success {
		reason = fmt.Sprintf("code verification failed (%d checks)", len(e.Checks))
	}
	return tools.Verification{Success: e.Success, Reason: reason, Observation: observation}
}
