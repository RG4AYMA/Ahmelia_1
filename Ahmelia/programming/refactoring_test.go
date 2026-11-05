package programming

import "testing"

func TestRefactorEngine_RenameProducesPatchesWithoutWriting(t *testing.T) {
	root := t.TempDir()
	project := CodeProject{ID: "p", Root: root, Language: "go"}
	engine := NewRefactorEngine()
	result, err := engine.Propose(RefactorRequest{
		ID: "r1", Project: project, Kind: RefactorRename,
		Description: "rename value", Target: "oldName", Replacement: "newName",
		Files: map[string]string{
			"a.go": "package p\nfunc f() { oldName := 1; _ = oldName }\n",
			"b.go": "package p\nvar oldName = 2\n",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Supported || result.FilesChanged != 2 || result.Occurrences != 3 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if len(result.Patches) != 2 {
		t.Fatalf("expected 2 patches, got %d", len(result.Patches))
	}
}

func TestRefactorEngine_RenameDoesNotMatchSubstrings(t *testing.T) {
	project := CodeProject{ID: "p", Root: t.TempDir(), Language: "go"}
	result, err := NewRefactorEngine().Propose(RefactorRequest{
		ID: "r2", Project: project, Kind: RefactorRename, Description: "rename", Target: "foo", Replacement: "bar",
		Files: map[string]string{"a.go": "package p\nvar fooBar = foo\n"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Occurrences != 1 || len(result.Patches) != 1 {
		t.Fatalf("unexpected: %+v", result)
	}
}

func TestRefactorEngine_UnsupportedKindsDoNotMutate(t *testing.T) {
	project := CodeProject{ID: "p", Root: t.TempDir(), Language: "go"}
	result, err := NewRefactorEngine().Propose(RefactorRequest{
		ID: "r3", Project: project, Kind: RefactorExtract, Description: "extract function",
		Files: map[string]string{"a.go": "package p\nfunc f() {}\n"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Supported || len(result.Patches) != 0 {
		t.Fatalf("expected unsupported proposal: %+v", result)
	}
}

func TestRefactorEngine_RejectsPathTraversal(t *testing.T) {
	project := CodeProject{ID: "p", Root: t.TempDir(), Language: "go"}
	_, err := NewRefactorEngine().Propose(RefactorRequest{
		ID: "r4", Project: project, Kind: RefactorRename, Description: "rename", Target: "foo", Replacement: "bar",
		Files: map[string]string{"../outside.go": "foo"},
	})
	if err == nil {
		t.Fatal("expected path traversal rejection")
	}
}
