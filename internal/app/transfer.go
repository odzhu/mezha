package app

import (
	"context"
	"fmt"
)

func UploadPath(
	ctx context.Context,
	rc RepoContext,
	params TransferParams,
	localPath, remotePath string,
) error {
	if cfg, _, err := LoadConfig(rc.RepoRoot); err == nil && cfg != nil && cfg.Microsandbox != nil {
		return uploadPathMicrosandbox(ctx, rc, params, localPath, remotePath)
	}
	return fmt.Errorf("microsandbox configuration missing in mezha.yaml")
}

func DownloadPath(
	ctx context.Context,
	rc RepoContext,
	params TransferParams,
	remotePath, localPath string,
) error {
	if cfg, _, err := LoadConfig(rc.RepoRoot); err == nil && cfg != nil && cfg.Microsandbox != nil {
		return downloadPathMicrosandbox(ctx, rc, params, remotePath, localPath)
	}
	return fmt.Errorf("microsandbox configuration missing in mezha.yaml")
}
func resolveLocalTransferPath(cwd, p string) string                 { return p }
func resolveRemoteTransferPath(dir, p string) (string, bool, error) { return p, false, nil }
