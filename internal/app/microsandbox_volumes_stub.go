//go:build !cgo

package app

import (
	"context"
	"fmt"
)

func listMicrosandboxVolumes(context.Context) error {
	return fmt.Errorf("local Microsandbox support requires a CGO-enabled Mezha build")
}

func destroyMicrosandboxVolume(context.Context, string) error {
	return fmt.Errorf("local Microsandbox support requires a CGO-enabled Mezha build")
}

func flushMicrosandboxVolumes(context.Context, string) error {
	return fmt.Errorf("local Microsandbox support requires a CGO-enabled Mezha build")
}
