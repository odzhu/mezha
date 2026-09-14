//go:build !cgo

package app

import (
	"context"
	"fmt"
)

func uploadPathMicrosandbox(context.Context, RepoContext, TransferParams, string, string) error {
	return fmt.Errorf("local Microsandbox transfer requires a CGO-enabled Mezha build")
}
func downloadPathMicrosandbox(context.Context, RepoContext, TransferParams, string, string) error {
	return fmt.Errorf("local Microsandbox transfer requires a CGO-enabled Mezha build")
}
