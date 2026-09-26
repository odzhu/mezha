package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestInitCreatesProvisionDevenv(t *testing.T) {
	tempDir := t.TempDir()
	rc := RepoContext{RepoRoot: tempDir}

	if err := Init(context.Background(), rc, false); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	configPath := filepath.Join(tempDir, "mezha.toml")

	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("expected mezha.toml to exist: %v", err)
	}

	expectedFiles := []string{
		filepath.Join(tempDir, ".mezha", "provision", "devenv.nix"),
		filepath.Join(tempDir, ".mezha", "provision", "devenv.yaml"),
		filepath.Join(tempDir, ".mezha", "provision", "common.nix"),
		filepath.Join(tempDir, ".mezha", "provision", "extension.nix"),
		filepath.Join(tempDir, ".mezha", "provision", "extension", "docker", "devenv.nix"),
		filepath.Join(tempDir, ".mezha", "provision", "extension", "docker", "devenv.yaml"),
		filepath.Join(tempDir, ".mezha", "provision", "extension", "k3s", "devenv.nix"),
		filepath.Join(tempDir, ".mezha", "provision", "extension", "k3s", "devenv.yaml"),
	}
	for _, f := range expectedFiles {
		if _, err := os.Stat(f); err != nil {
			t.Fatalf("expected %s to exist: %v", f, err)
		}
	}
	lockFile := filepath.Join(tempDir, ".mezha", "provision", "devenv.lock")
	if _, err := os.Stat(lockFile); err == nil {
		t.Fatalf("expected devenv.lock not to be produced, but it exists: %s", lockFile)
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

func TestResolveProvisionDir(t *testing.T) {
	projectDir := t.TempDir()
	globalDir := t.TempDir()
	t.Setenv("MEZHA_HOME", globalDir)

	globalProvisionDir := filepath.Join(globalDir, "provision")
	if err := os.MkdirAll(globalProvisionDir, 0o755); err != nil {
		t.Fatalf("mkdir global provision error = %v", err)
	}

	// Falls back to global when project provision dir does not exist
	resolved := resolveProvisionDir(projectDir)
	if resolved != globalProvisionDir {
		t.Fatalf("resolveProvisionDir() = %q, want global %q", resolved, globalProvisionDir)
	}

	// Home project provision takes precedence over global
	homeProjectProvisionDir := filepath.Join(
		globalDir,
		"projects",
		slugify(filepath.Base(projectDir)),
		"provision",
	)
	if err := os.MkdirAll(homeProjectProvisionDir, 0o755); err != nil {
		t.Fatalf("mkdir home project provision error = %v", err)
	}
	resolved = resolveProvisionDir(projectDir)
	if resolved != homeProjectProvisionDir {
		t.Fatalf(
			"resolveProvisionDir() = %q, want home project %q",
			resolved,
			homeProjectProvisionDir,
		)
	}

	// Project repo provision takes highest precedence
	projectProvisionDir := filepath.Join(projectDir, ".mezha", "provision")
	if err := os.MkdirAll(projectProvisionDir, 0o755); err != nil {
		t.Fatalf("mkdir project provision error = %v", err)
	}
	resolved = resolveProvisionDir(projectDir)
	if resolved != projectProvisionDir {
		t.Fatalf("resolveProvisionDir() = %q, want project %q", resolved, projectProvisionDir)
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

	expectedProvision := filepath.Join(
		globalDir,
		"projects",
		slugify(filepath.Base(projectDir)),
		"provision",
		"devenv.nix",
	)
	if _, err := os.Stat(expectedProvision); err != nil {
		t.Fatalf("expected home project provision to exist: %v", err)
	}
}
