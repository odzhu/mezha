package app

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func TestGitTrackedAndUntracked(t *testing.T) {
	tempDir := t.TempDir()
	repo, err := git.PlainInit(tempDir, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}

	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}

	// 1. Empty repo with untracked file
	if err := os.WriteFile(
		filepath.Join(tempDir, "untracked1.txt"),
		[]byte("1"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	paths, err := gitTrackedAndUntracked(tempDir)
	if err != nil {
		t.Fatalf("gitTrackedAndUntracked: %v", err)
	}
	if len(paths) != 1 || paths[0] != "untracked1.txt" {
		t.Errorf("paths = %v, want [\"untracked1.txt\"]", paths)
	}

	// 2. Add gitignore
	if err := os.WriteFile(
		filepath.Join(tempDir, ".gitignore"),
		[]byte("ignored.txt\nignored_dir/\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(tempDir, "ignored.txt"),
		[]byte("secret"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tempDir, "ignored_dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(tempDir, "ignored_dir", "sub.txt"),
		[]byte("secret2"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	// 3. Commit tracked files
	if _, err := worktree.Add(".gitignore"); err != nil {
		t.Fatal(err)
	}
	if _, err := worktree.Add("untracked1.txt"); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tempDir, "pkg", "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(tempDir, "pkg", "sub", "mod.go"),
		[]byte("package sub"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := worktree.Add("pkg/sub/mod.go"); err != nil {
		t.Fatal(err)
	}

	_, err = worktree.Commit("commit tracked files", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// 4. Create new untracked file
	if err := os.WriteFile(
		filepath.Join(tempDir, "new_untracked.txt"),
		[]byte("new"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	paths, err = gitTrackedAndUntracked(tempDir)
	if err != nil {
		t.Fatalf("gitTrackedAndUntracked: %v", err)
	}

	sort.Strings(paths)
	expected := []string{
		".gitignore",
		"new_untracked.txt",
		"pkg/sub/mod.go",
		"untracked1.txt",
	}
	sort.Strings(expected)

	if len(paths) != len(expected) {
		t.Fatalf("paths len = %d, want %d; paths=%v", len(paths), len(expected), paths)
	}
	for i := range expected {
		if paths[i] != expected[i] {
			t.Errorf("paths[%d] = %q, want %q", i, paths[i], expected[i])
		}
	}
}

func TestGitTrackedAndUntrackedInvalidRepo(t *testing.T) {
	tempDir := t.TempDir()
	_, err := gitTrackedAndUntracked(tempDir)
	if err == nil {
		t.Fatalf("expected error on non-git dir")
	}
}

func TestCurrentBranch(t *testing.T) {
	tempDir := t.TempDir()
	repo, err := git.PlainInit(tempDir, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}

	// Empty repo (unborn HEAD)
	_, err = currentBranch(context.Background(), tempDir)
	if err == nil {
		t.Fatalf("expected error on empty repo without commits")
	}

	// Commit on default branch
	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "file.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := worktree.Add("file.txt"); err != nil {
		t.Fatal(err)
	}
	_, err = worktree.Commit("first commit", &git.CommitOptions{
		Author: &object.Signature{Name: "Tester", Email: "tester@example.com", When: time.Now()},
	})
	if err != nil {
		t.Fatal(err)
	}

	branch, err := currentBranch(context.Background(), tempDir)
	if err != nil {
		t.Fatalf("currentBranch: %v", err)
	}
	if branch != "master" && branch != "main" {
		t.Errorf("got branch %q, want master or main", branch)
	}
}

func TestSetAndRequireSandboxGitRemote(t *testing.T) {
	tempDir := t.TempDir()
	_, err := git.PlainInit(tempDir, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}

	remoteName := "test-sandbox"
	remoteURL := "ssh://root@mezha-sandbox-12345//root/test/.git"

	// Not yet registered
	if err := requireSandboxGitRemote(context.Background(), tempDir, remoteName); err == nil {
		t.Fatalf("expected error when remote is not configured")
	}

	// Register remote
	if err := setSandboxGitRemote(
		context.Background(),
		tempDir,
		remoteName,
		remoteURL,
		false,
	); err != nil {
		t.Fatalf("setSandboxGitRemote: %v", err)
	}

	// Now required should succeed
	if err := requireSandboxGitRemote(context.Background(), tempDir, remoteName); err != nil {
		t.Fatalf("requireSandboxGitRemote failed after set: %v", err)
	}

	// Update remote URL
	newURL := "ssh://root@mezha-sandbox-12345//root/test2/.git"
	if err := setSandboxGitRemote(
		context.Background(),
		tempDir,
		remoteName,
		newURL,
		false,
	); err != nil {
		t.Fatalf("setSandboxGitRemote update: %v", err)
	}

	// Unregister
	if err := unregisterSandboxGitRemote(context.Background(), tempDir, remoteName); err != nil {
		t.Fatalf("unregisterSandboxGitRemote: %v", err)
	}

	// Should be missing again
	if err := requireSandboxGitRemote(context.Background(), tempDir, remoteName); err == nil {
		t.Fatalf("expected error after unregistering remote")
	}
}

func TestCountAheadBehind(t *testing.T) {
	tempDir := t.TempDir()
	repo, err := git.PlainInit(tempDir, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}
	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}

	sig := &object.Signature{Name: "Tester", Email: "tester@example.com", When: time.Now()}

	// Commit 1
	if err := os.WriteFile(filepath.Join(tempDir, "f1.txt"), []byte("1"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := worktree.Add("f1.txt"); err != nil {
		t.Fatal(err)
	}
	c1, err := worktree.Commit("commit 1", &git.CommitOptions{Author: sig})
	if err != nil {
		t.Fatal(err)
	}

	// Equal commits
	ahead, behind, err := countAheadBehind(repo, c1, c1)
	if err != nil {
		t.Fatalf("countAheadBehind: %v", err)
	}
	if ahead != 0 || behind != 0 {
		t.Errorf("equal commits: got ahead=%d, behind=%d, want 0, 0", ahead, behind)
	}

	// Commit 2 on local
	if err := os.WriteFile(filepath.Join(tempDir, "f2.txt"), []byte("2"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := worktree.Add("f2.txt"); err != nil {
		t.Fatal(err)
	}
	c2, err := worktree.Commit("commit 2", &git.CommitOptions{Author: sig})
	if err != nil {
		t.Fatal(err)
	}

	// c2 ahead of c1 by 1, behind by 0
	ahead, behind, err = countAheadBehind(repo, c2, c1)
	if err != nil {
		t.Fatalf("countAheadBehind: %v", err)
	}
	if ahead != 1 || behind != 0 {
		t.Errorf("got ahead=%d, behind=%d, want 1, 0", ahead, behind)
	}

	// c1 ahead of c2 by 0, behind by 1
	ahead, behind, err = countAheadBehind(repo, c1, c2)
	if err != nil {
		t.Fatalf("countAheadBehind: %v", err)
	}
	if ahead != 0 || behind != 1 {
		t.Errorf("got ahead=%d, behind=%d, want 0, 1", ahead, behind)
	}
}

func TestTrackedDirtyAndUntrackedPaths(t *testing.T) {
	tempDir := t.TempDir()
	repo, err := git.PlainInit(tempDir, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}
	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}

	sig := &object.Signature{Name: "Tester", Email: "tester@example.com", When: time.Now()}
	if err := os.WriteFile(
		filepath.Join(tempDir, "tracked.txt"),
		[]byte("initial"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := worktree.Add("tracked.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := worktree.Commit("initial", &git.CommitOptions{Author: sig}); err != nil {
		t.Fatal(err)
	}

	// Modify tracked, add untracked
	if err := os.WriteFile(
		filepath.Join(tempDir, "tracked.txt"),
		[]byte("modified"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(tempDir, "untracked.txt"),
		[]byte("new"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	dirty, err := trackedDirtyPaths(context.Background(), tempDir)
	if err != nil {
		t.Fatalf("trackedDirtyPaths: %v", err)
	}
	if len(dirty.copy) != 2 {
		t.Errorf("dirty.copy = %v, want 2 items", dirty.copy)
	}

	untracked, err := localUntrackedPaths(context.Background(), tempDir)
	if err != nil {
		t.Fatalf("localUntrackedPaths: %v", err)
	}
	if len(untracked) != 1 || untracked[0] != "untracked.txt" {
		t.Errorf("untracked = %v, want [\"untracked.txt\"]", untracked)
	}
}
