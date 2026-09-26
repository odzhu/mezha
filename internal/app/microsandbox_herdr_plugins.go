//go:build cgo

package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/odzhu/mezha/internal/execx"
	msb "github.com/superradcompany/microsandbox/sdk/go"
)

type herdrPluginList struct {
	Result struct {
		Plugins []herdrPlugin `json:"plugins"`
	} `json:"result"`
}

type herdrPlugin struct {
	ID      string            `json:"plugin_id"`
	Enabled bool              `json:"enabled"`
	Source  herdrPluginSource `json:"source"`
}

type herdrPluginSource struct {
	Kind           string `json:"kind"`
	Owner          string `json:"owner"`
	Repo           string `json:"repo"`
	Subdir         string `json:"subdir"`
	ResolvedCommit string `json:"resolved_commit"`
}

func syncHerdrPlugins(ctx context.Context, sandbox *msb.Sandbox) error {
	output, err := execx.Output(ctx, "herdr", "plugin", "list", "--json")
	if err != nil {
		return fmt.Errorf("list local Herdr plugins: %w", err)
	}
	var list herdrPluginList
	if err := json.Unmarshal(output, &list); err != nil {
		return fmt.Errorf("parse local Herdr plugins: %w", err)
	}
	for _, plugin := range list.Result.Plugins {
		if plugin.ID == herdrPluginID {
			continue
		}
		if err := syncHerdrPlugin(ctx, sandbox, plugin); err != nil {
			return err
		}
	}
	return nil
}

func syncHerdrPlugin(ctx context.Context, sandbox *msb.Sandbox, plugin herdrPlugin) error {
	if plugin.ID == "" {
		return fmt.Errorf("local Herdr plugin has no id")
	}
	if plugin.Source.Kind != "github" || plugin.Source.Owner == "" || plugin.Source.Repo == "" {
		return fmt.Errorf(
			"herdr plugin %q must be installed from GitHub to synchronize natively",
			plugin.ID,
		)
	}
	source := plugin.Source.Owner + "/" + plugin.Source.Repo
	if plugin.Source.Subdir != "" {
		source += "/" + plugin.Source.Subdir
	}

	current, err := sandboxHerdrPluginCurrent(ctx, sandbox, plugin)
	if err != nil {
		return err
	}
	if current {
		fmt.Printf("Herdr plugin %q is already installed at the requested commit.\n", plugin.ID)
	} else {
		// A previous Mezha version linked a copied plugin. Unlink it before native install.
		_ = runSandboxHerdr(ctx, sandbox, "plugin", "unlink", plugin.ID)
		args := []string{"plugin", "install", source, "--yes"}
		if plugin.Source.ResolvedCommit != "" {
			args = append(args, "--ref", plugin.Source.ResolvedCommit)
		}
		fmt.Printf("Installing Herdr plugin %q through devenv...\n", plugin.ID)
		if err := runSandboxHerdrInDevenv(ctx, sandbox, args...); err != nil {
			return fmt.Errorf(
				"install Herdr plugin %q in sandbox; add required build tools to .mezha/devenv.nix: %w",
				plugin.ID,
				err,
			)
		}
	}
	enabledCommand := "enable"
	if !plugin.Enabled {
		enabledCommand = "disable"
	}
	if err := runSandboxHerdr(ctx, sandbox, "plugin", enabledCommand, plugin.ID); err != nil {
		return fmt.Errorf("%s Herdr plugin %q in sandbox: %w", enabledCommand, plugin.ID, err)
	}

	localConfig, err := execx.Output(ctx, "herdr", "plugin", "config-dir", plugin.ID)
	if err != nil {
		return fmt.Errorf("get local Herdr plugin %q configuration directory: %w", plugin.ID, err)
	}
	remoteConfig, err := sandbox.Exec(
		ctx,
		"/root/.local/bin/herdr",
		[]string{"plugin", "config-dir", plugin.ID},
	)
	if err != nil {
		return fmt.Errorf("get remote Herdr plugin %q configuration directory: %w", plugin.ID, err)
	}
	if !remoteConfig.Success() {
		return fmt.Errorf(
			"get remote Herdr plugin %q configuration directory: %s",
			plugin.ID,
			strings.TrimSpace(remoteConfig.Stderr()),
		)
	}
	remoteConfigPath := strings.TrimSpace(remoteConfig.Stdout())
	if err := removeSandboxPath(ctx, sandbox, remoteConfigPath); err != nil {
		return err
	}
	if err := nativeUpload(
		ctx,
		sandbox,
		strings.TrimSpace(string(localConfig)),
		remoteConfigPath,
	); err != nil {
		return fmt.Errorf("copy Herdr plugin %q configuration: %w", plugin.ID, err)
	}
	return nil
}

