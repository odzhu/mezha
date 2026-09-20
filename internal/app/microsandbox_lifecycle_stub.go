//go:build !cgo

package app

import (
	"context"
	"fmt"
)

func startMicrosandbox(context.Context, string) error {
	return fmt.Errorf("local Microsandbox support requires a CGO-enabled Mezha build")
}

func stopMicrosandbox(context.Context, string) error {
	return fmt.Errorf("local Microsandbox support requires a CGO-enabled Mezha build")
}
