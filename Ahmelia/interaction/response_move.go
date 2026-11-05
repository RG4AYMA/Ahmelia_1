package interaction

// ResponseMove is a concrete conversational move that a response plan may
// perform. It is deliberately smaller than ResponseIntent: an intent states
// the communicative purpose, while a move describes one step in fulfilling it.
type ResponseMove string

const (
	MoveAnswer      ResponseMove = "answer"
	MoveAcknowledge ResponseMove = "acknowledge"
	MoveInvite      ResponseMove = "invite"
	MoveClarify     ResponseMove = "clarify"
	MovePropose     ResponseMove = "propose"
	MoveContinue    ResponseMove = "continue"
	MoveWait        ResponseMove = "wait"
	MoveResume      ResponseMove = "resume"
	MoveAsk         ResponseMove = "ask"
	MoveRedirect    ResponseMove = "redirect"
)

// ResponseMoveIsValid keeps the vocabulary centralized so later response
// planning layers do not create incompatible move strings.
func ResponseMoveIsValid(m ResponseMove) bool {
	switch m {
	case MoveAnswer, MoveAcknowledge, MoveInvite, MoveClarify, MovePropose,
		MoveContinue, MoveWait, MoveResume, MoveAsk, MoveRedirect:
		return true
	default:
		return false
	}
}
