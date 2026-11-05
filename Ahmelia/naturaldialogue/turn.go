package naturaldialogue

func ManageTurn(input string, responding bool) (TurnOwner, TurnAction) {
	if responding {
		return AhmeliaTurn, Yield
	}
	if input == "" {
		return WaitingForUser, Yield
	}
	return UserTurn, Continue
}
