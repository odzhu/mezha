package app

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	cli "github.com/urfave/cli/v3"
)

const (
	herdrPluginID      = "dev.mezha"
	herdrProjectDirEnv = "MEZHA_HERDR_PROJECT_DIR"
)

type herdrInvocationContext struct {
	WorkspaceCWD   string `json:"workspace_cwd"`
	FocusedPaneCWD string `json:"focused_pane_cwd"`
	Worktree       *struct {
		CheckoutPath string `json:"checkout_path"`
	} `json:"worktree"`
}

func newHerdrPluginCommand() *cli.Command {
	return &cli.Command{
		Name:   "herdr",
		Usage:  "Run Mezha as a Herdr plugin",
		Hidden: true,
		Commands: []*cli.Command{
			{
				Name:   "ui",
				Usage:  "Show the Mezha dashboard for the active Herdr workspace",
				Action: runHerdrDashboard,
			},
			{
				Name:            "execute",
				Usage:           "Execute a Mezha command for the active Herdr workspace",
				ArgsUsage:       "[--wait] [--] <command> [arguments...]",
				SkipFlagParsing: true,
				Action:          executeHerdrOperation,
			},
			{
				Name:      "dispatch",
				Usage:     "Open a Herdr pane for an interactive Mezha operation",
				ArgsUsage: "<dashboard|run|status|destroy>",
				Action:    dispatchHerdrOperation,
			},
		},
	}
}

func executeHerdrOperation(ctx context.Context, cmd *cli.Command) error {
	args := cmd.Args().Slice()
	wait := false
	if len(args) > 0 && args[0] == "--wait" {
		wait = true
		args = args[1:]
	}
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}
	if len(args) == 0 {
		return errors.New("execute requires a Mezha command")
	}
	command := strings.Join(args, " ")
	projectDir, err := herdrProjectDir()
	if err != nil {
		notifyHerdr(command+" failed", err.Error(), "request")
		return err
	}
	binary, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve Mezha executable: %w", err)
	}
	child := exec.CommandContext(ctx, binary, args...)
	child.Dir = projectDir
	child.Stdin = os.Stdin
	child.Stdout = os.Stdout
	child.Stderr = os.Stderr
	runErr := child.Run()
	if wait && terminalIsTerminal(int(os.Stdin.Fd())) {
		fmt.Print("\nPress Enter to close...")
		_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
	}
	if runErr != nil {
		notifyHerdr(command+" failed", runErr.Error(), "request")
		return fmt.Errorf("run Mezha %s: %w", command, runErr)
	}
	if os.Getenv("HERDR_PLUGIN_ACTION_ID") != "" {
		notifyHerdr("Mezha", command+" completed", "done")
	}
	return nil
}

func dispatchHerdrOperation(ctx context.Context, cmd *cli.Command) error {
	if cmd.NArg() != 1 {
		return errors.New("dispatch requires exactly one operation")
	}
	operation := cmd.Args().First()
	if operation != "dashboard" && operation != "run" && operation != "status" &&
		operation != "destroy" {
		return fmt.Errorf("operation %q has no Herdr pane", operation)
	}
	herdr := os.Getenv("HERDR_BIN_PATH")
	if herdr == "" {
		herdr = "herdr"
	}
	projectDir, err := herdrProjectDir()
	if err != nil {
		return err
	}
	args := []string{
		"plugin", "pane", "open",
		"--plugin", herdrPluginID,
		"--entrypoint", operation,
		"--cwd", projectDir,
		"--env", herdrProjectDirEnv + "=" + projectDir,
	}
	if operation == "run" || operation == "dashboard" {
		args = append(args, "--focus")
	}
	child := exec.CommandContext(ctx, herdr, args...)
	child.Stdout = os.Stdout
	child.Stderr = os.Stderr
	if err := child.Run(); err != nil {
		return fmt.Errorf("open Herdr pane for Mezha %s: %w", operation, err)
	}
	return nil
}

func herdrProjectDir() (string, error) {
	if projectDir := strings.TrimSpace(os.Getenv(herdrProjectDirEnv)); projectDir != "" {
		return projectDir, nil
	}
	raw := strings.TrimSpace(os.Getenv("HERDR_PLUGIN_CONTEXT_JSON"))
	if raw == "" {
		return "", errors.New("HERDR_PLUGIN_CONTEXT_JSON is not set")
	}
	var invocation herdrInvocationContext
	if err := json.Unmarshal([]byte(raw), &invocation); err != nil {
		return "", fmt.Errorf("parse Herdr plugin context: %w", err)
	}
	if invocation.Worktree != nil && strings.TrimSpace(invocation.Worktree.CheckoutPath) != "" {
		return invocation.Worktree.CheckoutPath, nil
	}
	if strings.TrimSpace(invocation.FocusedPaneCWD) != "" {
		return invocation.FocusedPaneCWD, nil
	}
	if strings.TrimSpace(invocation.WorkspaceCWD) != "" {
		return invocation.WorkspaceCWD, nil
	}
	return "", errors.New("herdr plugin context does not contain a workspace directory")
}

func notifyHerdr(title, body, sound string) {
	herdr := os.Getenv("HERDR_BIN_PATH")
	if herdr == "" {
		return
	}
	if len(body) > 240 {
		body = body[:240]
	}
	_ = exec.Command(herdr, "notification", "show", title, "--body", body, "--sound", sound).Run()
}
