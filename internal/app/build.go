package app

import (
	"context"
	"fmt"
)

type BuildParams struct {
	Recreate bool
}

// Build builds and imports the image configured for the project's Microsandbox.
func Build(ctx context.Context, rc RepoContext, params BuildParams) error {
	cfg, _, err := LoadConfig(rc.RepoRoot)
	if err != nil {
		return fmt.Errorf("load mezha configuration: %w", err)
	}
	if cfg == nil || cfg.Microsandbox == nil {
		return fmt.Errorf("microsandbox configuration missing in mezha.yaml")
	}
	return buildMicrosandbox(ctx, rc, params, cfg)
}
