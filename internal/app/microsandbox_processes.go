//go:build cgo

package app

import (
	"context"
	"fmt"
	"os"

	msb "github.com/superradcompany/microsandbox/sdk/go"
)

func processesMicrosandbox(ctx context.Context, params ProcessesParams) error {
	if _, err := msb.EnsureRuntime(ctx, msb.RuntimeConfig{}, msb.InstallOptions{}); err != nil {
		return fmt.Errorf("install Microsandbox runtime: %w", err)
	}
	handle, err := msb.GetSandbox(ctx, params.SandboxName)
	if msb.IsKind(err, msb.ErrSandboxNotFound) {
		return fmt.Errorf(
			"microsandbox %q does not exist; create it with mezha sandbox create",
			params.SandboxName,
		)
	}
	if err != nil {
		return fmt.Errorf("find Microsandbox %q: %w", params.SandboxName, err)
	}
	sandbox, err := handle.ConnectOrStart(ctx)
	if err != nil {
		return fmt.Errorf("connect to Microsandbox %q: %w", params.SandboxName, err)
	}
	defer func() { _ = sandbox.Detach(context.Background()) }()

	if err := ensurePersistentLinks(ctx, sandbox); err != nil {
		return err
	}

	args := params.Args
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}

	script := fmt.Sprintf(
		`if [ -d "%s" ]; then PATH="%s:$PATH"; export PATH; fi; if [ -d "%s" ]; then PATH="%s:$PATH"; export PATH; fi; export SSL_CERT_FILE=/etc/ssl/certs/ca-certificates.crt NIX_SSL_CERT_FILE=/etc/ssl/certs/ca-certificates.crt; cd %s && devenv processes "$@"`,
		managedDevenvProfileBin,
		managedDevenvProfileBin,
		persistentRuntimeBin,
		persistentRuntimeBin,
		managedDevenvPath,
	)
	cmdArgs := append([]string{"-c", script, "mezha-processes"}, args...)

	term := os.Getenv("TERM")
	if term == "" {
		term = "xterm-256color"
	}

	if interactiveTTYEnabled(nil) {
		code, err := sandbox.AttachWith(
			ctx,
			"sh",
			cmdArgs,
			sandboxAttachOptions(managedDevenvPath)...,
		)
		if err != nil {
			return fmt.Errorf("run devenv processes: %w", err)
		}
		if code != 0 {
			return fmt.Errorf("devenv processes exited with code %d", code)
		}
		return nil
	}

	output, err := sandbox.Exec(
		ctx,
		"sh",
		cmdArgs,
		msb.WithExecCwd(managedDevenvPath),
		msb.WithExecEnv(map[string]string{
			"TERM":              term,
			"PATH":              managedDevenvProfileBin + ":" + persistentRuntimeBin + ":/nix/var/nix/profiles/default/bin:/usr/local/bin:/usr/bin:/bin",
			"SSL_CERT_FILE":     "/etc/ssl/certs/ca-certificates.crt",
			"NIX_SSL_CERT_FILE": "/etc/ssl/certs/ca-certificates.crt",
		}),
	)
	if err != nil {
		return fmt.Errorf("run devenv processes: %w", err)
	}
	fmt.Print(output.Stdout())
	fmt.Fprint(os.Stderr, output.Stderr())
	if !output.Success() {
		return fmt.Errorf("devenv processes exited with code %d", output.ExitCode())
	}
	return nil
}
