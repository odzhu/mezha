//go:build !cgo

package app

import (
	"context"
	"fmt"
	"os"
)

func Run(context.Context, RepoContext, RunParams) error {
	return fmt.Errorf("local Microsandbox support requires a CGO-enabled Mezha build")
}

func Upload(context.Context, RepoContext, UploadParams) error {
	return fmt.Errorf("local Microsandbox transfer requires a CGO-enabled Mezha build")
}

func Download(context.Context, RepoContext, DownloadParams) error {
	return fmt.Errorf("local Microsandbox transfer requires a CGO-enabled Mezha build")
}

func interactiveTTYEnabled(tty *bool) bool {
	if tty != nil {
		return *tty
	}
	return terminalIsTerminal(int(os.Stdin.Fd())) && terminalIsTerminal(int(os.Stdout.Fd()))
}
