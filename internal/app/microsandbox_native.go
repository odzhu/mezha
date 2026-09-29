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
	if _, err := msb.EnsureRuntime(ctx, msb.RuntimeConfig{}, msb.InstallOptions{}); err != nil {
		return nil, nil, nil, fmt.Errorf("install Microsandbox runtime: %w", err)
	}
	sandboxExists := false
	if handle, err := msb.GetSandbox(ctx, params.SandboxName); err == nil {
		if recreate {
			if err := handle.Destroy(ctx, msb.WithDestroyForce()); err != nil {
				return nil, nil, nil, fmt.Errorf("recreate sandbox %q: %w", params.SandboxName, err)
			}
		} else {
			sandboxExists = true
		}
	}
	if !sandboxExists {
		if err := ensureStateVolume(ctx, params.SandboxName, *cfg.Microsandbox); err != nil {
			return nil, nil, nil, err
		}
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

// ensureStateVolume seeds the shared persistent volume by installing Nix and devenv into Debian.
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
	if ready, err := stateVolumeReady(ctx, name, ".mezha-state-v4", volume); err != nil {
		return err
	} else if ready {
		return nil
	}

	fmt.Printf(
		"Seeding persistent Nix state volume: %s (this may take 2-3 minutes on first use)...\n",
		name,
	)
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
		msb.WithImage(defaultDebianImage),
		msb.WithDetached(),
		msb.WithPullPolicy(msb.PullPolicyIfMissing),
		msb.WithUser("0"),
		msb.WithReplace(),
		msb.WithMounts(map[string]msb.MountConfig{"/nix": mount}),
	}
	if spec.CPUs != 0 {
		bootstrapOpts = append(bootstrapOpts, msb.WithCPUs(spec.CPUs))
	}
	if spec.MemoryMiB != 0 {
		bootstrapOpts = append(bootstrapOpts, msb.WithMemory(spec.MemoryMiB))
	} else {
		bootstrapOpts = append(bootstrapOpts, msb.WithMemory(4096))
	}
	bootstrapOpts = append(bootstrapOpts, runtimeOpts...)
	bootstrap, err := msb.CreateSandbox(ctx, bootstrapName, bootstrapOpts...)
	if err != nil {
		return fmt.Errorf("create state volume bootstrap sandbox: %w", err)
	}

	steps := []struct {
		desc string
		cmd  string
		env  map[string]string
	}{
		{
			desc: "[1/4] Installing Debian prerequisites (curl, xz-utils, ca-certificates)...",
			cmd: `set -eu
set -o pipefail
rm -rf /nix/* /nix/.[!.]* /nix/..?* 2>/dev/null || true
apt-get update -qq
apt-get install -y -qq --no-install-recommends curl xz-utils ca-certificates
mkdir -m 0755 -p /nix
mkdir -p /etc/nix
cat << "EOF" > /etc/nix/nix.conf
build-users-group =
experimental-features = nix-command flakes
trusted-users = root
EOF`,
			env: map[string]string{
				"DEBIAN_FRONTEND": "noninteractive",
				"PATH":            "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
			},
		},
		{
			desc: "[2/4] Installing Nix into persistent state volume...",
			cmd: `set -eu
if [ ! -d /nix/store ]; then
  curl --proto "=https" --tlsv1.2 -sSf -L https://nixos.org/nix/install | sh -s -- --no-daemon
fi`,
			env: map[string]string{
				"DEBIAN_FRONTEND": "noninteractive",
				"PATH":            "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
			},
		},
		{
			desc: "[3/4] Installing devenv and git into Nix profile...",
			cmd: `set -eu
export PATH="/root/.nix-profile/bin:$PATH"
nix-env --install --attr devenv git -f "<nixpkgs>" || nix-env --install --attr devenv git -f https://github.com/NixOS/nixpkgs/tarball/nixpkgs-unstable`,
			env: map[string]string{
				"DEBIAN_FRONTEND": "noninteractive",
				"PATH":            "/root/.nix-profile/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
			},
		},
		{
			desc: "[4/4] Finalizing persistent runtime environment...",
			cmd: `set -eu
mkdir -p /nix/mezha/root/.mezha/runtime-bin /nix/mezha/services/docker /nix/mezha/services/k3s /nix/mezha/etc/nix /nix/mezha/etc/ssl
cp -a /root/.nix-profile/bin/. /nix/mezha/root/.mezha/runtime-bin/
ln -sf /bin/sh /nix/mezha/root/.mezha/runtime-bin/sh
ln -sf /bin/mkdir /nix/mezha/root/.mezha/runtime-bin/mkdir
cp -a /etc/nix/. /nix/mezha/etc/nix/
cp -a /etc/ssl/. /nix/mezha/etc/ssl/ 2>/dev/null || true
cp -a /root/. /nix/mezha/root/
touch /nix/.mezha-state-v4
sync`,
			env: map[string]string{
				"PATH": "/root/.nix-profile/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
			},
		},
	}

	for _, step := range steps {
		fmt.Printf("  %s\n", step.desc)
		if err := execStreaming(
			ctx,
			bootstrap,
			"sh",
			[]string{"-c", step.cmd},
			msb.WithExecEnv(step.env),
		); err != nil {
			stopAndDestroySandbox(bootstrap)
			return fmt.Errorf("seed state volume (%s): %w", step.desc, err)
		}
	}

	stopAndDestroySandbox(bootstrap)
	fmt.Printf("Seeded persistent Nix state volume: %s\n", name)
	return nil
}

