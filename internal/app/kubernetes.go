package app

import "context"

func kubernetesSandboxName(sandboxName string) string { return sandboxName + "-k8s" }
func ensureKubernetesSandbox(ctx context.Context, sandboxName string) ([]byte, error) {
	return nil, nil
}
func microsandboxExists(ctx context.Context, sandboxName string) bool       { return false }
func removeKubernetesSandbox(ctx context.Context, sandboxName string) error { return nil }
