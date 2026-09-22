//go:build cgo

package app

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	msb "github.com/superradcompany/microsandbox/sdk/go"
)

func listMicrosandboxes(ctx context.Context) error {
	sandboxes, err := listMicrosandboxSandboxHandles(ctx)
	if err != nil {
		return err
	}

	fmt.Println("NAME\tSTATUS\tBACKEND\tVOLUMES")
	for _, sandbox := range sandboxes {
		volumes, err := sandboxVolumeNames(sandbox)
		if err != nil {
			return err
		}
		fmt.Printf(
			"%s\t%s\t%s\t%s\n",
			sandbox.Name(),
			sandbox.Status(),
			sandbox.BackendKind(),
			strings.Join(volumes, ","),
		)
	}
	return nil
}

func listMicrosandboxSandboxHandles(ctx context.Context) ([]*msb.SandboxHandle, error) {
	if err := msb.EnsureInstalled(ctx); err != nil {
		return nil, fmt.Errorf("install Microsandbox runtime: %w", err)
	}

	var sandboxes []*msb.SandboxHandle
	var cursor string
	for {
		options := []msb.SandboxListOption{}
		if cursor != "" {
			options = append(options, msb.WithListCursor(cursor))
		}
		page, err := msb.ListSandboxesWith(ctx, options...)
		if err != nil {
			return nil, fmt.Errorf("list Microsandboxes: %w", err)
		}
		sandboxes = append(sandboxes, page.Sandboxes...)
		if page.NextCursor == nil || strings.TrimSpace(*page.NextCursor) == "" {
			return sandboxes, nil
		}
		cursor = *page.NextCursor
	}
}

func sandboxVolumeNames(sandbox *msb.SandboxHandle) ([]string, error) {
	var manifest struct {
		Mounts []struct {
			Type string `json:"type"`
			Name string `json:"name"`
		} `json:"mounts"`
	}
	if err := json.Unmarshal([]byte(sandbox.ConfigJSON()), &manifest); err != nil {
		return nil, fmt.Errorf("read Microsandbox %q configuration: %w", sandbox.Name(), err)
	}
	volumes := make([]string, 0, len(manifest.Mounts))
	for _, mount := range manifest.Mounts {
		if strings.EqualFold(mount.Type, "named") && mount.Name != "" {
			volumes = append(volumes, mount.Name)
		}
	}
	sort.Strings(volumes)
	return volumes, nil
}

func microsandboxVolumeUsers(ctx context.Context) (map[string][]string, error) {
	sandboxes, err := listMicrosandboxSandboxHandles(ctx)
	if err != nil {
		return nil, err
	}
	users := make(map[string][]string)
	for _, sandbox := range sandboxes {
		volumes, err := sandboxVolumeNames(sandbox)
		if err != nil {
			return nil, err
		}
		for _, volume := range volumes {
			users[volume] = append(users[volume], sandbox.Name())
		}
	}
	for _, names := range users {
		sort.Strings(names)
	}
	return users, nil
}
