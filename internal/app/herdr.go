package app

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/odzhu/mezha/internal/execx"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

func herdrCommandAvailable() bool {
	_, err := exec.LookPath("herdr")
	return err == nil
}

// clearHerdrSSHControlSockets prevents a recreated sandbox from reusing a
// ControlMaster connection that still targets the replaced sandbox.
func clearHerdrSSHControlSockets() error {
	dir := filepath.Join("/tmp", fmt.Sprintf("hssh-%d", os.Getuid()))
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read Herdr SSH control socket directory: %w", err)
	}
	for _, entry := range entries {
		if entry.Type()&os.ModeSocket == 0 {
			continue
		}
		if err := os.Remove(filepath.Join(dir, entry.Name())); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove Herdr SSH control socket: %w", err)
		}
	}
	return nil
}

type herdrMachine struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Target string `json:"target"`
}

// registerHerdrMachine saves the sandbox SSH endpoint when Herdr is installed.
func registerHerdrMachine(ctx context.Context, sandboxName string) error {
	if !herdrCommandAvailable() {
		return nil
	}
	target, err := ensureSandboxSSHConfig("", "", sandboxName)
	if err != nil {
		return fmt.Errorf("configure sandbox SSH for Herdr: %w", err)
	}
	if err := trustSandboxSSHHost(ctx, target, sandboxName); err != nil {
		return err
	}
	machines, err := listHerdrMachines(ctx)
	if err != nil {
		return err
	}
	for _, machine := range machines {
		if machine.Target == target && machine.Label == sandboxName {
			return nil
		}
	}
	if err := execx.Stream(
		ctx,
		"",
		"herdr",
		"machine",
		"add",
		target,
		"--label",
		sandboxName,
	); err != nil {
		return fmt.Errorf("register sandbox with Herdr: %w", err)
	}
	return nil
}

// unregisterHerdrMachine removes every saved profile for the sandbox endpoint.
func unregisterHerdrMachine(ctx context.Context, sandboxName string) (bool, error) {
	if !herdrCommandAvailable() {
		return false, nil
	}
	target := sandboxSSHHostAlias(sandboxName)
	machines, err := listHerdrMachines(ctx)
	if err != nil {
		return false, err
	}
	removed := false
	for _, machine := range machines {
		if machine.Target != target || machine.Label != sandboxName {
			continue
		}
		if err := execx.Stream(ctx, "", "herdr", "machine", "remove", machine.ID); err != nil {
			return false, fmt.Errorf("deregister sandbox from Herdr: %w", err)
		}
		removed = true
	}
	return removed, nil
}

type stdioConn struct {
	io.Reader
	io.WriteCloser
}

func (c *stdioConn) Close() error                     { return c.WriteCloser.Close() }
func (c *stdioConn) LocalAddr() net.Addr              { return &net.IPAddr{} }
func (c *stdioConn) RemoteAddr() net.Addr             { return &net.IPAddr{} }
func (c *stdioConn) SetDeadline(time.Time) error      { return nil }
func (c *stdioConn) SetReadDeadline(time.Time) error  { return nil }
func (c *stdioConn) SetWriteDeadline(time.Time) error { return nil }

func trustSandboxSSHHost(ctx context.Context, target, sandboxName string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve home directory for sandbox SSH host key: %w", err)
	}
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		return fmt.Errorf("create SSH directory: %w", err)
	}
	knownHosts := filepath.Join(sshDir, "known_hosts")

	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve mezha executable: %w", err)
	}

	cmd := exec.CommandContext(ctx, executable, "ssh-proxy", "--sandbox", sandboxName)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("create proxy stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("create proxy stdout pipe: %w", err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start proxy command: %w", err)
	}
	defer func() {
		_ = stdin.Close()
		_ = cmd.Wait()
	}()

	var hostKey ssh.PublicKey
	clientConfig := &ssh.ClientConfig{
		User: "root",
		HostKeyCallback: func(_ string, _ net.Addr, key ssh.PublicKey) error {
			hostKey = key
			return nil
		},
		Timeout: 10 * time.Second,
	}

	conn, _, reqs, _ := ssh.NewClientConn(
		&stdioConn{Reader: stdout, WriteCloser: stdin},
		target,
		clientConfig,
	)
	if conn != nil {
		_ = conn.Close()
	}
	if reqs != nil {
		go ssh.DiscardRequests(reqs)
	}

	if hostKey == nil {
		_ = stdin.Close()
		_ = cmd.Wait()
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return fmt.Errorf("fetch sandbox SSH host key: %s", msg)
		}
		return fmt.Errorf("fetch sandbox SSH host key: handshake failed")
	}

	if err := updateKnownHosts(knownHosts, target, hostKey); err != nil {
		return fmt.Errorf("trust sandbox SSH host key: %w", err)
	}
	return nil
}

func updateKnownHosts(path, target string, key ssh.PublicKey) error {
	var remaining []string
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read known_hosts: %w", err)
	}
	if len(data) > 0 {
		for _, line := range strings.Split(string(data), "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" || strings.HasPrefix(trimmed, "#") {
				if trimmed != "" {
					remaining = append(remaining, line)
				}
				continue
			}
			fields := strings.Fields(trimmed)
			if len(fields) < 2 {
				remaining = append(remaining, line)
				continue
			}
			if isHostPatternMatch(fields[0], target) {
				continue
			}
			remaining = append(remaining, line)
		}
	}
	remaining = append(remaining, knownhosts.Line([]string{target}, key))
	output := strings.Join(remaining, "\n") + "\n"
	return os.WriteFile(path, []byte(output), 0o600)
}

func isHostPatternMatch(pattern, target string) bool {
	if strings.HasPrefix(pattern, "|1|") {
		return matchHashedHost(pattern, target)
	}
	for _, h := range strings.Split(pattern, ",") {
		h = strings.TrimSpace(h)
		if h == target || h == "["+target+"]" || strings.HasPrefix(h, "["+target+"]:") {
			return true
		}
	}
	return false
}

func matchHashedHost(pattern, hostname string) bool {
	parts := strings.Split(pattern, "|")
	if len(parts) != 4 || parts[1] != "1" {
		return false
	}
	salt, err := base64.StdEncoding.DecodeString(parts[2])
	if err != nil {
		return false
	}
	expectedHash, err := base64.StdEncoding.DecodeString(parts[3])
	if err != nil {
		return false
	}
	mac := hmac.New(sha1.New, salt)
	mac.Write([]byte(hostname))
	return hmac.Equal(mac.Sum(nil), expectedHash)
}

func listHerdrMachines(ctx context.Context) ([]herdrMachine, error) {
	output, err := execx.Output(ctx, "herdr", "machine", "list", "--json")
	if err != nil {
		return nil, fmt.Errorf("list Herdr machines: %w", err)
	}
	var machines []herdrMachine
	if err := json.Unmarshal(output, &machines); err != nil {
		return nil, fmt.Errorf("parse Herdr machine list: %w", err)
	}
	return machines, nil
}
