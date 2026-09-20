//go:build !cgo

package app

import (
	"context"
	"fmt"
)

func provisionMicrosandbox(context.Context, RepoContext, ProvisionParams, *MezhaConfig) error {
	return fmt.Errorf("local Microsandbox support requires a CGO-enabled Mezha build")
}
