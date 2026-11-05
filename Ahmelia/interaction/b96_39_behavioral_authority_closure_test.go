package interaction

import (
	"testing"
	"time"
)

func TestB9639_SubjectAuthorityBlocksCrossOwnerUserModelRead(t *testing.T) {
	self := Analyze("¿Tú tienes color favorito?", Context{Language: "es"})
	if self.Subject != UserObservationSubjectSelf {
		t.Fatalf("subject=%q, want self", self.Subject)
	}
	if _, ok := DetectUserModelQuery(self); ok {
		t.Fatal("self-owned question must never enter User Model query")
	}

	user := Analyze("¿Cuál es mi color favorito?", Context{Language: "es"})
	if user.Subject != UserObservationSubjectUser {
		t.Fatalf("subject=%q, want user", user.Subject)
	}
	if _, ok := DetectUserModelQuery(user); !ok {
		t.Fatal("user-owned preference question should enter User Model query")
	}
}

func TestB9639_UserModelWriteReadSameTurnLifecycle(t *testing.T) {
	store, err := NewUserModelStore(NewUserModel("test-user"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	req := Analyze("Mi color favorito es negro", Context{Language: "es"})
	obs := DetectUserObservationsFromSemantic(req, now, "turn-test")
	if len(obs) != 1 {
		t.Fatalf("observations=%+v", obs)
	}
	outcome, handled, err := ApplyUserObservation(store, obs[0], now)
	if err != nil || !handled || !outcome.Applied {
		t.Fatalf("write outcome=%+v handled=%t err=%v", outcome, handled, err)
	}
	prefs, err := GetRelevantPreferences(store, UserModelActorCognition, UserModelQueryContext{Topics: []string{"color favorito"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(prefs) != 1 || prefs[0].Value != "negro" {
		t.Fatalf("read-after-write=%+v, want color favorito=negro", prefs)
	}
}

func TestB9639_FeedbackVocabularyIncludesShortPraise(t *testing.T) {
	for _, input := range []string{"Bien hecho", "Perfecto", "Muy bien", "Buen trabajo", "Excelente"} {
		r := Analyze(input, Context{Language: "es"})
		if !r.CommunicativeActs.Has(ActFeedback) {
			t.Fatalf("%q was not classified as feedback: %+v", input, r.CommunicativeActs)
		}
		intent := DetermineResponseIntent(r, ConversationSnapshot{Language: "es"})
		if intent.Kind != ResponseIntentAcknowledge {
			t.Fatalf("%q intent=%q, want acknowledge", input, intent.Kind)
		}
	}
}
