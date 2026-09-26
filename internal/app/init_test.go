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
	tempDir := t.TempDir()
	provisionDir := filepath.Join(tempDir, ".mezha", "provision")

	if err := os.MkdirAll(provisionDir, 0o755); err != nil {
		t.Fatalf("mkdir error = %v", err)
	}
	devenvFile := filepath.Join(provisionDir, "devenv.nix")
	if err := os.WriteFile(devenvFile, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write devenv.nix error = %v", err)
	}

	resolved := resolveProvisionDir(tempDir)
	if resolved != provisionDir {
		t.Fatalf("resolveProvisionDir() = %q, want %q", resolved, provisionDir)
	}
}
