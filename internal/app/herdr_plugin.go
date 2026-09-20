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

const herdrPluginID = "dev.mezha"

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
				Name:      "execute",
				Usage:     "Execute a Mezha operation for the active Herdr workspace",
				ArgsUsage: "<operation>",
				Flags: []cli.Flag{
					&cli.BoolFlag{Name: "wait", Usage: "Wait for input before closing the pane"},
				},
				Action: executeHerdrOperation,
			},
			{
				Name:      "dispatch",
				Usage:     "Open a Herdr pane for an interactive Mezha operation",
				ArgsUsage: "<run|status|destroy>",
				Action:    dispatchHerdrOperation,
			},
		},
	}
}

func executeHerdrOperation(ctx context.Context, cmd *cli.Command) error {
	if cmd.NArg() != 1 {
		return errors.New("execute requires exactly one operation")
	}
	operation := cmd.Args().First()
	args, ok := herdrOperationArgs(operation)
	if !ok {
		return fmt.Errorf("unsupported Herdr operation %q", operation)
	}
	projectDir, err := herdrProjectDir()
	if err != nil {
		notifyHerdr(operation+" failed", err.Error(), "request")
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
	if err := child.Run(); err != nil {
		notifyHerdr(operation+" failed", err.Error(), "request")
		return fmt.Errorf("run Mezha %s: %w", operation, err)
	}
	if cmd.Bool("wait") && terminalIsTerminal(int(os.Stdin.Fd())) {
		fmt.Print("\nPress Enter to close...")
		_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
	}
	if os.Getenv("HERDR_PLUGIN_ACTION_ID") != "" {
		notifyHerdr("Mezha", operation+" completed", "done")
	}
	return nil
}

func herdrOperationArgs(operation string) ([]string, bool) {
	switch operation {
	case "run":
		return []string{"run", "--herdr", "true"}, true
	case "provision":
		return []string{"provision", "--herdr", "true"}, true
	case "start", "stop", "status", "upload", "download", "pull", "push", "destroy":
		return []string{operation}, true
	default:
		return nil, false
	}
}

func dispatchHerdrOperation(ctx context.Context, cmd *cli.Command) error {
	if cmd.NArg() != 1 {
		return errors.New("dispatch requires exactly one operation")
	}
	operation := cmd.Args().First()
	if operation != "run" && operation != "status" && operation != "destroy" {
		return fmt.Errorf("operation %q has no Herdr pane", operation)
	}
	herdr := os.Getenv("HERDR_BIN_PATH")
	if herdr == "" {
		herdr = "herdr"
	}
	args := []string{"plugin", "pane", "open", "--plugin", herdrPluginID, "--entrypoint", operation}
	if workspaceID := os.Getenv("HERDR_WORKSPACE_ID"); workspaceID != "" {
		args = append(args, "--workspace", workspaceID)
	}
	if operation == "run" {
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
