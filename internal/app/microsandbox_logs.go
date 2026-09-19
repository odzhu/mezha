//go:build cgo

package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	msb "github.com/superradcompany/microsandbox/sdk/go"
)

func logsMicrosandbox(ctx context.Context, params LogsParams) error {
	if params.Since < 0 {
		return fmt.Errorf("--since must be greater than or equal to zero")
	}
	handle, err := msb.GetSandbox(ctx, params.SandboxName)
	if err != nil {
		return fmt.Errorf("get Microsandbox %q: %w", params.SandboxName, err)
	}
	sources, err := microsandboxLogSources(params.Sources)
	if err != nil {
		return err
	}
	since := time.Time{}
	if params.Since > 0 {
		since = time.Now().Add(-params.Since)
	}
	if params.Follow {
		stream, err := handle.LogStream(
			ctx,
			msb.LogStreamOptions{Sources: sources, Since: since, Follow: true},
		)
		if err != nil {
			return err
		}
		defer stream.Close()
		for {
			entry, err := stream.Recv(ctx)
			if ctx.Err() != nil {
				return nil
			}
			if err != nil {
				return err
			}
			if entry == nil {
				return nil
			}
			printMicrosandboxLog(*entry)
		}
	}
	entries, err := handle.Logs(
		ctx,
		msb.LogOptions{Tail: uint64(params.Tail), Since: since, Sources: sources},
	)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		fmt.Println("No logs found.")
		return nil
	}
	for _, entry := range entries {
		printMicrosandboxLog(entry)
	}
	return nil
}

func microsandboxLogSources(values []string) ([]msb.LogSource, error) {
	result := make([]msb.LogSource, 0, len(values))
	for _, value := range values {
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "", "sandbox":
		case "stdout":
			result = append(result, msb.LogSourceStdout)
		case "stderr":
			result = append(result, msb.LogSourceStderr)
		case "output":
			result = append(result, msb.LogSourceOutput)
		case "system", "runtime":
			result = append(result, msb.LogSourceSystem)
		default:
			return nil, fmt.Errorf("unknown Microsandbox log source %q", value)
		}
	}
	return result, nil
}
func printMicrosandboxLog(entry msb.LogEntry) {
	fmt.Printf(
		"%s %s: %s",
		entry.Timestamp.Local().Format(time.RFC3339),
		entry.Source,
		entry.Text(),
	)
	if !strings.HasSuffix(entry.Text(), "\n") {
		fmt.Println()
	}
}
