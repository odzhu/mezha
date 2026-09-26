//go:build cgo

package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/odzhu/mezha/internal/execx"
	msb "github.com/superradcompany/microsandbox/sdk/go"
)

func provisionMicrosandbox(
	ctx context.Context,
	rc RepoContext,
	params ProvisionParams,
	cfg *MezhaConfig,
) error {
	if _, err := msb.EnsureRuntime(ctx, msb.RuntimeConfig{}, msb.InstallOptions{}); err != nil {
		return fmt.Errorf("install Microsandbox runtime: %w", err)
	}
	_, lookupErr := msb.GetSandbox(ctx, params.SandboxName)
	sandboxExisted := lookupErr == nil
	if lookupErr != nil && !msb.IsKind(lookupErr, msb.ErrSandboxNotFound) {
		return fmt.Errorf("find Microsandbox %q: %w", params.SandboxName, lookupErr)
	}
	if params.Recreate && sandboxExisted {
		if _, err := unregisterHerdrMachine(ctx, params.SandboxName); err != nil {
			return err
		}
		handle, err := msb.GetSandbox(ctx, params.SandboxName)
		if err != nil {
			return fmt.Errorf("find Microsandbox %q: %w", params.SandboxName, err)
		}
		if err := handle.Destroy(ctx, msb.WithDestroyForce()); err != nil {
			return fmt.Errorf("recreate sandbox %q: %w", params.SandboxName, err)
		}
		if err := clearHerdrSSHControlSockets(); err != nil {
			return err
		}
		sandboxExisted = false
	}
	if params.VolumesFlush {
		if err := flushMicrosandboxVolumes(ctx, params.SandboxName); err != nil {
			return err
		}
	}
	if !sandboxExisted {
		if err := ensureStateVolume(ctx, params.SandboxName, *cfg.Microsandbox); err != nil {
			return err
		}
	}
	opts, err := cfg.Microsandbox.sandboxOptions(rc.RepoRoot, params.SandboxName)
	if err != nil {
		return err
	}
	fmt.Printf("Starting or connecting to Microsandbox: %s...\n", params.SandboxName)
	sandbox, err := msb.ConnectOrCreateSandbox(ctx, params.SandboxName, opts...)
	if err != nil {
		return fmt.Errorf("start Microsandbox %q: %w", params.SandboxName, err)
	}
	defer func() { _ = sandbox.Detach(context.Background()) }()
	fmt.Printf("Connected to Microsandbox: %s\n", params.SandboxName)

	herdrEnabled := params.Herdr && herdrCommandAvailable()

	if err := ensureManagedDevenvConfig(ctx, sandbox, rc.RepoRoot); err != nil {
		return err
	}
	repoDir := params.RemoteRepoDir
	if repoDir == "" {
		repoDir = "/workspace"
	}
	if err := runMezhaInitSandboxTask(ctx, sandbox, rc, repoDir, herdrEnabled); err != nil {
		return err
	}
	if err := ensureDevenvServices(ctx, sandbox, herdrEnabled); err != nil {
		return err
	}
	if herdrEnabled {
		if err := registerHerdrMachine(ctx, params.SandboxName); err != nil {
			return err
		}
		if err := syncHerdrPlugins(ctx, sandbox); err != nil {
			return err
		}
	}
	fmt.Printf("Provisioned Microsandbox: %s\n", params.SandboxName)
	return nil
}

func runMezhaInitSandboxTask(
	ctx context.Context,
	sandbox *msb.Sandbox,
	rc RepoContext,
	repoDir string,
	herdrEnabled bool,
) error {
	env := map[string]string{
		"HOME":        "/root",
		"USER":        "root",
		"MSB_WORKDIR": repoDir,
		"PATH":        "/nix/mezha/root/.mezha/runtime-bin",
	}

	primaryRoot, linkedWorktree, err := linkedWorktreePrimaryRepoRoot(rc.RepoRoot)
	if err != nil {
		return err
	}
	primaryName := filepath.Base(primaryRoot)
	worktreeName := filepath.Base(rc.RepoRoot)
	primaryDir := filepath.ToSlash(filepath.Join("/nix/mezha/projects", primaryName))
	projectDir := primaryDir
	if linkedWorktree {
		projectDir = filepath.ToSlash(
			filepath.Join("/nix/mezha/worktrees", primaryName, worktreeName),
		)
	}

	hostHome, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve host home directory: %w", err)
	}
	homeRelative := func(path string) string {
		rel, relErr := filepath.Rel(hostHome, path)
		if relErr != nil || rel == "." || rel == ".." ||
			strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return ""
		}
		return filepath.ToSlash(rel)
	}

	env["MSB_PROJECT"] = projectDir
	env["MSB_PRIMARY"] = primaryDir
	env["MSB_REMOTE"] = repoDir
	env["MSB_HOST_PROJECT"] = rc.RepoRoot
	env["MSB_HOST_PRIMARY"] = primaryRoot
	env["MSB_HOME_PROJECT"] = homeRelative(rc.RepoRoot)
	env["MSB_HOME_PRIMARY"] = homeRelative(primaryRoot)

	if herdrEnabled {
		versionOutput, err := execx.Output(ctx, "herdr", "--version")
		if err != nil {
			return fmt.Errorf("get Herdr version: %w", err)
		}
		matches := herdrVersion.FindStringSubmatch(strings.TrimSpace(string(versionOutput)))
		if matches == nil {
			return fmt.Errorf(
				"unrecognized Herdr version %q",
				strings.TrimSpace(string(versionOutput)),
			)
		}
		env["MSB_HERDR_VERSION"] = matches[1]
	}

	fmt.Printf("Running mezha:init-sandbox task...\n")
	code, err := sandbox.AttachWith(ctx, nativeDevenvPath, []string{
		"tasks",
		"run",
		"mezha:init-sandbox",
		"--show-output",
		"--from",
		"path:" + managedDevenvPath,
	}, msb.WithAttachCwd(managedDevenvPath), msb.WithAttachEnv(env))
	if err != nil {
		return fmt.Errorf("run mezha:init-sandbox task: %w", err)
	}
	if code != 0 {
		return fmt.Errorf("run mezha:init-sandbox task exited with code %d", code)
	}
	if herdrEnabled {
		if err := syncSandboxHerdrConfig(ctx, sandbox); err != nil {
			return err
		}
	}
	return nil
}
