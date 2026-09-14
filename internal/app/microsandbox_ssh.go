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
	handle, err := msb.GetSandbox(ctx, name)
	if err != nil {
		return fmt.Errorf("find Microsandbox %q: %w", name, err)
	}
	sandbox, err := handle.ConnectOrStart(ctx)
	if err != nil {
		return fmt.Errorf("connect to Microsandbox %q: %w", name, err)
	}
	defer sandbox.Detach(context.Background())
	server, err := sandbox.SSH().PrepareServer(ctx)
	if err != nil {
		return fmt.Errorf("prepare Microsandbox SSH server: %w", err)
	}
	defer server.Close(context.Background())
	return server.ServeConnection(ctx)
}
