package integration

import (
	"ahmelia/execution"
	"ahmelia/production"
)

// ProductionArtifactResolver bridges the domain-neutral execution artifact
// gate to the existing ProductionContext. Execution remains unaware of
// ProductionArtifact; integration is the only layer that translates the
// domain-specific record into the generic execution contract.
type ProductionArtifactResolver struct {
	Context *production.ProductionContext
}

func (r ProductionArtifactResolver) ResolveArtifact(id string) (execution.ArtifactRequirement, bool) {
	if r.Context == nil {
		return execution.ArtifactRequirement{}, false
	}
	a, ok := r.Context.ResolveArtifact(id)
	if !ok {
		return execution.ArtifactRequirement{}, false
	}
	return execution.ArtifactRequirement{
		ID:       a.ID,
		Checksum: a.ChecksumSHA256,
		Verified: a.Verified,
	}, true
}
