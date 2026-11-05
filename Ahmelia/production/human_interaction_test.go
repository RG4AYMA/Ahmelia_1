package production

import (
	"context"
	"testing"

	"ahmelia/autonomy"
)

func TestHumanInteractionReusesConversationManager(t *testing.T) {
	cm := autonomy.NewConversationManager()
	cm.SetCurrentTopic("production")
	h, err := NewHumanInteraction(cm)
	if err != nil {
		t.Fatal(err)
	}
	d := autonomy.InitiativeDecision{Action: autonomy.ActionAskUser, Reason: "approval required"}
	got, err := h.Evaluate(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	if got.Strategy == autonomy.StrategyNone {
		t.Fatal("se esperaba interacción humana")
	}
}

func TestHumanInteractionDoesNotBypassExecutionAuthority(t *testing.T) {
	d := autonomy.InitiativeDecision{Action: autonomy.ActionAskUser}
	if !NeedsHumanResponse(d) {
		t.Fatal("AskUser debe requerir interacción")
	}
	if CanExecuteAfterInteraction(d) {
		t.Fatal("AskUser no debe otorgar autoridad de ejecución")
	}
}

func TestHumanInteractionInformAndSuggestAreHumanInteraction(t *testing.T) {
	for _, action := range []autonomy.DecisionAction{autonomy.ActionInformUser, autonomy.ActionSuggest} {
		if !NeedsHumanResponse(autonomy.InitiativeDecision{Action: action}) {
			t.Fatalf("%s debe requerir interacción humana", action)
		}
		if CanExecuteAfterInteraction(autonomy.InitiativeDecision{Action: action}) {
			t.Fatalf("%s no debe otorgar ejecución", action)
		}
	}
}

func TestHumanInteractionRejectsUnavailableDependencies(t *testing.T) {
	if _, err := NewHumanInteraction(nil); err == nil {
		t.Fatal("nil manager debe fallar")
	}
	h := &HumanInteraction{}
	if _, err := h.Evaluate(context.Background(), autonomy.InitiativeDecision{}); err == nil {
		t.Fatal("manager ausente debe fallar")
	}
}

func TestHumanInteractionCancelledContext(t *testing.T) {
	cm := autonomy.NewConversationManager()
	h, err := NewHumanInteraction(cm)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := h.Evaluate(ctx, autonomy.InitiativeDecision{}); err == nil {
		t.Fatal("contexto cancelado debe detener interacción")
	}
}
