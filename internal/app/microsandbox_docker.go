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
const nativeDevenvPath = "/home/devenv/.nix-profile/bin/devenv"

// dockerCommand runs Docker and optional k3s in the same Microsandbox exec
// job as the requested command. Microsandbox exec jobs have isolated runtime
// namespaces, so devenv tasks cannot host these daemons for a separate job.
func dockerCommand(command string, args []string, kubernetes bool) (string, []string) {
	commandLine := shellQuote(command)
	for _, arg := range args {
		commandLine += " " + shellQuote(arg)
	}

	script := `set -eu
if ! docker --context default info >/dev/null 2>&1; then
  rm -f /var/run/docker.pid
  dockerd --host=unix:///var/run/docker.sock --storage-driver=vfs >/tmp/mezha-dockerd.log 2>&1 &
  for _ in $(seq 1 30); do
    if docker --context default info >/dev/null 2>&1; then
      break
    fi
    sleep 1
  done
  if ! docker --context default info; then
    cat /tmp/mezha-dockerd.log >&2 || true
    exit 1
  fi
fi
docker context use default >/dev/null
`
	if kubernetes {
		script += kubernetesBootstrap
	}
	// Do not exec: the shell must remain alive to stop k3s when the command ends.
	script += commandLine

	return "devenv", []string{
		"shell",
		"--from", "path:" + managedDevenvPath,
		"--",
		"sh", "-c", strings.TrimSpace(script),
	}
}

const kubernetesBootstrap = `
command -v k3s >/dev/null 2>&1 || { echo "k3s is required when services.k3s.enabled is true; restore it to .mezha/devenv.nix and recreate the sandbox" >&2; exit 1; }
command -v kubectl >/dev/null 2>&1 || { echo "kubectl is required when services.k3s.enabled is true" >&2; exit 1; }
# Keep k3s in this exec job: its remotedialer connection is tied to this
# namespace and is closed when a devenv task finishes.
k3s_dir=/var/lib/rancher/k3s
mkdir -p "$k3s_dir"
kubeconfig="$k3s_dir/k3s.yaml"
k3s server --data-dir "$k3s_dir" --node-name mezha-k3s --https-listen-port=16443 --docker --write-kubeconfig "$kubeconfig" --write-kubeconfig-mode 644 >/tmp/mezha-k3s.log 2>&1 &
k3s_pid=$!
cleanup_k3s() {
  kill -TERM "$k3s_pid" 2>/dev/null || true
  wait "$k3s_pid" 2>/dev/null || true
}
trap cleanup_k3s EXIT INT TERM
for _ in $(seq 1 90); do
  if kubectl --kubeconfig "$kubeconfig" get nodes --no-headers 2>/dev/null | awk '$2 ~ /^Ready/ { ready=1 } END { exit !ready }'; then
    mkdir -p "$HOME/.kube"
    cp "$kubeconfig" "$HOME/.kube/config"
    break
  fi
  if ! kill -0 "$k3s_pid" 2>/dev/null; then
    cat /tmp/mezha-k3s.log >&2 || true
    exit 1
  fi
  sleep 1
done
if ! kubectl --kubeconfig "$kubeconfig" get nodes --no-headers 2>/dev/null | awk '$2 ~ /^Ready/ { ready=1 } END { exit !ready }'; then
  cat /tmp/mezha-k3s.log >&2 || true
  exit 1
fi
`

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
	if output.Success() {
		return nil
	}

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
		return nil
	}

	return fmt.Errorf(
		"managed devenv configuration %s is missing; add it to provision.add or recreate the sandbox",
		managedDevenvConfig,
	)
}
