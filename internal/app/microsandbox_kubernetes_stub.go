//go:build !cgo

package app

import (
	"context"
	"fmt"
	msb "github.com/superradcompany/microsandbox/sdk/go"
)

func configureNativeKubernetes(context.Context, *msb.Sandbox, string) error {
	return fmt.Errorf("local Microsandbox Kubernetes requires a CGO-enabled Mezha build")
}
