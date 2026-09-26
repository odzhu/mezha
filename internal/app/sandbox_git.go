package app

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/odzhu/mezha/internal/execx"
)

func PullSandboxBranch(
	ctx context.Context,
	rc RepoContext,
	params GitParams,
	rebase, merge bool,
) error {
	if cfg, _, err := LoadConfig(rc.RepoRoot); err == nil && cfg != nil && cfg.Microsandbox != nil {
		if err := RepairSandboxGitRemote(ctx, rc, params, false); err != nil {
			return err
		}
		return pullSandboxBranchInternal(ctx, rc, params, rebase, merge)
	}
	return fmt.Errorf("microsandbox configuration missing in mezha.toml")
}

func PushSandboxBranch(
	ctx context.Context,
	rc RepoContext,
	params GitParams,
	forceWithLease bool,
) error {
	if cfg, _, err := LoadConfig(rc.RepoRoot); err == nil && cfg != nil && cfg.Microsandbox != nil {
		if err := RepairSandboxGitRemote(ctx, rc, params, false); err != nil {
			return err
		}
		branch, err := currentBranch(ctx, rc.RepoRoot)
		if err != nil {
			return err
		}
		return pushSandboxBranchInternal(ctx, rc, params.SandboxName, branch, forceWithLease)
	}
	return fmt.Errorf("microsandbox configuration missing in mezha.toml")
}

func RepairSandboxGitRemote(
	ctx context.Context,
	rc RepoContext,
	params GitParams,
	replace bool,
) error {
	remoteRepoDir, err := sandboxProjectDir(rc, params.RemoteRepoDir)
	if err != nil {
		return err
	}
	params.RemoteRepoDir = remoteRepoDir
	if cfg, _, err := LoadConfig(rc.RepoRoot); err == nil && cfg != nil && cfg.Microsandbox != nil {
		return repairMicrosandboxGitRemote(ctx, rc, params, replace)
	}
	return fmt.Errorf("microsandbox configuration missing in mezha.toml")
}
func SandboxGitStatus(ctx context.Context, rc RepoContext, params GitParams) error {
	branch, err := currentBranch(ctx, rc.RepoRoot)
	if err != nil {
		return err
	}
	if cfg, _, err := LoadConfig(rc.RepoRoot); err == nil && cfg != nil && cfg.Microsandbox != nil {
		if err := RepairSandboxGitRemote(ctx, rc, params, false); err != nil {
			return err
		}
		return sandboxGitStatusMicrosandbox(ctx, rc, params, branch)
	}
	return fmt.Errorf("microsandbox configuration missing in mezha.toml")
}
func SSHProxy(ctx context.Context, sandboxName string) error {
	if nativeMicrosandboxConfigured() {
		return microsandboxSSHProxy(ctx, sandboxName)
	}
	return fmt.Errorf("microsandbox configuration missing in mezha.toml")
}

func currentBranch(ctx context.Context, repoRoot string) (string, error) {
	output, err := execx.Output(
		ctx,
		"git",
		"-C",
		repoRoot,
		"symbolic-ref",
		"--quiet",
		"--short",
		"HEAD",
	)
	if err != nil {
		return "", fmt.Errorf(
			"current Git HEAD is detached or unborn; check out a branch with a commit before synchronizing it: %w",
			err,
		)
	}
	branch := strings.TrimSpace(string(output))
	if branch == "" {
		return "", fmt.Errorf("current Git branch is empty")
	}
	return branch, nil
}

func gitIdentityValue(ctx context.Context, repoRoot, key string) (string, error) {
	for _, scope := range []string{"--local", "--global"} {
		output, err := execx.Output(ctx, "git", "-C", repoRoot, "config", scope, "--get", key)
		if err == nil {
			return strings.TrimSpace(string(output)), nil
		}
	}
	return "", nil
}

func setSandboxGitRemote(
	ctx context.Context,
	repoRoot, remoteName, url string,
	replace bool,
) error {
	if err := validateSandboxGitRemoteName(remoteName); err != nil {
		return err
	}
	output, err := execx.Output(ctx, "git", "-C", repoRoot, "remote", "get-url", remoteName)
	if err != nil {
		if err := execx.Stream(ctx, repoRoot, "git", "remote", "add", remoteName, url); err != nil {
			return fmt.Errorf("register sandbox Git remote: %w", err)
		}
		return nil
	}
	if !replace && !strings.Contains(string(output), "ssh://root@mezha-sandbox-") {
		return fmt.Errorf(
			"existing %q remote is not managed by mezha; use --replace-sandbox-remote to replace it",
			remoteName,
		)
	}
	if err := execx.Stream(ctx, repoRoot, "git", "remote", "set-url", remoteName, url); err != nil {
		return fmt.Errorf("update sandbox Git remote: %w", err)
	}
	return nil
}

