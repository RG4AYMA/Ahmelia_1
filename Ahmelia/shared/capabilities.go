package shared

import (
	"sort"
	"strings"

	"ahmelia/actioncatalog"
)

// ============================================================================
// CAPABILITIES.GO — vista compatible del catálogo canónico
// ============================================================================
//
// shared ya no mantiene la semántica de capacidades en paralelo. El paquete
// conserva estos nombres por compatibilidad con el clasificador y consumidores
// históricos, pero todos derivan del catálogo canónico de capability/.

var ActionLabels = actioncatalog.ActionLabels()
var ActionSkipInCapabilities = actioncatalog.ActionSkipInCapabilities()
var WiredActions = actioncatalog.WiredActions()
var ActionPatterns = actioncatalog.ActionTriggers()

type SafetyTier = actioncatalog.SafetyTier

const (
	SafetyTierSafe        = actioncatalog.SafetyTierSafe
	SafetyTierReversible  = actioncatalog.SafetyTierReversible
	SafetyTierDestructive = actioncatalog.SafetyTierDestructive
	SafetyTierSensitive   = actioncatalog.SafetyTierSensitive
)

var ActionSafety = actioncatalog.ActionSafety()

func CapabilityLabels() []string {
	defs := actioncatalog.BuiltinActions()
	seen := map[string]bool{}
	labels := make([]string, 0, len(defs))
	for _, def := range defs {
		if !def.VisibleCapability {
			continue
		}
		label := def.Label
		if label == "" {
			label = humanizeAction(def.Action)
		}
		if seen[label] {
			continue
		}
		seen[label] = true
		labels = append(labels, label)
	}
	sort.Strings(labels)
	return labels
}

func humanizeAction(action string) string {
	words := strings.Split(action, "_")
	if len(words) > 0 && len(words[0]) > 0 {
		words[0] = strings.ToUpper(words[0][:1]) + words[0][1:]
	}
	return strings.Join(words, " ")
}
