package core

import (
	"context"
	"strings"
	"testing"
	"time"

	"ahmelia/initiative"
	"ahmelia/interaction"
	"ahmelia/language"
)

// newIdleInitiativeEngine builds an Engine whose only interaction was one user
// turn on a real channel, `silence` ago. Nothing about the scene is scripted:
// the idle evaluation must decide from state whether Ahmelia speaks.
func newIdleInitiativeEngine(t *testing.T, silence time.Duration) (*Engine, *[]string) {
	isolateTestWorkspace(t)
	t.Helper()
	t.Setenv("AHMELIA_DISABLE_TTS", "1")

	e := NewEngine(nil, nil, nil, nil, nil)
	if e.conversationState == nil || e.focusArbiter == nil || e.conversationalInitiative == nil || e.initiativeSignals == nil {
		t.Fatalf("engine is missing a dependency of idle initiative: conversationState=%t focusArbiter=%t conversationalInitiative=%t initiativeSignals=%t",
			e.conversationState != nil, e.focusArbiter != nil, e.conversationalInitiative != nil, e.initiativeSignals != nil)
	}

	// Fase 2 (2026-10-18): una iniciativa solo habla oración generada (el
	// fallback genérico quedó sin voz). El generador de prueba entrega
	// contenido determinista; el caso «sin contenido» tiene su propio test.
	e.conversationGenerator = language.NewConversationalGenerator(&phase5FakeLanguageClient{
		available: true,
		response:  `{"response":"¿Cómo va la arquitectura?"}`,
	})

	// NewEngine loads core/memory/conversation_state.json from disk. A test
	// about "silence after ONE turn" must not inherit the real history.
	e.conversationState = interaction.NewConversationState()

	spoken := []string{}
	e.SetResponseCallback(func(msg string) { spoken = append(spoken, msg) })

	e.lastInputChannel = interaction.ChannelText
	e.conversationState.RecordTurn(interaction.SemanticRequest{
		Raw: "estoy diseñando la arquitectura", Normalized: "estoy diseñando la arquitectura",
		Kind: interaction.KindConversation, Topic: "arquitectura", Channel: interaction.ChannelText,
	}, "Entendido.")

	// Simulate silence by moving the last interaction into the past, the same
	// state a real session would be in after `silence` without a user turn.
	past := time.Now().Add(-silence)
	e.conversationState.LastTurnAt = past
	if n := len(e.conversationState.Turns); n > 0 {
		e.conversationState.Turns[n-1].At = past
	}
	return e, &spoken
}

// Item 1 acceptance: after 2 minutes of silence Ahmelia speaks first, through
// the arbiter, with no user turn. The failure messages name the stage so the
// test also answers "where does it stop?" instead of only "it did not speak".
func TestIdleInitiativeSpeaksFirstAfterSilence(t *testing.T) {
	e, spoken := newIdleInitiativeEngine(t, 2*time.Minute)

	e.evaluateIdleConversationalInitiative()
	if e.lastInitiativeProposal == nil {
		t.Fatal("stage 1 (idle evaluation): no initiative proposal was stored after 2 minutes of silence; " +
			"the block is before the arbiter (decision was not INTERVENE, or the channel/availability gates closed it)")
	}
	if e.lastInitiativeProposal.Initiative == nil {
		t.Fatal("stage 1: the stored proposal carries no delivery payload, so nothing could ever speak it")
	}

	if !e.runIdleFocusArbitration(context.Background()) {
		t.Fatal("stage 2 (arbitration/delivery): a proposal was stored but was not delivered; " +
			"the arbiter rejected it or executeInitiativeProposal failed")
	}
	if len(*spoken) != 1 || (*spoken)[0] == "" {
		t.Fatalf("expected exactly one spontaneous utterance, got %q", *spoken)
	}
	if (*spoken)[0] != "¿Cómo va la arquitectura?" {
		t.Fatalf("the spontaneous turn must be generated content, not a generic fallback: %q", (*spoken)[0])
	}
	if e.lastInitiativeProposal != nil {
		t.Fatal("the proposal must be consumed once arbitrated")
	}

	snap := e.conversationState.Snapshot()
	if snap.LastAssistant != (*spoken)[0] {
		t.Fatalf("the spontaneous turn was spoken but not recorded in ConversationState: last_assistant=%q spoken=%q", snap.LastAssistant, (*spoken)[0])
	}
	if e.lastAutonomousTurnAt.IsZero() {
		t.Fatal("delivery must start the yield boundary (lastAutonomousTurnAt)")
	}
}

