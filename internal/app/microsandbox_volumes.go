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
func listMicrosandboxVolumes(ctx context.Context) error {
	if err := msb.EnsureInstalled(ctx); err != nil {
		return fmt.Errorf("install Microsandbox runtime: %w", err)
	}
	volumes, err := msb.ListVolumes(ctx)
	if err != nil {
		return fmt.Errorf("list Microsandbox volumes: %w", err)
	}
	users, err := microsandboxVolumeUsers(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("%-32s\t%s\t%s\t%s\n", "NAME", "KIND", "SIZE", "SANDBOXES")
	for _, volume := range volumes {
		size := "-"
		if capacity := volume.CapacityBytes(); capacity != nil {
			size = formatVolumeBytes(*capacity)
		} else if quota := volume.QuotaMiB(); quota != nil {
			size = fmt.Sprintf("%dMiB", *quota)
		}
		fmt.Printf(
			"%-32s\t%s\t%s\t%s\n",
			volume.Name(),
			volume.Kind(),
			size,
			strings.Join(users[volume.Name()], ","),
		)
	}
	return nil
}

func formatVolumeBytes(bytes uint64) string {
	const unit = uint64(1024)
	if bytes < unit {
		return fmt.Sprintf("%dB", bytes)
	}
	units := []string{"KiB", "MiB", "GiB", "TiB", "PiB"}
	value := float64(bytes)
	for _, suffix := range units {
		value /= float64(unit)
		if value < float64(unit) || suffix == units[len(units)-1] {
			return fmt.Sprintf("%.1f%s", value, suffix)
		}
	}
	return fmt.Sprintf("%dB", bytes)
}

func destroyMicrosandboxVolume(ctx context.Context, name string) error {
	if err := msb.EnsureInstalled(ctx); err != nil {
		return fmt.Errorf("install Microsandbox runtime: %w", err)
	}
	volume, err := msb.GetVolume(ctx, name)
	if msb.IsKind(err, msb.ErrVolumeNotFound) {
		return fmt.Errorf("microsandbox volume %q does not exist", name)
	}
	if err != nil {
		return fmt.Errorf("find Microsandbox volume %q: %w", name, err)
	}
	if err := volume.Remove(ctx); err != nil {
		return fmt.Errorf("remove Microsandbox volume %q: %w", name, err)
	}
	fmt.Printf("Removed Microsandbox volume: %s\n", name)
	return nil
}

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
