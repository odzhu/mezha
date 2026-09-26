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
	devenvPath := filepath.Join(tempDir, ".mezha", "provision", "devenv.nix")

	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("expected mezha.toml to exist: %v", err)
	}
	if _, err := os.Stat(devenvPath); err != nil {
		t.Fatalf("expected .mezha/provision/devenv.nix to exist: %v", err)
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