// stateVolumeReady checks a disk-backed volume from inside a sandbox.
func stateVolumeReady(
	ctx context.Context,
	name, marker string,
	volume MicrosandboxVolume,
) (bool, error) {
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
		msb.WithImage(defaultDebianImage),
		msb.WithDetached(),
		msb.WithPullPolicy(msb.PullPolicyIfMissing),
		msb.WithUser("0"),
		msb.WithReplace(),
		msb.WithMounts(map[string]msb.MountConfig{
			"/mnt/mezha": msb.Mount.NamedWith(
				name,
				msb.MountOptions{
					Readonly: volume.ReadOnly,
					Noexec:   volume.NoExec,
					Nosuid:   volume.NoSUID,
					Nodev:    volume.NoDev,
				},
				msb.NamedVolumeOptions{
					Mode:     volume.Mode,
					Kind:     volume.Kind,
					SizeMiB:  volume.SizeMiB,
					QuotaMiB: volume.QuotaMiB,
				},
			),
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
			"set -eu; test -f /mnt/mezha/" + marker + " && test -d /mnt/mezha/mezha/root/.mezha/runtime-bin && test -x /mnt/mezha/mezha/root/.mezha/runtime-bin/devenv",
		},
		msb.WithExecEnv(
			map[string]string{
				"PATH": "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
			},
		),
	)
	if err != nil {
		return false, fmt.Errorf("inspect state volume %q: %w", name, err)
	}
	return output.Success(), nil
}

// persistentRuntimeExecEnv makes the seeded Nix profile available to exec sessions.
func persistentRuntimeExecEnv() msb.ExecOption {
	return msb.WithExecEnv(map[string]string{
		"PATH":              managedDevenvProfileBin + ":" + persistentRuntimeBin + ":/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"SSL_CERT_FILE":     "/etc/ssl/certs/ca-certificates.crt",
		"NIX_SSL_CERT_FILE": "/etc/ssl/certs/ca-certificates.crt",
	})
}

