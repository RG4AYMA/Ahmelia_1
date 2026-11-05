package socialinteraction

// B96.38.18 — migration registry. Existing behavior is classified; it is not executed here.
type LegacyDisposition string

const (
	Keep      LegacyDisposition = "KEEP"
	Move      LegacyDisposition = "MOVE"
	Adapt     LegacyDisposition = "ADAPT"
	Deprecate LegacyDisposition = "DEPRECATE"
	Wrap      LegacyDisposition = "WRAP"
)

type LegacyEntry struct {
	Name        string
	Disposition LegacyDisposition
	Owner       string
}
type LegacyRegistry struct{ Entries []LegacyEntry }

func NewLegacyRegistry() *LegacyRegistry {
	return &LegacyRegistry{Entries: []LegacyEntry{{"greetings", Move, "socialinteraction"}, {"farewells", Move, "socialinteraction"}, {"thanks", Move, "socialinteraction"}, {"courtesy", Move, "socialinteraction"}, {"humor", Adapt, "socialinteraction"}, {"followups", Wrap, "B96.37/B96.39"}, {"proactive_behavior", Wrap, "B96.37"}}}
}
