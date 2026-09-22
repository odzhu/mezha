//go:build cgo

package app

import (
	"context"
	"fmt"

	msb "github.com/superradcompany/microsandbox/sdk/go"
)

func startMicrosandbox(ctx context.Context, name string) error {
	if _, err := msb.EnsureRuntime(ctx, msb.RuntimeConfig{}, msb.InstallOptions{}); err != nil {
		return fmt.Errorf("install Microsandbox runtime: %w", err)
	}
	handle, err := msb.GetSandbox(ctx, name)
	if msb.IsKind(err, msb.ErrSandboxNotFound) {
		return fmt.Errorf(
			"microsandbox %q does not exist; create it with mezha sandbox create",
			name,
		)
	}
	if err != nil {
		return fmt.Errorf("find Microsandbox %q: %w", name, err)
	}
	sandbox, err := handle.StartDetached(ctx)
	if err != nil {
		return fmt.Errorf("start Microsandbox %q: %w", name, err)
	}
	if err := sandbox.Detach(context.Background()); err != nil {
		return fmt.Errorf("detach from Microsandbox %q: %w", name, err)
	}
	fmt.Printf("Started Microsandbox: %s\n", name)
	return nil
}

func stopMicrosandbox(ctx context.Context, name string) error {
	if _, err := msb.EnsureRuntime(ctx, msb.RuntimeConfig{}, msb.InstallOptions{}); err != nil {
		return fmt.Errorf("install Microsandbox runtime: %w", err)
	}
	handle, err := msb.GetSandbox(ctx, name)
	if msb.IsKind(err, msb.ErrSandboxNotFound) {
		return fmt.Errorf("microsandbox %q does not exist", name)
	}
	if err != nil {
		return fmt.Errorf("find Microsandbox %q: %w", name, err)
	}
	if err := handle.Stop(ctx); err != nil {
		return fmt.Errorf("stop Microsandbox %q: %w", name, err)
	}
	fmt.Printf("Stopped Microsandbox: %s\n", name)
	return nil
}
