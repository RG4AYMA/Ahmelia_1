package autonomy

import (
	"sync"
	"testing"
)

func TestConversationManager_InitialState(t *testing.T) {
	cm := NewConversationManager()

	if cm.turnState != TurnAhmeliaIdle {
		t.Errorf("se esperaba estado inicial TurnAhmeliaIdle, se obtuvo %s", cm.turnState)
	}
	if cm.DeferredCount() != 0 {
		t.Errorf("se esperaba 0 temas diferidos, se obtuvo %d", cm.DeferredCount())
	}
}

func TestConversationManager_Rule1_UserSpeaking_Wait_ReturnsNone(t *testing.T) {
	cm := NewConversationManager()
	cm.SetTurnState(TurnUserSpeaking)

	decision := InitiativeDecision{
		Action: ActionWait,
		Reason: "Observaciones normales",
	}

	res := cm.EvaluateStrategy(decision)
	if res.Strategy != StrategyNone {
		t.Errorf("se esperaba StrategyNone, se obtuvo %s", res.Strategy)
	}
	if cm.DeferredCount() != 0 {
		t.Errorf("no se debió postergar nada en ActionWait, diferidos: %d", cm.DeferredCount())
	}
}

func TestConversationManager_Rule1_UserSpeaking_InformUser_Defers(t *testing.T) {
	cm := NewConversationManager()
	cm.SetTurnState(TurnUserSpeaking)
	cm.SetCurrentTopic("programacion_go")

	decision := InitiativeDecision{
		Action:       ActionInformUser,
		TargetGoalID: "goal_42",
		Reason:       "Cambio de estado",
	}

	res := cm.EvaluateStrategy(decision)
	if res.Strategy != StrategyDefer {
		t.Errorf("se esperaba StrategyDefer, se obtuvo %s", res.Strategy)
	}
	if !res.Deferred {
		t.Error("se esperaba Deferred=true en la decisión")
	}
	if cm.DeferredCount() != 1 {
		t.Errorf("se esperaba 1 tema diferido en cola, se obtuvo %d", cm.DeferredCount())
	}
}

func TestConversationManager_Rule1_UserSpeaking_CriticalAskUser_Interrupts(t *testing.T) {
	cm := NewConversationManager()
	cm.SetTurnState(TurnUserSpeaking)

	decision := InitiativeDecision{
		Action: ActionAskUser,
		Risk:   RiskHigh,
		Reason: "Señal crítica",
	}

	res := cm.EvaluateStrategy(decision)
	if res.Strategy != StrategyInterruptLightly {
		t.Errorf("se esperaba StrategyInterruptLightly ante señal crítica, se obtuvo %s", res.Strategy)
	}
	if res.Deferred {
		t.Error("una señal crítica no debe diferirse")
	}
}

func TestConversationManager_Rule2_Idle_ResumeGoal_UsesDeferredQueue(t *testing.T) {
	cm := NewConversationManager()
	cm.SetTurnState(TurnAhmeliaIdle)
	cm.DeferTopic("refactor_autonomy", "goal_10", "Diferido previamente")

	decision := InitiativeDecision{
		Action:       ActionResumeGoal,
		TargetGoalID: "goal_fallback",
		Reason:       "Inactividad",
	}

	res := cm.EvaluateStrategy(decision)
	if res.Strategy != StrategyResumeTopic {
		t.Errorf("se esperaba StrategyResumeTopic, se obtuvo %s", res.Strategy)
	}
	if res.Topic != "refactor_autonomy" {
		t.Errorf("se esperaba recuperar el tema diferido 'refactor_autonomy', se obtuvo '%s'", res.Topic)
	}
	if cm.DeferredCount() != 0 {
		t.Errorf("se esperaba cola diferida vacía tras pop, se obtuvo %d", cm.DeferredCount())
	}
}