// Fase 2 (2026-10-18): la iniciativa cuyo generador no entrega oración NO habla
// el fallback genérico — el defecto vivo de la corrida run2 («Encontré algo
// útil relacionado con lo que estamos haciendo.» dicho solo porque hubo
// señales). El intento cierra la puerta del turno y no registra habla.
func TestFase2InitiativeWithoutGeneratedContentStaysSilent(t *testing.T) {
	e, spoken := newIdleInitiativeEngine(t, 2*time.Minute)
	e.conversationGenerator = language.NewConversationalGenerator(&phase5FakeLanguageClient{available: false})

	e.evaluateIdleConversationalInitiative()
	if e.lastInitiativeProposal == nil {
		t.Fatal("stage 1 (idle evaluation): no initiative proposal stored; the silence test needs an intervention attempt")
	}
	if e.runIdleFocusArbitration(context.Background()) {
		t.Fatal("stage 2 (delivery): delivery claimed success with no generated content")
	}
	if len(*spoken) != 0 {
		t.Fatalf("the initiative spoke a generic fallback instead of staying silent: %q", *spoken)
	}
	if snap := e.conversationState.Snapshot(); strings.Contains(snap.LastAssistant, "útil") {
		t.Fatalf("the fallback speech leaked into conversation state: %q", snap.LastAssistant)
	}
	if !e.lastAutonomousTurnAt.IsZero() {
		t.Fatal("a silent attempt must not open the autonomous-turn boundary")
	}
	if e.lastInitiativeUserTurnAt.IsZero() {
		t.Fatal("the failed attempt must close the per-turn door (no tick retry)")
	}
}

// After speaking, silence alone must never create another turn: a new user turn
// has to be observed first (the yield boundary the code already documents).
func TestIdleInitiativeDoesNotRepeatWithoutNewUserTurn(t *testing.T) {
	e, spoken := newIdleInitiativeEngine(t, 2*time.Minute)

	e.evaluateIdleConversationalInitiative()
	if !e.runIdleFocusArbitration(context.Background()) {
		t.Skip("first delivery did not happen; TestIdleInitiativeSpeaksFirstAfterSilence reports the failing stage")
	}

	for i := 0; i < 5; i++ {
		e.evaluateIdleConversationalInitiative()
		e.runIdleFocusArbitration(context.Background())
	}
	if len(*spoken) != 1 {
		t.Fatalf("Ahmelia spoke again with no new user turn: %q", *spoken)
	}
}

// An initiative proposal with no evidence must be judged by policy, not crash
// the arbiter (isEligible used to index Evidence[0] unconditionally).
func TestFocusArbiterInitiativeWithoutEvidenceDoesNotPanic(t *testing.T) {
	arbiter := NewFocusArbiter(initiative.NewInterruptionPolicy())
	p := &ProposalContract{
		ID: "no-evidence", Kind: ProposalKindInitiative, CreatedAt: time.Now(),
		Intention: "check in", Priority: ProposalPriorityHigh, Channel: interaction.ChannelText,
	}
	if _, err := arbiter.Arbitrate([]*ProposalContract{p}, time.Now()); err != nil {
		t.Fatal(err)
	}
}

func idleOpening(ch interaction.InputChannel, availability initiative.UserAvailability) *ProposalContract {
	return &ProposalContract{
		ID: "idle-opening", Kind: ProposalKindInitiative, CreatedAt: time.Now(),
		Intention: "useful_information", Priority: ProposalPriorityMedium, Urgency: ProposalUrgencyNone,
		Channel:    ch,
		Evidence:   []ProposalEvidence{{Confidence: 0.9}},
		Initiative: &InitiativeDelivery{Idle: true, Opportunity: initiative.InitiativeOpportunity{UserAvailability: availability}},
	}
}

// Opening a turn with a free user is not an interruption: non-critical
// urgency must not block it.
func TestFocusArbiterAllowsOpeningWithFreeUser(t *testing.T) {
	arbiter := NewFocusArbiter(initiative.NewInterruptionPolicy())
	p := idleOpening(interaction.ChannelText, initiative.AvailabilityAvailable)
	res, err := arbiter.Arbitrate([]*ProposalContract{p}, time.Now())
	if err != nil || res.Selected != p {
		t.Fatalf("opening with a free user was rejected: err=%v reasoning=%q", err, res.Reasoning)
	}
}

// The same proposal must still be refused when the user is occupied, or when
// there is no channel to speak on.
func TestFocusArbiterStillProtectsOccupiedUserAndUnknownChannel(t *testing.T) {
	arbiter := NewFocusArbiter(initiative.NewInterruptionPolicy())

	busy := idleOpening(interaction.ChannelText, initiative.AvailabilityBusy)
	if res, _ := arbiter.Arbitrate([]*ProposalContract{busy}, time.Now()); res.Selected != nil {
		t.Fatal("non-critical initiative interrupted a busy user")
	}

	inFocus := idleOpening(interaction.ChannelText, initiative.AvailabilityAvailable)
	inFocus.InterruptionCost.InFocus = true
	if res, _ := arbiter.Arbitrate([]*ProposalContract{inFocus}, time.Now()); res.Selected != nil {
		t.Fatal("non-critical initiative interrupted a user in focus")
	}

	noChannel := idleOpening(interaction.ChannelUnknown, initiative.AvailabilityAvailable)
	if res, _ := arbiter.Arbitrate([]*ProposalContract{noChannel}, time.Now()); res.Selected != nil {
		t.Fatal("initiative spoke on an unknown channel")
	}
}
