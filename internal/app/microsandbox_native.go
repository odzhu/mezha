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

func openMicrosandbox(ctx context.Context, rc RepoContext, params TransferParams, recreate bool) (*msb.Sandbox, *MicrosandboxSpec, func(), error) {
	cfg, _, err := LoadConfig(rc.RepoRoot)
	if err != nil {
		return nil, nil, nil, err
	}
	if cfg == nil || cfg.Microsandbox == nil {
		return nil, nil, nil, fmt.Errorf("microsandbox configuration is missing")
	}
	if err := msb.EnsureInstalled(ctx); err != nil {
		return nil, nil, nil, fmt.Errorf("install Microsandbox runtime: %w", err)
	}
	if recreate {
		if h, e := msb.GetSandbox(ctx, params.SandboxName); e == nil {
			if e = h.Destroy(ctx, msb.WithDestroyForce()); e != nil {
				return nil, nil, nil, e
			}
		}
	}
	opts, err := cfg.Microsandbox.sandboxOptions(rc.RepoRoot, params.SandboxName)
	if err != nil {
		return nil, nil, nil, err
	}
	sandbox, err := msb.ConnectOrCreateSandbox(ctx, params.SandboxName, opts...)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("start Microsandbox %q: %w", params.SandboxName, err)
	}
	return sandbox, cfg.Microsandbox, func() { _ = sandbox.Detach(context.Background()) }, nil
}

func nativeUpload(ctx context.Context, sandbox *msb.Sandbox, local, remote string) error {
	info, err := os.Lstat(local)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return filepath.Walk(local, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if info.IsDir() {
				return nil
			}
			rel, err := filepath.Rel(local, path)
			if err != nil {
				return err
			}
			return sandbox.FS().CopyFromHost(ctx, path, filepath.ToSlash(filepath.Join(remote, rel)))
		})
	}
	return sandbox.FS().CopyFromHost(ctx, local, remote)
}

func nativeDownload(ctx context.Context, sandbox *msb.Sandbox, remote, local string) error {
	entries, err := sandbox.FS().List(ctx, remote)
	if err == nil && len(entries) > 0 {
		if err := os.MkdirAll(local, 0o755); err != nil {
			return err
		}
		for _, entry := range entries {
			name := filepath.Base(strings.TrimRight(entry.Path, "/"))
			if err := nativeDownload(ctx, sandbox, entry.Path, filepath.Join(local, name)); err != nil {
				return err
			}
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(local), 0o755); err != nil {
		return err
	}
	return sandbox.FS().CopyToHost(ctx, remote, local)
}
