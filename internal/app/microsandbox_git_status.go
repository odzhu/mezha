//go:build cgo

package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/odzhu/mezha/internal/execx"
	msb "github.com/superradcompany/microsandbox/sdk/go"
)

func sandboxGitStatusMicrosandbox(
	ctx context.Context,
	rc RepoContext,
	params GitParams,
	branch string,
) error {
	if _, err := execx.Output(
		ctx,
		"git",
		"-C",
		rc.RepoRoot,
		"fetch",
		params.SandboxName,
		branch,
	); err != nil {
		return fmt.Errorf("fetch sandbox branch: %w", err)
	}
	local, err := execx.Output(
		ctx,
		"git",
		"-C",
		rc.RepoRoot,
		"rev-list",
		"--left-right",
		"--count",
		"HEAD..."+params.SandboxName+"/"+branch,
	)
	if err != nil {
		return fmt.Errorf("compare sandbox branch: %w", err)
	}
	remoteURL, err := execx.Output(
		ctx,
		"git",
		"-C",
		rc.RepoRoot,
		"remote",
		"get-url",
		params.SandboxName,
	)
	if err != nil {
		return fmt.Errorf("get sandbox Git remote URL: %w", err)
	}
	fmt.Printf(
		"Local branch: %s\nSandbox remote: %s\n",
		branch,
		strings.TrimSpace(string(remoteURL)),
	)
	fmt.Printf("Ahead/behind sandbox: %s\n\n", strings.TrimSpace(string(local)))

	handle, err := msb.GetSandbox(ctx, params.SandboxName)
	if err != nil {
		return fmt.Errorf("get Microsandbox %q: %w", params.SandboxName, err)
	}
	sandbox, err := handle.ConnectOrStart(ctx)
	if err != nil {
		return fmt.Errorf("connect to Microsandbox %q: %w", params.SandboxName, err)
	}
	defer func() { _ = sandbox.Detach(context.Background()) }()
	out, err := sandbox.Exec(
		ctx,
		"sh",
		[]string{
			"-lc",
			"git -C \"$1\" status --short && git -C \"$1\" branch --show-current",
			"_",
			params.RemoteRepoDir,
		},
	)
	if err != nil {
		return fmt.Errorf("get sandbox Git status: %w", err)
	}
	if !out.Success() {
		return fmt.Errorf("get sandbox Git status: %s", strings.TrimSpace(out.Stderr()))
	}
	fmt.Printf("Sandbox status:\n%s", out.Stdout())
	return nil
}
