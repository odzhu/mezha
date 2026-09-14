//go:build !cgo

package app

import (
	"context"
	"fmt"
)

func buildMicrosandbox(context.Context, RepoContext, BuildParams, *MezhaConfig) error {
	return fmt.Errorf("building a Microsandbox image requires a CGO-enabled Mezha build")
}
