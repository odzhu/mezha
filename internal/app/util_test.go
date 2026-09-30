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

func TestRepoContextEffectiveConfig(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "mezha.toml")
	content := "version = 1\n[sandbox]\nname = \"effective-sandbox\"\n"
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	// Returns cached Config when non-nil.
	cachedCfg := &MezhaConfig{Sandbox: SandboxConfig{Name: "cached-box"}}
	rcWithConfig := RepoContext{
		RepoRoot: tempDir,
		Config:   cachedCfg,
	}
	effective, err := rcWithConfig.EffectiveConfig()
	if err != nil {
		t.Fatalf("EffectiveConfig with cached config: %v", err)
	}
	if effective.Sandbox.Name != "cached-box" {
		t.Errorf("expected cached-box, got %q", effective.Sandbox.Name)
	}

	// Falls back to LoadConfig when Config is nil.
	rcWithoutConfig := RepoContext{
		RepoRoot: tempDir,
	}
	effectiveLoaded, err := rcWithoutConfig.EffectiveConfig()
	if err != nil {
		t.Fatalf("EffectiveConfig with nil config: %v", err)
	}
	if effectiveLoaded.Sandbox.Name != "effective-sandbox" {
		t.Errorf("expected effective-sandbox, got %q", effectiveLoaded.Sandbox.Name)
	}
}

func TestResolveRepoContextConfigAttached(t *testing.T) {
	tempDir := t.TempDir()
	_, err := git.PlainInit(tempDir, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}

	configPath := filepath.Join(tempDir, "mezha.toml")
	content := "version = 1\n[sandbox]\nname = \"repo-attached-sandbox\"\n"
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
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
		t.Fatalf("ResolveRepoContext failed: %v", err)
	}

	if repoCtx.Config == nil {
		t.Fatal("expected repoCtx.Config to be populated")
	}
	if repoCtx.Config.Sandbox.Name != "repo-attached-sandbox" {
		t.Errorf(
			"Config.Sandbox.Name = %q, want repo-attached-sandbox",
			repoCtx.Config.Sandbox.Name,
		)
	}
	evalConfigPath, _ := filepath.EvalSymlinks(configPath)
	if repoCtx.ConfigPath != evalConfigPath {
		t.Errorf("ConfigPath = %q, want %q", repoCtx.ConfigPath, evalConfigPath)
	}
	if len(repoCtx.LoadedConfigs) == 0 {
		t.Fatal("expected repoCtx.LoadedConfigs to contain active config path")
	}
	if repoCtx.LoadedConfigs[len(repoCtx.LoadedConfigs)-1] != evalConfigPath {
		t.Errorf(
			"last LoadedConfigs = %q, want %q",
			repoCtx.LoadedConfigs[len(repoCtx.LoadedConfigs)-1],
			evalConfigPath,
		)
	}
}

func TestResolveRepoContextInvalidConfigError(t *testing.T) {
	tempDir := t.TempDir()
	_, err := git.PlainInit(tempDir, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}

	configPath := filepath.Join(tempDir, "mezha.toml")
	content := "[services.docker]\nenabled = true\n"
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

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
		t.Fatal("expected ResolveRepoContext to fail with invalid config, got nil")
	}
	if !strings.Contains(err.Error(), "load configuration:") {
		t.Errorf("expected error wrapping 'load configuration:', got %v", err)
	}
}

func TestResolveSandboxProjectDirDefault(t *testing.T) {
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

	rc, err := ResolveRepoContext(context.Background())
	if err != nil {
		t.Fatalf("ResolveRepoContext failed: %v", err)
	}

	cmd := New()
	remoteDir, err := resolveSandboxProjectDir(cmd, rc.Config, rc)
	if err != nil {
		t.Fatalf("resolveSandboxProjectDir failed with default config: %v", err)
	}
	if filepath.Base(remoteDir) != rc.RepoName {
		t.Errorf("remoteDir %q does not end with repo name %q", remoteDir, rc.RepoName)
	}
}
