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

const managedDevenvPath = "/root/.config/mezha/services/devenv"
const managedDevenvConfig = managedDevenvPath + "/devenv.nix"
const persistentRuntimeBin = "/nix/mezha/root/.mezha/runtime-bin"
const nativeDevenvPath = persistentRuntimeBin + "/devenv"
const managedDevenvUserConfig = managedDevenvPath + "/user-devenv.nix"
const managedDevenvWrapper = `# Mezha managed devenv services wrapper v3
args@{ pkgs, ... }:
let
  user = import ./user-devenv.nix;
  base = user args;
in
base // {
  packages = (base.packages or []) ++ [ pkgs.docker pkgs.k3s pkgs.kubectl ];
  env = (base.env or {}) // {
    KUBECONFIG = "/var/lib/rancher/k3s/k3s.yaml";
  };
  processes = (base.processes or {}) // {
    mezha-docker = {
      start.enable = false;
      exec = ''
        rm -f /var/run/docker.pid
        exec dockerd --host=unix:///var/run/docker.sock --storage-driver=vfs
      '';
      ready.exec = "docker info >/dev/null";
      restart.on = "always";
      shutdown.grace = 30;
    };
    mezha-k3s = {
      start.enable = false;
      after = [ "devenv:processes:mezha-docker" ];
      exec = ''
        exec k3s server \
          --data-dir /var/lib/rancher/k3s \
          --node-name mezha-k3s \
          --https-listen-port=16443 \
          --docker \
          --write-kubeconfig /var/lib/rancher/k3s/k3s.yaml \
          --write-kubeconfig-mode 644
      '';
      ready.exec = ''
        kubectl --kubeconfig /var/lib/rancher/k3s/k3s.yaml get nodes --no-headers |
          awk '$2 ~ /^Ready/ { ready=1 } END { exit !ready }'
      '';
      restart.on = "always";
      shutdown.grace = 30;
    };
  };
}
`

// devenvDirectCommand keeps managed tools on PATH without activating the
// managed environment as the command's project.
func devenvDirectCommand(command string, commandArgs []string) (string, []string) {
	args := []string{
		"shell",
		"--reload",
		"--from",
		"path:" + managedDevenvPath,
		"--",
		"sh",
		"-c",
		"export HOME=/root; unset DEVENV_ROOT _DEVENV_HOOK_DIR; exec \"$@\"",
		"mezha-direct",
		command,
	}
	return nativeDevenvPath, append(args, commandArgs...)
}

// devenvBashCommand starts Bash with the shared direct-session environment.
func devenvBashCommand(bashArgs []string) (string, []string) {
	return devenvDirectCommand("bash", bashArgs)
}

func devenvInteractiveShellCommand() (string, []string) {
	return devenvBashCommand([]string{"-il"})
}

// ensureDevenvServices starts the singleton devenv process manager and waits
// until its requested services pass their configured readiness probes.
func ensureDevenvServices(ctx context.Context, sandbox *msb.Sandbox, kubernetes bool) error {
	processes := []string{"mezha-docker"}
	if kubernetes {
		processes = append(processes, "mezha-k3s")
	}
	args := []string{"up", "--detach", "--from", "path:" + managedDevenvPath}
	args = append(args, processes...)
	fmt.Println("Starting devenv services...")
	code, err := sandbox.AttachWith(
		ctx,
		nativeDevenvPath,
		args,
		msb.WithAttachCwd(managedDevenvPath),
	)
	if err != nil {
		return fmt.Errorf("start devenv services: %w", err)
	}
	if code != 0 {
		printDevenvDaemonLog(ctx, sandbox)
		return fmt.Errorf("start devenv services exited with code %d", code)
	}

	fmt.Println("Waiting for devenv services to become ready...")
	code, err = sandbox.AttachWith(ctx, nativeDevenvPath, []string{
		"processes", "wait", "--from", "path:" + managedDevenvPath, "--timeout", "120",
	}, msb.WithAttachCwd(managedDevenvPath))
	if err != nil {
		return fmt.Errorf("wait for devenv services: %w", err)
	}
	if code != 0 {
		printDevenvDaemonLog(ctx, sandbox)
		return fmt.Errorf("wait for devenv services exited with code %d", code)
	}
	return nil
}

func printDevenvDaemonLog(ctx context.Context, sandbox *msb.Sandbox) {
	output, err := sandbox.Exec(ctx, "sh", []string{
		"-c",
		"for log in /tmp/devenv-*/processes/daemon.log; do [ -f \"$log\" ] || continue; echo \"--- $log ---\" >&2; tail -n 200 \"$log\" >&2; done",
	}, persistentRuntimeExecEnv())
	if err != nil {
		return
	}
	_, _ = fmt.Fprint(os.Stdout, output.Stdout())
	_, _ = fmt.Fprint(os.Stderr, output.Stderr())
}

