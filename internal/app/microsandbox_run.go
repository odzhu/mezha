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
	if err := msb.EnsureInstalled(ctx); err != nil {
		return fmt.Errorf("install Microsandbox runtime: %w", err)
	}
	_, lookupErr := msb.GetSandbox(ctx, params.SandboxName)
	sandboxExisted := lookupErr == nil
	if params.Recreate {
		if handle, err := msb.GetSandbox(ctx, params.SandboxName); err == nil {
			if err := handle.Destroy(ctx, msb.WithDestroyForce()); err != nil {
				return fmt.Errorf("recreate sandbox %q: %w", params.SandboxName, err)
			}
		}
		sandboxExisted = false
	}
	if !sandboxExisted {
		if err := ensureDevenvImage(ctx, rc.RepoRoot); err != nil {
			return fmt.Errorf("import Microsandbox image: %w", err)
		}
		if err := ensureNixVolume(ctx, params.SandboxName, cfg.Microsandbox.Volumes); err != nil {
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

	if !sandboxExisted {
		if err := applyCreateConfig(ctx, sandbox, cfg.Create); err != nil {
			return err
		}
	}
	// The repository is the default working directory. A microsandbox.workdir
	// explicitly overrides it for commands that intentionally run elsewhere.
	repoDir := params.RemoteRepoDir
	if repoDir == "" {
		repoDir = cfg.Microsandbox.Workdir
	}
	if repoDir == "" {
		repoDir = "/workspace"
	}
	workdir := cfg.Microsandbox.Workdir
	if workdir == "" {
		workdir = repoDir
	}

	needsPublish := !sandboxExisted
	if sandboxExisted {
		branch, err := currentBranch(ctx, rc.RepoRoot)
		if err != nil {
			return err
		}
		hasBranch, err := microsandboxBranchExists(ctx, sandbox, repoDir, branch)
		if err != nil {
			return err
		}
		needsPublish = !hasBranch
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
	}

	for i, directive := range cfg.Run {
		var output *msb.ExecOutput
		if cfg.Docker.Enabled || params.Kubernetes {
			if directive.Shell {
				command, args := dockerCommand(
					"sh",
					[]string{"-c", directive.Command[0]},
					params.Kubernetes,
				)
				output, err = sandbox.Exec(ctx, command, args, msb.WithExecCwd(workdir))
			} else {
				command, args := dockerCommand(
					directive.Command[0],
					directive.Command[1:],
					params.Kubernetes,
				)
				output, err = sandbox.Exec(ctx, command, args, msb.WithExecCwd(workdir))
			}
		} else if directive.Shell {
			output, err = sandbox.Shell(ctx, directive.Command[0], msb.WithExecCwd(workdir))
		} else {
			output, err = sandbox.Exec(
				ctx,
				directive.Command[0],
				directive.Command[1:],
				msb.WithExecCwd(workdir),
			)
		}
		if err != nil {
			return fmt.Errorf("run directive %d: %w", i, err)
		}
		fmt.Print(output.Stdout())
		fmt.Fprint(os.Stderr, output.Stderr())
		if !output.Success() {
			return fmt.Errorf("run directive %d exited with code %d", i, output.ExitCode())
		}
	}
	if len(params.RemoteCommand) != 0 {
		command, args := remoteCommand(params.RemoteCommand, params.NoLoginShell)
		if cfg.Docker.Enabled || params.Kubernetes {
			command, args = dockerCommand(command, args, params.Kubernetes)
		}
		if interactiveTTYEnabled(params.TTY) {
			code, err := sandbox.AttachWith(ctx, command, args, msb.WithAttachCwd(workdir))
			if err != nil {
				return err
			}
			if code != 0 {
				return fmt.Errorf("remote command exited with code %d", code)
			}
			return nil
		}

		output, err := sandbox.Exec(ctx, command, args, msb.WithExecCwd(workdir))
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
	devenv, err := sandboxCommandPath(ctx, sandbox, workdir, "devenv")
	if err != nil {
		return err
	}
	if devenv != "" {
		command, args := devenv, []string{"shell", "--no-reload"}
		if cfg.Docker.Enabled || params.Kubernetes {
			// dockerCommand already starts the managed devenv shell. Wrapping a
			// second `devenv shell` here runs its enterShell tasks twice.
			command, args = dockerCommand("bash", nil, params.Kubernetes)
		}
		code, err := sandbox.AttachWith(ctx, command, args, msb.WithAttachCwd(workdir))
		if err != nil {
			return err
		}
		if code != 0 {
			return fmt.Errorf("devenv shell exited with code %d", code)
		}
		return nil
	}

	// AttachShell cannot accept a working-directory override. Use AttachWith
	// so an interactive shell starts in the repository. Prefer bash and use sh
	// only when bash is unavailable.
	shell, err := sandboxCommandPath(ctx, sandbox, workdir, "bash")
	if err != nil {
		return err
	}
	if shell == "" {
		shell, err = sandboxCommandPath(ctx, sandbox, workdir, "sh")
		if err != nil {
			return err
		}
	}
	if shell == "" {
		return fmt.Errorf("no interactive shell found: neither bash nor sh is available")
	}
	var shellArgs []string
	if !params.NoLoginShell && filepath.Base(shell) == "bash" {
		shellArgs = []string{"-lc", shellBootstrap + "; exec \"$0\" -l", shell}
	}
	if cfg.Docker.Enabled || params.Kubernetes {
		shell, shellArgs = dockerCommand(shell, shellArgs, params.Kubernetes)
	}
	code, err := sandbox.AttachWith(ctx, shell, shellArgs, msb.WithAttachCwd(workdir))
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
		[]string{"-c", shellBootstrap + "; command -v " + name},
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

// shellBootstrap makes the profile-installed tools available and gives the
// attached PTY a usable initial size. Microsandbox's PTY can initially report
// zero rows or columns, which causes terminal-aware commands such as devenv to
// fail with EINVAL.
const shellBootstrap = `if [ -t 0 ]; then stty rows 24 cols 80 2>/dev/null || :; fi; if [ -d "$HOME/.nix-profile/bin" ]; then PATH="$HOME/.nix-profile/bin:$PATH"; export PATH; fi`

// remoteCommand runs commands through a login shell by default. The fixed
// script preserves every command argument without shell interpolation.
func remoteCommand(command []string, noLoginShell bool) (string, []string) {
	if noLoginShell {
		return command[0], command[1:]
	}
	args := make([]string, 0, len(command)+4)
	args = append(args, "-lc", shellBootstrap+`; exec "$@"`, "mezha-run")
	args = append(args, command...)
	return "bash", args
}

// applyCreateConfig applies declarative initialization only after a sandbox
// has been created. It is deliberately not repeated for existing sandboxes.
func applyCreateConfig(ctx context.Context, sandbox *msb.Sandbox, create CreateConfig) error {
	for i, add := range create.Add {
		if add.Source == "" || add.Target == "" {
			return fmt.Errorf("create.add entry %d requires source and target", i)
		}
		target := add.Target
		info, err := os.Stat(add.Source)
		if err != nil {
			return fmt.Errorf("create.add entry %d: %w", i, err)
		}
		if !info.IsDir() && strings.HasSuffix(target, "/") {
			target = filepath.ToSlash(filepath.Join(target, filepath.Base(add.Source)))
		}
		if err := nativeUpload(ctx, sandbox, add.Source, target); err != nil {
			return fmt.Errorf("create.add entry %d: %w", i, err)
		}
	}
	for i, directive := range create.Run {
		var (
			output *msb.ExecOutput
			err    error
		)
		if directive.Shell {
			output, err = sandbox.Shell(ctx, directive.Command[0])
		} else {
			output, err = sandbox.Exec(ctx, directive.Command[0], directive.Command[1:])
		}
		if err != nil {
			return fmt.Errorf("create.run directive %d: %w", i, err)
		}
		fmt.Print(output.Stdout())
		fmt.Fprint(os.Stderr, output.Stderr())
		if !output.Success() {
			return fmt.Errorf("create.run directive %d exited with code %d", i, output.ExitCode())
		}
	}
	return nil
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
		"git",
		[]string{"rev-parse", "--verify", "--quiet", "refs/heads/" + branch},
		msb.WithExecCwd(repoDir),
	)
	if err != nil {
		return false, fmt.Errorf("check Microsandbox repository branch: %w", err)
	}
	return output.Success(), nil
}
