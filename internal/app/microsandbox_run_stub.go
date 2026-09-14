//go:build !cgo

package app

import (
	"context"
	"fmt"
)

func runMicrosandbox(_ context.Context, _ RepoContext, _ RunParams, _ *MezhaConfig) error {
	return fmt.Errorf("local Microsandbox support requires a CGO-enabled Mezha build")
}
