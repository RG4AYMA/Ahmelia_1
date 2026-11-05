package production

// WorkQueryKind identifies a structured question about production state.
type WorkQueryKind string

const (
	WorkQueryContinuation WorkQueryKind = "continuation"
	WorkQueryActive       WorkQueryKind = "active_work"
	WorkQueryRecentResult WorkQueryKind = "result"
	WorkQueryTest         WorkQueryKind = "test"
	WorkQueryLocation     WorkQueryKind = "location"
	WorkQueryProgress     WorkQueryKind = "progress"
	WorkQueryArtifacts    WorkQueryKind = "artifacts"
	WorkQueryCapabilities WorkQueryKind = "capabilities"
)

// WorkQueryResult is evidence about production state, not a response string.
type WorkQueryResult struct {
	Kind         WorkQueryKind
	Found        bool
	Work         WorkSession
	Artifact     ProductionArtifact
	HasArtifact  bool
	RecentResult string
	Capabilities []string
}
