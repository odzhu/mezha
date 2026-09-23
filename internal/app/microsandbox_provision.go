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
		if err := registerHerdrMachine(ctx, params.SandboxName); err != nil {
			return err
		}
		if err := syncHerdrPlugins(ctx, sandbox); err != nil {
			return err
		}
	}
	fmt.Printf("Provisioned Microsandbox: %s\n", params.SandboxName)
	return nil
}

// ensureSandboxProjectDir creates the canonical project hierarchy and links every
// host-style path to it. Directory hard links are not supported by Unix, so
// symbolic links provide the required path aliases.
func ensureSandboxProjectDir(
	ctx context.Context,
	sandbox *msb.Sandbox,
	dir, hostRepoRoot string,
) error {
	primaryRoot, linkedWorktree, err := linkedWorktreePrimaryRepoRoot(hostRepoRoot)
	if err != nil {
		return err
	}
	primaryName := filepath.Base(primaryRoot)
	worktreeName := filepath.Base(hostRepoRoot)
	primaryDir := filepath.ToSlash(filepath.Join("/nix/mezha/projects", primaryName))
	projectDir := primaryDir
	if linkedWorktree {
		projectDir = filepath.ToSlash(
			filepath.Join("/nix/mezha/worktrees", primaryName, worktreeName),
		)
	}

	hostHome, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve host home directory: %w", err)
	}
	homeRelative := func(path string) string {
		rel, relErr := filepath.Rel(hostHome, path)
		if relErr != nil || rel == "." || rel == ".." ||
			strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return ""
		}
		return filepath.ToSlash(rel)
	}

	output, err := sandbox.Exec(ctx, "sh", []string{"-eu", "-c", `
project="$1"; primary="$2"; remote="$3"; host_project="$4"; host_primary="$5"
home_project="$6"; home_primary="$7"
mkdir -p /nix/mezha/projects /nix/mezha/worktrees /root/.herdr
# Canonical project paths are always directories. Remove stale links left by
# earlier layouts before recreating them.
for path in "$primary" "$project"; do
  if [ -L "$path" ]; then rm -f "$path"; fi
done
mkdir -p "$primary" "$project"

link_path() {
  target="$1" link="$2"
  [ "$target" = "$link" ] && return
  if [ -d "$link" ]; then
    target_physical=$(CDPATH= cd "$target" && pwd -P)
    link_physical=$(CDPATH= cd "$link" && pwd -P)
    # A parent alias can already make this host or home path resolve to the
    # canonical directory. Creating another link here would create a loop.
    [ "$target_physical" = "$link_physical" ] && return
  fi
  mkdir -p "$(dirname "$link")"
  if [ -e "$link" ] && [ ! -L "$link" ]; then
    rm -rf "$link"
  fi
  ln -sfn "$target" "$link"
}

link_path /nix/mezha/projects /projects
link_path /nix/mezha/worktrees /worktrees
link_path /nix/mezha/projects /root/projects
link_path /nix/mezha/worktrees /root/worktrees
link_path /nix/mezha/worktrees /root/.herdr/worktrees
link_path "$project" "$remote"
link_path "$project" "$host_project"
link_path "$primary" "$host_primary"
if [ -n "$home_project" ]; then link_path "$project" "$HOME/$home_project"; fi
if [ -n "$home_primary" ]; then link_path "$primary" "$HOME/$home_primary"; fi
`, "mezha-project-links", projectDir, primaryDir, dir, hostRepoRoot, primaryRoot,
		homeRelative(hostRepoRoot), homeRelative(primaryRoot)})
	if err != nil {
		return fmt.Errorf("create sandbox project directory: %w", err)
	}
	if !output.Success() {
		return fmt.Errorf("create sandbox project directory: %s", output.Stderr())
	}
	return nil
}
