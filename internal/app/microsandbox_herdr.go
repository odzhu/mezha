//go:build cgo

package app

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
	msb "github.com/superradcompany/microsandbox/sdk/go"
)

var herdrVersion = regexp.MustCompile(`^herdr (\d+\.\d+\.\d+)$`)

// syncSandboxHerdrConfig copies local keybindings so remote Herdr servers can
// invoke synchronized plugin actions, while retaining Mezha's terminal setup.
func syncSandboxHerdrConfig(ctx context.Context, sandbox *msb.Sandbox) error {
	config := map[string]any{}
	configPath, err := localHerdrConfigPath()
	if err != nil {
		return err
	}
	contents, err := os.ReadFile(configPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read local Herdr configuration: %w", err)
	}
	if err == nil {
		var local map[string]any
		if _, err := toml.Decode(string(contents), &local); err != nil {
			return fmt.Errorf("parse local Herdr configuration: %w", err)
		}
		if keys, ok := local["keys"]; ok {
			config["keys"] = keys
		}
	}
	config["terminal"] = map[string]string{
		"default_shell": "bash",
		"shell_mode":    "non_login",
		"new_cwd":       "follow",
	}

	var encoded bytes.Buffer
	if err := toml.NewEncoder(&encoded).Encode(config); err != nil {
		return fmt.Errorf("encode sandbox Herdr configuration: %w", err)
	}
	temp, err := os.CreateTemp("", "mezha-herdr-*.toml")
	if err != nil {
		return fmt.Errorf("create sandbox Herdr configuration: %w", err)
	}
	defer func() { _ = os.Remove(temp.Name()) }()
	if _, err := temp.Write(encoded.Bytes()); err != nil {
		_ = temp.Close()
		return fmt.Errorf("write sandbox Herdr configuration: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close sandbox Herdr configuration: %w", err)
	}
	if err := nativeUpload(
		ctx,
		sandbox,
		temp.Name(),
		"/root/.config/mezha/services/herdr/config.toml",
	); err != nil {
		return fmt.Errorf("copy Herdr keybindings to sandbox: %w", err)
	}
	// Reload an already-running remote server; a first connection loads it normally.
	_, _ = sandbox.Exec(ctx, "/root/.local/bin/herdr", []string{"server", "reload-config"})
	return nil
}

func localHerdrConfigPath() (string, error) {
	if path := strings.TrimSpace(os.Getenv("HERDR_CONFIG_PATH")); path != "" {
		return path, nil
	}
	if configHome := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); configHome != "" {
		return filepath.Join(configHome, "herdr", "config.toml"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve local Herdr configuration: %w", err)
	}
	return filepath.Join(home, ".config", "herdr", "config.toml"), nil
}
