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
const managedDevenvProfileBin = managedDevenvPath + "/.devenv/profile/bin"
const managedDevenvUserExtensions = managedDevenvPath + "/extensions-user"
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
		"export HOME=/root SSL_CERT_FILE=/etc/ssl/certs/ca-certificates.crt NIX_SSL_CERT_FILE=/etc/ssl/certs/ca-certificates.crt CURL_CA_BUNDLE=/etc/ssl/certs/ca-certificates.crt REQUESTS_CA_BUNDLE=/etc/ssl/certs/ca-certificates.crt; unset DEVENV_ROOT _DEVENV_HOOK_DIR; exec \"$@\"",
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
func ensureDevenvServices(ctx context.Context, sandbox *msb.Sandbox, herdrEnabled bool) error {
	hasProcesses, err := devenvHasEnabledProcesses(ctx, sandbox, herdrEnabled)
	if err != nil {
		return err
	}
	if !hasProcesses {
		return nil
	}
	args := []string{"up", "--detach", "--from", "path:" + managedDevenvPath}
	if herdrEnabled {
		args = append(args, "--option", "processes.herdr.start.enable:bool", "true")
	}
	attachEnv := msb.WithAttachEnv(map[string]string{
		"SSL_CERT_FILE":      "/etc/ssl/certs/ca-certificates.crt",
		"NIX_SSL_CERT_FILE":  "/etc/ssl/certs/ca-certificates.crt",
		"CURL_CA_BUNDLE":     "/etc/ssl/certs/ca-certificates.crt",
		"REQUESTS_CA_BUNDLE": "/etc/ssl/certs/ca-certificates.crt",
	})
	fmt.Println("Starting devenv services...")
	code, err := sandbox.AttachWith(
		ctx,
		nativeDevenvPath,
		args,
		attachEnv,
		msb.WithAttachCwd(managedDevenvPath),
	)
	if err != nil {
		execEnv := map[string]string{
			"SSL_CERT_FILE":      "/etc/ssl/certs/ca-certificates.crt",
			"NIX_SSL_CERT_FILE":  "/etc/ssl/certs/ca-certificates.crt",
			"CURL_CA_BUNDLE":     "/etc/ssl/certs/ca-certificates.crt",
			"REQUESTS_CA_BUNDLE": "/etc/ssl/certs/ca-certificates.crt",
		}
		if streamErr := execStreaming(
			ctx,
			sandbox,
			nativeDevenvPath,
			args,
			msb.WithExecCwd(managedDevenvPath),
			msb.WithExecEnv(execEnv),
		); streamErr != nil {
			printDevenvDaemonLog(ctx, sandbox)
			return fmt.Errorf("start devenv services: %w", streamErr)
		}
	} else if code != 0 {
		printDevenvDaemonLog(ctx, sandbox)
		return fmt.Errorf("start devenv services exited with code %d", code)
	}

	fmt.Println("Waiting for devenv services to become ready...")
	waitArgs := []string{
		"processes",
		"wait",
		"--from",
		"path:" + managedDevenvPath,
		"--timeout",
		"120",
	}
	if herdrEnabled {
		waitArgs = append(waitArgs, "--option", "processes.herdr.start.enable:bool", "true")
	}
	code, err = sandbox.AttachWith(
		ctx,
		nativeDevenvPath,
		waitArgs,
		attachEnv,
		msb.WithAttachCwd(managedDevenvPath),
	)
	if err != nil {
		execEnv := map[string]string{
			"SSL_CERT_FILE":      "/etc/ssl/certs/ca-certificates.crt",
			"NIX_SSL_CERT_FILE":  "/etc/ssl/certs/ca-certificates.crt",
			"CURL_CA_BUNDLE":     "/etc/ssl/certs/ca-certificates.crt",
			"REQUESTS_CA_BUNDLE": "/etc/ssl/certs/ca-certificates.crt",
		}
		if streamErr := execStreaming(
			ctx,
			sandbox,
			nativeDevenvPath,
			waitArgs,
			msb.WithExecCwd(managedDevenvPath),
			msb.WithExecEnv(execEnv),
		); streamErr != nil {
			printDevenvDaemonLog(ctx, sandbox)
			return fmt.Errorf("wait for devenv services: %w", streamErr)
		}
	} else if code != 0 {
		printDevenvDaemonLog(ctx, sandbox)
		return fmt.Errorf("wait for devenv services exited with code %d", code)
	}
	return nil
}

