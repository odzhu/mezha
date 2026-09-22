package app

import (
	"context"
	"fmt"
)

func Logs(ctx context.Context, rc RepoContext, params LogsParams) error {
	if cfg, _, err := LoadConfig(rc.RepoRoot); err == nil && cfg != nil && cfg.Microsandbox != nil {
		return logsMicrosandbox(ctx, params)
	}
	return fmt.Errorf("microsandbox configuration missing in mezha.toml")
}
