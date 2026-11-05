package interaction

import "testing"

func TestResponseMoveVocabulary(t *testing.T) {
	moves := []ResponseMove{
		MoveAnswer, MoveAcknowledge, MoveInvite, MoveClarify, MovePropose,
		MoveContinue, MoveWait, MoveResume, MoveAsk, MoveRedirect,
	}
	for _, move := range moves {
		if !ResponseMoveIsValid(move) {
			t.Fatalf("expected valid response move %q", move)
		}
	}
	if ResponseMoveIsValid(ResponseMove("invent")) {
		t.Fatal("unknown response move must be rejected")
	}
}
