//go:build cgo

package app

import (
	"context"
	"fmt"
	"time"

	msb "github.com/superradcompany/microsandbox/sdk/go"
)

func pullDevenvImage(ctx context.Context) error {
	if _, err := msb.EnsureRuntime(ctx, msb.RuntimeConfig{}, msb.InstallOptions{}); err != nil {
		return fmt.Errorf("install Microsandbox runtime: %w", err)
	}

	name := fmt.Sprintf("mezha-image-pull-%d", time.Now().UnixNano())
	fmt.Printf("Pulling Microsandbox image: %s...\n", defaultDevenvImage)
	sandbox, err := msb.CreateSandbox(
		ctx,
		name,
		msb.WithImage(defaultDevenvImage),
		msb.WithDetached(),
		msb.WithPullPolicy(msb.PullPolicyAlways),
		msb.WithUser("0"),
	)
	if err != nil {
		return fmt.Errorf("pull image %q: %w", defaultDevenvImage, err)
	}
	if err := sandbox.Destroy(ctx, msb.WithDestroyForce()); err != nil {
		return fmt.Errorf("remove image pull sandbox: %w", err)
	}
	fmt.Printf("Pulled Microsandbox image: %s\n", defaultDevenvImage)
	return nil
}
