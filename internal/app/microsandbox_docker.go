//go:build cgo

package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	msb "github.com/superradcompany/microsandbox/sdk/go"
)

const managedDevenvPath = "/sandbox"
const managedDevenvConfig = managedDevenvPath + "/devenv.nix"
const nativeDevenvPath = "/home/devenv/.nix-profile/bin/devenv"

// dockerCommand enters Mezha's managed devenv environment. Its enterShell
// tasks provision Docker and k3s before the requested command is started.
func dockerCommand(command string, args []string, _ bool) (string, []string) {
	return "devenv", append(
		[]string{"shell", "--from", "path:" + managedDevenvPath, "--", command},
		args...,
	)
}

// ensureManagedDevenvConfig repairs existing sandboxes that predate the managed file.
func ensureManagedDevenvConfig(
	ctx context.Context,
	sandbox *msb.Sandbox,
	provision ProvisionConfig,
) error {
	output, err := sandbox.Exec(ctx, "test", []string{"-f", managedDevenvConfig})
	if err != nil {
		return fmt.Errorf("inspect managed devenv configuration: %w", err)
	}
	if output.Success() {
		return nil
	}

	for _, add := range provision.Add {
		target := add.Target
		if strings.HasSuffix(target, "/") {
			info, statErr := os.Stat(add.Source)
			if statErr != nil || info.IsDir() {
				continue
			}
			target = filepath.ToSlash(filepath.Join(target, filepath.Base(add.Source)))
		}
		if filepath.Clean(target) != managedDevenvConfig {
			continue
		}
		if err := nativeUpload(ctx, sandbox, add.Source, target); err != nil {
			return fmt.Errorf("restore managed devenv configuration: %w", err)
		}
		return nil
	}

	return fmt.Errorf(
		"managed devenv configuration %s is missing; add it to provision.add or recreate the sandbox",
		managedDevenvConfig,
	)
}
