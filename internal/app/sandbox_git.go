package app

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport"
	transportssh "github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

func PullSandboxBranch(
	ctx context.Context,
	rc RepoContext,
	params GitParams,
) error {
	if cfg, _, err := LoadConfig(rc.RepoRoot); err == nil && cfg != nil && cfg.Microsandbox != nil {
		if err := RepairSandboxGitRemote(ctx, rc, params, false); err != nil {
			return err
		}
		return pullSandboxBranchInternal(ctx, rc, params)
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

func currentBranch(_ context.Context, repoRoot string) (string, error) {
	repo, err := git.PlainOpenWithOptions(repoRoot, &git.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		return "", fmt.Errorf("open git repository: %w", err)
	}
	head, err := repo.Head()
	if err != nil {
		return "", fmt.Errorf(
			"current Git HEAD is detached or unborn; check out a branch with a commit before synchronizing it: %w",
			err,
		)
	}
	if !head.Name().IsBranch() {
		return "", fmt.Errorf(
			"current Git HEAD is detached or unborn; check out a branch with a commit before synchronizing it",
		)
	}
	branch := head.Name().Short()
	if branch == "" {
		return "", fmt.Errorf("current Git branch is empty")
	}
	return branch, nil
}

func gitIdentityValue(_ context.Context, repoRoot, key string) (string, error) {
	repo, err := git.PlainOpenWithOptions(repoRoot, &git.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		return "", nil
	}
	parts := strings.SplitN(key, ".", 2)
	if len(parts) != 2 {
		return "", nil
	}
	section, option := parts[0], parts[1]
	for _, scope := range []config.Scope{config.LocalScope, config.GlobalScope} {
		cfg, err := repo.ConfigScoped(scope)
		if err != nil || cfg == nil || cfg.Raw == nil {
			continue
		}
		if sec := cfg.Raw.Section(section); sec != nil {
			if val := sec.Option(option); val != "" {
				return val, nil
			}
		}
	}
	return "", nil
}

func setSandboxGitRemote(
	_ context.Context,
	repoRoot, remoteName, url string,
	replace bool,
) error {
	if err := validateSandboxGitRemoteName(remoteName); err != nil {
		return err
	}
	repo, err := git.PlainOpenWithOptions(repoRoot, &git.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		return fmt.Errorf("open git repository: %w", err)
	}
	existing, err := repo.Remote(remoteName)
	if err != nil {
		if _, err := repo.CreateRemote(&config.RemoteConfig{
			Name: remoteName,
			URLs: []string{url},
		}); err != nil {
			return fmt.Errorf("register sandbox Git remote: %w", err)
		}
		return nil
	}
	urls := existing.Config().URLs
	if len(urls) > 0 && !replace && !strings.Contains(urls[0], "ssh://root@mezha-sandbox-") {
		return fmt.Errorf(
			"existing %q remote is not managed by mezha; use --replace-sandbox-remote to replace it",
			remoteName,
		)
	}
	remCfg := existing.Config()
	remCfg.URLs = []string{url}
	if err := repo.DeleteRemote(remoteName); err != nil {
		return fmt.Errorf("update sandbox Git remote: %w", err)
	}
	if _, err := repo.CreateRemote(remCfg); err != nil {
		return fmt.Errorf("update sandbox Git remote: %w", err)
	}
	return nil
}

type sandboxSSHTunnel struct {
	listener  net.Listener
	targetURL string
}

func (t *sandboxSSHTunnel) Close() error {
	if t.listener != nil {
		return t.listener.Close()
	}
	return nil
}

func startSandboxSSHTunnel(
	ctx context.Context,
	sandboxName, originalURL string,
) (*sandboxSSHTunnel, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("start local SSH tunnel listener: %w", err)
	}

	executable, err := os.Executable()
	if err != nil {
		_ = listener.Close()
		return nil, fmt.Errorf("resolve mezha executable for tunnel: %w", err)
	}

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer func() { _ = c.Close() }()
				cmd := exec.CommandContext(ctx, executable, "ssh-proxy", "--sandbox", sandboxName)
				cmd.Stdin = c
				cmd.Stdout = c
				_ = cmd.Run()
			}(conn)
		}
	}()

	endpoint, err := transport.NewEndpoint(originalURL)
	if err != nil {
		_ = listener.Close()
		return nil, fmt.Errorf("parse remote URL: %w", err)
	}
	_, portStr, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		_ = listener.Close()
		return nil, fmt.Errorf("resolve tunnel listener port: %w", err)
	}
	port, _ := strconv.Atoi(portStr)
	endpoint.Host = "127.0.0.1"
	endpoint.Port = port

	return &sandboxSSHTunnel{
		listener:  listener,
		targetURL: endpoint.String(),
	}, nil
}

func sandboxSSHAuth() (transport.AuthMethod, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home directory for SSH auth: %w", err)
	}
	sshDir := filepath.Join(home, ".ssh")

	var signers []ssh.Signer
	if sock := os.Getenv("SSH_AUTH_SOCK"); sock != "" {
		if conn, err := net.Dial("unix", sock); err == nil {
			defer func() { _ = conn.Close() }()
			ag := agent.NewClient(conn)
			if agentSigners, err := ag.Signers(); err == nil {
				signers = append(signers, agentSigners...)
			}
		}
	}

	for _, name := range []string{"id_ed25519", "id_ecdsa", "id_rsa", "id_dsa"} {
		keyPath := filepath.Join(sshDir, name)
		if data, err := os.ReadFile(keyPath); err == nil {
			if signer, err := ssh.ParsePrivateKey(data); err == nil {
				signers = append(signers, signer)
			}
		}
	}

	if len(signers) == 0 {
		return nil, fmt.Errorf("no SSH signers available from agent or ~/.ssh")
	}

	return &transportssh.PublicKeysCallback{
		User: "root",
		Callback: func() ([]ssh.Signer, error) {
			return signers, nil
		},
		HostKeyCallbackHelper: transportssh.HostKeyCallbackHelper{
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		},
	}, nil
}

