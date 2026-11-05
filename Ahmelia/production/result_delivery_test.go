package production

import (
	"ahmelia/tools"
	"testing"
	"time"
)

func TestResultDeliveryUsesVerifiedResult(t *testing.T) {
	d := NewResultDelivery()
	verified := tools.VerifiedResult{ActionID: "a1", Success: true, Reason: "verified", Attempts: []tools.Attempt{{Number: 1, Execution: tools.Result{ActionID: "a1", Success: true, Output: "created"}}}}
	r, err := d.Deliver("p1", "g1", "t1", verified)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Success || r.Output != "created" || r.ActionID != "a1" {
		t.Fatalf("unexpected result: %+v", r)
	}
	if r.DeliveredAt.IsZero() || !r.DeliveredAt.Before(time.Now().Add(time.Second)) {
		t.Fatal("delivery timestamp missing")
	}
	if DeliveryStatusOf(r) != DeliveryDelivered {
		t.Fatal("expected delivered")
	}
}

func TestResultDeliveryRejectsUnverifiedFailure(t *testing.T) {
	d := NewResultDelivery()
	_, err := d.Deliver("p1", "g1", "t1", tools.VerifiedResult{ActionID: "a1", Success: false, Reason: "not confirmed"})
	if err == nil {
		t.Fatal("expected rejection")
	}
}

func TestResultDeliveryRequiresIdentity(t *testing.T) {
	d := NewResultDelivery()
	_, err := d.Deliver("", "g1", "t1", tools.VerifiedResult{ActionID: "a1", Success: true})
	if err == nil {
		t.Fatal("expected identity validation")
	}
}
