//go:build !cgo

package app

import (
	"context"
	"fmt"
)

func processesMicrosandbox(context.Context, ProcessesParams) error {
	return fmt.Errorf("local Microsandbox processes require a CGO-enabled Mezha build")
}
