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

func runMicrosandbox(
	ctx context.Context,
	rc RepoContext,
	params RunParams,
	cfg *MezhaConfig,
) error {
	if _, err := msb.EnsureRuntime(ctx, msb.RuntimeConfig{}, msb.InstallOptions{}); err != nil {
		return fmt.Errorf("install Microsandbox runtime: %w", err)
	}
	_, lookupErr := msb.GetSandbox(ctx, params.SandboxName)
	sandboxExisted := lookupErr == nil
	reregisterHerdr := params.Herdr
	if params.Recreate {
		if handle, err := msb.GetSandbox(ctx, params.SandboxName); err == nil {
			registered, err := unregisterHerdrMachine(ctx, params.SandboxName)
			if err != nil {
				return err
			}
			reregisterHerdr = reregisterHerdr || registered
			if err := handle.Destroy(ctx, msb.WithDestroyForce()); err != nil {
				return fmt.Errorf("recreate sandbox %q: %w", params.SandboxName, err)
			}
			if err := clearHerdrSSHControlSockets(); err != nil {
				return err
			}
		}
		if params.VolumesFlush {
			if err := flushMicrosandboxVolumes(ctx, params.SandboxName); err != nil {
				return err
			}
		}
		sandboxExisted = false
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
	sandbox, err := msb.ConnectOrCreateSandbox(ctx, params.SandboxName, opts...)
	if err != nil {
		return fmt.Errorf("start Microsandbox %q: %w", params.SandboxName, err)
	}
	defer func() { _ = sandbox.Detach(context.Background()) }()

	if err := ensurePersistentLinks(ctx, sandbox); err != nil {
		return err
	}

	// Commands use the repository by default; direct Mezha sessions start in /root.
	repoDir := params.RemoteRepoDir
	if repoDir == "" {
		repoDir = "/workspace"
	}
	sessionWorkdir := "/root"
	herdrEnabled := reregisterHerdr && herdrCommandAvailable()

	if err := ensureManagedDevenvConfig(ctx, sandbox, rc.RepoRoot); err != nil {
		return err
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

	branch, err := currentBranch(ctx, rc.RepoRoot)
	if err != nil {
		return err
	}
	needsPublish := !sandboxExisted
	if sandboxExisted {
		hasBranch, err := microsandboxBranchExists(ctx, sandbox, repoDir, branch)
		if err != nil {
			return err
		}
		needsPublish = !hasBranch
		if rc.IsLinkedWorktree && !needsPublish {
			linkedWorktreeExists, err := microsandboxLinkedWorktreeExists(ctx, sandbox, repoDir)
			if err != nil {
				return err
			}
			needsPublish = !linkedWorktreeExists
		}
	}
	if needsPublish {
		if err := publishBranchToMicrosandbox(
			ctx,
			sandbox,
			rc,
			params.SandboxName,
			repoDir,
			params.ReplaceSandboxRemote,
		); err != nil {
			return err
		}
	} else if primaryBranch, err := sandboxPrimaryBranch(ctx, rc, branch); err != nil {
		return err
	} else if err := repairSandboxPrimaryBranch(
		ctx,
		sandbox,
		canonicalSandboxProjectPaths(rc).primary,
		primaryBranch,
		rc.IsLinkedWorktree,
	); err != nil {
		return err
	} else if err := syncSandboxGitIdentity(ctx, sandbox, rc.RepoRoot, repoDir); err != nil {
		return err
	} else if err := repairMicrosandboxGitRemote(
		ctx,
		rc,
		GitParams{SandboxName: params.SandboxName, RemoteRepoDir: repoDir},
		params.ReplaceSandboxRemote,
	); err != nil {
		return err
	}
	if rc.IsLinkedWorktree {
		if err := attachSandboxLinkedWorktreeBranch(
			ctx,
			sandbox,
			canonicalSandboxProjectPaths(rc).worktree,
			branch,
		); err != nil {
			return err
		}
	}

	if len(params.RemoteCommand) != 0 {
		interactive := interactiveTTYEnabled(params.TTY)
		command, args := devenvBashCommand(
			remoteCommand(params.RemoteCommand, params.NoLoginShell, interactive),
		)
		if interactive {
			code, err := sandbox.AttachWith(
				ctx,
				command,
				args,
				sandboxAttachOptions(sessionWorkdir)...)
			if err != nil {
				return err
			}
			if code != 0 {
				return fmt.Errorf("remote command exited with code %d", code)
			}
			return nil
		}

		output, err := sandbox.Exec(ctx, command, args, msb.WithExecCwd(sessionWorkdir))
		if err != nil {
			return err
		}
		fmt.Print(output.Stdout())
		fmt.Fprint(os.Stderr, output.Stderr())
		if !output.Success() {
			return fmt.Errorf("remote command exited with code %d", output.ExitCode())
		}
		return nil
	}
	if !interactiveTTYEnabled(params.TTY) {
		return fmt.Errorf("interactive shell requires a terminal; pass a command after --")
	}
	devenv, err := sandboxCommandPath(ctx, sandbox, sessionWorkdir, "devenv")
	if err != nil {
		return err
	}
	if devenv != "" {
		command, args := devenvInteractiveShellCommand()
		code, attachErr := sandbox.AttachWith(
			ctx,
			command,
			args,
			sandboxAttachOptions(sessionWorkdir)...)
		if attachErr == nil && code == 0 {
			return nil
		}
		if attachErr != nil {
			fmt.Fprintf(os.Stderr, "devenv shell failed (%v); falling back to bash\n", attachErr)
		} else {
			fmt.Fprintf(os.Stderr, "devenv shell exited with code %d; falling back to bash\n", code)
		}
	}

	// AttachShell cannot accept a working-directory override. Use AttachWith
	// so an interactive shell starts in /root. Prefer bash and use sh only when
	// bash is unavailable. Do not wrap this fallback in devenv.
	shell, err := sandboxCommandPath(ctx, sandbox, sessionWorkdir, "bash")
	if err != nil {
		return err
	}
	if shell == "" {
		shell, err = sandboxCommandPath(ctx, sandbox, sessionWorkdir, "sh")
		if err != nil {
			return err
		}
	}
	if shell == "" {
		return fmt.Errorf("no interactive shell found: neither bash nor sh is available")
	}
	var shellArgs []string
	if !params.NoLoginShell && filepath.Base(shell) == "bash" {
		shellArgs = []string{"-lc", shellBootstrap() + "; exec \"$0\" -l", shell}
	}
	code, err := sandbox.AttachWith(ctx, shell, shellArgs, sandboxAttachOptions(sessionWorkdir)...)
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("shell exited with code %d", code)
	}
	return nil
}

// sandboxCommandPath returns the path of a command available in the sandbox.
// The bootstrap is needed because devenv may be installed in the Nix profile.
func sandboxCommandPath(
	ctx context.Context,
	sandbox *msb.Sandbox,
	workdir, name string,
) (string, error) {
	output, err := sandbox.Exec(
		ctx,
		"sh",
		[]string{"-c", shellBootstrap() + "; command -v " + name},
		msb.WithExecCwd(workdir),
	)
	if err != nil {
		return "", fmt.Errorf("check for %s in sandbox: %w", name, err)
	}
	if !output.Success() {
		return "", nil
	}
	return strings.TrimSpace(output.Stdout()), nil
}

// sandboxAttachOptions passes the local terminal type to interactive sessions.
func sandboxAttachOptions(workdir string) []msb.AttachOption {
	term := os.Getenv("TERM")
	if term == "" {
		term = "xterm-256color"
	}
	return []msb.AttachOption{
		msb.WithAttachCwd(workdir),
		msb.WithAttachEnv(map[string]string{"TERM": term}),
	}
}

// shellBootstrap makes profile-installed tools available and sets the attached
// PTY to the dimensions of the local terminal when Microsandbox reports zero.
func shellBootstrap() string {
	cols, rows := terminalSize(int(os.Stdout.Fd()))
	return fmt.Sprintf(
		`if [ -t 0 ]; then stty rows %d cols %d 2>/dev/null || :; fi; if [ -d "/nix/mezha/root/.mezha/runtime-bin" ]; then PATH="/nix/mezha/root/.mezha/runtime-bin:$PATH"; export PATH; fi; if [ -d "$HOME/.nix-profile/bin" ]; then PATH="$HOME/.nix-profile/bin:$PATH"; export PATH; fi`,
		rows,
		cols,
	)
}

// remoteCommand returns Bash arguments for a direct Mezha command. The fixed
// script preserves every command argument without shell interpolation.
func remoteCommand(command []string, noLoginShell, interactive bool) []string {
	mode := "-lc"
	bootstrap := shellBootstrap() + "; "
	if interactive {
		mode = "-ilc"
	}
	if noLoginShell {
		mode = "-c"
		bootstrap = ""
	}
	args := make([]string, 0, len(command)+4)
	script := bootstrap + `exec devenv shell --no-tui --quiet -- "$@"`
	if noLoginShell {
		script = `exec "$@"`
	}
	args = append(args, mode, script, "mezha-run")
	args = append(args, command...)
	return args
}

// microsandboxBranchExists identifies sandboxes that were created before a
// repository branch was successfully published, so they can be repaired by run.
func microsandboxBranchExists(
	ctx context.Context,
	sandbox *msb.Sandbox,
	repoDir, branch string,
) (bool, error) {
	output, err := sandbox.Exec(
		ctx,
		"sh",
		[]string{
			"-c",
			`test -e "$1/.git" && git -C "$1" rev-parse --verify --quiet "$2"`,
			"mezha-check-repo",
			repoDir,
			"refs/heads/" + branch,
		},
	)
	if err != nil {
		return false, fmt.Errorf("check Microsandbox repository branch: %w", err)
	}
	return output.Success(), nil
}

func microsandboxLinkedWorktreeExists(
	ctx context.Context,
	sandbox *msb.Sandbox,
	repoDir string,
) (bool, error) {
	output, err := sandbox.Exec(ctx, "sh", []string{
		"-c",
		`test -f "$1/.git" && grep -q '^gitdir: ' "$1/.git"`,
		"mezha-check-linked-worktree",
		repoDir,
	})
	if err != nil {
		return false, fmt.Errorf("check Microsandbox linked worktree: %w", err)
	}
	return output.Success(), nil
}
