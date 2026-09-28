//go:build cgo

package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport"
	msb "github.com/superradcompany/microsandbox/sdk/go"
)

func sandboxGitStatusMicrosandbox(
	ctx context.Context,
	rc RepoContext,
	params GitParams,
	branch string,
) error {
	repo, err := git.PlainOpenWithOptions(rc.RepoRoot, &git.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		return fmt.Errorf("open git repository: %w", err)
	}
	rem, err := repo.Remote(params.SandboxName)
	if err != nil || len(rem.Config().URLs) == 0 {
		return fmt.Errorf("sandbox remote is missing: %w", err)
	}
	remoteURL := rem.Config().URLs[0]

	tunnel, err := startSandboxSSHTunnel(ctx, params.SandboxName, remoteURL)
	if err != nil {
		return err
	}
	defer func() { _ = tunnel.Close() }()

	auth, err := sandboxSSHAuth()
	if err != nil {
		return err
	}

	fetchOpts := &git.FetchOptions{
		RemoteURL: tunnel.targetURL,
		RefSpecs: []config.RefSpec{
			config.RefSpec(
				fmt.Sprintf(
					"+refs/heads/%s:refs/remotes/%s/%s",
					branch,
					params.SandboxName,
					branch,
				),
			),
		},
		Auth: auth,
	}
	if err := repo.FetchContext(ctx, fetchOpts); err != nil {
		if !errors.Is(err, git.NoErrAlreadyUpToDate) &&
			!errors.Is(err, transport.ErrEmptyRemoteRepository) {
			return fmt.Errorf("fetch sandbox branch: %w", err)
		}
	}

	headRef, err := repo.Head()
	if err != nil {
		return fmt.Errorf("resolve HEAD: %w", err)
	}
	remoteRef, err := repo.Reference(
		plumbing.ReferenceName(fmt.Sprintf("refs/remotes/%s/%s", params.SandboxName, branch)),
		true,
	)
	var ahead, behind int
	if err != nil {
		if errors.Is(err, plumbing.ErrReferenceNotFound) {
			ahead, err = countCommitsBetween(repo, headRef.Hash(), plumbing.ZeroHash)
			if err != nil {
				return fmt.Errorf("compare sandbox branch: %w", err)
			}
		} else {
			return fmt.Errorf("resolve remote branch reference: %w", err)
		}
	} else {
		ahead, behind, err = countAheadBehind(repo, headRef.Hash(), remoteRef.Hash())
		if err != nil {
			return fmt.Errorf("compare sandbox branch: %w", err)
		}
	}
	fmt.Printf(
		"Local branch: %s\nSandbox remote: %s\n",
		branch,
		remoteURL,
	)
	fmt.Printf("Ahead/behind sandbox: %d\t%d\n\n", ahead, behind)

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
