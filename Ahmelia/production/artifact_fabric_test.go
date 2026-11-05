package production

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestArtifactFabricRecordsObservableIdentity(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "scene.blend")
	if err := os.WriteFile(path, []byte("artifact-v1"), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx := &ProductionContext{}
	if err := ctx.RecordArtifact(ProductionArtifact{ID: "scene-1", Name: "scene.blend", Path: path, Type: "3d", Producer: "blender"}); err != nil {
		t.Fatal(err)
	}
	got, ok := ctx.ResolveArtifact("scene-1")
	if !ok {
		t.Fatal("artifact not recorded")
	}
	if got.Version != 1 || got.SizeBytes != int64(len("artifact-v1")) || got.ChecksumSHA256 == "" || !got.IntegrityVerified || got.Verified {
		t.Fatalf("artifact identity not captured: %+v", got)
	}
	if !strings.Contains(got.IntegrityReason, "SHA-256") {
		t.Fatalf("unexpected integrity reason: %q", got.IntegrityReason)
	}
}

func TestArtifactFabricDetectsMutation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "output.txt")
	if err := os.WriteFile(path, []byte("v1"), 0o600); err != nil {
		t.Fatal(err)
	}
	a, err := EnrichArtifactFromPath(ProductionArtifact{ID: "a1", Name: "output.txt", Path: path})
	if err != nil {
		t.Fatal(err)
	}
	ok, _, err := VerifyArtifact(a)
	if err != nil || !ok {
		t.Fatalf("expected original artifact to verify: ok=%v err=%v", ok, err)
	}
	if err := os.WriteFile(path, []byte("v2"), 0o600); err != nil {
		t.Fatal(err)
	}
	ok, reason, err := VerifyArtifact(a)
	if err != nil || ok || !strings.Contains(reason, "differ") {
		t.Fatalf("expected mutation detection: ok=%v reason=%q err=%v", ok, reason, err)
	}
}

func TestArtifactFabricPreservesUnavailableRemoteArtifact(t *testing.T) {
	a := ProductionArtifact{ID: "remote-1", Name: "remote.bin", Path: `Z:\worker\remote.bin`, Type: "binary", Producer: "remote-worker"}
	got := EnrichArtifactIfAvailable(a)
	if got.Verified || got.ChecksumSHA256 != "" {
		t.Fatalf("unavailable artifact must not be falsely verified: %+v", got)
	}
}
