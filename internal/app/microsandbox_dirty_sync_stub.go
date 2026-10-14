//go:build !cgo

package app

import (
	"context"
	"fmt"
)

func uploadDirtyRepoToMicrosandbox(context.Context, string, string, RepoContext, dirtyPaths) error {
	return fmt.Errorf("local Microsandbox transfer requires a CGO-enabled Mezha build")
}
func downloadDirtyRepoFromMicrosandbox(context.Context, string, string, RepoContext) error {
	return fmt.Errorf("local Microsandbox transfer requires a CGO-enabled Mezha build")
}
