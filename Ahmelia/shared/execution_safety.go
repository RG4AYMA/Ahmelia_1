package shared

// This package keeps the historical API surface, but risk metadata is owned
// by actioncatalog/. The alias prevents a second semantic SafetyTier type.

func SafetyTierOf(action string) SafetyTier {
	return ActionSafety[action]
}

func RequiresConfirmation(action string) bool {
	switch ActionSafety[action] {
	case SafetyTierSafe, SafetyTierReversible:
		return false
	default:
		return true
	}
}

func RequiresAuthorization(action string) bool {
	switch ActionSafety[action] {
	case SafetyTierSafe, SafetyTierReversible, SafetyTierDestructive:
		return false
	default:
		return true
	}
}
