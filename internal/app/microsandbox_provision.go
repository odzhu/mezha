//go:build cgo

package app

import (
	"context"
	"fmt"

	msb "github.com/superradcompany/microsandbox/sdk/go"
)

func provisionMicrosandbox(
	ctx context.Context,
	rc RepoContext,
	params ProvisionParams,
	cfg *MezhaConfig,
) error {
	if _, err := msb.EnsureRuntime(ctx, msb.RuntimeConfig{}, msb.InstallOptions{}); err != nil {
		return fmt.Errorf("install Microsandbox runtime: %w", err)
	}
	_, lookupErr := msb.GetSandbox(ctx, params.SandboxName)
	sandboxExisted := lookupErr == nil
	if lookupErr != nil && !msb.IsKind(lookupErr, msb.ErrSandboxNotFound) {
		return fmt.Errorf("find Microsandbox %q: %w", params.SandboxName, lookupErr)
	}
	if params.Recreate && sandboxExisted {
		if _, err := unregisterHerdrMachine(ctx, params.SandboxName); err != nil {
			return err
		}
		handle, err := msb.GetSandbox(ctx, params.SandboxName)
		if err != nil {
			return fmt.Errorf("find Microsandbox %q: %w", params.SandboxName, err)
		}
		if err := handle.Destroy(ctx, msb.WithDestroyForce()); err != nil {
			return fmt.Errorf("recreate sandbox %q: %w", params.SandboxName, err)
		}
		sandboxExisted = false
	}
	if params.VolumesFlush {
		if err := flushMicrosandboxVolumes(ctx, params.SandboxName); err != nil {
			return err
		}
	}
	if !sandboxExisted {
		if err := ensureStateVolume(ctx, params.SandboxName, *cfg.Microsandbox); err != nil {
			return err
		}
	}
	opts, err := cfg.Microsandbox.sandboxOptions(rc.RepoRoot, params.SandboxName)
	if err != nil {
		return err
	}
	fmt.Printf("Starting or connecting to Microsandbox: %s...\n", params.SandboxName)
	sandbox, err := msb.ConnectOrCreateSandbox(ctx, params.SandboxName, opts...)
	if err != nil {
		return fmt.Errorf("start Microsandbox %q: %w", params.SandboxName, err)
	}
	defer func() { _ = sandbox.Detach(context.Background()) }()
	fmt.Printf("Connected to Microsandbox: %s\n", params.SandboxName)

	if err := ensurePersistentLinks(ctx, sandbox); err != nil {
		return err
	}
	if err := ensureSandboxProjectDir(ctx, sandbox, params.RemoteRepoDir, rc.RepoRoot); err != nil {
		return err
	}
	if !sandboxExisted {
		if err := applyProvisionConfig(ctx, sandbox, cfg.Provision); err != nil {
			return err
		}
	}
	useDevenv := cfg.Services.Docker.Enabled || params.Kubernetes
	herdrEnabled := params.Herdr && herdrCommandAvailable()
	if useDevenv || herdrEnabled {
		if err := ensureManagedDevenvConfig(ctx, sandbox, cfg.Provision); err != nil {
			return err
		}
	}
	if useDevenv {
		fmt.Println("Starting core devenv services...")
		if err := ensureDevenvServices(ctx, sandbox, params.Kubernetes); err != nil {
			return err
		}
	}
	if herdrEnabled {
		if err := ensureSandboxHerdr(ctx, sandbox, useDevenv); err != nil {
			return err
		}
		if err := registerHerdrMachine(ctx, params.SandboxName, !sandboxExisted); err != nil {
			return err
		}
		if err := syncHerdrPlugins(ctx, sandbox); err != nil {
			return err
		}
	}
	fmt.Printf("Provisioned Microsandbox: %s\n", params.SandboxName)
	return nil
}

// ensureSandboxProjectDir creates the project directory and exposes it at its
// host path so tools invoked in the sandbox can use host-relative paths.
func ensureSandboxProjectDir(
	ctx context.Context,
	sandbox *msb.Sandbox,
	dir, hostRepoRoot string,
) error {
	output, err := sandbox.Exec(ctx, "sh", []string{"-eu", "-c", `
target="$1"
link="$2"
mkdir -p "$target"
if [ "$target" = "$link" ]; then
  exit 0
fi
mkdir -p "$(dirname "$link")"
if [ -e "$link" ] && [ ! -L "$link" ]; then
  echo "sandbox host-path project link already exists and is not a symlink: $link" >&2
  exit 1
fi
ln -sfn "$target" "$link"
`, "mezha-project-link", dir, hostRepoRoot})
	if err != nil {
		return fmt.Errorf("create sandbox project directory: %w", err)
	}
	if !output.Success() {
		return fmt.Errorf("create sandbox project directory: %s", output.Stderr())
	}
	return nil
}
