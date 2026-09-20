//go:build cgo

package app

import (
	"context"
	"fmt"
	"os"

	msb "github.com/superradcompany/microsandbox/sdk/go"
)

func provisionMicrosandbox(
	ctx context.Context,
	rc RepoContext,
	params ProvisionParams,
	cfg *MezhaConfig,
) error {
	if err := msb.EnsureInstalled(ctx); err != nil {
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
		if err := ensureDevenvImage(ctx, rc.RepoRoot); err != nil {
			return fmt.Errorf("import Microsandbox image: %w", err)
		}
		if err := ensureStateVolume(ctx, params.SandboxName, cfg.Microsandbox.Volumes); err != nil {
			return err
		}
	}
	opts, err := cfg.Microsandbox.sandboxOptions(rc.RepoRoot, params.SandboxName)
	if err != nil {
		return err
	}
	sandbox, err := msb.ConnectOrCreateSandbox(ctx, params.SandboxName, opts...)
	if err != nil {
		return fmt.Errorf("start Microsandbox %q: %w", params.SandboxName, err)
	}
	defer func() { _ = sandbox.Detach(context.Background()) }()

	if err := ensurePersistentLinks(ctx, sandbox); err != nil {
		return err
	}
	if !sandboxExisted {
		if err := applyProvisionConfig(ctx, sandbox, cfg.Provision); err != nil {
			return err
		}
	}
	useDevenv := cfg.Services.Docker.Enabled || params.Kubernetes
	if useDevenv {
		command, args := dockerCommand("true", nil, params.Kubernetes)
		output, err := sandbox.Exec(ctx, command, args, msb.WithExecCwd(managedDevenvPath))
		if err != nil {
			return fmt.Errorf("provision core devenv services: %w", err)
		}
		fmt.Print(output.Stdout())
		fmt.Fprint(os.Stderr, output.Stderr())
		if !output.Success() {
			return fmt.Errorf("provision core devenv services exited with code %d", output.ExitCode())
		}
	}
	if params.Herdr && herdrCommandAvailable() {
		workdir := cfg.Microsandbox.Workdir
		if workdir == "" {
			workdir = params.RemoteRepoDir
		}
		if err := ensureSandboxHerdr(ctx, sandbox, workdir, useDevenv); err != nil {
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