func devenvHasEnabledProcesses(
	ctx context.Context,
	sandbox *msb.Sandbox,
	herdrEnabled bool,
) (bool, error) {
	if herdrEnabled {
		return true, nil
	}
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
		"for log in /tmp/devenv-*/processes/daemon.log /tmp/devenv-*/processes/logs/*; do [ -f \"$log\" ] && [ -s \"$log\" ] || continue; echo \"--- $log ---\" >&2; tail -n 200 \"$log\" >&2; done",
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
	if err := ensurePersistentLinks(ctx, sandbox); err != nil {
		return err
	}
	output, err := sandbox.Exec(
		ctx,
		persistentRuntimeBin+"/mkdir",
		[]string{"-p", managedDevenvPath, managedDevenvUserExtensions},
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

	if err := syncBaseProvisionFiles(ctx, sandbox); err != nil {
		return fmt.Errorf("sync base managed devenv configuration: %w", err)
	}

	extensionsDir := resolveExtensionsDir(repoRoot)
	if err := syncExtensionsDir(
		ctx,
		sandbox,
		extensionsDir,
		managedDevenvUserExtensions,
	); err != nil {
		return fmt.Errorf("sync managed user extensions: %w", err)
	}

	return nil
}

// syncBaseProvisionFiles writes embedded base devenv files to the sandbox.
func syncBaseProvisionFiles(ctx context.Context, sandbox *msb.Sandbox) error {
	files, err := defaultProvisionFiles()
	if err != nil {
		return err
	}
	parents := make(map[string]bool)
	for _, file := range files {
		parent := filepath.ToSlash(filepath.Dir(filepath.Join(managedDevenvPath, file.relPath)))
		parents[parent] = true
	}
	parentList := make([]string, 0, len(parents)+1)
	parentList = append(parentList, "-p")
	for p := range parents {
		parentList = append(parentList, p)
	}
	if len(parentList) > 1 {
		output, err := sandbox.Exec(
			ctx,
			persistentRuntimeBin+"/mkdir",
			parentList,
			persistentRuntimeExecEnv(),
		)
		if err != nil {
			return fmt.Errorf("create base provision directories: %w", err)
		}
		if !output.Success() {
			return fmt.Errorf(
				"create base provision directories: %s",
				strings.TrimSpace(output.Stderr()),
			)
		}
	}
	for _, file := range files {
		guestPath := filepath.ToSlash(filepath.Join(managedDevenvPath, file.relPath))
		if existing, err := sandbox.FS().
			ReadString(ctx, guestPath); err == nil &&
			existing == file.content {
			continue
		}
		if err := sandbox.FS().WriteString(ctx, guestPath, file.content); err != nil {
			return fmt.Errorf("write %s: %w", guestPath, err)
		}
	}
	return nil
}

// syncExtensionsDir uploads the user extensions directory excluding lock files.
func syncExtensionsDir(ctx context.Context, sandbox *msb.Sandbox, local, remote string) error {
	info, err := os.Lstat(local)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("extensions path is not a directory: %s", local)
	}
	output, err := sandbox.Exec(
		ctx,
		persistentRuntimeBin+"/sh",
		[]string{
			"-c",
			"mkdir -p \"$1\" && find \"$1\" -mindepth 1 ! -name \"*.lock\" -delete 2>/dev/null || true",
			"mezha-clean-dir",
			remote,
		},
		persistentRuntimeExecEnv(),
	)
	if err != nil {
		return fmt.Errorf("prepare remote extensions directory: %w", err)
	}
	if !output.Success() {
		return fmt.Errorf(
			"prepare remote extensions directory: %s",
			strings.TrimSpace(output.Stderr()),
		)
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
