package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func TestResolveRepoContextEmptyRepo(t *testing.T) {
	tempDir := t.TempDir()
	_, err := git.PlainInit(tempDir, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(origDir) }()

	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}

	repoCtx, err := ResolveRepoContext(context.Background())
	if err != nil {
		t.Fatalf("ResolveRepoContext failed on empty repo: %v", err)
	}

	if repoCtx.RepoName != filepath.Base(tempDir) {
		t.Errorf("RepoName = %q, want %q", repoCtx.RepoName, filepath.Base(tempDir))
	}
	if repoCtx.GitRef == "" {
		t.Errorf("GitRef is empty")
	}
}

func TestResolveRepoContextSubdirectory(t *testing.T) {
	tempDir := t.TempDir()
	_, err := git.PlainInit(tempDir, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}

	subDir := filepath.Join(tempDir, "sub", "deep")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(origDir) }()

	if err := os.Chdir(subDir); err != nil {
		t.Fatal(err)
	}

	repoCtx, err := ResolveRepoContext(context.Background())
	if err != nil {
		t.Fatalf("ResolveRepoContext failed from subdirectory: %v", err)
	}

	evalRepoRoot, _ := filepath.EvalSymlinks(tempDir)
	if repoCtx.RepoRoot != evalRepoRoot {
		t.Errorf("RepoRoot = %q, want %q", repoCtx.RepoRoot, evalRepoRoot)
	}
}

func TestResolveRepoContextNotInGitRepo(t *testing.T) {
	tempDir := t.TempDir()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(origDir) }()

	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}

	_, err = ResolveRepoContext(context.Background())
	if err == nil {
		t.Fatalf("expected error when running outside git repo")
	}
	if !strings.Contains(err.Error(), "must be run inside a git repository") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestResolveRepoContextWithCommitAndDetachedHead(t *testing.T) {
	tempDir := t.TempDir()
	repo, err := git.PlainInit(tempDir, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}

	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}

	filePath := filepath.Join(tempDir, "file.txt")
	if err := os.WriteFile(filePath, []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := worktree.Add("file.txt"); err != nil {
		t.Fatal(err)
	}

	commitHash, err := worktree.Commit("initial commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(origDir) }()

	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}

	// 1. On branch with commit
	repoCtx, err := ResolveRepoContext(context.Background())
	if err != nil {
		t.Fatalf("ResolveRepoContext on branch: %v", err)
	}
	if repoCtx.GitRef != "master" && repoCtx.GitRef != "main" {
		t.Errorf("GitRef = %q, want master or main", repoCtx.GitRef)
	}

	// 2. Detached HEAD
	if err := worktree.Checkout(&git.CheckoutOptions{Hash: commitHash}); err != nil {
		t.Fatalf("Checkout hash: %v", err)
	}

	repoCtxDetached, err := ResolveRepoContext(context.Background())
	if err != nil {
		t.Fatalf("ResolveRepoContext detached: %v", err)
	}
	expectedShortHash := commitHash.String()[:7]
	if repoCtxDetached.GitRef != expectedShortHash {
		t.Errorf("GitRef = %q, want %q", repoCtxDetached.GitRef, expectedShortHash)
	}
}

func TestSlugify(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Hello World!", "hello-world"},
		{"repo_name/feature@123", "repo-name-f-eb70a9e"},
		{"---special---chars---", "special-chars"},
		{"", "sandbox"},
		{"   ", "sandbox"},
		{"$$$%%%^^^", "sandbox"},
		{
			"this-is-a-very-long-name-that-exceeds-sixty-three-characters-and-should-be-truncated-properly",
			"this-is-a-v-c1f0f9c",
		},
		{
			"aimy-feature-add-knowledgememory-xrd",
			"aimy-featur-bf7a9a2",
		},
	}

	for _, tt := range tests {
		got := slugify(tt.input)
		if got != tt.want {
			t.Errorf("slugify(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestResolveRemoteWorkdir(t *testing.T) {
	repoRoot := "/Users/test/projects/my-repo"
	remoteRepoDir := "/sandbox/my-repo"

	t.Run("invocation cwd equals repo root", func(t *testing.T) {
		got, err := resolveRemoteWorkdir(repoRoot, remoteRepoDir, repoRoot)
		if err != nil {
			t.Fatalf("resolveRemoteWorkdir: %v", err)
		}
		if got != "/sandbox/my-repo" {
			t.Errorf("got %q, want /sandbox/my-repo", got)
		}
	})

	t.Run("invocation cwd is subdirectory", func(t *testing.T) {
		invocationCWD := filepath.Join(repoRoot, "src", "backend")
		got, err := resolveRemoteWorkdir(repoRoot, remoteRepoDir, invocationCWD)
		if err != nil {
			t.Fatalf("resolveRemoteWorkdir: %v", err)
		}
		if got != "/sandbox/my-repo/src/backend" {
			t.Errorf("got %q, want /sandbox/my-repo/src/backend", got)
		}
	})

	t.Run("invocation cwd outside repo root", func(t *testing.T) {
		invocationCWD := "/Users/test/other-dir"
		_, err := resolveRemoteWorkdir(repoRoot, remoteRepoDir, invocationCWD)
		if err == nil {
			t.Fatalf("expected error for cwd outside repo root")
		}
	})
}

func TestWaitDelay(t *testing.T) {
	d := waitDelay()
	if d <= 0 {
		t.Errorf("waitDelay returned non-positive duration: %v", d)
	}
}
