package core

import (
	"testing"

	"ahmelia/autonomy"
	"ahmelia/capability"
)

// approveTestProductionExecution supplies explicit authorization evidence for
// E2E tests whose purpose is to validate successful production behavior. The
// runtime itself remains fail-closed: production code defaults to ApprovalNone
// and the identity/permission owner is the only source allowed to provide a
// grant.
func approveTestProductionExecution(t *testing.T, e *Engine) {
	t.Helper()
	if e == nil {
		t.Fatal("nil engine")
	}
	e.capabilityApprovalResolver = func(_ capability.ID) autonomy.Approval {
		return autonomy.ApprovalGranted
	}
}
