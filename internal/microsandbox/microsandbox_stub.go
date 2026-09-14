//go:build !cgo

package microsandbox

import (
	"context"
	"fmt"
)

const (
	SandboxSuffix = "-k8s"
	Host          = "host.containers.internal"
	APIPort       = uint16(6443)
	DockerPort    = uint16(2375)
)

func Name(primarySandbox string) string { return primarySandbox + SandboxSuffix }

func EnsureKubernetes(context.Context, string) ([]byte, error) {
	return nil, fmt.Errorf("Kubernetes support requires a CGO-enabled Mezha build because the microsandbox Go SDK uses CGO")
}

func Exists(context.Context, string) bool { return false }

func RemoveKubernetes(context.Context, string) error { return nil }
