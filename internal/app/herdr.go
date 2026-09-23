package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/odzhu/mezha/internal/execx"
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
	if err := trustSandboxSSHHost(ctx, target); err != nil {
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

func trustSandboxSSHHost(ctx context.Context, target string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve home directory for sandbox SSH host key: %w", err)
	}
	knownHosts := filepath.Join(home, ".ssh", "known_hosts")

	// A sandbox with a reused name has a new SSH server identity. The alias is
	// local to Mezha's proxy, so replace its old identity before recording it.
	_, _, err = execx.Run(
		ctx,
		"ssh-keygen",
		[]string{"-R", target, "-f", knownHosts},
		execx.RunOptions{IgnoreExitCode: true},
	)
	if err != nil {
		return fmt.Errorf("remove previous sandbox SSH host key: %w", err)
	}

	_, stderr, err := execx.Run(ctx, "ssh", []string{
		"-o", "UserKnownHostsFile=" + knownHosts,
		"-o", "GlobalKnownHostsFile=/dev/null",
		"-o", "StrictHostKeyChecking=accept-new",
		target,
		"true",
	}, execx.RunOptions{CaptureStdout: true, CaptureStderr: true})
	if err != nil {
		if message := strings.TrimSpace(string(stderr)); message != "" {
			return fmt.Errorf("trust sandbox SSH host key: %w: %s", err, message)
		}
		return fmt.Errorf("trust sandbox SSH host key: %w", err)
	}
	return nil
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
