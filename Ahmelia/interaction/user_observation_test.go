package interaction

import (
	"testing"
	"time"
)

func TestDetectUserObservationsFromSemanticRejectsSelfOwnership(t *testing.T) {
	r := Analyze("Mi color favorito es negro", Context{})
	r.Subject = UserObservationSubjectSelf
	if got := DetectUserObservationsFromSemantic(r, time.Now(), "turn-1"); len(got) != 0 {
		t.Fatalf("self-owned semantic turn must not create user observations: %+v", got)
	}
}