func pushSandboxBranchInternal(
	ctx context.Context,
	rc RepoContext,
	remoteName, branch string,
	forceWithLease bool,
) error {
	repo, err := git.PlainOpenWithOptions(rc.RepoRoot, &git.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		return fmt.Errorf("open git repository: %w", err)
	}
	rem, err := repo.Remote(remoteName)
	if err != nil || len(rem.Config().URLs) == 0 {
		return fmt.Errorf(
			"sandbox Git remote is not configured; create the sandbox with `mezha sandbox create` first",
		)
	}
	tunnel, err := startSandboxSSHTunnel(ctx, remoteName, rem.Config().URLs[0])
	if err != nil {
		return err
	}
	defer func() { _ = tunnel.Close() }()

	auth, err := sandboxSSHAuth()
	if err != nil {
		return err
	}

	pushOpts := &git.PushOptions{
		RemoteURL: tunnel.targetURL,
		RefSpecs: []config.RefSpec{
			config.RefSpec(fmt.Sprintf("refs/heads/%s:refs/heads/%s", branch, branch)),
		},
		Auth:     auth,
		Progress: os.Stdout,
	}
	if forceWithLease {
		pushOpts.Force = true
		pushOpts.ForceWithLease = &git.ForceWithLease{
			RefName: plumbing.NewBranchReferenceName(branch),
		}
	}
	fmt.Printf("Pushing branch %q to Microsandbox Git remote over SSH...\n", branch)
	if err := repo.PushContext(ctx, pushOpts); err != nil {
		if errors.Is(err, git.NoErrAlreadyUpToDate) {
			fmt.Println("Already up to date.")
			return nil
		}
		return fmt.Errorf("push branch to Microsandbox Git remote: %w", err)
	}
	return nil
}

func pullSandboxBranchInternal(
	ctx context.Context,
	rc RepoContext,
	params GitParams,
) error {
	branch, err := currentBranch(ctx, rc.RepoRoot)
	if err != nil {
		return err
	}
	repo, err := git.PlainOpenWithOptions(rc.RepoRoot, &git.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		return fmt.Errorf("open git repository: %w", err)
	}
	rem, err := repo.Remote(params.SandboxName)
	if err != nil || len(rem.Config().URLs) == 0 {
		return fmt.Errorf(
			"sandbox Git remote is not configured; create the sandbox with `mezha sandbox create` first",
		)
	}
	tunnel, err := startSandboxSSHTunnel(ctx, params.SandboxName, rem.Config().URLs[0])
	if err != nil {
		return err
	}
	defer func() { _ = tunnel.Close() }()

	auth, err := sandboxSSHAuth()
	if err != nil {
		return err
	}

	worktree, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("open git worktree: %w", err)
	}

	fmt.Printf("Pulling branch %q from Microsandbox Git remote over SSH...\n", branch)
	pullOpts := &git.PullOptions{
		RemoteURL:     tunnel.targetURL,
		ReferenceName: plumbing.NewBranchReferenceName(branch),
		SingleBranch:  true,
		Auth:          auth,
		Progress:      os.Stdout,
	}
	if err := worktree.PullContext(ctx, pullOpts); err != nil {
		if errors.Is(err, git.NoErrAlreadyUpToDate) {
			fmt.Println("Already up to date.")
			return nil
		}
		return fmt.Errorf("pull branch from Microsandbox Git remote: %w", err)
	}
	return nil
}

func countAheadBehind(repo *git.Repository, localHash, remoteHash plumbing.Hash) (int, int, error) {
	if localHash == remoteHash {
		return 0, 0, nil
	}
	localCommit, err := repo.CommitObject(localHash)
	if err != nil {
		return 0, 0, err
	}
	remoteCommit, err := repo.CommitObject(remoteHash)
	if err != nil {
		return 0, 0, err
	}
	bases, err := localCommit.MergeBase(remoteCommit)
	if err != nil {
		return 0, 0, err
	}
	var baseHash plumbing.Hash
	if len(bases) > 0 {
		baseHash = bases[0].Hash
	}
	ahead, err := countCommitsBetween(repo, localHash, baseHash)
	if err != nil {
		return 0, 0, err
	}
	behind, err := countCommitsBetween(repo, remoteHash, baseHash)
	if err != nil {
		return 0, 0, err
	}
	return ahead, behind, nil
}

func countCommitsBetween(repo *git.Repository, from, to plumbing.Hash) (int, error) {
	if from == to || from.IsZero() {
		return 0, nil
	}
	cIter, err := repo.Log(&git.LogOptions{From: from})
	if err != nil {
		return 0, err
	}
	defer cIter.Close()
	count := 0
	for {
		c, err := cIter.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, err
		}
		if !to.IsZero() && c.Hash == to {
			break
		}
		count++
	}
	return count, nil
}

func requireSandboxGitRemote(_ context.Context, repoRoot, remoteName string) error {
	if err := validateSandboxGitRemoteName(remoteName); err != nil {
		return err
	}
	repo, err := git.PlainOpenWithOptions(repoRoot, &git.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		return fmt.Errorf("open git repository: %w", err)
	}
	if _, err := repo.Remote(remoteName); err != nil {
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
