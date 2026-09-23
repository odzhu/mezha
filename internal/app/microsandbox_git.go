//go:build cgo

package app

import (
	"context"
	"fmt"
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
	if err := ensureSandboxProjectDir(ctx, sandbox, remoteDir, rc.RepoRoot); err != nil {
		return err
	}
	branch, err := currentBranch(ctx, rc.RepoRoot)
	if err != nil {
		return err
	}
	paths := canonicalSandboxProjectPaths(rc)
	primaryRepoDir := paths.primary
	script := `set -eu
repo="$1"; branch="$2"; linked_worktree="$3"
mkdir -p "$repo"
if [ ! -d "$repo/.git" ]; then git -C "$repo" init -q; fi
if [ "$linked_worktree" = true ]; then
  # The primary checkout only owns the shared Git directory. The requested
  # project path is added below as a real linked worktree after its branch is
  # received from the host.
  git -C "$repo" config receive.denyCurrentBranch ignore
else
  git -C "$repo" config receive.denyCurrentBranch updateInstead
  rm -f "$repo/.git/hooks/push-to-checkout"
  if ! git -C "$repo" rev-parse --verify HEAD >/dev/null 2>&1; then git -C "$repo" symbolic-ref HEAD "refs/heads/$branch"; fi
fi`
	out, err := sandbox.Exec(
		ctx,
		"sh",
		[]string{"-lc", script, "_", primaryRepoDir, branch, fmt.Sprint(rc.IsLinkedWorktree)},
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
	url := sandboxGitURL(host, rc, remoteDir)
	if err := setSandboxGitRemote(ctx, rc.RepoRoot, name, url, replace); err != nil {
		return err
	}
	if err := syncSandboxGitIdentity(ctx, sandbox, rc.RepoRoot, primaryRepoDir); err != nil {
		return err
	}
	if err := pushSandboxBranchInternal(ctx, rc, name, branch, false); err != nil {
		return err
	}
	if rc.IsLinkedWorktree {
		if err := setupSandboxLinkedWorktree(
			ctx,
			sandbox,
			primaryRepoDir,
			paths.worktree,
			branch,
		); err != nil {
			return err
		}
	}
	return nil
}

func setupSandboxLinkedWorktree(
	ctx context.Context,
	sandbox *msb.Sandbox,
	primaryRepoDir, worktreeDir, branch string,
) error {
	output, err := sandbox.Exec(ctx, "sh", []string{"-eu", "-c", `
primary="$1"; worktree="$2"; branch="$3"
# The actual checkout is under /nix/mezha/worktrees. Host-style and configured
# remote paths are aliases created by ensureSandboxProjectDir.
if [ -e "$worktree" ]; then
  git -C "$primary" worktree remove --force "$worktree" || rm -rf "$worktree"
fi
mkdir -p "$(dirname "$worktree")"
git -C "$primary" worktree add --force --detach "$worktree" "refs/heads/$branch"
hook="$primary/.git/hooks/post-receive"
cat >"$hook" <<EOF
#!/bin/sh
unset GIT_DIR GIT_WORK_TREE
while read -r old new ref; do
  git -C "$worktree" checkout --detach --force "\$new" >/dev/null 2>&1 || exit 1
done
EOF
chmod +x "$hook"
`, "mezha-link-worktree", primaryRepoDir, worktreeDir, branch})
	if err != nil {
		return fmt.Errorf("create Microsandbox linked worktree: %w", err)
	}
	if !output.Success() {
		return fmt.Errorf(
			"create Microsandbox linked worktree: %s",
			strings.TrimSpace(output.Stderr()),
		)
	}
	return nil
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
