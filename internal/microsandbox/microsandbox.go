//go:build cgo

package microsandbox

import (
	"context"
	"fmt"
	"strings"
	"time"

	msb "github.com/superradcompany/microsandbox/sdk/go"
)

const (
	SandboxSuffix = "-k8s"
	// Host is the address Microsandbox guests use to reach the local host.
	Host       = "host.containers.internal"
	APIPort    = uint16(6443)
	DockerPort = uint16(2375)

	kubernetesImage = "rancher/k3s:v1.32.3-k3s1"
	dockerDindImage = "docker:27-dind"

	kubernetesStartupTimeout = 5 * time.Minute
)

func Name(primarySandbox string) string { return primarySandbox + SandboxSuffix }

func EnsureKubernetes(ctx context.Context, primarySandbox string) ([]byte, error) {
	fmt.Println("Preparing microsandbox runtime...")
	if err := msb.EnsureInstalled(ctx); err != nil {
		return nil, fmt.Errorf("install microsandbox runtime: %w", err)
	}
	name := Name(primarySandbox)
	fmt.Printf("Starting Kubernetes microsandbox %s (the first run pulls %s)...\n", name, dockerDindImage)
	startupCtx, cancel := context.WithTimeout(ctx, kubernetesStartupTimeout)
	defer cancel()
	sandbox, err := msb.ConnectOrCreateSandbox(startupCtx, name,
		msb.WithImage(dockerDindImage),
		msb.WithMemory(4096),
		msb.WithRootDisk(msb.RootDisk.Flat(msb.RootDiskFlatOptions{SizeMiB: 20480, Clone: msb.FlatCloneAuto})),
		msb.WithPortBindings(
			msb.PortBinding{Bind: "0.0.0.0", HostPort: APIPort, GuestPort: APIPort},
			msb.PortBinding{Bind: "0.0.0.0", HostPort: DockerPort, GuestPort: DockerPort},
		),
		msb.WithDetached(),
	)
	if err != nil {
		return nil, fmt.Errorf("create Kubernetes microsandbox %q: %w", name, err)
	}
	defer sandbox.Detach(context.Background())

	fmt.Printf("Initializing k3s in Kubernetes microsandbox %s (the first run pulls %s)...\n", name, kubernetesImage)
	setup := fmt.Sprintf(`set -eu
# ConnectOrCreateSandbox boots an idle VM; it deliberately does not run an
# OCI image's default command. Start docker:dind explicitly before using its
# daemon for k3s.
if ! pgrep -x dockerd >/dev/null 2>&1; then
  dockerd-entrypoint.sh dockerd >/tmp/dockerd.log 2>&1 &
fi
for i in $(seq 1 60); do
  docker info >/dev/null 2>&1 && break
  sleep 1
done
if ! docker info >/dev/null 2>&1; then
  cat /tmp/dockerd.log >&2 || true
  exit 1
fi
apk add --no-cache socat >/dev/null
if ! pgrep -x socat >/dev/null 2>&1; then
  socat TCP-LISTEN:%d,fork,reuseaddr UNIX-CONNECT:/var/run/docker.sock >/tmp/docker-tcp-proxy.log 2>&1 &
fi
if ! docker inspect k3s >/dev/null 2>&1; then
  docker run -d --name k3s --privileged --network host %s server \
    --write-kubeconfig-mode 644 \
    --tls-san %s \
    --tls-san 127.0.0.1
fi
until docker exec k3s kubectl get nodes >/dev/null 2>&1; do sleep 2; done
`, DockerPort, kubernetesImage, Host)
	result, err := sandbox.Shell(startupCtx, setup)
	if err != nil {
		return nil, fmt.Errorf("initialize Kubernetes microsandbox %q: %w", name, err)
	}
	if !result.Success() {
		return nil, fmt.Errorf("initialize Kubernetes microsandbox %q: command exited with code %d: %s", name, result.ExitCode(), strings.TrimSpace(result.Stderr()))
	}

	kubeconfig, err := sandbox.Exec(startupCtx, "docker", []string{"exec", "k3s", "cat", "/etc/rancher/k3s/k3s.yaml"})
	if err != nil {
		return nil, fmt.Errorf("read kubeconfig from Kubernetes microsandbox %q: %w", name, err)
	}
	if !kubeconfig.Success() {
		return nil, fmt.Errorf("read kubeconfig from Kubernetes microsandbox %q: %s", name, strings.TrimSpace(kubeconfig.Stderr()))
	}
	endpoint := fmt.Sprintf("https://%s:%d", Host, APIPort)
	return []byte(strings.ReplaceAll(kubeconfig.Stdout(), "https://127.0.0.1:6443", endpoint)), nil
}

func Exists(ctx context.Context, primarySandbox string) bool {
	_, err := msb.GetSandbox(ctx, Name(primarySandbox))
	return err == nil
}

func RemoveKubernetes(ctx context.Context, primarySandbox string) error {
	name := Name(primarySandbox)
	sandbox, err := msb.GetSandbox(ctx, name)
	if msb.IsKind(err, msb.ErrSandboxNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("get Kubernetes microsandbox %q: %w", name, err)
	}
	if err := sandbox.Destroy(ctx, msb.WithDestroyForce()); err != nil {
		return fmt.Errorf("delete Kubernetes microsandbox %q: %w", name, err)
	}
	return nil
}
