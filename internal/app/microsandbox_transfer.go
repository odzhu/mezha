//go:build cgo

package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

func uploadPathMicrosandbox(
	ctx context.Context,
	rc RepoContext,
	params TransferParams,
	localPath, remotePath string,
) error {
	localPath = resolveLocalTransferPath(rc.InvocationCWD, localPath)
	info, err := os.Lstat(localPath)
	if err != nil {
		return fmt.Errorf("stat local upload path: %w", err)
	}
	if info.Mode()&os.ModeType != 0 && !info.IsDir() {
		return fmt.Errorf("unsupported upload path: %s", localPath)
	}
	sandbox, _, closeSandbox, err := openMicrosandbox(ctx, rc, params, params.Recreate)
	if err != nil {
		return err
	}
	defer closeSandbox()
	if err := ensureSandboxProjectDir(ctx, sandbox, params.RemoteRepoDir, rc.RepoRoot); err != nil {
		return err
	}
	_, targetDir, err := resolveRemoteTransferPath(params.RemoteRepoDir, remotePath)
	if err != nil {
		return err
	}
	remote := remotePath
	if remote == "" {
		remote = params.RemoteRepoDir
		if !info.IsDir() {
			remote = filepath.Join(remote, filepath.Base(filepath.Clean(localPath)))
		}
	}
	if !filepath.IsAbs(remote) {
		remote = filepath.Join(params.RemoteRepoDir, remote)
	}
	if info.IsDir() && targetDir {
		remote = filepath.Join(remote, filepath.Base(filepath.Clean(localPath)))
	}
	fmt.Printf("Uploading %s to Microsandbox...\n", localPath)
	return nativeUpload(ctx, sandbox, localPath, filepath.ToSlash(remote))
}

func downloadPathMicrosandbox(
	ctx context.Context,
	rc RepoContext,
	params TransferParams,
	remotePath, localPath string,
) error {
	sandbox, _, closeSandbox, err := openMicrosandbox(ctx, rc, params, false)
	if err != nil {
		return err
	}
	defer closeSandbox()
	if err := ensureSandboxProjectDir(ctx, sandbox, params.RemoteRepoDir, rc.RepoRoot); err != nil {
		return err
	}
	remote, targetDir, err := resolveRemoteTransferPath(params.RemoteRepoDir, remotePath)
	if err != nil {
		return err
	}
	if localPath == "" {
		localPath = rc.InvocationCWD
	}
	localPath = resolveLocalTransferPath(rc.InvocationCWD, localPath)
	if targetDir {
		localPath = filepath.Join(localPath, filepath.Base(filepath.Clean(remote)))
	}
	if err := assertLocalPathIsSafe(rc.InvocationCWD, localPath); err != nil {
		return err
	}
	fmt.Printf("Downloading %s from Microsandbox...\n", remote)
	return nativeDownload(ctx, sandbox, filepath.ToSlash(remote), localPath)
}
