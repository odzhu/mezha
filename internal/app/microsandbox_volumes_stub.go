//go:build !cgo

package app

import (
	"context"
	"fmt"
)

func flushMicrosandboxVolumes(context.Context, string) error {
	return fmt.Errorf("local Microsandbox support requires a CGO-enabled Mezha build")
}
