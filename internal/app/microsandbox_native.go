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
		if existing, err := msb.GetVolume(ctx, name); err == nil {
			ready, err := existing.FS().Exists(ctx, ".mezha-nix-store-v3")
			if err != nil {
				return fmt.Errorf("inspect Nix volume %q: %w", name, err)
			}
			if ready {
				return nil
			}
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
		defer func() { _ = bootstrap.Destroy(context.Background(), msb.WithDestroyForce()) }()
		defer func() { _ = bootstrap.Detach(context.Background()) }()
		output, err := bootstrap.Exec(
			ctx,
			"sh",
			[]string{
				"-c",
				"set -eu; set -o pipefail; rm -rf /mnt/mezha-nix/* /mnt/mezha-nix/.[!.]* /mnt/mezha-nix/..?*; tar -C /nix/store -cf - . | tar -C /mnt/mezha-nix -xpf -; sync; touch /mnt/mezha-nix/.mezha-nix-store-v3",
			},
		)
		if err != nil {
			return fmt.Errorf("seed Nix volume: %w", err)
		}
		if !output.Success() {
			return fmt.Errorf("seed Nix volume: %s", strings.TrimSpace(output.Stderr()))
		}
		return nil
	}
	return nil
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
