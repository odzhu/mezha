package app

import (
	"context"
	"fmt"
)

// Provision creates and initializes a sandbox without synchronizing repository data.
func Provision(ctx context.Context, rc RepoContext, params ProvisionParams) error {
	cfg, _, err := LoadConfig(rc.RepoRoot)
	if err != nil {
		return fmt.Errorf("load mezha configuration: %w", err)
	}
	if cfg == nil || cfg.Microsandbox == nil {
		return fmt.Errorf("microsandbox configuration missing in mezha.toml")
	}
	return provisionMicrosandbox(ctx, rc, params, cfg)
}
