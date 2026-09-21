//go:build cgo

package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	msb "github.com/superradcompany/microsandbox/sdk/go"
)

func uploadDirtyRepoToMicrosandbox(
	ctx context.Context,
	sandboxName, remoteRepoDir, repoRoot string,
	dirty dirtyPaths,
) error {
	knownUntracked, err := loadSyncedUntrackedPaths(repoRoot)
	if err != nil {
		return err
	}
	for _, relativePath := range knownUntracked {
		if _, err := os.Lstat(
			filepath.Join(repoRoot, filepath.FromSlash(relativePath)),
		); os.IsNotExist(
			err,
		) {
			dirty.delete = append(dirty.delete, relativePath)
		}
	}
	if len(dirty.copy) == 0 && len(dirty.delete) == 0 {
		fmt.Println("No dirty files to upload.")
		return nil
	}
	handle, err := msb.GetSandbox(ctx, sandboxName)
	if err != nil {
		return err
	}
	sandbox, err := handle.ConnectOrStart(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = sandbox.Detach(context.Background()) }()
	createdDirs := make(map[string]struct{})
	for _, relativePath := range dirty.copy {
		local := filepath.Join(repoRoot, filepath.FromSlash(relativePath))
		remote := filepath.ToSlash(filepath.Join(remoteRepoDir, relativePath))
		info, err := os.Lstat(local)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if !info.IsDir() {
			remoteDir := filepath.ToSlash(filepath.Dir(remote))
			if _, ok := createdDirs[remoteDir]; !ok {
				out, err := sandbox.Exec(ctx, "mkdir", []string{"-p", remoteDir})
				if err != nil {
					return fmt.Errorf("create remote directory for %s: %w", relativePath, err)
				}
				if !out.Success() {
					return fmt.Errorf(
						"create remote directory for %s: %s",
						relativePath,
						strings.TrimSpace(out.Stderr()),
					)
				}
				createdDirs[remoteDir] = struct{}{}
			}
			fmt.Printf("Uploading %s...\n", relativePath)
			if err := sandbox.FS().CopyFromHost(ctx, local, remote); err != nil {
				return err
			}
		}
	}
	for _, relativePath := range dirty.delete {
		remote := filepath.ToSlash(filepath.Join(remoteRepoDir, relativePath))
		fmt.Printf("Removing %s...\n", relativePath)
		_ = sandbox.FS().Remove(ctx, remote)
	}
	untracked, err := localUntrackedPaths(ctx, repoRoot)
	if err != nil {
		return err
	}
	return saveSyncedUntrackedPaths(repoRoot, untracked)
}

func downloadDirtyRepoFromMicrosandbox(
	ctx context.Context,
	sandboxName, remoteRepoDir, repoRoot string,
) error {
	handle, err := msb.GetSandbox(ctx, sandboxName)
	if err != nil {
		return err
	}
	sandbox, err := handle.ConnectOrStart(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = sandbox.Detach(context.Background()) }()
	out, err := sandbox.Exec(
		ctx,
		"git",
		[]string{"diff", "--name-status", "-z", "HEAD"},
		msb.WithExecCwd(remoteRepoDir),
	)
	if err != nil {
		return fmt.Errorf("find sandbox tracked working-tree changes: %w", err)
	}
	if !out.Success() {
		return fmt.Errorf(
			"find sandbox tracked working-tree changes: %s",
			strings.TrimSpace(out.Stderr()),
		)
	}
	dirty, err := parseDirtyPaths(out.StdoutBytes())
	if err != nil {
		return err
	}
	outUntracked, err := sandbox.Exec(
		ctx,
		"git",
		[]string{"ls-files", "--others", "--exclude-standard", "-z"},
		msb.WithExecCwd(remoteRepoDir),
	)
	if err != nil {
		return fmt.Errorf("find sandbox untracked files: %w", err)
	}
	if !outUntracked.Success() {
		return fmt.Errorf(
			"find sandbox untracked files: %s",
			strings.TrimSpace(outUntracked.Stderr()),
		)
	}
	for _, raw := range bytesSplit(outUntracked.StdoutBytes(), 0) {
		if path := string(raw); path != "" && !ignoredSyncPath(path) {
			dirty.copy = append(dirty.copy, path)
		}
	}
	if len(dirty.copy) == 0 && len(dirty.delete) == 0 {
		fmt.Println("No dirty files to download.")
		return nil
	}
	for _, relativePath := range dirty.copy {
		local := filepath.Join(repoRoot, filepath.FromSlash(relativePath))
		remote := filepath.ToSlash(filepath.Join(remoteRepoDir, relativePath))
		fmt.Printf("Downloading %s...\n", relativePath)
		if err := os.MkdirAll(filepath.Dir(local), 0o755); err != nil {
			return err
		}
		if err := sandbox.FS().CopyToHost(ctx, remote, local); err != nil {
			return err
		}
	}
	for _, relativePath := range dirty.delete {
		local := filepath.Join(repoRoot, filepath.FromSlash(relativePath))
		fmt.Printf("Removing %s...\n", relativePath)
		_ = os.Remove(local)
	}
	return nil
}
