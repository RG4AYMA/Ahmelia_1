package capability

import "ahmelia/actioncatalog"

// SafetyTier is an alias to the leaf action contract so capability and its
// consumers cannot invent a second risk type.
type SafetyTier = actioncatalog.SafetyTier

const (
	SafetyTierSafe        = actioncatalog.SafetyTierSafe
	SafetyTierReversible  = actioncatalog.SafetyTierReversible
	SafetyTierDestructive = actioncatalog.SafetyTierDestructive
	SafetyTierSensitive   = actioncatalog.SafetyTierSensitive
)

// BuiltinActionCatalog exposes the canonical local action metadata while
// keeping the package-local Capability contract separate from lexical data.
func BuiltinActionCatalog() []actioncatalog.ActionDefinition {
	return actioncatalog.BuiltinActions()
}

func BuiltinCapabilities() []Capability {
	defs := actioncatalog.BuiltinActions()
	result := make([]Capability, 0, len(defs))
	for _, def := range defs {
		result = append(result, Capability{
			ID:          ID(def.Action),
			Domain:      DomainLocal,
			Lane:        LaneAction,
			Description: def.Description,
			Permission:  PermissionLevel(def.Permission),
			Action:      def.Action,
		})
	}
	return result
}

func ActionDefinitionFor(action string) (actioncatalog.ActionDefinition, bool) {
	return actioncatalog.ActionDefinitionFor(action)
}

func StandaloneExecutableActions() map[string]bool {
	return actioncatalog.StandaloneExecutableActions()
}
