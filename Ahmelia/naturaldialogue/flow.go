package naturaldialogue

func FlowFor(turn TurnOwner, closing, transition bool) FlowState {
	if closing {
		return Closing
	}
	if transition {
		return Transitioning
	}
	if turn == WaitingForUser {
		return Engaged
	}
	if turn == UserTurn {
		return Continuing
	}
	return Open
}
