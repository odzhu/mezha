//go:build !cgo

package app

import (
	"context"
	"fmt"
)

func destroyMicrosandbox(context.Context, DestroyParams) error {
	return fmt.Errorf("local Microsandbox support requires a CGO-enabled Mezha build")
}
