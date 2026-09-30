package app

import (
	"context"
	"fmt"
	"strings"
)

func Processes(ctx context.Context, rc RepoContext, params ProcessesParams) error {
	if cfg, err := rc.EffectiveConfig(); err == nil && cfg != nil && cfg.Microsandbox != nil {
		return processesMicrosandbox(ctx, params)
	}
	return fmt.Errorf("microsandbox configuration missing in mezha.toml")
}

// extractSandboxFlag removes --sandbox or -s flags from the argument list.
func extractSandboxFlag(args []string, defaultVal string) (string, []string) {
	sandbox := defaultVal
	filtered := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			filtered = append(filtered, args[i+1:]...)
			break
		}
		if arg == "--sandbox" || arg == "-s" {
			if i+1 < len(args) {
				sandbox = args[i+1]
				i++
				continue
			}
		} else if strings.HasPrefix(arg, "--sandbox=") {
			sandbox = strings.TrimPrefix(arg, "--sandbox=")
			continue
		} else if strings.HasPrefix(arg, "-s=") {
			sandbox = strings.TrimPrefix(arg, "-s=")
			continue
		}
		filtered = append(filtered, arg)
	}
	return sandbox, filtered
}
