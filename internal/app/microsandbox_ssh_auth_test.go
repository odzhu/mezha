package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSSHKeyID(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		wantType string
		wantData string
		wantOK   bool
	}{
		{
			name:     "standard ed25519",
			line:     "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI user@host",
			wantType: "ssh-ed25519",
			wantData: "AAAAC3NzaC1lZDI1NTE5AAAAI",
			wantOK:   true,
		},
		{
			name:     "standard rsa without comment",
			line:     "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAAB",
			wantType: "ssh-rsa",
			wantData: "AAAAB3NzaC1yc2EAAAADAQABAAAB",
			wantOK:   true,
		},
		{
			name:     "ecdsa key",
			line:     "ecdsa-sha2-nistp256 AAAAE2VjZHNh... user@host",
			wantType: "ecdsa-sha2-nistp256",
			wantData: "AAAAE2VjZHNh...",
			wantOK:   true,
		},
		{
			name:     "sk-ssh-ed25519 key",
			line:     "sk-ssh-ed25519@openssh.com AAAAGnNrLXNzaC... user@host",
			wantType: "sk-ssh-ed25519@openssh.com",
			wantData: "AAAAGnNrLXNzaC...",
			wantOK:   true,
		},
		{
			name:     "key with authorized_keys options prefix",
			line:     `no-port-forwarding,no-agent-forwarding ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI admin`,
			wantType: "ssh-ed25519",
			wantData: "AAAAC3NzaC1lZDI1NTE5AAAAI",
			wantOK:   true,
		},
		{
			name:   "comment line",
			line:   "# ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI comment",
			wantOK: false,
		},
		{
			name:   "empty line",
			line:   "",
			wantOK: false,
		},
		{
			name:   "single token",
			line:   "ssh-ed25519",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotType, gotData, gotOK := sshKeyID(tt.line)
			if gotOK != tt.wantOK {
				t.Fatalf("sshKeyID(%q) ok = %v, want %v", tt.line, gotOK, tt.wantOK)
			}
			if gotOK {
				if gotType != tt.wantType || gotData != tt.wantData {
					t.Errorf(
						"sshKeyID(%q) = (%q, %q), want (%q, %q)",
						tt.line,
						gotType,
						gotData,
						tt.wantType,
						tt.wantData,
					)
				}
			}
		})
	}
}

func TestAuthorizeMicrosandboxSSHKeys_NewFile(t *testing.T) {
	dir := t.TempDir()
	authKeysPath := filepath.Join(dir, "ssh", "authorized_keys")

	keys := []string{
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI111 key1@host",
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI222 key2@host",
	}

	if err := authorizeMicrosandboxSSHKeys(authKeysPath, keys); err != nil {
		t.Fatalf("authorizeMicrosandboxSSHKeys error = %v", err)
	}

	data, err := os.ReadFile(authKeysPath)
	if err != nil {
		t.Fatalf("read authorized keys: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, keys[0]) || !strings.Contains(content, keys[1]) {
		t.Errorf("authorized_keys missing keys, got:\n%s", content)
	}

	info, err := os.Stat(authKeysPath)
	if err != nil {
		t.Fatalf("stat authorized_keys: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("expected file mode 0600, got %o", perm)
	}
}

func TestAuthorizeMicrosandboxSSHKeys_Idempotent(t *testing.T) {
	dir := t.TempDir()
	authKeysPath := filepath.Join(dir, "ssh", "authorized_keys")

	keys := []string{
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI111 key1@host",
	}

	if err := authorizeMicrosandboxSSHKeys(authKeysPath, keys); err != nil {
		t.Fatalf("first call: %v", err)
	}
	data1, err := os.ReadFile(authKeysPath)
	if err != nil {
		t.Fatalf("read first: %v", err)
	}

	// Calling a second time with the same key (even with different comment).
	keysWithDifferentComment := []string{
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI111 different-comment@host",
	}
	if err := authorizeMicrosandboxSSHKeys(authKeysPath, keysWithDifferentComment); err != nil {
		t.Fatalf("second call: %v", err)
	}
	data2, err := os.ReadFile(authKeysPath)
	if err != nil {
		t.Fatalf("read second: %v", err)
	}

	if string(data1) != string(data2) {
		t.Errorf(
			"idempotency failed; file changed between calls:\nfirst:\n%s\nsecond:\n%s",
			data1,
			data2,
		)
	}
}

func TestAuthorizeMicrosandboxSSHKeys_AppendMissing(t *testing.T) {
	dir := t.TempDir()
	authKeysPath := filepath.Join(dir, "ssh", "authorized_keys")

	initial := []string{
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI111 key1@host",
	}
	if err := authorizeMicrosandboxSSHKeys(authKeysPath, initial); err != nil {
		t.Fatalf("initial call: %v", err)
	}

	more := []string{
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI111 key1@host",
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI222 key2@host",
	}
	if err := authorizeMicrosandboxSSHKeys(authKeysPath, more); err != nil {
		t.Fatalf("append call: %v", err)
	}

	data, err := os.ReadFile(authKeysPath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected exactly 2 lines, got %d:\n%s", len(lines), string(data))
	}
	if lines[0] != initial[0] || lines[1] != more[1] {
		t.Errorf("unexpected content:\n%s", string(data))
	}
}

func TestAuthorizeMicrosandboxSSHKeys_PreservesExistingWithoutNewline(t *testing.T) {
	dir := t.TempDir()
	authKeysPath := filepath.Join(dir, "ssh", "authorized_keys")
	if err := os.MkdirAll(filepath.Dir(authKeysPath), 0o700); err != nil {
		t.Fatal(err)
	}
	existing := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI000 existing@host"
	if err := os.WriteFile(authKeysPath, []byte(existing), 0o600); err != nil {
		t.Fatal(err)
	}

	newKey := []string{"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI111 new@host"}
	if err := authorizeMicrosandboxSSHKeys(authKeysPath, newKey); err != nil {
		t.Fatalf("authorize error: %v", err)
	}

	data, err := os.ReadFile(authKeysPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d:\n%s", len(lines), string(data))
	}
	if lines[0] != existing || lines[1] != newKey[0] {
		t.Errorf("unexpected content:\n%s", string(data))
	}
}

func TestMicrosandboxHomeDir(t *testing.T) {
	t.Run("with MSB_HOME", func(t *testing.T) {
		t.Setenv("MSB_HOME", "/custom/msb")
		got, err := microsandboxHomeDir()
		if err != nil {
			t.Fatalf("error = %v", err)
		}
		if got != "/custom/msb" {
			t.Errorf("got %q, want %q", got, "/custom/msb")
		}
	})

	t.Run("default", func(t *testing.T) {
		t.Setenv("MSB_HOME", "")
		home, err := os.UserHomeDir()
		if err != nil {
			t.Fatal(err)
		}
		got, err := microsandboxHomeDir()
		if err != nil {
			t.Fatalf("error = %v", err)
		}
		want := filepath.Join(home, ".microsandbox")
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
}
