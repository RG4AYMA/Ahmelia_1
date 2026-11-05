package memory

import "testing"

func TestSessionSnapshotIsStateOnlyProjection(t *testing.T) {
	s := NewSession()
	s.SetFileResults([]SessionFile{{Path: "a.txt"}})
	snap := s.Snapshot()
	if snap.State != "file_select" || len(snap.FileResults) != 1 {
		t.Fatalf("unexpected snapshot: %+v", snap)
	}
	snap.FileResults[0].Path = "mutated"
	if s.FileResults[0].Path != "a.txt" {
		t.Fatal("snapshot mutated session-owned state")
	}
}
