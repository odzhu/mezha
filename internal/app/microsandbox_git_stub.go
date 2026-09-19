//go:build !cgo

package app

import (
	"context"
	"fmt"
)

func sandboxGitStatusMicrosandbox(context.Context, RepoContext, GitParams, string) error {
	return fmt.Errorf("local Microsandbox Git status requires a CGO-enabled Mezha build")
}

func publishBranchToMicrosandbox(
	context.Context,
	interface{},
	RepoContext,
	string,
	string,
	bool,
) error {
	return fmt.Errorf("local Microsandbox Git requires a CGO-enabled Mezha build")
}
