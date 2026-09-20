package app

import "context"

// Start boots an existing Microsandbox without opening a session.
func Start(ctx context.Context, params LifecycleParams) error {
	return startMicrosandbox(ctx, params.SandboxName)
}

// Stop gracefully stops an existing Microsandbox without deleting it.
func Stop(ctx context.Context, params LifecycleParams) error {
	return stopMicrosandbox(ctx, params.SandboxName)
}
