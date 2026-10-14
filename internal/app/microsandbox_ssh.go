//go:build cgo

package app

import (
	"context"
	"fmt"

	msb "github.com/superradcompany/microsandbox/sdk/go"
)

// nativeMicrosandboxConfigured is intentionally environment-independent here:
// the SSH proxy is invoked by Git from a different working directory, so the
// native path is selected when the local Microsandbox runtime has this name.
func nativeMicrosandboxConfigured() bool { return true }

func microsandboxSSHProxy(ctx context.Context, name string) error {
	if err := ensureMicrosandboxSSHAuthorizedKeys(ctx); err != nil {
		return fmt.Errorf("ensure Microsandbox SSH authorized keys: %w", err)
	}
	handle, err := msb.GetSandbox(ctx, name)
	if err != nil {
		return fmt.Errorf("find Microsandbox %q: %w", name, err)
	}
	sandbox, err := handle.Connect(ctx)
	if err != nil {
		return fmt.Errorf("connect to Microsandbox %q: %w", name, err)
	}
	defer func() { _ = sandbox.Detach(context.Background()) }()
	authKeysPath, err := microsandboxAuthorizedKeysPath()
	if err != nil {
		return fmt.Errorf("resolve Microsandbox SSH authorized keys path: %w", err)
	}
	server, err := sandbox.SSH().PrepareServer(ctx, msb.WithSSHAuthorizedKeysPath(authKeysPath))
	if err != nil {
		return fmt.Errorf("prepare Microsandbox SSH server: %w", err)
	}
	defer func() { _ = server.Close(context.Background()) }()
	return server.ServeConnection(ctx)
}
