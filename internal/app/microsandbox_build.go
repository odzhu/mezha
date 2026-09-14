//go:build cgo

package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/odzhu/mezha/internal/execx"
	msb "github.com/superradcompany/microsandbox/sdk/go"
)

// buildMicrosandbox builds the Dockerfile configured for a native
// Microsandbox project. Docker receives the repository root as its context so
// Dockerfile COPY and ADD instructions work as they do from the project root.
func buildMicrosandbox(ctx context.Context, rc RepoContext, params BuildParams, cfg *MezhaConfig) error {
	if cfg == nil || cfg.Microsandbox == nil {
		return fmt.Errorf("microsandbox configuration missing in mezha.yaml")
	}

	dockerfile := cfg.Microsandbox.Dockerfile
	if dockerfile == "" {
		if cfg.Microsandbox.Image != "" {
			return fmt.Errorf("microsandbox.dockerfile is required to build an image; microsandbox.image references an existing image")
		}
		return fmt.Errorf("microsandbox.dockerfile is required")
	}
	info, err := os.Stat(dockerfile)
	if err != nil {
		return fmt.Errorf("read microsandbox.dockerfile %q: %w", dockerfile, err)
	}
	if info.IsDir() {
		return fmt.Errorf("microsandbox.dockerfile %q is a directory", dockerfile)
	}
	image, err := cfg.Microsandbox.imageReference()
	if err != nil {
		return err
	}

	// --load is required when Docker's buildx docker-container driver is active;
	// without it the tagged result remains only in BuildKit's cache and cannot
	// be exported with docker save for Microsandbox.
	args := []string{"build", "--load", "--file", filepath.Clean(dockerfile), "--tag", image}
	if params.Recreate {
		args = append(args, "--no-cache")
	}
	args = append(args, rc.RepoRoot)

	fmt.Printf("Building image %s from %s...\n", image, dockerfile)
	if err := execx.Stream(ctx, rc.RepoRoot, "docker", args...); err != nil {
		return fmt.Errorf("build Microsandbox image: %w", err)
	}
	archive, err := os.CreateTemp("", "mezha-image-*.tar")
	if err != nil {
		return fmt.Errorf("create Docker image archive: %w", err)
	}
	archivePath := archive.Name()
	if err := archive.Close(); err != nil {
		return fmt.Errorf("close Docker image archive: %w", err)
	}
	defer os.Remove(archivePath)
	if err := execx.Stream(ctx, rc.RepoRoot, "docker", "save", "--output", archivePath, image); err != nil {
		return fmt.Errorf("export Docker image %q: %w", image, err)
	}
	if err := msb.EnsureInstalled(ctx); err != nil {
		return fmt.Errorf("install Microsandbox runtime: %w", err)
	}
	if _, err := msb.Image.Load(ctx, archivePath, image); err != nil {
		return fmt.Errorf("import image %q into Microsandbox: %w", image, err)
	}

	fmt.Printf("Built image: %s\n", image)
	return nil
}
