package app

import "context"

// ListSandboxes prints all locally known Microsandboxes.
func ListSandboxes(ctx context.Context) error {
	return listMicrosandboxes(ctx)
}
