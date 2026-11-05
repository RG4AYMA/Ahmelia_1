package naturaldialogue

import "testing"

func TestB9639GreetingFlow(t *testing.T) {
	e := NewE2E()
	d := e.Run("Hola")
	if d.Action != Continue || d.Flow != Continuing {
		t.Fatalf("%+v", d)
	}
}
func TestB9639Question(t *testing.T) {
	d := NewE2E().Run("¿Cómo funciona esto?")
	if d.Question != Question || d.FollowUp != FollowExpected {
		t.Fatalf("%+v", d)
	}
}
func TestB9639MultiQuestion(t *testing.T) {
	d := NewE2E().Run("¿Qué haces? ¿Y cómo aprendes?")
	if d.Question != MultiQuestion {
		t.Fatal(d.Question)
	}
}
func TestB9639Continuation(t *testing.T) {
	d := NewE2E().I.Evaluate("Blender", "Y ahora quiero hacer el modelo.", WaitingForUser)
	if d.Topic != CurrentTopic || d.Closing {
		t.Fatalf("%+v", d)
	}
}
func TestB9639Transition(t *testing.T) {
	d := NewE2E().I.Evaluate("Blender", "Cambiemos de tema, hablemos de audio", WaitingForUser)
	if d.Topic != NewTopic || d.Flow != Transitioning {
		t.Fatalf("%+v", d)
	}
}
func TestB9639Correction(t *testing.T) {
	d := NewE2E().Run("No, en realidad quería decir otra cosa")
	if !d.Corrected {
		t.Fatal("correction not detected")
	}
}
func TestB9639Clarification(t *testing.T) {
	d := NewE2E().Run("No entiendo")
	if !d.Clarify {
		t.Fatal("clarification not detected")
	}
}
func TestB9639Closing(t *testing.T) {
	d := NewE2E().Run("Chao, nos vemos")
	if !d.Closing || d.Action != Close {
		t.Fatalf("%+v", d)
	}
}
func TestB9639Adaptation(t *testing.T) {
	d := NewE2E().Run("Explícame el código paso a paso")
	if d.Adaptation != AdaptTechnical || d.Rhythm != Explanatory {
		t.Fatalf("%+v", d)
	}
}
func TestB9639TurnYield(t *testing.T) {
	d := NewE2E().I.Evaluate("", "hola", AhmeliaTurn)
	if d.Action != Yield || d.Turn != AhmeliaTurn {
		t.Fatalf("%+v", d)
	}
}
func TestB9639Threads(t *testing.T) {
	m := NewThreadManager()
	m.Switch("Blender")
	m.Switch("Ahmelia")
	if len(m.List()) != 2 {
		t.Fatal(len(m.List()))
	}
}
