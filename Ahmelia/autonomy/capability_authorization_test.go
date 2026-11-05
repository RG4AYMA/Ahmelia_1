package autonomy

import (
	"testing"

	"ahmelia/capability"
)

func authorizationTestCapability(permission capability.PermissionLevel) capability.Capability {
	return capability.Capability{
		ID:         "test_capability",
		Domain:     capability.DomainLocal,
		Action:     "test_action",
		Permission: permission,
	}
}

func TestAuthorizeCapability_AutomaticAllowsExecution(t *testing.T) {
	result, err := AuthorizeCapability(authorizationTestCapability(capability.PermissionAutomatic), ApprovalNone)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Allowed {
		t.Fatal("PermissionAutomatic debe permitir ejecución")
	}
	if result.Authority != AuthorityExecute {
		t.Fatalf("Authority = %q, esperaba %q", result.Authority, AuthorityExecute)
	}
	if result.NeedsApproval {
		t.Fatal("PermissionAutomatic no debe requerir aprobación")
	}
}

func TestAuthorizeCapability_NotifyAllowsExecution(t *testing.T) {
	result, err := AuthorizeCapability(authorizationTestCapability(capability.PermissionNotify), ApprovalNone)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Allowed {
		t.Fatal("PermissionNotify debe permitir ejecución")
	}
	if result.Authority != AuthorityExecute {
		t.Fatalf("Authority = %q, esperaba %q", result.Authority, AuthorityExecute)
	}
	if result.NeedsApproval {
		t.Fatal("PermissionNotify no debe requerir aprobación previa")
	}
}

func TestAuthorizeCapability_AskFirstWithoutApprovalBlocks(t *testing.T) {
	result, err := AuthorizeCapability(authorizationTestCapability(capability.PermissionAskFirst), ApprovalNone)
	if err != nil {
		t.Fatal(err)
	}
	if result.Allowed {
		t.Fatal("PermissionAskFirst sin aprobación no debe permitir ejecución")
	}
	if result.Authority != AuthorityRecommend {
		t.Fatalf("Authority = %q, esperaba %q", result.Authority, AuthorityRecommend)
	}
	if !result.NeedsApproval {
		t.Fatal("PermissionAskFirst debe marcar NeedsApproval")
	}
}

func TestAuthorizeCapability_AskFirstGrantedAllowsExecution(t *testing.T) {
	result, err := AuthorizeCapability(authorizationTestCapability(capability.PermissionAskFirst), ApprovalGranted)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Allowed {
		t.Fatal("ApprovalGranted debe permitir ejecución")
	}
	if result.Authority != AuthorityExecute {
		t.Fatalf("Authority = %q, esperaba %q", result.Authority, AuthorityExecute)
	}
}

func TestAuthorizeCapability_AskFirstDeniedBlocks(t *testing.T) {
	result, err := AuthorizeCapability(authorizationTestCapability(capability.PermissionAskFirst), ApprovalDenied)
	if err != nil {
		t.Fatal(err)
	}
	if result.Allowed {
		t.Fatal("ApprovalDenied no debe permitir ejecución")
	}
	if result.Authority != AuthorityRecommend {
		t.Fatalf("Authority = %q, esperaba %q", result.Authority, AuthorityRecommend)
	}
}

func TestAuthorizeCapability_InvalidApprovalFailsClosed(t *testing.T) {
	_, err := AuthorizeCapability(authorizationTestCapability(capability.PermissionAutomatic), Approval("maybe"))
	if err == nil {
		t.Fatal("un Approval desconocido debe producir error")
	}
}

func TestAuthorizeCapabilityID_UsesRegisteredCapability(t *testing.T) {
	r := capability.NewRegistry()
	r.MustRegister(capability.Capability{
		ID:         "protected_action",
		Domain:     capability.DomainLocal,
		Action:     "hacer_algo",
		Permission: capability.PermissionAskFirst,
	})

	pending, err := AuthorizeCapabilityID(r, "protected_action", ApprovalNone)
	if err != nil {
		t.Fatal(err)
	}
	if pending.Allowed {
		t.Fatal("una capability ask_first descubierta desde Registry debe quedar bloqueada sin aprobación")
	}

	approved, err := AuthorizeCapabilityID(r, "protected_action", ApprovalGranted)
	if err != nil {
		t.Fatal(err)
	}
	if !approved.Allowed {
		t.Fatal("la misma capability debe permitir ejecución tras aprobación")
	}
}

func TestAuthorizeCapabilityID_MissingCapabilityFails(t *testing.T) {
	r := capability.NewRegistry()
	_, err := AuthorizeCapabilityID(r, "missing", ApprovalGranted)
	if err == nil {
		t.Fatal("una capability no registrada debe producir error")
	}
}

func TestAuthorizeCapabilityID_NilRegistryFails(t *testing.T) {
	_, err := AuthorizeCapabilityID(nil, "missing", ApprovalGranted)
	if err == nil {
		t.Fatal("Registry nil debe producir error")
	}
}
