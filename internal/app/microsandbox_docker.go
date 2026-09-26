//go:build cgo

package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	msb "github.com/superradcompany/microsandbox/sdk/go"
)

const managedDevenvPath = "/root/.config/mezha/services/devenv"
const managedDevenvConfig = managedDevenvPath + "/devenv.nix"
const persistentRuntimeBin = "/nix/mezha/root/.mezha/runtime-bin"
const nativeDevenvPath = persistentRuntimeBin + "/devenv"

// devenvDirectCommand keeps managed tools on PATH without activating the
// managed environment as the command's project.
func devenvDirectCommand(command string, commandArgs []string) (string, []string) {
	args := []string{
		"shell",
		"--reload",
		"--from",
		"path:" + managedDevenvPath,
		"--",
		"sh",
		"-c",
		"export HOME=/root; unset DEVENV_ROOT _DEVENV_HOOK_DIR; exec \"$@\"",
		"mezha-direct",
		command,
	}
	return nativeDevenvPath, append(args, commandArgs...)
}

// devenvBashCommand starts Bash with the shared direct-session environment.
func devenvBashCommand(bashArgs []string) (string, []string) {
	return devenvDirectCommand("bash", bashArgs)
}

func devenvInteractiveShellCommand() (string, []string) {
	return devenvBashCommand([]string{"-il"})
}

// ensureDevenvServices starts the singleton devenv process manager and waits
// until its requested services pass their configured readiness probes.
func ensureDevenvServices(ctx context.Context, sandbox *msb.Sandbox) error {
	hasProcesses, err := devenvHasEnabledProcesses(ctx, sandbox)
	if err != nil {
		return err
	}
	if !hasProcesses {
		return nil
	}
	args := []string{"up", "--detach", "--from", "path:" + managedDevenvPath}
	fmt.Println("Starting devenv services...")
	code, err := sandbox.AttachWith(
		ctx,
		nativeDevenvPath,
		args,
		msb.WithAttachCwd(managedDevenvPath),
	)
	if err != nil {
		return fmt.Errorf("start devenv services: %w", err)
	}
	if code != 0 {
		printDevenvDaemonLog(ctx, sandbox)
		return fmt.Errorf("start devenv services exited with code %d", code)
	}

	fmt.Println("Waiting for devenv services to become ready...")
	code, err = sandbox.AttachWith(ctx, nativeDevenvPath, []string{
		"processes", "wait", "--from", "path:" + managedDevenvPath, "--timeout", "120",
	}, msb.WithAttachCwd(managedDevenvPath))
	if err != nil {
		return fmt.Errorf("wait for devenv services: %w", err)
	}
	if code != 0 {
		printDevenvDaemonLog(ctx, sandbox)
		return fmt.Errorf("wait for devenv services exited with code %d", code)
	}
	return nil
}

func devenvHasEnabledProcesses(ctx context.Context, sandbox *msb.Sandbox) (bool, error) {
	output, err := sandbox.Exec(
		ctx,
		nativeDevenvPath,
		[]string{
			"eval",
			"--quiet",
			"--no-tui",
			"--from",
			"path:" + managedDevenvPath,
			"--",
			"processes",
		},
		persistentRuntimeExecEnv(),
		msb.WithExecCwd(managedDevenvPath),
	)
	if err != nil {
		return false, fmt.Errorf("evaluate devenv processes: %w", err)
	}
	if !output.Success() {
		return false, fmt.Errorf(
			"evaluate devenv processes: %s",
			strings.TrimSpace(output.Stderr()),
		)
	}
	return parseDevenvHasEnabledProcesses(output.Stdout())
}

func parseDevenvHasEnabledProcesses(stdout string) (bool, error) {
	stdout = strings.TrimSpace(stdout)
	start := strings.Index(stdout, "{")
	end := strings.LastIndex(stdout, "}")
	if start < 0 || end <= start {
		return false, nil
	}
	var res struct {
		Processes map[string]struct {
			Start struct {
				Enable bool `json:"enable"`
			} `json:"start"`
		} `json:"processes"`
	}
	if err := json.Unmarshal([]byte(stdout[start:end+1]), &res); err != nil {
		return false, fmt.Errorf("parse devenv processes evaluation: %w", err)
	}
	for _, proc := range res.Processes {
		if proc.Start.Enable {
			return true, nil
		}
	}
	return false, nil
}

func printDevenvDaemonLog(ctx context.Context, sandbox *msb.Sandbox) {
	output, err := sandbox.Exec(ctx, "sh", []string{
		"-c",
		"for log in /tmp/devenv-*/processes/daemon.log; do [ -f \"$log\" ] || continue; echo \"--- $log ---\" >&2; tail -n 200 \"$log\" >&2; done",
	}, persistentRuntimeExecEnv())
	if err != nil {
		return
	}
	_, _ = fmt.Fprint(os.Stdout, output.Stdout())
	_, _ = fmt.Fprint(os.Stderr, output.Stderr())
}

