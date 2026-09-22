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
const commandDevenvPath = managedDevenvPath + "/.mezha/command-devenv"
const nativeDevenvPath = "/home/devenv/.nix-profile/bin/devenv"
const managedDevenvUserConfig = managedDevenvPath + "/.mezha/user-devenv.nix"
const managedDevenvWrapperMarker = "# Mezha managed devenv services wrapper v3"
const managedDevenvWrapperPrefix = "# Mezha managed devenv services wrapper"

const managedDevenvWrapper = `# Mezha managed devenv services wrapper v3
args@{ pkgs, ... }:
let
  user = import /sandbox/.mezha/user-devenv.nix;
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

// devenvCommand runs a command in the specified devenv configuration.
func devenvCommand(devenvPath, command string, args []string) (string, []string) {
	commandLine := shellQuote(command)
	for _, arg := range args {
		commandLine += " " + shellQuote(arg)
	}
	return "devenv", []string{
		"shell",
		"--from", "path:" + devenvPath,
		"--",
		"sh", "-c", commandLine,
	}
}

// dockerCommand runs a command in the managed devenv environment.
func dockerCommand(command string, args []string, _ bool) (string, []string) {
	return devenvCommand(managedDevenvPath, command, args)
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
	code, err := sandbox.AttachWith(ctx, "devenv", args, msb.WithAttachCwd(managedDevenvPath))
	if err != nil {
		return fmt.Errorf("start devenv services: %w", err)
	}
	if code != 0 {
		printDevenvDaemonLog(ctx, sandbox)
		return fmt.Errorf("start devenv services exited with code %d", code)
	}

	fmt.Println("Waiting for devenv services to become ready...")
	code, err = sandbox.AttachWith(ctx, "devenv", []string{
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
	})
	if err != nil {
		return
	}
	_, _ = fmt.Fprint(os.Stdout, output.Stdout())
	_, _ = fmt.Fprint(os.Stderr, output.Stderr())
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
			if filepath.Clean(target) != managedDevenvConfig {
				continue
			}
			if err := nativeUpload(ctx, sandbox, add.Source, target); err != nil {
				return fmt.Errorf("restore managed devenv configuration: %w", err)
			}
			break
		}
		output, err = sandbox.Exec(ctx, "test", []string{"-f", managedDevenvConfig})
		if err != nil {
			return fmt.Errorf("inspect restored managed devenv configuration: %w", err)
		}
		if !output.Success() {
			return fmt.Errorf(
				"managed devenv configuration %s is missing; add it to provision.add or recreate the sandbox",
				managedDevenvConfig,
			)
		}
	}

	return ensureManagedDevenvServicesConfig(ctx, sandbox)
}

// ensureCommandDevenvConfig combines managed and repository configurations.
func ensureCommandDevenvConfig(
	ctx context.Context,
	sandbox *msb.Sandbox,
	workdir string,
) (string, error) {
	projectConfig := filepath.ToSlash(filepath.Join(workdir, "devenv.nix"))
	output, err := sandbox.Exec(ctx, "test", []string{"-f", projectConfig})
	if err != nil {
		return "", fmt.Errorf("inspect project devenv configuration: %w", err)
	}
	if !output.Success() {
		return managedDevenvPath, nil
	}

	output, err = sandbox.Exec(ctx, "mkdir", []string{"-p", commandDevenvPath})
	if err != nil {
		return "", fmt.Errorf("create command devenv configuration directory: %w", err)
	}
	if !output.Success() {
		return "", fmt.Errorf(
			"create command devenv configuration directory: %s",
			strings.TrimSpace(output.Stderr()),
		)
	}
	config := fmt.Sprintf("{ ... }: { imports = [ %s %s ]; }\n", managedDevenvConfig, projectConfig)
	if err := sandbox.FS().WriteString(ctx, commandDevenvPath+"/devenv.nix", config); err != nil {
		return "", fmt.Errorf("write command devenv configuration: %w", err)
	}
	return commandDevenvPath, nil
}

// ensureManagedDevenvServicesConfig wraps configuration with Mezha services.
func ensureManagedDevenvServicesConfig(ctx context.Context, sandbox *msb.Sandbox) error {
	content, err := sandbox.FS().ReadString(ctx, managedDevenvConfig)
	if err != nil {
		return fmt.Errorf("read managed devenv configuration: %w", err)
	}
	if strings.HasPrefix(content, managedDevenvWrapperMarker) {
		return nil
	}
	output, err := sandbox.Exec(ctx, "mkdir", []string{"-p", filepath.Dir(managedDevenvUserConfig)})
	if err != nil {
		return fmt.Errorf("create managed devenv configuration directory: %w", err)
	}
	if !output.Success() {
		return fmt.Errorf(
			"create managed devenv configuration directory: %s",
			strings.TrimSpace(output.Stderr()),
		)
	}
	if !strings.HasPrefix(content, managedDevenvWrapperPrefix) {
		if err := sandbox.FS().WriteString(ctx, managedDevenvUserConfig, content); err != nil {
			return fmt.Errorf("preserve project devenv configuration: %w", err)
		}
	}
	if err := sandbox.FS().WriteString(ctx, managedDevenvConfig, managedDevenvWrapper); err != nil {
		return fmt.Errorf("write managed devenv services configuration: %w", err)
	}
	return nil
}