func sandboxHerdrPluginCurrent(
	ctx context.Context,
	sandbox *msb.Sandbox,
	local herdrPlugin,
) (bool, error) {
	output, err := runSandboxHerdrOutput(
		ctx,
		sandbox,
		"/root/.local/bin/herdr",
		[]string{"plugin", "list", "--json"},
	)
	if err != nil {
		return false, fmt.Errorf("list sandbox Herdr plugins: %w", err)
	}
	var list herdrPluginList
	if err := json.Unmarshal([]byte(output.Stdout()), &list); err != nil {
		return false, fmt.Errorf("parse sandbox Herdr plugins: %w", err)
	}
	for _, remote := range list.Result.Plugins {
		if remote.ID != local.ID {
			continue
		}
		return remote.Source.Kind == local.Source.Kind &&
			remote.Source.Owner == local.Source.Owner &&
			remote.Source.Repo == local.Source.Repo &&
			remote.Source.Subdir == local.Source.Subdir &&
			remote.Source.ResolvedCommit == local.Source.ResolvedCommit, nil
	}
	return false, nil
}

func runSandboxHerdr(ctx context.Context, sandbox *msb.Sandbox, args ...string) error {
	_, err := runSandboxHerdrOutput(ctx, sandbox, "/root/.local/bin/herdr", args)
	return err
}

func runSandboxHerdrInDevenv(ctx context.Context, sandbox *msb.Sandbox, args ...string) error {
	const pluginDevenvPath = "/tmp/mezha-herdr-plugin"
	setup, err := sandbox.Exec(ctx, "sh", []string{"-c", `set -eu
mkdir -p "$1"
cat >"$1/devenv.nix" <<'EOF'
{ pkgs, ... }:
{
  imports = [ /root/.config/mezha/services/devenv/devenv.nix ];
  packages = [ pkgs.go ];
  scripts.mezha-herdr.exec = ''
    export PATH=${pkgs.go}/bin:$PATH
    exec /root/.local/bin/herdr "$@"
  '';
}
EOF
`, "mezha-herdr-plugin-env", pluginDevenvPath})
	if err != nil {
		return fmt.Errorf("prepare Herdr plugin environment: %w", err)
	}
	if !setup.Success() {
		return fmt.Errorf(
			"prepare Herdr plugin environment: %s",
			strings.TrimSpace(setup.Stderr()),
		)
	}
	// This devenv project is only needed while installing a plugin.
	defer func() { _ = removeSandboxPath(context.Background(), sandbox, pluginDevenvPath) }()
	command := append(
		[]string{"shell", "--from", "path:" + pluginDevenvPath, "--", "mezha-herdr"},
		args...,
	)
	code, err := sandbox.AttachWith(ctx, nativeDevenvPath, command)
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("herdr plugin installation exited with code %d", code)
	}
	return nil
}

func runSandboxHerdrOutput(
	ctx context.Context,
	sandbox *msb.Sandbox,
	command string,
	args []string,
) (*msb.ExecOutput, error) {
	output, err := sandbox.Exec(ctx, command, args)
	if err != nil {
		return nil, err
	}
	if !output.Success() {
		return nil, fmt.Errorf("%s", strings.TrimSpace(output.Stderr()))
	}
	return output, nil
}

func removeSandboxPath(ctx context.Context, sandbox *msb.Sandbox, path string) error {
	output, err := sandbox.Exec(ctx, "rm", []string{"-rf", path})
	if err != nil {
		return fmt.Errorf("remove sandbox path %q: %w", path, err)
	}
	if !output.Success() {
		return fmt.Errorf("remove sandbox path %q: %s", path, strings.TrimSpace(output.Stderr()))
	}
	return nil
}