func pushSandboxBranchInternal(
	ctx context.Context,
	rc RepoContext,
	remoteName, branch string,
	forceWithLease bool,
) error {
	if err := requireSandboxGitRemote(ctx, rc.RepoRoot, remoteName); err != nil {
		return err
	}
	// Record the selected sandbox as this branch's upstream. Branch settings are
	// stored in the host repository's shared Git config, so linked worktrees use
	// the same sandbox tracking relationship.
	args := []string{"push", "--set-upstream"}
	if forceWithLease {
		args = append(args, "--force-with-lease")
	}
	args = append(args, remoteName, "HEAD:refs/heads/"+branch)
	fmt.Printf("Pushing branch %q to Microsandbox Git remote over SSH...\n", branch)
	if err := execx.Stream(ctx, rc.RepoRoot, "git", args...); err != nil {
		return fmt.Errorf("push branch to Microsandbox Git remote: %w", err)
	}
	return nil
}

func pullSandboxBranchInternal(
	ctx context.Context,
	rc RepoContext,
	params GitParams,
	rebase, merge bool,
) error {
	branch, err := currentBranch(ctx, rc.RepoRoot)
	if err != nil {
		return err
	}
	if err := requireSandboxGitRemote(ctx, rc.RepoRoot, params.SandboxName); err != nil {
		return err
	}
	args := []string{"pull", "--ff-only"}
	if rebase {
		args = []string{"pull", "--rebase"}
	}
	if merge {
		args = []string{"pull", "--no-ff"}
	}
	args = append(args, params.SandboxName, "refs/heads/"+branch)
	fmt.Printf("Pulling branch %q from Microsandbox Git remote over SSH...\n", branch)
	if err := execx.Stream(ctx, rc.RepoRoot, "git", args...); err != nil {
		return fmt.Errorf("pull branch from Microsandbox Git remote: %w", err)
	}
	return nil
}

func requireSandboxGitRemote(ctx context.Context, repoRoot, remoteName string) error {
	if err := validateSandboxGitRemoteName(remoteName); err != nil {
		return err
	}
	if _, err := execx.Output(
		ctx,
		"git",
		"-C",
		repoRoot,
		"remote",
		"get-url",
		remoteName,
	); err != nil {
		return fmt.Errorf(
			"sandbox Git remote is not configured; create the sandbox with `mezha sandbox create` first: %w",
			err,
		)
	}
	return nil
}

func validateSandboxGitRemoteName(remoteName string) error {
	switch remoteName {
	case "origin", "upstream":
		return fmt.Errorf("sandbox name %q is reserved for a standard Git remote", remoteName)
	}
	return nil
}

func ensureSandboxSSHConfig(_, _, sandboxName string) (string, error) {
	if err := ensureMicrosandboxSSHAuthorizedKeys(context.Background()); err != nil {
		return "", fmt.Errorf("ensure Microsandbox SSH authorized keys: %w", err)
	}
	hostAlias := sandboxSSHHostAlias(sandboxName)

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory for Microsandbox SSH config: %w", err)
	}
	sshDir := filepath.Join(home, ".ssh")
	configPath := filepath.Join(sshDir, "config")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		return "", fmt.Errorf("create SSH config directory: %w", err)
	}
	executable, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve mezha executable for Microsandbox SSH proxy: %w", err)
	}

	begin := "# >>> mezha sandbox " + hostAlias + " >>>"
	end := "# <<< mezha sandbox " + hostAlias + " <<<"
	block := strings.Join([]string{
		begin,
		"Host " + hostAlias,
		"    User root",
		"    StrictHostKeyChecking no",
		"    UserKnownHostsFile ~/.ssh/known_hosts",
		"    GlobalKnownHostsFile /dev/null",
		"    LogLevel ERROR",
		"    ServerAliveInterval 15",
		"    ServerAliveCountMax 3",
		"    ProxyCommand " + shellQuote(
			executable,
		) + " ssh-proxy --sandbox " + shellQuote(
			sandboxName,
		),
		end,
		"",
	}, "\n")
	contents, err := os.ReadFile(configPath)
	if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("read SSH config: %w", err)
	}
	updated := string(contents)
	if start := strings.Index(updated, begin); start >= 0 {
		if stop := strings.Index(updated[start:], end); stop >= 0 {
			stop += start + len(end)
			updated = updated[:start] + block + updated[stop:]
		} else {
			updated += "\n" + block
		}
	} else {
		if len(updated) > 0 && !strings.HasSuffix(updated, "\n") {
			updated += "\n"
		}
		updated += block
	}
	if err := os.WriteFile(configPath, []byte(updated), 0o600); err != nil {
		return "", fmt.Errorf("write SSH config: %w", err)
	}
	return hostAlias, nil
}

func sandboxSSHHostAlias(sandboxName string) string {
	digest := sha256.Sum256([]byte(sandboxName))
	return fmt.Sprintf("mezha-sandbox-%x", digest[:6])
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
