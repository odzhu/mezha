//go:build cgo

package app

import "strings"

const dockerPath = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"

// dockerCommand runs a command with Docker and optional k3s in the same exec job.
func dockerCommand(command string, args []string, kubernetes bool) (string, []string) {
	commandLine := shellQuote(command)
	for _, arg := range args {
		commandLine += " " + shellQuote(arg)
	}

	script := `set -eu
PATH=` + dockerPath + `
export PATH
`
	script += `if ! docker --context default info >/dev/null 2>&1; then
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
	// Do not exec here: the shell must remain alive to run the k3s cleanup trap.
	script += commandLine

	return "sh", []string{"-c", strings.TrimSpace(script)}
}

const kubernetesBootstrap = `
command -v k3s >/dev/null 2>&1 || { echo "k3s is required when sandbox.kubernetes is enabled; rebuild the Mezha image" >&2; exit 1; }
command -v kubectl >/dev/null 2>&1 || { echo "kubectl is required when sandbox.kubernetes is enabled" >&2; exit 1; }
# Exec jobs have isolated runtime namespaces. Start k3s in this job, alongside
# the requested command, just as Mezha starts dockerd above. Use that Docker
# daemon as the Kubernetes runtime so Docker-built images are immediately usable.
k3s_dir=/var/lib/rancher/k3s
mkdir -p "$k3s_dir"
kubeconfig="$k3s_dir/k3s.yaml"
k3s server --data-dir "$k3s_dir" --node-name mezha-k3s --https-listen-port=16443 --docker --write-kubeconfig "$kubeconfig" --write-kubeconfig-mode 644 >/tmp/mezha-k3s.log 2>&1 &
k3s_pid=$!
cleanup_k3s() {
  # Stop k3s and its control-plane children before the exec job ends.
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
