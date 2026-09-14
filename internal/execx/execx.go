package execx

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type RunOptions struct {
	Dir            string
	InheritStdio   bool
	CaptureStdout  bool
	CaptureStderr  bool
	IgnoreExitCode bool
}

func Require(command string) error {
	if _, err := exec.LookPath(command); err != nil {
		return fmt.Errorf("required command not found: %s", command)
	}
	return nil
}

func Run(ctx context.Context, name string, args []string, opts RunOptions) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if opts.Dir != "" {
		cmd.Dir = opts.Dir
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if opts.InheritStdio {
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	} else {
		if opts.CaptureStdout {
			cmd.Stdout = &stdout
		}
		if opts.CaptureStderr {
			cmd.Stderr = &stderr
		}
	}

	err := cmd.Run()
	if err != nil && opts.IgnoreExitCode {
		var exitErr *exec.ExitError
		if ok := AsExitError(err, &exitErr); ok {
			return stdout.Bytes(), stderr.Bytes(), nil
		}
	}

	return stdout.Bytes(), stderr.Bytes(), err
}

func Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	stdout, stderr, err := Run(ctx, name, args, RunOptions{CaptureStdout: true, CaptureStderr: true})
	if err != nil {
		if len(stderr) > 0 {
			return nil, fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(stderr)))
		}
		return nil, fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return stdout, nil
}

func Stream(ctx context.Context, dir, name string, args ...string) error {
	_, _, err := Run(ctx, name, args, RunOptions{Dir: dir, InheritStdio: true})
	if err != nil {
		return fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return nil
}

func AsExitError(err error, target **exec.ExitError) bool {
	exitErr, ok := err.(*exec.ExitError)
	if ok {
		*target = exitErr
		return true
	}
	return false
}
