//go:build cgo

package app

import (
	"context"
	"fmt"
	"strings"

	msb "github.com/superradcompany/microsandbox/sdk/go"
)

func configureNativeKubernetes(ctx context.Context, primarySandbox *msb.Sandbox, primarySandboxName string) error {
	kubeconfig, err := ensureKubernetesSandbox(ctx, primarySandboxName)
	if err != nil {
		return err
	}

	// Ensure the primary sandbox allows traffic to the host API/Docker ports
	// via its network configuration. For this iteration, we dynamically check/configure the
	// primary sandbox clients over exec.
	fmt.Printf("Configuring kubectl and Docker in Microsandbox %s...\n", primarySandboxName)
	script := fmt.Sprintf(`set -eu
mkdir -p "$HOME/.kube"
cat >"$HOME/.kube/config" <<'EOF'
%s
EOF
docker context inspect mezha-k8s >/dev/null 2>&1 || docker context create mezha-k8s --docker "host=tcp://%s:%d" >/dev/null
docker context use mezha-k8s >/dev/null`, kubeconfig, "10.0.2.2", 2375)

	// 10.0.2.2 is the standard Microsandbox (smoltcp/QEMU SLIRP) gateway address pointing to the host
	out, err := primarySandbox.Shell(ctx, script)
	if err != nil {
		return fmt.Errorf("configure Kubernetes clients in Microsandbox %q: %w", primarySandboxName, err)
	}
	if !out.Success() {
		return fmt.Errorf("configure Kubernetes clients in Microsandbox %q: %s", primarySandboxName, strings.TrimSpace(out.Stderr()))
	}
	return nil
}
