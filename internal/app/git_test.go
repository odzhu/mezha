package app

import (
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
	if err := os.WriteFile(filepath.Join(tempDir, "untracked1.txt"), []byte("1"), 0o644); err != nil {
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
	if err := os.WriteFile(filepath.Join(tempDir, ".gitignore"), []byte("ignored.txt\nignored_dir/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "ignored.txt"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tempDir, "ignored_dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "ignored_dir", "sub.txt"), []byte("secret2"), 0o644); err != nil {
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
	if err := os.WriteFile(filepath.Join(tempDir, "pkg", "sub", "mod.go"), []byte("package sub"), 0o644); err != nil {
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
	if err := os.WriteFile(filepath.Join(tempDir, "new_untracked.txt"), []byte("new"), 0o644); err != nil {
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