// ensureManagedDevenvConfig installs the service configuration below root's config directory.
func ensureManagedDevenvConfig(
	ctx context.Context,
	sandbox *msb.Sandbox,
	repoRoot string,
) error {
	output, err := sandbox.Exec(
		ctx,
		persistentRuntimeBin+"/mkdir",
		[]string{"-p", managedDevenvPath},
		persistentRuntimeExecEnv(),
	)
	if err != nil {
		return fmt.Errorf("create managed devenv configuration directory: %w", err)
	}
	if !output.Success() {
		return fmt.Errorf(
			"create managed devenv configuration directory: %s",
			strings.TrimSpace(output.Stderr()),
		)
	}

	provisionDir := resolveProvisionDir(repoRoot)
	if info, err := os.Stat(provisionDir); err == nil && info.IsDir() {
		devenvFile := filepath.Join(provisionDir, "devenv.nix")
		if _, err := os.Stat(devenvFile); err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("managed devenv configuration is missing: %s", devenvFile)
			}
			return fmt.Errorf("inspect managed devenv configuration: %w", err)
		}
		if err := syncProvisionDir(ctx, sandbox, provisionDir, managedDevenvPath); err != nil {
			return fmt.Errorf("sync managed devenv provision configuration: %w", err)
		}
	} else {
		output, err := sandbox.Exec(
			ctx,
			persistentRuntimeBin+"/sh",
			[]string{"-c", "test -f \"$1\"", "mezha-test-file", managedDevenvConfig},
			persistentRuntimeExecEnv(),
		)
		if err != nil {
			return fmt.Errorf("inspect managed devenv configuration: %w", err)
		}
		if !output.Success() {
			return fmt.Errorf(
				"managed devenv configuration %s is missing; create %s or run mezha init",
				managedDevenvConfig,
				filepath.Join(repoRoot, ".mezha", "provision", "devenv.nix"),
			)
		}
	}

	return ensureDevenvBashHook(ctx, sandbox)
}

// syncProvisionDir uploads the provision directory excluding lock files.
func syncProvisionDir(ctx context.Context, sandbox *msb.Sandbox, local, remote string) error {
	info, err := os.Lstat(local)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("provision path is not a directory: %s", local)
	}
	return filepath.Walk(local, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !info.IsDir() && strings.HasSuffix(info.Name(), ".lock") {
			return nil
		}
		rel, err := filepath.Rel(local, path)
		if err != nil {
			return err
		}
		guestPath := filepath.ToSlash(filepath.Join(remote, rel))
		if info.IsDir() {
			return sandbox.FS().Mkdir(ctx, guestPath)
		}
		return sandbox.FS().CopyFromHost(ctx, path, guestPath)
	})
}

// ensureDevenvBashHook enables directory-based devenv activation for Bash.
func ensureDevenvBashHook(ctx context.Context, sandbox *msb.Sandbox) error {
	output, err := sandbox.Exec(ctx, persistentRuntimeBin+"/sh", []string{"-c", `set -eu
bashrc="/root/.bashrc"
touch "$bashrc"
sed -i '/^# mezha devenv hook$/,/^eval "$(devenv hook bash)"$/d' "$bashrc"
cat >> "$bashrc" <<'EOF'
# mezha devenv hook
if [ "${MEZHA_HERDR_PANE:-}" = 1 ]; then
  unset DEVENV_ROOT MEZHA_HERDR_PANE
fi
eval "$(devenv hook bash)"
EOF
`}, persistentRuntimeExecEnv())
	if err != nil {
		return fmt.Errorf("install devenv Bash hook: %w", err)
	}
	if !output.Success() {
		return fmt.Errorf("install devenv Bash hook: %s", strings.TrimSpace(output.Stderr()))
	}
	return nil
}

// ensureDevenvBashProfile makes direct login shells enter the project and load its hook.
func ensureDevenvBashProfile(ctx context.Context, sandbox *msb.Sandbox, workdir string) error {
	output, err := sandbox.Exec(ctx, persistentRuntimeBin+"/sh", []string{"-c", `set -eu
profile="/root/.bash_profile"
touch "$profile"
sed -i '/^# mezha project shell$/,/^# mezha project shell end$/d' "$profile"
cat >> "$profile" <<EOF
# mezha project shell
sed -i '/^# mezha project shell$/,/^# mezha project shell end$/d' "$profile"
cd "$1"
if [ -f "/root/.bashrc" ]; then
  . "/root/.bashrc"
fi
# mezha project shell end
EOF
`, "mezha-bash-profile", workdir}, persistentRuntimeExecEnv())
	if err != nil {
		return fmt.Errorf("install devenv Bash profile: %w", err)
	}
	if !output.Success() {
		return fmt.Errorf("install devenv Bash profile: %s", strings.TrimSpace(output.Stderr()))
	}
	return nil
}
