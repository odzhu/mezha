//go:build !cgo

package app

import (
	"context"
	"fmt"
)

func logsMicrosandbox(context.Context, LogsParams) error {
	return fmt.Errorf("local Microsandbox logs require a CGO-enabled Mezha build")
}
