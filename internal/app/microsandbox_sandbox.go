//go:build cgo

package app

import (
	"context"
	"fmt"
	"strings"

	msb "github.com/superradcompany/microsandbox/sdk/go"
)

func listMicrosandboxes(ctx context.Context) error {
	if err := msb.EnsureInstalled(ctx); err != nil {
		return fmt.Errorf("install Microsandbox runtime: %w", err)
	}

	fmt.Println("NAME\tSTATUS\tBACKEND")
	var cursor string
	for {
		options := []msb.SandboxListOption{}
		if cursor != "" {
			options = append(options, msb.WithListCursor(cursor))
		}
		page, err := msb.ListSandboxesWith(ctx, options...)
		if err != nil {
			return fmt.Errorf("list Microsandboxes: %w", err)
		}
		for _, sandbox := range page.Sandboxes {
			fmt.Printf("%s\t%s\t%s\n", sandbox.Name(), sandbox.Status(), sandbox.BackendKind())
		}
		if page.NextCursor == nil || strings.TrimSpace(*page.NextCursor) == "" {
			return nil
		}
		cursor = *page.NextCursor
	}
}