// ensureManagedDevenvConfig installs the service configuration below root's config directory.
func ensureManagedDevenvConfig(
	ctx context.Context,
	sandbox *msb.Sandbox,
	provision ProvisionConfig,
) error {
	output, err := sandbox.Exec(
		ctx,
		persistentRuntimeBin+"/sh",
		[]string{"-c", "test -f \"$1\"", "mezha-test-file", managedDevenvUserConfig},
		persistentRuntimeExecEnv(),
	)
	if err != nil {
		return fmt.Errorf("inspect managed user devenv configuration: %w", err)
	}
	if !output.Success() {
		for _, add := range provision.Add {
			target := add.Target
			if strings.HasSuffix(target, "/") {
				info, statErr := os.Stat(add.Source)
				if statErr != nil || info.IsDir() {
					continue
				}
				target = filepath.ToSlash(filepath.Join(target, filepath.Base(add.Source)))
			}
			if filepath.Clean(target) != managedDevenvUserConfig {
				continue
			}
			if err := nativeUpload(ctx, sandbox, add.Source, target); err != nil {
				return fmt.Errorf("restore managed user devenv configuration: %w", err)
			}
			break
		}
		output, err = sandbox.Exec(
			ctx,
			persistentRuntimeBin+"/sh",
			[]string{"-c", "test -f \"$1\"", "mezha-test-file", managedDevenvUserConfig},
			persistentRuntimeExecEnv(),
		)
		if err != nil {
			return fmt.Errorf("inspect restored managed user devenv configuration: %w", err)
		}
		if !output.Success() {
			return fmt.Errorf(
				"managed user devenv configuration %s is missing; add it to provision.add or recreate the sandbox",
				managedDevenvUserConfig,
			)
		}
	}

	if err := ensureManagedDevenvServicesConfig(ctx, sandbox); err != nil {
		return err
	}
	return ensureDevenvBashHook(ctx, sandbox)
}

// ensureDevenvBashHook enables directory-based devenv activation for Bash.
func ensureDevenvBashHook(ctx context.Context, sandbox *msb.Sandbox) error {
	output, err := sandbox.Exec(ctx, persistentRuntimeBin+"/sh", []string{"-c", `set -eu
bashrc="/root/.bashrc"
touch "$bashrc"
sed -i '/^# mezha devenv hook$/,/^eval "$(devenv hook bash)"$/d' "$bashrc"
cat >> "$bashrc" <<'EOF'
# mezha devenv hook
if [ "${MEZHA_HERDR_PANE:-}" = 1 ]; then
  unset DEVENV_ROOT MEZHA_HERDR_PANE
fi
eval "$(devenv hook bash)"
EOF
`}, persistentRuntimeExecEnv())
	if err != nil {
		return fmt.Errorf("install devenv Bash hook: %w", err)
	}
	if !output.Success() {
		return fmt.Errorf("install devenv Bash hook: %s", strings.TrimSpace(output.Stderr()))
	}
	return nil
}

// ensureDevenvBashProfile makes direct login shells enter the project and load its hook.
func ensureDevenvBashProfile(ctx context.Context, sandbox *msb.Sandbox, workdir string) error {
	output, err := sandbox.Exec(ctx, persistentRuntimeBin+"/sh", []string{"-c", `set -eu
profile="/root/.bash_profile"
touch "$profile"
sed -i '/^# mezha project shell$/,/^# mezha project shell end$/d' "$profile"
cat >> "$profile" <<EOF
# mezha project shell
sed -i '/^# mezha project shell$/,/^# mezha project shell end$/d' "$profile"
cd "$1"
if [ -f "/root/.bashrc" ]; then
  . "/root/.bashrc"
fi
# mezha project shell end
EOF
`, "mezha-bash-profile", workdir}, persistentRuntimeExecEnv())
	if err != nil {
		return fmt.Errorf("install devenv Bash profile: %w", err)
	}
	if !output.Success() {
		return fmt.Errorf("install devenv Bash profile: %s", strings.TrimSpace(output.Stderr()))
	}
	return nil
}

// ensureManagedDevenvServicesConfig writes Mezha's service wrapper.
func ensureManagedDevenvServicesConfig(ctx context.Context, sandbox *msb.Sandbox) error {
	output, err := sandbox.Exec(
		ctx,
		persistentRuntimeBin+"/mkdir",
		[]string{"-p", filepath.Dir(managedDevenvUserConfig)},
		persistentRuntimeExecEnv(),
	)
	if err != nil {
		return fmt.Errorf("create managed devenv configuration directory: %w", err)
	}
	if !output.Success() {
		return fmt.Errorf(
			"create managed devenv configuration directory: %s",
			strings.TrimSpace(output.Stderr()),
		)
	}
	if err := sandbox.FS().WriteString(ctx, managedDevenvConfig, managedDevenvWrapper); err != nil {
		return fmt.Errorf("write managed devenv services configuration: %w", err)
	}
	return nil
}
