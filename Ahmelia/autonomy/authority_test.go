package autonomy

import "testing"

func TestCanExecute_OnlyAuthorityExecuteAllowsExecution(t *testing.T) {
	all := []DecisionAuthority{
		AuthorityNone, AuthorityRecommend, AuthorityInitiate,
		AuthoritySelectGoal, AuthorityPlan, AuthorityResolve,
		AuthorityExecute, AuthorityVerify, AuthorityRecover,
	}
	for _, a := range all {
		got := CanExecute(a)
		want := a == AuthorityExecute
		if got != want {
			t.Errorf("CanExecute(%q) = %v, esperaba %v", a, got, want)
		}
	}
}

func TestNewAuthorityDecision_AllowedSeCalculaNuncaSeDeclara(t *testing.T) {
	exec := NewAuthorityDecision("Executor", AuthorityExecute, "acción autorizada por el ciclo")
	if !exec.Allowed {
		t.Fatal("una decisión con AuthorityExecute debe quedar Allowed=true")
	}

	rec := NewAuthorityDecision("Reasoning", AuthorityRecommend, "sería conveniente abrir VSCode")
	if rec.Allowed {
		t.Fatal("una decisión con AuthorityRecommend debe quedar Allowed=false, aunque el Source la proponga")
	}
}

func TestAuthorityForInitiativeAction_ResumeGoalYExecuteSonInitiate(t *testing.T) {
	for _, action := range []DecisionAction{ActionResumeGoal, ActionExecute} {
		if got := AuthorityForInitiativeAction(action); got != AuthorityInitiate {
			t.Errorf("AuthorityForInitiativeAction(%q) = %q, esperaba %q", action, got, AuthorityInitiate)
		}
	}
}

func TestAuthorityForInitiativeAction_AccionesConversacionalesSonRecommend(t *testing.T) {
	for _, action := range []DecisionAction{ActionAskUser, ActionInformUser, ActionSuggest} {
		if got := AuthorityForInitiativeAction(action); got != AuthorityRecommend {
			t.Errorf("AuthorityForInitiativeAction(%q) = %q, esperaba %q", action, got, AuthorityRecommend)
		}
	}
}

func TestAuthorityForInitiativeAction_WaitYNoActionSonNone(t *testing.T) {
	for _, action := range []DecisionAction{ActionWait, ActionNoAction} {
		if got := AuthorityForInitiativeAction(action); got != AuthorityNone {
			t.Errorf("AuthorityForInitiativeAction(%q) = %q, esperaba %q", action, got, AuthorityNone)
		}
	}
}

func TestAuthorityForInitiativeAction_ValorDesconocidoQuedaEnNone(t *testing.T) {
	// Simula que B75 agregara un DecisionAction nuevo sin actualizar este
	// puente: el default no debe heredar ejecución por accidente.
	if got := AuthorityForInitiativeAction(DecisionAction("ALGO_NUEVO")); got != AuthorityNone {
		t.Errorf("un DecisionAction desconocido debe caer en AuthorityNone, dio %q", got)
	}
}

func TestAllowsCycleExecution_SoloResumeGoalYExecuteHabilitan(t *testing.T) {
	habilitan := []DecisionAction{ActionResumeGoal, ActionExecute}
	for _, action := range habilitan {
		decision := InitiativeDecision{Action: action}
		if !AllowsCycleExecution(decision) {
			t.Errorf("AllowsCycleExecution(%q) = false, esperaba true", action)
		}
	}
}

func TestAllowsCycleExecution_ElRestoBloqueaElCiclo(t *testing.T) {
	bloquean := []DecisionAction{
		ActionNoAction, ActionWait, ActionAskUser, ActionInformUser, ActionSuggest,
	}
	for _, action := range bloquean {
		decision := InitiativeDecision{Action: action}
		if AllowsCycleExecution(decision) {
			t.Errorf("AllowsCycleExecution(%q) = true, esperaba false (no debe habilitar el ciclo ejecutivo)", action)
		}
	}
}
