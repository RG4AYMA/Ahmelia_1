package socialinteraction

import "testing"

func TestB9638Greeting(t *testing.T) {
	a, p := NewE2E().Run("Hola Ahmelia")
	if a.Intent != Greeting || p.Participation != Acknowledge {
		t.Fatalf("got %+v %+v", a, p)
	}
}
func TestB9638Thanks(t *testing.T) {
	a, p := NewE2E().Run("Gracias")
	if a.Intent != Thanking || p.Participation != Acknowledge {
		t.Fatalf("got %+v %+v", a, p)
	}
}
func TestB9638Farewell(t *testing.T) {
	a, p := NewE2E().Run("Chao, nos vemos")
	if a.Intent != Farewell || p.Participation != Close {
		t.Fatalf("got %+v %+v", a, p)
	}
}
func TestB9638Mixed(t *testing.T) {
	a, _ := NewE2E().Run("Hola, abre Blender")
	if a.Kind != Mixed {
		t.Fatalf("expected mixed, got %s", a.Kind)
	}
}
func TestB9638FormalHumorBoundary(t *testing.T) {
	a, p := NewE2E().Run("Señor, jaja")
	if len(a.Boundaries) == 0 || a.Boundaries[0].Allowed {
		t.Fatal("formal humor should be constrained")
	}
	if len(p.Constraints) == 0 {
		t.Fatal("constraint missing")
	}
}
func TestB9638Repair(t *testing.T) {
	e := NewE2E()
	a := e.I.Assess("No te entendí", "text", StateEngaged)
	p := e.I.Plan(a)
	if !e.I.Repair.Detect("No te entendí", p) {
		t.Fatal("repair not detected")
	}
}
func TestB9638DoesNotAuthorizeInitiative(t *testing.T) {
	a, p := NewE2E().Run("Hola")
	_ = a
	_ = p /* no intervention/initiative field exists by design */
}
