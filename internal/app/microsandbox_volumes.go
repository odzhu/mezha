//go:build cgo

package app

import (
	"context"
	"fmt"
	"strings"

	msb "github.com/superradcompany/microsandbox/sdk/go"
)

// flushMicrosandboxVolumes removes volumes managed by a sandbox, including
// Nix, Docker, Kubernetes, and cache volumes retained across recreation.
func flushMicrosandboxVolumes(ctx context.Context, sandboxName string) error {
	volumes, err := msb.ListVolumes(ctx)
	if err != nil {
		return fmt.Errorf("list Microsandbox volumes: %w", err)
	}
	prefix := sandboxName + "-"
	for _, volume := range volumes {
		if !strings.HasPrefix(volume.Name(), prefix) {
			continue
		}
		if err := msb.RemoveVolume(ctx, volume.Name()); err != nil {
			return fmt.Errorf("remove Microsandbox volume %q: %w", volume.Name(), err)
		}
		fmt.Printf("Removed Microsandbox volume: %s\n", volume.Name())
	}
	return nil
}
