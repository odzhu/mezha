//go:build cgo

package app

import (
	"context"
	"fmt"

	msb "github.com/superradcompany/microsandbox/sdk/go"
)

func destroyMicrosandbox(ctx context.Context, params DestroyParams) (bool, error) {
	handle, err := msb.GetSandbox(ctx, params.SandboxName)
	if msb.IsKind(err, msb.ErrSandboxNotFound) {
		fmt.Printf("Nothing to destroy: sandbox %q does not exist.\n", params.SandboxName)
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("find Microsandbox %q: %w", params.SandboxName, err)
	}
	if !params.Force {
		confirmed, err := confirmDestroy(params.SandboxName)
		if err != nil {
			return false, err
		}
		if !confirmed {
			fmt.Println("Aborted; nothing was deleted.")
			return false, nil
		}
	}
	if err := handle.Destroy(ctx, msb.WithDestroyForce()); err != nil {
		return false, fmt.Errorf("destroy Microsandbox %q: %w", params.SandboxName, err)
	}
	fmt.Printf("Destroyed Microsandbox: %s\n", params.SandboxName)
	return true, nil
}
