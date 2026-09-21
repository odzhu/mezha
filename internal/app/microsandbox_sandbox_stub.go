//go:build !cgo

package app

import (
	"context"
	"fmt"
)

func listMicrosandboxes(context.Context) error {
	return fmt.Errorf("local Microsandbox support requires a CGO-enabled Mezha build")
}
