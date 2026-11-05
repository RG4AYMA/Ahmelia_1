package knowledge

import "strings"

// InternetSourceEnv is the explicit runtime configuration switch for the
// external knowledge source. The source stays disabled unless this value is
// deliberately enabled by the host environment.
const InternetSourceEnv = "AHMELIA_KNOWLEDGE_INTERNET"

// InternetSourceConfig controls the InternetSource without coupling it to a
// particular configuration system. The Engine decides whether external search
// is enabled; the source itself remains responsible only for typed routing.
type InternetSourceConfig struct {
	Enabled bool
}

func NewInternetSourceWithConfig(fn QueryFunc, cfg InternetSourceConfig) *InternetSource {
	return &InternetSource{fn: fn, Enabled: cfg.Enabled}
}

// InternetSourceEnabled reads the canonical runtime flag using the supplied
// environment reader. This indirection keeps tests deterministic and avoids
// embedding process-global configuration state inside the source.
func InternetSourceEnabled(getenv func(string) string) bool {
	if getenv == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(getenv(InternetSourceEnv))) {
	case "1", "true", "yes", "on", "enabled":
		return true
	default:
		return false
	}
}
