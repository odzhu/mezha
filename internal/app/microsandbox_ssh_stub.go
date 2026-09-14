//go:build !cgo

package app

import (
	"context"
	"fmt"
)

func nativeMicrosandboxConfigured() bool { return false }
func microsandboxSSHProxy(context.Context, string) error {
	return fmt.Errorf("local Microsandbox SSH requires a CGO-enabled Mezha build")
}
