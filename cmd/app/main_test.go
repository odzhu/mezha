package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-git/go-git/v5"
)

func TestRun(t *testing.T) {
	err := run(context.Background(), []string{"mezha", "--help"})
	if err != nil {
		t.Fatalf("expected run with --help to succeed, got: %v", err)
	}
}

func TestConfigCommands(t *testing.T) {
	tempDir := t.TempDir()
	_, err := git.PlainInit(tempDir, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}

	configPath := filepath.Join(tempDir, "mezha.toml")
	content := "version = 1\n[sandbox]\nname = \"cli-test-sandbox\"\n"
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

	for _, sub := range [][]string{
		{"mezha", "config", "check"},
		{"mezha", "config", "validate"},
		{"mezha", "config", "show"},
		{"mezha", "config", "path"},
		{"mezha", "config", "path", "--all"},
	} {
		if err := run(context.Background(), sub); err != nil {
			t.Errorf("command %v failed: %v", sub, err)
		}
	}
}

func TestMain(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	os.Args = []string{"mezha", "--help"}
	main()
}
