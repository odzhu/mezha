//go:build cgo

package app

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	msb "github.com/superradcompany/microsandbox/sdk/go"
)

func publishBranchToMicrosandbox(
	ctx context.Context,
	sandbox *msb.Sandbox,
	rc RepoContext,
	name, remoteDir string,
	replace bool,
) error {
	branch, err := currentBranch(ctx, rc.RepoRoot)
	if err != nil {
		return err
	}
	script := `set -eu
repo="$1"; branch="$2"
mkdir -p "$repo"
if [ ! -d "$repo/.git" ]; then git -C "$repo" init -q; fi
git -C "$repo" config receive.denyCurrentBranch updateInstead
# updateInstead safely updates the checked-out branch after a push. Remove the
# old custom hook because it overrides updateInstead and prevented initial sync.
rm -f "$repo/.git/hooks/push-to-checkout"
if ! git -C "$repo" rev-parse --verify HEAD >/dev/null 2>&1; then git -C "$repo" symbolic-ref HEAD "refs/heads/$branch"; fi`
	out, err := sandbox.Exec(
		ctx,
		"sh",
		[]string{"-lc", script, "_", remoteDir, branch},
	)
	if err != nil {
		return fmt.Errorf("initialize Microsandbox git repository: %w", err)
	}
	if !out.Success() {
		return fmt.Errorf(
			"initialize Microsandbox git repository: %s",
			strings.TrimSpace(out.Stderr()),
		)
	}
	host, err := ensureSandboxSSHConfig("", "", name)
	if err != nil {
		return err
	}
	url := fmt.Sprintf(
		"ssh://root@%s//%s/.git",
		host,
		strings.TrimPrefix(filepath.ToSlash(remoteDir), "/"),
	)
	if err := setSandboxGitRemote(ctx, rc.RepoRoot, name, url, replace); err != nil {
		return err
	}
	if err := syncSandboxGitIdentity(ctx, sandbox, rc.RepoRoot, remoteDir); err != nil {
		return err
	}
	return pushSandboxBranchInternal(ctx, rc, name, branch, false)
}

func syncSandboxGitIdentity(
	ctx context.Context,
	sandbox *msb.Sandbox,
	repoRoot, remoteDir string,
) error {
	userName, err := gitIdentityValue(ctx, repoRoot, "user.name")
	if err != nil {
		return fmt.Errorf("read local Git user.name: %w", err)
	}
	userEmail, err := gitIdentityValue(ctx, repoRoot, "user.email")
	if err != nil {
		return fmt.Errorf("read local Git user.email: %w", err)
	}
	output, err := sandbox.Exec(ctx, "sh", []string{"-eu", "-c", `
repo="$1"; git_name="$2"; git_email="$3"
set_identity() {
  key="$1" value="$2"
  if [ -n "$value" ]; then
    git -C "$repo" config "$key" "$value"
  else
    git -C "$repo" config --unset-all "$key" || :
  fi
}
set_identity user.name "$git_name"
set_identity user.email "$git_email"
`, "mezha-sync-git-identity", remoteDir, userName, userEmail})
	if err != nil {
		return fmt.Errorf("set Microsandbox Git identity: %w", err)
	}
	if !output.Success() {
		return fmt.Errorf("set Microsandbox Git identity: %s", strings.TrimSpace(output.Stderr()))
	}
	return nil
}