func TestConversationManager_Rule2_Idle_ResumeGoal_FallbackToTargetGoalID(t *testing.T) {
	cm := NewConversationManager()
	cm.SetTurnState(TurnAhmeliaIdle)

	decision := InitiativeDecision{
		Action:       ActionResumeGoal,
		TargetGoalID: "goal_73_1",
		Reason:       "Inactividad",
	}

	res := cm.EvaluateStrategy(decision)
	if res.Strategy != StrategyResumeTopic {
		t.Errorf("se esperaba StrategyResumeTopic, se obtuvo %s", res.Strategy)
	}
	if res.Topic != "goal_73_1" {
		t.Errorf("se esperaba que el tema fuera TargetGoalID 'goal_73_1', se obtuvo '%s'", res.Topic)
	}
}

func TestConversationManager_Rule3_InformUser_AppendsContextWithoutChangingTopic(t *testing.T) {
	cm := NewConversationManager()
	cm.SetTurnState(TurnAhmeliaIdle)
	cm.SetCurrentTopic("arquitectura_b76")

	decision := InitiativeDecision{
		Action: ActionInformUser,
		Reason: "Cambio de estado en objetivo ajeno",
	}

	res := cm.EvaluateStrategy(decision)
	if res.Strategy != StrategyAppendContext {
		t.Errorf("se esperaba StrategyAppendContext, se obtuvo %s", res.Strategy)
	}
	if res.Topic != "arquitectura_b76" {
		t.Errorf("el tema debió mantenerse intacto en 'arquitectura_b76', se obtuvo '%s'", res.Topic)
	}
	if cm.CurrentTopic() != "arquitectura_b76" {
		t.Errorf("CurrentTopic no debió cambiar en el manager, es '%s'", cm.CurrentTopic())
	}
}

func TestConversationManager_Rule4_Suggest_AppendsContext(t *testing.T) {
	cm := NewConversationManager()
	cm.SetTurnState(TurnAhmeliaIdle)

	decision := InitiativeDecision{
		Action: ActionSuggest,
		Reason: "Sugerencia proactiva",
	}

	res := cm.EvaluateStrategy(decision)
	if res.Strategy != StrategyAppendContext {
		t.Errorf("se esperaba StrategyAppendContext, se obtuvo %s", res.Strategy)
	}
}

func TestConversationManager_QueueOperations(t *testing.T) {
	cm := NewConversationManager()

	cm.DeferTopic("topic_1", "goal_1", "reason_1")
	cm.DeferTopic("topic_2", "goal_2", "reason_2")

	if cm.DeferredCount() != 2 {
		t.Fatalf("se esperaban 2 temas diferidos, hay %d", cm.DeferredCount())
	}

	dt1, ok1 := cm.PopDeferredTopic()
	if !ok1 || dt1.Topic != "topic_1" {
		t.Errorf("se esperaba recuperar FIFO 'topic_1', se obtuvo '%s'", dt1.Topic)
	}

	dt2, ok2 := cm.PopDeferredTopic()
	if !ok2 || dt2.Topic != "topic_2" {
		t.Errorf("se esperaba recuperar FIFO 'topic_2', se obtuvo '%s'", dt2.Topic)
	}

	_, ok3 := cm.PopDeferredTopic()
	if ok3 {
		t.Error("se esperaba false al extraer de cola vacía")
	}
}

func TestConversationManager_ConcurrencySafety(t *testing.T) {
	cm := NewConversationManager()
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			cm.SetTurnState(TurnUserSpeaking)
			cm.SetCurrentTopic("concurrency_test")
			cm.DeferTopic("topic_concurrent", "goal_c", "testing")
			_ = cm.DeferredCount()
			_, _ = cm.PopDeferredTopic()
			cm.SetTurnState(TurnAhmeliaIdle)
			_ = cm.EvaluateStrategy(InitiativeDecision{Action: ActionInformUser})
		}(i)
	}

	wg.Wait()
}
