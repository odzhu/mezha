package app

import (
	"context"
	"fmt"
)

// ListVolumes prints all local Microsandbox volumes.
func ListVolumes(ctx context.Context) error {
	return listMicrosandboxVolumes(ctx)
}

// DestroyVolume removes a named Microsandbox volume.
func DestroyVolume(ctx context.Context, name string, force bool) error {
	if !force {
		confirmed, err := confirmVolumeDestroy(name)
		if err != nil {
			return err
		}
		if !confirmed {
			fmt.Println("Aborted; nothing was deleted.")
			return nil
		}
	}
	return destroyMicrosandboxVolume(ctx, name)
}
