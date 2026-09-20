//go:build cgo

package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	msb "github.com/superradcompany/microsandbox/sdk/go"
)

func openMicrosandbox(
	ctx context.Context,
	rc RepoContext,
	params TransferParams,
	recreate bool,
) (*msb.Sandbox, *MicrosandboxSpec, func(), error) {
	cfg, _, err := LoadConfig(rc.RepoRoot)
	if err != nil {
		return nil, nil, nil, err
	}
	if cfg == nil || cfg.Microsandbox == nil {
		return nil, nil, nil, fmt.Errorf("microsandbox configuration is missing")
	}
	if err := msb.EnsureInstalled(ctx); err != nil {
		return nil, nil, nil, fmt.Errorf("install Microsandbox runtime: %w", err)
	}
	if recreate {
		if h, e := msb.GetSandbox(ctx, params.SandboxName); e == nil {
			if e = h.Destroy(ctx, msb.WithDestroyForce()); e != nil {
				return nil, nil, nil, e
			}
		}
	}
	if err := ensureDevenvImage(ctx, rc.RepoRoot); err != nil {
		return nil, nil, nil, fmt.Errorf("import Microsandbox image: %w", err)
	}
	if err := ensureNixVolume(ctx, params.SandboxName, cfg.Microsandbox.Volumes); err != nil {
		return nil, nil, nil, err
	}
	opts, err := cfg.Microsandbox.sandboxOptions(rc.RepoRoot, params.SandboxName)
	if err != nil {
		return nil, nil, nil, err
	}
	sandbox, err := msb.ConnectOrCreateSandbox(ctx, params.SandboxName, opts...)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("start Microsandbox %q: %w", params.SandboxName, err)
	}
	return sandbox, cfg.Microsandbox, func() { _ = sandbox.Detach(context.Background()) }, nil
}

// ensureNixVolume seeds the persistent Nix store volume from the native image
// before it is mounted at /nix/store in the primary sandbox.
func ensureNixVolume(ctx context.Context, sandboxName string, volumes []MicrosandboxVolume) error {
	for _, volume := range volumes {
		target := filepath.Clean(volume.Target)
		if target != "/nix" && target != "/nix/store" {
			continue
		}
		name := sandboxName + "-" + volume.Name
		if ready, err := nixVolumeReady(ctx, name, ".mezha-nix-store-v3"); err != nil {
			return err
		} else if ready {
			return nil
		}

		bootstrapName := sandboxName + "-nix-seed"
		if sandbox, err := msb.GetSandbox(ctx, bootstrapName); err == nil {
			if err := sandbox.Destroy(ctx, msb.WithDestroyForce()); err != nil {
				return fmt.Errorf("remove Nix volume bootstrap sandbox: %w", err)
			}
		}
		mount := msb.Mount.NamedWith(name, msb.MountOptions{}, msb.NamedVolumeOptions{
			Mode:     volume.Mode,
			Kind:     volume.Kind,
			SizeMiB:  volume.SizeMiB,
			QuotaMiB: volume.QuotaMiB,
		})
		bootstrap, err := msb.CreateSandbox(ctx, bootstrapName,
			msb.WithImage(defaultDevenvImage),
			msb.WithDetached(),
			msb.WithUser("0"),
			msb.WithMounts(map[string]msb.MountConfig{"/mnt/mezha-nix": mount}),
		)
		if err != nil {
			return fmt.Errorf("create Nix volume bootstrap sandbox: %w", err)
		}
		output, err := bootstrap.Exec(
			ctx,
			"sh",
			[]string{
				"-c",
				"set -eu; set -o pipefail; rm -rf /mnt/mezha-nix/* /mnt/mezha-nix/.[!.]* /mnt/mezha-nix/..?*; tar -C /nix/store -cf - . | tar -C /mnt/mezha-nix -xpf -; touch /mnt/mezha-nix/.mezha-nix-store-v3; sync",
			},
		)
		if err != nil {
			stopAndDestroySandbox(bootstrap)
			return fmt.Errorf("seed Nix volume: %w", err)
		}
		if !output.Success() {
			stopAndDestroySandbox(bootstrap)
			return fmt.Errorf("seed Nix volume: %s", strings.TrimSpace(output.Stderr()))
		}
		// A disk-backed volume is not guaranteed to flush recent writes to its
		// backing image if the sandbox is force-destroyed immediately; a
		// graceful stop lets the guest unmount and sync first.
		stopAndDestroySandbox(bootstrap)
		return nil
	}
	return nil
}

