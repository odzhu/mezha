package app

import (
	"crypto/ed25519"
	"crypto/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

func generateTestSSHKey(t *testing.T) ssh.PublicKey {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate ed25519 key: %v", err)
	}
	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatalf("convert to ssh.PublicKey: %v", err)
	}
	return sshPub
}

func TestUpdateKnownHosts_NewFile(t *testing.T) {
	tempDir := t.TempDir()
	knownHostsPath := filepath.Join(tempDir, "known_hosts")
	key := generateTestSSHKey(t)

	target := "mezha-sandbox-test"
	if err := updateKnownHosts(knownHostsPath, target, key); err != nil {
		t.Fatalf("updateKnownHosts failed: %v", err)
	}

	data, err := os.ReadFile(knownHostsPath)
	if err != nil {
		t.Fatalf("read known_hosts: %v", err)
	}
	expectedLine := knownhosts.Line([]string{target}, key) + "\n"
	if string(data) != expectedLine {
		t.Errorf("got %q, want %q", string(data), expectedLine)
	}
}

func TestUpdateKnownHosts_ReplacesExisting(t *testing.T) {
	tempDir := t.TempDir()
	knownHostsPath := filepath.Join(tempDir, "known_hosts")

	oldKey := generateTestSSHKey(t)
	newKey := generateTestSSHKey(t)
	otherKey := generateTestSSHKey(t)

	target := "mezha-sandbox-test"
	otherTarget := "other-host.example.com"

	initial := strings.Join([]string{
		"# Existing comments",
		knownhosts.Line([]string{otherTarget}, otherKey),
		knownhosts.Line([]string{target}, oldKey),
		"",
	}, "\n")
	if err := os.WriteFile(knownHostsPath, []byte(initial), 0o600); err != nil {
		t.Fatalf("write initial known_hosts: %v", err)
	}

	if err := updateKnownHosts(knownHostsPath, target, newKey); err != nil {
		t.Fatalf("updateKnownHosts failed: %v", err)
	}

	data, err := os.ReadFile(knownHostsPath)
	if err != nil {
		t.Fatalf("read known_hosts: %v", err)
	}
	content := string(data)

	if strings.Contains(content, knownhosts.Line([]string{target}, oldKey)) {
		t.Errorf("old key for %s was not removed", target)
	}
	if !strings.Contains(content, knownhosts.Line([]string{otherTarget}, otherKey)) {
		t.Errorf("other host entry was unexpectedly modified")
	}
	if !strings.Contains(content, knownhosts.Line([]string{target}, newKey)) {
		t.Errorf("new key for %s was not added", target)
	}
}

func TestUpdateKnownHosts_ReplacesHashed(t *testing.T) {
	tempDir := t.TempDir()
	knownHostsPath := filepath.Join(tempDir, "known_hosts")

	newKey := generateTestSSHKey(t)
	oldKey := generateTestSSHKey(t)

	target := "mezha-sandbox-hashed"
	hashedPattern := knownhosts.HashHostname(target)
	initial := strings.Join([]string{
		hashedPattern + " " + string(ssh.MarshalAuthorizedKey(oldKey)),
	}, "\n")
	if err := os.WriteFile(knownHostsPath, []byte(initial), 0o600); err != nil {
		t.Fatalf("write initial known_hosts: %v", err)
	}

	if err := updateKnownHosts(knownHostsPath, target, newKey); err != nil {
		t.Fatalf("updateKnownHosts failed: %v", err)
	}

	data, err := os.ReadFile(knownHostsPath)
	if err != nil {
		t.Fatalf("read known_hosts: %v", err)
	}
	content := string(data)

	if strings.Contains(content, hashedPattern) {
		t.Errorf("hashed pattern was not removed for %s", target)
	}
	if !strings.Contains(content, knownhosts.Line([]string{target}, newKey)) {
		t.Errorf("new key for %s was not added", target)
	}
}

func TestIsHostPatternMatch(t *testing.T) {
	target := "mezha-sandbox-abc"
	tests := []struct {
		pattern string
		target  string
		want    bool
	}{
		{pattern: "mezha-sandbox-abc", target: target, want: true},
		{pattern: "[mezha-sandbox-abc]:22", target: target, want: true},
		{pattern: "host1,mezha-sandbox-abc,host2", target: target, want: true},
		{pattern: "mezha-sandbox-def", target: target, want: false},
		{pattern: knownhosts.HashHostname(target), target: target, want: true},
		{pattern: knownhosts.HashHostname("other-host"), target: target, want: false},
	}

	for _, tt := range tests {
		got := isHostPatternMatch(tt.pattern, tt.target)
		if got != tt.want {
			t.Errorf(
				"isHostPatternMatch(%q, %q) = %v, want %v",
				tt.pattern,
				tt.target,
				got,
				tt.want,
			)
		}
	}
}
