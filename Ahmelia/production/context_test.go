package production

import (
	"path/filepath"
	"testing"
)

func TestProductionContextIsDomainNeutral(t *testing.T) {
	root := filepath.Join("produced", "creative-project")
	ctx := &ProductionContext{}
	ctx.Set("p-1", "blender", ProductionProject{ID: "creative-project", Name: "Creative Project", Path: root}, []ProductionArtifact{
		{ID: "a-image", Name: "render.png", Path: filepath.Join(root, "render.png"), Type: "image", Producer: "blender"},
		{ID: "a-model", Name: "scene.blend", Path: filepath.Join(root, "scene.blend"), Type: "3d-model", Producer: "blender"},
	})

	a, ok := ctx.ResolveArtifact("scene.blend")
	if !ok || a.Type != "3d-model" {
		t.Fatalf("expected generic artifact resolution, got %#v, %v", a, ok)
	}

	if got, ok := ctx.DescribeProject(); !ok || got == "" {
		t.Fatalf("expected project description, got %q, %v", got, ok)
	}
	if got, ok := ctx.DescribeArtifacts(); !ok || got == "" {
		t.Fatalf("expected artifact description, got %q, %v", got, ok)
	}
}

func TestProductionContextCanRecordIncrementally(t *testing.T) {
	ctx := &ProductionContext{}
	if err := ctx.RecordArtifact(ProductionArtifact{
		ID: "a1", Name: "output.mp4", Path: filepath.Join("out", "output.mp4"), Type: "video",
	}); err != nil {
		t.Fatal(err)
	}
	if err := ctx.RecordArtifact(ProductionArtifact{
		ID: "a2", Name: "output.png", Path: filepath.Join("out", "output.png"), Type: "image",
	}); err != nil {
		t.Fatal(err)
	}
	if _, ok := ctx.ResolveArtifact("output.mp4"); !ok {
		t.Fatal("expected video artifact")
	}
}