// ensurePersistentLinks runs before any devenv command or managed config sync.
func ensurePersistentLinks(ctx context.Context, sandbox *msb.Sandbox) error {
	output, err := sandbox.Exec(ctx, persistentRuntimeBin+"/sh", []string{"-c", `set -eu
if [ ! -f /nix/.mezha-state-v4 ]; then
  echo "shared persistent /nix volume is not mounted; recreate the sandbox" >&2
  exit 1
fi
persist_link() {
  target="$1"
  source="$2"
  mkdir -p "$source" "$(dirname "$target")"
  if [ -L "$target" ]; then
    target_link=$(readlink "$target")
    if [ "$target_link" = "$source" ] || { [ "$target" = "/root" ] && [ "$target_link" = "/nix/root" ]; }; then
      return
    fi
  fi
  if [ "$target" = "/root" ] && [ -d "/root" ] && [ ! -L "/root" ]; then
    cp -a /root/. "$source/" 2>/dev/null || true
  fi
  rm -rf "$target"
  ln -s "$source" "$target"
}
persist_link /var/lib/docker /nix/mezha/services/docker
persist_link /var/lib/rancher/k3s /nix/mezha/services/k3s
persist_link /root /nix/mezha/root
persist_link /etc/nix /nix/mezha/etc/nix
persist_link /etc/ssl /nix/mezha/etc/ssl
if [ ! -e /nix/root ]; then
  ln -s /nix/mezha/root /nix/root
fi
if [ ! -f /etc/ssl/certs/ca-certificates.crt ] || [ ! -s /etc/ssl/certs/ca-certificates.crt ]; then
  mkdir -p /etc/ssl/certs
  cacert=$(find /nix/store -name "ca-bundle.crt" -print -quit 2>/dev/null)
  if [ -n "$cacert" ]; then
    ln -sf "$cacert" /etc/ssl/certs/ca-certificates.crt
    ln -sf "$cacert" /etc/ssl/certs/ca-bundle.crt
  fi
fi
if [ -f /etc/profile ]; then
  sed -i -e 's|PATH="/usr/local/sbin|PATH="${PATH:+$PATH:}/usr/local/sbin|' \
         -e 's|PATH="/usr/local/bin|PATH="${PATH:+$PATH:}/usr/local/bin|' /etc/profile
fi
mkdir -p /etc/profile.d
cat << 'EOF' > /etc/profile.d/mezha.sh
if [ -d "/root/.config/mezha/services/devenv/.devenv/profile/bin" ]; then
  case ":$PATH:" in
    *:/root/.config/mezha/services/devenv/.devenv/profile/bin:*) ;;
    *) PATH="/root/.config/mezha/services/devenv/.devenv/profile/bin:$PATH" ;;
  esac
fi
if [ -d "/nix/mezha/root/.mezha/runtime-bin" ]; then
  case ":$PATH:" in
    *:/nix/mezha/root/.mezha/runtime-bin:*) ;;
    *) PATH="/nix/mezha/root/.mezha/runtime-bin:$PATH" ;;
  esac
fi
export PATH
export SSL_CERT_FILE=/etc/ssl/certs/ca-certificates.crt
export NIX_SSL_CERT_FILE=/etc/ssl/certs/ca-certificates.crt
EOF
link_runtime_bins() {
  dir="$1"
  [ -d "$dir" ] || return 0
  for b in "$dir"/*; do
    [ -x "$b" ] || continue
    name=$(basename "$b")
    case "$name" in
      sh|bash|bashbug) continue ;;
    esac
    ln -sf "$b" "/nix/mezha/root/.mezha/runtime-bin/$name"
    ln -sf "$b" "/usr/local/bin/$name" 2>/dev/null || true
  done
}
link_runtime_bins /root/.config/mezha/services/devenv/.devenv/profile/bin
for b in /nix/mezha/root/.mezha/runtime-bin/*; do
  [ -x "$b" ] && ln -sf "$b" "/usr/local/bin/$(basename "$b")" 2>/dev/null || true
done
PATH=/root/.config/mezha/services/devenv/.devenv/profile/bin:/nix/mezha/root/.mezha/runtime-bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
rm -rf /home
mkdir -p /home/devenv`},
		msb.WithExecCwd("/"),
		persistentRuntimeExecEnv(),
	)
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

func execStreaming(
	ctx context.Context,
	sandbox *msb.Sandbox,
	cmd string,
	args []string,
	opts ...msb.ExecOption,
) error {
	handle, err := sandbox.ExecStream(ctx, cmd, args, opts...)
	if err != nil {
		return err
	}
	defer func() { _ = handle.Close() }()

	var exitCode int
	for {
		event, err := handle.Recv(ctx)
		if err != nil {
			return err
		}
		switch event.Kind {
		case msb.ExecEventStdout:
			_, _ = os.Stdout.Write(event.Data)
		case msb.ExecEventStderr:
			_, _ = os.Stderr.Write(event.Data)
		case msb.ExecEventExited:
			exitCode = event.ExitCode
		case msb.ExecEventFailed:
			if event.Failure != nil {
				return fmt.Errorf("exec failed: %s (%s)", event.Failure.Message, event.Failure.Kind)
			}
			return fmt.Errorf("exec failed")
		case msb.ExecEventDone:
			if exitCode != 0 {
				return fmt.Errorf("command exited with code %d", exitCode)
			}
			return nil
		}
	}
}
