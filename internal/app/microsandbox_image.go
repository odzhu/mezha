package app

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// imageReference returns the configured OCI image or the stable local tag
// assigned to a Dockerfile build.
func (s MicrosandboxSpec) imageReference() (string, error) {
	image := strings.TrimSpace(s.Image)
	dockerfile := strings.TrimSpace(s.Dockerfile)
	if image != "" && dockerfile != "" {
		return "", fmt.Errorf("microsandbox.image and microsandbox.dockerfile cannot both be set")
	}
	if image != "" {
		return image, nil
	}
	if dockerfile == "" {
		return "", fmt.Errorf("microsandbox.image or microsandbox.dockerfile is required")
	}

	// The tag includes the Dockerfile contents so changing a generated or
	// project Dockerfile causes `mezha run` to build and import a fresh image.
	contents, err := os.ReadFile(dockerfile)
	if err != nil {
		return "", fmt.Errorf("read microsandbox.dockerfile %q: %w", dockerfile, err)
	}
	sum := sha256.Sum256(append([]byte(filepath.Clean(dockerfile)+"\x00"), contents...))
	return fmt.Sprintf("mezha-%s-%x:latest", slugify(filepath.Base(filepath.Dir(dockerfile))), sum[:6]), nil
}
