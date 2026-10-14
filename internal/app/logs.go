package app

import (
	"context"
	"fmt"
)

func Logs(ctx context.Context, rc RepoContext, params LogsParams) error {
	if cfg, err := rc.EffectiveConfig(); err == nil && cfg != nil && cfg.Microsandbox != nil {
		return logsMicrosandbox(ctx, params)
	}
	return fmt.Errorf("microsandbox configuration missing in mezha.toml")
}
