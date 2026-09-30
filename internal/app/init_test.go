package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestInitCreatesExtensionsSample(t *testing.T) {
	tempDir := t.TempDir()
	rc := RepoContext{RepoRoot: tempDir}

	if err := Init(context.Background(), rc, false); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	configPath := filepath.Join(tempDir, "mezha.toml")

	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("expected mezha.toml to exist: %v", err)
	}

	sampleFile := filepath.Join(tempDir, ".mezha", "extensions", "sample", "devenv.nix")
	if _, err := os.Stat(sampleFile); err != nil {
		t.Fatalf("expected %s to exist: %v", sampleFile, err)
	}

	provisionDir := filepath.Join(tempDir, ".mezha", "provision")
	if _, err := os.Stat(provisionDir); err == nil {
		t.Fatalf("expected .mezha/provision not to exist, but it does: %s", provisionDir)
	}

	// Repeated init without force should fail
	if err := Init(context.Background(), rc, false); err == nil {
		t.Fatal("expected error on repeated Init() without force, got nil")
	}

	// Repeated init with force should succeed
	if err := Init(context.Background(), rc, true); err != nil {
		t.Fatalf("Init(force=true) error = %v", err)
	}
}

func TestResolveExtensionsDir(t *testing.T) {
	projectDir := t.TempDir()
	globalDir := t.TempDir()
	t.Setenv("MEZHA_HOME", globalDir)

	globalExtensionsDir := filepath.Join(globalDir, "extensions")
	if err := os.MkdirAll(globalExtensionsDir, 0o755); err != nil {
		t.Fatalf("mkdir global extensions error = %v", err)
	}

	// Falls back to global when project extensions dir does not exist
	resolved := resolveExtensionsDir(projectDir)
	if resolved != globalExtensionsDir {
		t.Fatalf("resolveExtensionsDir() = %q, want global %q", resolved, globalExtensionsDir)
	}

	// Home project extensions takes precedence over global
	homeProjectExtensionsDir := filepath.Join(
		globalDir,
		"projects",
		slugify(filepath.Base(projectDir)),
		"extensions",
	)
	if err := os.MkdirAll(homeProjectExtensionsDir, 0o755); err != nil {
		t.Fatalf("mkdir home project extensions error = %v", err)
	}
	resolved = resolveExtensionsDir(projectDir)
	if resolved != homeProjectExtensionsDir {
		t.Fatalf(
			"resolveExtensionsDir() = %q, want home project %q",
			resolved,
			homeProjectExtensionsDir,
		)
	}

	// Project repo extensions takes highest precedence
	projectExtensionsDir := filepath.Join(projectDir, ".mezha", "extensions")
	if err := os.MkdirAll(projectExtensionsDir, 0o755); err != nil {
		t.Fatalf("mkdir project extensions error = %v", err)
	}
	resolved = resolveExtensionsDir(projectDir)
	if resolved != projectExtensionsDir {
		t.Fatalf("resolveExtensionsDir() = %q, want project %q", resolved, projectExtensionsDir)
	}
}

func TestInitHomeProject(t *testing.T) {
	globalDir := t.TempDir()
	projectDir := t.TempDir()
	t.Setenv("MEZHA_HOME", globalDir)

	rc := RepoContext{RepoRoot: projectDir}
	if err := InitHomeProject(rc, false); err != nil {
		t.Fatalf("InitHomeProject error = %v", err)
	}

	expectedConfig := filepath.Join(
		globalDir,
		"projects",
		slugify(filepath.Base(projectDir)),
		"mezha.toml",
	)
	if _, err := os.Stat(expectedConfig); err != nil {
		t.Fatalf("expected home project config to exist: %v", err)
	}

	expectedExtension := filepath.Join(
		globalDir,
		"projects",
		slugify(filepath.Base(projectDir)),
		"extensions",
		"sample",
		"devenv.nix",
	)
	if _, err := os.Stat(expectedExtension); err != nil {
		t.Fatalf("expected home project extension to exist: %v", err)
	}
}
