package integration

import (
	"testing"
	"time"

	"ahmelia/production"
)

func TestProductionArtifactResolverPreservesVerificationIdentity(t *testing.T) {
	ctx := &production.ProductionContext{}
	ctx.Set("p1", "test", production.ProductionProject{ID: "p1", Path: "."}, []production.ProductionArtifact{{
		ID: "artifact-1", Name: "scene.blend", Path: "scene.blend", Verified: true,
		ChecksumSHA256: "abc123", Version: 2, CreatedAt: time.Now(),
	}})
	resolver := ProductionArtifactResolver{Context: ctx}
	got, ok := resolver.ResolveArtifact("artifact-1")
	if !ok {
		t.Fatal("expected artifact to resolve")
	}
	if !got.Verified || got.Checksum != "abc123" || got.ID != "artifact-1" {
		t.Fatalf("unexpected resolved artifact: %#v", got)
	}
}
