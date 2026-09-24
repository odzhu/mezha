package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/odzhu/mezha/internal/execx"
)

// microsandboxHomeDir returns the Microsandbox home directory.
func microsandboxHomeDir() (string, error) {
	if h := strings.TrimSpace(os.Getenv("MSB_HOME")); h != "" {
		return h, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home directory: %w", err)
	}
	return filepath.Join(home, ".microsandbox"), nil
}

// microsandboxAuthorizedKeysPath returns the path to the Microsandbox authorized_keys file.
func microsandboxAuthorizedKeysPath() (string, error) {
	msbHome, err := microsandboxHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(msbHome, "ssh", "authorized_keys"), nil
}

// ensureMicrosandboxSSHAuthorizedKeys idempotently ensures host SSH public keys are authorized in Microsandbox.
func ensureMicrosandboxSSHAuthorizedKeys(ctx context.Context) error {
	authKeysPath, err := microsandboxAuthorizedKeysPath()
	if err != nil {
		return err
	}
	candidateKeys, err := findHostSSHPublicKeys(ctx)
	if err != nil {
		return fmt.Errorf("find host SSH public keys: %w", err)
	}
	if len(candidateKeys) == 0 {
		return fmt.Errorf("no SSH public keys found")
	}
	return authorizeMicrosandboxSSHKeys(authKeysPath, candidateKeys)
}

// findHostSSHPublicKeys returns public SSH keys available on the host machine.
func findHostSSHPublicKeys(ctx context.Context) ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve user home directory: %w", err)
	}
	sshDir := filepath.Join(home, ".ssh")

	var candidates []string
	seen := make(map[string]bool)

	addKey := func(line string) {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			return
		}
		keyType, keyData, ok := sshKeyID(trimmed)
		if !ok {
			return
		}
		id := keyType + " " + keyData
		if !seen[id] {
			seen[id] = true
			candidates = append(candidates, trimmed)
		}
	}

	addFile := func(path string) {
		data, err := os.ReadFile(path)
		if err != nil {
			return
		}
		for _, line := range strings.Split(string(data), "\n") {
			addKey(line)
		}
	}

	standardKeys := []string{
		"id_ed25519.pub",
		"id_ecdsa.pub",
		"id_rsa.pub",
		"id_dsa.pub",
	}
	for _, name := range standardKeys {
		addFile(filepath.Join(sshDir, name))
	}

	if matches, err := filepath.Glob(filepath.Join(sshDir, "*.pub")); err == nil {
		for _, match := range matches {
			addFile(match)
		}
	}

	agentCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if stdout, _, err := execx.Run(
		agentCtx,
		"ssh-add",
		[]string{"-L"},
		execx.RunOptions{CaptureStdout: true},
	); err == nil {
		for _, line := range strings.Split(string(stdout), "\n") {
			addKey(line)
		}
	}

	if len(candidates) == 0 {
		defaultKey := filepath.Join(sshDir, "id_ed25519")
		if _, err := os.Stat(defaultKey); os.IsNotExist(err) {
			if err := os.MkdirAll(sshDir, 0o700); err != nil {
				return nil, fmt.Errorf("create SSH directory: %w", err)
			}
			genCtx, genCancel := context.WithTimeout(ctx, 10*time.Second)
			defer genCancel()
			_, _, err := execx.Run(
				genCtx,
				"ssh-keygen",
				[]string{"-t", "ed25519", "-N", "", "-f", defaultKey},
				execx.RunOptions{},
			)
			if err != nil {
				return nil, fmt.Errorf("generate default SSH key: %w", err)
			}
			addFile(defaultKey + ".pub")
		}
	}

	return candidates, nil
}

// authorizeMicrosandboxSSHKeys idempotently adds candidate public keys to the authorized_keys file.
func authorizeMicrosandboxSSHKeys(authKeysPath string, candidateKeys []string) error {
	existingBytes, err := os.ReadFile(authKeysPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read authorized keys: %w", err)
	}

	seen := make(map[string]bool)
	if len(existingBytes) > 0 {
		for _, line := range strings.Split(string(existingBytes), "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" || strings.HasPrefix(trimmed, "#") {
				continue
			}
			if keyType, keyData, ok := sshKeyID(trimmed); ok {
				seen[keyType+" "+keyData] = true
			}
		}
	}

	var toAdd []string
	for _, cand := range candidateKeys {
		trimmed := strings.TrimSpace(cand)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		keyType, keyData, ok := sshKeyID(trimmed)
		if !ok {
			continue
		}
		id := keyType + " " + keyData
		if !seen[id] {
			seen[id] = true
			toAdd = append(toAdd, trimmed)
		}
	}

	if len(toAdd) == 0 {
		return nil
	}

	sshDir := filepath.Dir(authKeysPath)
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		return fmt.Errorf("create Microsandbox SSH directory: %w", err)
	}

	var builder strings.Builder
	builder.Write(existingBytes)
	if len(existingBytes) > 0 && !strings.HasSuffix(string(existingBytes), "\n") {
		builder.WriteString("\n")
	}
	for _, key := range toAdd {
		builder.WriteString(key)
		builder.WriteString("\n")
	}

	if err := os.WriteFile(authKeysPath, []byte(builder.String()), 0o600); err != nil {
		return fmt.Errorf("write Microsandbox SSH authorized keys: %w", err)
	}
	_ = os.Chmod(authKeysPath, 0o600)
	return nil
}

// sshKeyID extracts the key type and key data from an SSH public key or authorized_keys line.
func sshKeyID(line string) (keyType, keyData string, ok bool) {
	fields := strings.Fields(line)
	for i := 0; i+1 < len(fields); i++ {
		field := fields[i]
		if strings.HasPrefix(field, "#") {
			return "", "", false
		}
		if isSSHKeyType(field) {
			return field, fields[i+1], true
		}
	}
	return "", "", false
}

// isSSHKeyType reports whether s is a recognized SSH public key algorithm prefix.
func isSSHKeyType(s string) bool {
	switch {
	case strings.HasPrefix(s, "ssh-"),
		strings.HasPrefix(s, "ecdsa-"),
		strings.HasPrefix(s, "sk-ssh-"),
		strings.HasPrefix(s, "sk-ecdsa-"):
		return true
	default:
		return false
	}
}
