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
		fmt.Println("Provisioning core devenv services...")
		command, args := dockerCommand("true", nil, params.Kubernetes)
		code, err := sandbox.AttachWith(
			ctx,
			command,
			args,
			msb.WithAttachCwd(managedDevenvPath),
		)
		if err != nil {
			return fmt.Errorf("provision core devenv services: %w", err)
		}
		if code != 0 {
			return fmt.Errorf("provision core devenv services exited with code %d", code)
		}
	}
	if herdrEnabled {
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
