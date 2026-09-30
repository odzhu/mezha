package app

import (
	"fmt"

	secretspec "github.com/cachix/secretspec/secretspec-go"
)

// loadSecretSpec resolves configured secrets and exports them for Mezha's process.
func loadSecretSpec(config SecretSpecConfig) (func(), error) {
	if !config.Enabled {
		return func() {}, nil
	}

	builder := secretspec.New().WithCaller(secretspec.CallerContext{
		Name:      "mezha",
		Operation: "sandbox",
	})
	if config.Path != "" {
		builder.WithPath(config.Path)
	}
	if config.Provider != "" {
		builder.WithProvider(config.Provider)
	}
	if config.Profile != "" {
		builder.WithProfile(config.Profile)
	}
	if config.Scope != "" {
		builder.WithScope(config.Scope)
	}
	if config.Reason != "" {
		builder.WithReason(config.Reason)
	}

	resolved, err := builder.Load()
	if err != nil {
		return nil, fmt.Errorf("resolve SecretSpec secrets: %w", err)
	}
	if err := resolved.SetAsEnv(); err != nil {
		_ = resolved.Close()
		return nil, fmt.Errorf("export SecretSpec secrets: %w", err)
	}
	return func() { _ = resolved.Close() }, nil
}
