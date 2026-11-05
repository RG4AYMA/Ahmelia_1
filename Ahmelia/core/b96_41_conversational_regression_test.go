package core

import (
	"strings"
	"sync"
	"testing"
	"time"

	"ahmelia/interaction"
)

func seedFavoriteColor(t *testing.T, e *Engine, value string) {
	t.Helper()
	if e == nil || e.userModelStore == nil {
		t.Fatal("test engine missing UserModelStore")
	}
	now := timeNowForTest()
	observation := interaction.UserObservation{
		Subject:    interaction.UserObservationSubjectUser,
		Domain:     interaction.UserModelDomainPreference,
		Key:        "color favorito",
		Value:      value,
		Confidence: .99,
		Provenance: interaction.UserModelSourceRef{Kind: interaction.UserSourceExplicitUser, SourceID: "regression-seed", ActorID: "user", ObservedAt: now},
		Evidence:   "regression-seed",
		ObservedAt: now,
	}
	if _, handled, err := interaction.ApplyUserObservation(e.userModelStore, observation, now); err != nil || !handled {
		t.Fatalf("seed observation failed: handled=%v err=%v", handled, err)
	}
	if err := e.userModelStore.Save(interaction.UserModelPath, interaction.UserModelActorSystem); err != nil {
		t.Fatalf("save seed observation: %v", err)
	}
}

// timeNowForTest exists only to keep this regression file independent from any
// time provider abstraction that may evolve later.
func timeNowForTest() time.Time { return time.Now() }

func TestB9641FiveTurnConversationalRegression(t *testing.T) {
	e := newB963334AuditEngine(t)
	seedFavoriteColor(t, e, "negro")
	var responses []string
	e.SetResponseCallback(func(msg string) { responses = append(responses, msg) })

	turns := []string{
		"hola como estas?",
		"nada quería saber si sabías mi color favorito",
		"cual es mi color favorito?",
		"y tu tienes color favorito?",
		"tranquila pronto podrás empezar a ver el mundo, hare todo lo posible para que puedas empezar a ver por ti misma, asi podras saber cual es tu color favorito.",
	}
	for i, input := range turns {
		before := len(responses)
		e.Process(input)
		if len(responses) != before+1 {
			t.Fatalf("turn %d %q produced %d response(s), want exactly one", i+1, input, len(responses)-before)
		}
	}

	if !strings.Contains(strings.ToLower(responses[1]), "negro") || !strings.Contains(strings.ToLower(responses[2]), "negro") {
		t.Fatalf("user-model responses lost favorite_color evidence: %q / %q", responses[1], responses[2])
	}
	if strings.Contains(strings.ToLower(responses[3]), "tu color favorito es") {
		t.Fatalf("self question leaked user preference into self response: %q", responses[3])
	}
	if e.lastSituation.ID == "" || len(e.lastSituation.Relations) == 0 {
		t.Fatalf("final turn did not build relational Situation: %+v", e.lastSituation)
	}
	if e.lastConversationalCognition.Action.Primary != "propose" {
		t.Fatalf("future/proposal anchor did not reach action proposal: %q", e.lastConversationalCognition.Action.Primary)
	}
}

func TestB9641GenericAttributeKnowledgeQuestionReachesUserModel(t *testing.T) {
	e := newB963334AuditEngine(t)
	seedFavoriteColor(t, e, "negro")
	var responses []string
	e.SetResponseCallback(func(msg string) { responses = append(responses, msg) })

	for _, input := range []string{
		"¿Sabes cuál es mi color favorito?",
		"¿Tú sabes cuál es mi color favorito?",
	} {
		before := len(responses)
		e.Process(input)
		if len(responses) != before+1 {
			t.Fatalf("input %q produced %d response(s), want exactly one", input, len(responses)-before)
		}
		if !strings.Contains(strings.ToLower(responses[len(responses)-1]), "negro") {
			t.Fatalf("input %q did not retrieve user preference: %q", input, responses[len(responses)-1])
		}
		if e.lastConversationalCognition.Action.Primary == "clarify" {
			t.Fatalf("input %q unexpectedly entered clarification: %+v", input, e.lastConversationalCognition)
		}
	}
}

func TestB934TurnOwnershipSerializesTwentyConcurrentTurns(t *testing.T) {
	e := buildTestEngine(t)
	var mu sync.Mutex
	responses := 0
	e.SetResponseCallback(func(string) {
		mu.Lock()
		responses++
		mu.Unlock()
	})
	inputs := make([]string, 20)
	for i := range inputs {
		inputs[i] = "hola como estas"
	}
	var wg sync.WaitGroup
	for _, input := range inputs {
		wg.Add(1)
		go func(in string) {
			defer wg.Done()
			e.Process(in)
		}(input)
	}
	wg.Wait()
	mu.Lock()
	got := responses
	mu.Unlock()
	if got != len(inputs) {
		t.Fatalf("serialized turn ownership lost responses: got=%d want=%d", got, len(inputs))
	}
}
