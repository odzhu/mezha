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
	if err := ensureStateVolume(ctx, params.SandboxName, *cfg.Microsandbox); err != nil {
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
	if err := ensurePersistentLinks(ctx, sandbox); err != nil {
		_ = sandbox.Detach(context.Background())
		return nil, nil, nil, err
	}
	return sandbox, cfg.Microsandbox, func() { _ = sandbox.Detach(context.Background()) }, nil
}

// ensureStateVolume seeds the shared persistent volume from the native image.
func ensureStateVolume(
	ctx context.Context,
	sandboxName string,
	spec MicrosandboxSpec,
) error {
	volumes := spec.Volumes
	volume := MicrosandboxVolume{
		Name: "state", Target: "/nix", Mode: "ensure-exists", Kind: "disk", SizeMiB: 51200,
	}
	for _, configured := range volumes {
		if filepath.Clean(configured.Target) == "/nix" {
			volume = configured
			break
		}
	}
	name := sandboxName + "-" + volume.Name
	if ready, err := stateVolumeReady(ctx, name, ".mezha-state-v2"); err != nil {
		return err
	} else if ready {
		return nil
	}

	fmt.Printf("Seeding persistent Nix state volume: %s (this may take several minutes)...\n", name)
	bootstrapName := sandboxName + "-state-seed"
	if sandbox, err := msb.GetSandbox(ctx, bootstrapName); err == nil {
		if err := sandbox.Destroy(ctx, msb.WithDestroyForce()); err != nil {
			return fmt.Errorf("remove state volume bootstrap sandbox: %w", err)
		}
	}
	mount := msb.Mount.NamedWith(name, msb.MountOptions{}, msb.NamedVolumeOptions{
		Mode: volume.Mode, Kind: volume.Kind, SizeMiB: volume.SizeMiB, QuotaMiB: volume.QuotaMiB,
	})
	runtimeOpts, err := spec.runtimeOptions()
	if err != nil {
		return fmt.Errorf("configure state volume bootstrap sandbox: %w", err)
	}
	bootstrapOpts := []msb.SandboxOption{
		msb.WithImage(defaultDevenvImage),
		msb.WithDetached(),
		msb.WithUser("0"),
		msb.WithMounts(map[string]msb.MountConfig{"/mnt/mezha": mount}),
	}
	bootstrapOpts = append(bootstrapOpts, runtimeOpts...)
	bootstrap, err := msb.CreateSandbox(ctx, bootstrapName, bootstrapOpts...)
	if err != nil {
		return fmt.Errorf("create state volume bootstrap sandbox: %w", err)
	}
	pulse := progressPulse("Seeding persistent Nix state volume is still running")
	output, err := bootstrap.Exec(ctx, "sh", []string{"-c", `set -eu
set -o pipefail
rm -rf /mnt/mezha/* /mnt/mezha/.[!.]* /mnt/mezha/..?*
tar -C /nix -cf - . | tar -C /mnt/mezha -xpf -
mkdir -p /mnt/mezha/mezha/root /mnt/mezha/mezha/home /mnt/mezha/mezha/sandbox /mnt/mezha/mezha/docker /mnt/mezha/mezha/k3s
cp -a /home/. /mnt/mezha/mezha/home/
touch /mnt/mezha/.mezha-state-v2
sync`})
	pulse()
	if err != nil {
		stopAndDestroySandbox(bootstrap)
		return fmt.Errorf("seed state volume: %w", err)
	}
	if !output.Success() {
		stopAndDestroySandbox(bootstrap)
		return fmt.Errorf("seed state volume: %s", strings.TrimSpace(output.Stderr()))
	}
	stopAndDestroySandbox(bootstrap)
	fmt.Printf("Seeded persistent Nix state volume: %s\n", name)
	return nil
}

// stateVolumeReady checks a disk-backed volume from inside a sandbox.
func stateVolumeReady(ctx context.Context, name, marker string) (bool, error) {
	if _, err := msb.GetVolume(ctx, name); err != nil {
		return false, nil
	}
	probeName := name + "-probe"
	if probe, err := msb.GetSandbox(ctx, probeName); err == nil {
		if err := probe.Destroy(ctx, msb.WithDestroyForce()); err != nil {
			return false, fmt.Errorf("remove state volume probe: %w", err)
		}
	}
	probe, err := msb.CreateSandbox(
		ctx,
		probeName,
		msb.WithImage(defaultDevenvImage),
		msb.WithDetached(),
		msb.WithUser("0"),
		msb.WithMounts(map[string]msb.MountConfig{
			"/mnt/mezha": msb.Mount.Named(name, msb.MountOptions{}),
		}),
	)
	if err != nil {
		return false, fmt.Errorf("create state volume probe: %w", err)
	}
	defer stopAndDestroySandbox(probe)
	output, err := probe.Exec(
		ctx,
		"sh",
		[]string{
			"-c",
			"set -eu; target=$(readlink /home/devenv/.nix-profile/bin/devenv); test -f /mnt/mezha/" + marker + " && test -x /mnt/mezha/store/${target#/nix/store/}",
		},
	)
	if err != nil {
		return false, fmt.Errorf("inspect state volume %q: %w", name, err)
	}
	return output.Success(), nil
}

// ensurePersistentLinks runs before any devenv shell or initialization command.
func ensurePersistentLinks(ctx context.Context, sandbox *msb.Sandbox) error {
	output, err := sandbox.Exec(ctx, "sh", []string{"-c", `set -eu
if [ ! -f /nix/.mezha-state-v2 ]; then
  echo "shared persistent /nix volume is not mounted; recreate the sandbox" >&2
  exit 1
fi
persist_link() {
  target="$1"
  source="$2"
  mkdir -p "$source" "$(dirname "$target")"
  if [ -L "$target" ] && [ "$(readlink "$target")" = "$source" ]; then
    return
  fi
  rm -rf "$target"
  ln -s "$source" "$target"
}
home_source=/nix/mezha/home
mkdir -p "$home_source"
if [ -d /home ] && [ -z "$(find "$home_source" -mindepth 1 -maxdepth 1 -print -quit)" ]; then
  cp -a /home/. "$home_source"/
fi
persist_link /home "$home_source"
persist_link /sandbox /nix/mezha/sandbox
persist_link /var/lib/docker /nix/mezha/docker
persist_link /var/lib/rancher/k3s /nix/mezha/k3s
persist_link /root /nix/mezha/root`}, msb.WithExecCwd("/"))
	if err != nil {
		return fmt.Errorf("create persistent state symlinks: %w", err)
	}
	if !output.Success() {
		return fmt.Errorf(
			"create persistent state symlinks: %s",
			strings.TrimSpace(output.Stderr()),
		)
	}
	return nil
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