// nixVolumeReady checks a disk-backed volume from inside a sandbox. VolumeFs
// addresses the host disk image on the local backend, not its mounted filesystem.
func nixVolumeReady(ctx context.Context, name, marker string) (bool, error) {
	if _, err := msb.GetVolume(ctx, name); err != nil {
		return false, nil
	}
	probeName := name + "-probe"
	if probe, err := msb.GetSandbox(ctx, probeName); err == nil {
		if err := probe.Destroy(ctx, msb.WithDestroyForce()); err != nil {
			return false, fmt.Errorf("remove Nix volume probe: %w", err)
		}
	}
	probe, err := msb.CreateSandbox(
		ctx,
		probeName,
		msb.WithImage(defaultDevenvImage),
		msb.WithDetached(),
		msb.WithUser("0"),
		msb.WithMounts(map[string]msb.MountConfig{
			"/mnt/mezha-nix": msb.Mount.Named(name, msb.MountOptions{}),
		}),
	)
	if err != nil {
		return false, fmt.Errorf("create Nix volume probe: %w", err)
	}
	defer stopAndDestroySandbox(probe)
	output, err := probe.Exec(
		ctx,
		"sh",
		[]string{
			"-c",
			"set -eu; target=$(readlink /home/devenv/.nix-profile/bin/devenv); test -x /mnt/mezha-nix/${target#/nix/store/}",
		},
	)
	if err != nil {
		return false, fmt.Errorf("inspect Nix volume %q: %w", name, err)
	}
	return output.Success(), nil
}

// stopAndDestroySandbox gracefully stops a sandbox so the guest can unmount
// and flush disk-backed volumes before the sandbox is force-destroyed as a
// fallback. Force-destroying immediately after a write can drop recent
// writes on disk-backed volumes.
func stopAndDestroySandbox(sandbox *msb.Sandbox) {
	_ = sandbox.Stop(context.Background())
	time.Sleep(500 * time.Millisecond)
	_ = sandbox.Destroy(context.Background(), msb.WithDestroyForce())
}

func nativeUpload(ctx context.Context, sandbox *msb.Sandbox, local, remote string) error {
	info, err := os.Lstat(local)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return filepath.Walk(local, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			rel, err := filepath.Rel(local, path)
			if err != nil {
				return err
			}
			guestPath := filepath.ToSlash(filepath.Join(remote, rel))
			if info.IsDir() {
				return sandbox.FS().Mkdir(ctx, guestPath)
			}
			return sandbox.FS().CopyFromHost(ctx, path, guestPath)
		})
	}
	parent := filepath.ToSlash(filepath.Dir(remote))
	if parent != "." && parent != "/" {
		if err := sandbox.FS().Mkdir(ctx, parent); err != nil {
			return err
		}
	}
	return sandbox.FS().CopyFromHost(ctx, local, remote)
}

func nativeDownload(ctx context.Context, sandbox *msb.Sandbox, remote, local string) error {
	entries, err := sandbox.FS().List(ctx, remote)
	if err == nil && len(entries) > 0 {
		if err := os.MkdirAll(local, 0o755); err != nil {
			return err
		}
		for _, entry := range entries {
			name := filepath.Base(strings.TrimRight(entry.Path, "/"))
			if err := nativeDownload(
				ctx,
				sandbox,
				entry.Path,
				filepath.Join(local, name),
			); err != nil {
				return err
			}
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(local), 0o755); err != nil {
		return err
	}
	return sandbox.FS().CopyToHost(ctx, remote, local)
}
