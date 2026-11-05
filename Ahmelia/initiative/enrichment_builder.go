package initiative

import "time"

// BuildEnrichment is the narrow transport boundary used by Engine. The Engine
// owns the actual domain reads; B96.37.5 only receives detached observations.
func BuildEnrichment(
	now time.Time,
	context ContextEnrichment,
	cognition CognitionEnrichment,
	userModel UserModelEnrichment,
	historical HistoricalMemoryEnrichment,
	work WorkEnrichment,
	commitment CommitmentEnrichment,
	autonomy AutonomyEnrichment,
	availability AvailabilityEnrichment,
	privacy PrivacySafetyEnrichment,
	channel ChannelEnrichment,
) InitiativeSourceEnrichment {
	if now.IsZero() {
		now = time.Now()
	}
	fix := func(t *time.Time) {
		if t.IsZero() {
			*t = now
		}
	}
	fix(&context.ObservedAt)
	fix(&cognition.ObservedAt)
	fix(&userModel.ObservedAt)
	fix(&historical.ObservedAt)
	fix(&work.UpdatedAt)
	fix(&commitment.LatestAt)
	fix(&autonomy.ObservedAt)
	fix(&availability.ObservedAt)
	fix(&privacy.ObservedAt)
	fix(&channel.ObservedAt)
	return InitiativeSourceEnrichment{context, cognition, userModel, historical, work, commitment, autonomy, availability, privacy, channel}
}
