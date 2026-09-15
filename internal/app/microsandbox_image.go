//go:build cgo

package app

import (
	"context"
	"fmt"
	"os"

	"github.com/odzhu/mezha/internal/execx"
	msb "github.com/superradcompany/microsandbox/sdk/go"
)

// ensureDevenvImage imports the fixed native devenv image through Docker.
// Docker archives normalize registry layer encodings that Microsandbox's
// direct image puller cannot materialize.
func ensureDevenvImage(ctx context.Context, workdir string) error {
	if _, err := msb.Image.Get(ctx, defaultDevenvImage); err == nil {
		return nil
	}
	fmt.Printf("Importing image %s into Microsandbox...\n", defaultDevenvImage)
	if err := execx.Stream(ctx, workdir, "docker", "pull", defaultDevenvImage); err != nil {
		return fmt.Errorf("pull image %q: %w", defaultDevenvImage, err)
	}

	archive, err := os.CreateTemp("", "mezha-image-*.tar")
	if err != nil {
		return fmt.Errorf("create Docker image archive: %w", err)
	}
	archivePath := archive.Name()
	if err := archive.Close(); err != nil {
		return fmt.Errorf("close Docker image archive: %w", err)
	}
	defer func() { _ = os.Remove(archivePath) }()
	if err := execx.Stream(
		ctx,
		workdir,
		"docker",
		"save",
		"--output",
		archivePath,
		defaultDevenvImage,
	); err != nil {
		return fmt.Errorf("export image %q: %w", defaultDevenvImage, err)
	}
	if _, err := msb.Image.Load(ctx, archivePath, defaultDevenvImage); err != nil {
		return fmt.Errorf("import image %q into Microsandbox: %w", defaultDevenvImage, err)
	}
	return nil
}
