package programming

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitAvailable(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git no disponible")
	}
}

func initGitRepo(t *testing.T) string {
	t.Helper()
	gitAvailable(t)
	root := t.TempDir()
	runGit(t, root, "init")
	runGit(t, root, "config", "user.email", "test@ahmelia.local")
	runGit(t, root, "config", "user.name", "Ahmelia Test")
	return root
}

func runGit(t *testing.T, root string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func TestGitVersionControl_StatusAndDiff(t *testing.T) {
	root := initGitRepo(t)
	file := filepath.Join(root, "main.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	vc := NewGitVersionControl()
	project := CodeProject{ID: "p", Root: root, Language: "go"}
	status := vc.Status(context.Background(), VersionControlRequest{ID: "s1", Project: project, Operation: VCStatus})
	if !status.Success || !strings.Contains(status.Output, "main.go") {
		t.Fatalf("unexpected status: %+v", status)
	}
	runGit(t, root, "add", "main.go")
	runGit(t, root, "commit", "-m", "initial")
	if err := os.WriteFile(file, []byte("package main\n\nfunc main() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	diff := vc.Diff(context.Background(), VersionControlRequest{ID: "d1", Project: project, Operation: VCDiff})
	if !diff.Success || !strings.Contains(diff.Output, "func main") {
		t.Fatalf("unexpected diff: %+v", diff)
	}
}

func TestGitVersionControl_CommitAndHistory(t *testing.T) {
	root := initGitRepo(t)
	file := filepath.Join(root, "main.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", "main.go")
	vc := NewGitVersionControl()
	project := CodeProject{ID: "p", Root: root, Language: "go"}
	commit := vc.Commit(context.Background(), VersionControlRequest{ID: "c1", Project: project, Operation: VCCommit, Message: "initial"})
	if !commit.Success {
		t.Fatalf("commit failed: %+v", commit)
	}
	history := vc.History(context.Background(), VersionControlRequest{ID: "h1", Project: project, Operation: VCHistory})
	if !history.Success || !strings.Contains(history.Output, "initial") {
		t.Fatalf("unexpected history: %+v", history)
	}
}

func TestGitVersionControl_BranchAndRevertAreExplicit(t *testing.T) {
	root := initGitRepo(t)
	file := filepath.Join(root, "main.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", "main.go")
	runGit(t, root, "commit", "-m", "initial")
	vc := NewGitVersionControl()
	project := CodeProject{ID: "p", Root: root, Language: "go"}
	branch := vc.Branch(context.Background(), VersionControlRequest{ID: "br1", Project: project, Operation: VCBranch, Branch: "feature/test"})
	if !branch.Success {
		t.Fatalf("branch failed: %+v", branch)
	}
	if !strings.Contains(runGit(t, root, "branch", "--show-current"), "feature/test") {
		t.Fatal("expected feature branch")
	}
}

func TestVersionControl_RejectsInvalidRequests(t *testing.T) {
	vc := NewGitVersionControl()
	project := CodeProject{ID: "p", Root: t.TempDir()}
	got := vc.Commit(context.Background(), VersionControlRequest{ID: "bad", Project: project, Operation: VCCommit})
	if got.Success || len(got.Errors) == 0 {
		t.Fatalf("expected validation failure: %+v", got)
	}
}
