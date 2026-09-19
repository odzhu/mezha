//go:build cgo

package app

import (
	"context"
	"fmt"
	"os"
)

func Run(ctx context.Context, rc RepoContext, params RunParams) error {
	cfg, _, err := LoadConfig(rc.RepoRoot)
	if err != nil {
		return fmt.Errorf("load mezha configuration: %w", err)
	}
	if cfg == nil || cfg.Microsandbox == nil {
		return fmt.Errorf("microsandbox configuration missing in mezha.yaml")
	}
	return runMicrosandbox(ctx, rc, params, cfg)
}
func Upload(ctx context.Context, rc RepoContext, params UploadParams) error {
	if cfg, _, err := LoadConfig(rc.RepoRoot); err == nil && cfg != nil && cfg.Microsandbox != nil {
		dirty, err := trackedDirtyPaths(ctx, rc.RepoRoot)
		if err != nil {
			return err
		}
		return uploadDirtyRepoToMicrosandbox(
			ctx,
			params.SandboxName,
			params.RemoteRepoDir,
			rc.RepoRoot,
			dirty,
		)
	}
	return fmt.Errorf("microsandbox configuration missing in mezha.yaml")
}
func Download(ctx context.Context, rc RepoContext, params DownloadParams) error {
	if cfg, _, err := LoadConfig(rc.RepoRoot); err == nil && cfg != nil && cfg.Microsandbox != nil {
		return downloadDirtyRepoFromMicrosandbox(
			ctx,
			params.SandboxName,
			params.RemoteRepoDir,
			rc.RepoRoot,
		)
	}
	return fmt.Errorf("microsandbox configuration missing in mezha.yaml")
}
func interactiveTTYEnabled(tty *bool) bool {
	if tty != nil {
		return *tty
	}
	return terminalIsTerminal(int(os.Stdin.Fd())) && terminalIsTerminal(int(os.Stdout.Fd()))
}
