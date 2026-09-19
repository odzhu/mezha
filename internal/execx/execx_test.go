package execx

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRequire(t *testing.T) {
	// Should find a standard executable like "sh" or "go"
	if err := Require("sh"); err != nil {
		t.Fatalf("expected sh to be found, got error: %v", err)
	}

	// Should fail on a non-existent binary
	err := Require("non_existent_binary_xyz_123")
	if err == nil {
		t.Fatalf("expected error for non-existent binary, got nil")
	}
	if !strings.Contains(err.Error(), "required command not found") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestRun(t *testing.T) {
	ctx := context.Background()

	t.Run("basic capture stdout and stderr", func(t *testing.T) {
		stdout, stderr, err := Run(
			ctx,
			"sh",
			[]string{"-c", "echo hello && echo error >&2"},
			RunOptions{
				CaptureStdout: true,
				CaptureStderr: true,
			},
		)
		if err != nil {
			t.Fatalf("Run failed: %v", err)
		}
		if strings.TrimSpace(string(stdout)) != "hello" {
			t.Errorf("stdout = %q, want 'hello'", string(stdout))
		}
		if strings.TrimSpace(string(stderr)) != "error" {
			t.Errorf("stderr = %q, want 'error'", string(stderr))
		}
	})

	t.Run("with working directory", func(t *testing.T) {
		tempDir := t.TempDir()
		stdout, _, err := Run(ctx, "pwd", nil, RunOptions{
			Dir:           tempDir,
			CaptureStdout: true,
		})
		if err != nil {
			t.Fatalf("Run failed: %v", err)
		}
		got := strings.TrimSpace(string(stdout))
		evalGot, _ := filepath.EvalSymlinks(got)
		evalTemp, _ := filepath.EvalSymlinks(tempDir)
		if evalGot != evalTemp {
			t.Errorf("pwd = %q, want %q", evalGot, evalTemp)
		}
	})

	t.Run("inherit stdio", func(t *testing.T) {
		stdout, stderr, err := Run(ctx, "sh", []string{"-c", "exit 0"}, RunOptions{
			InheritStdio: true,
		})
		if err != nil {
			t.Fatalf("Run failed: %v", err)
		}
		if len(stdout) != 0 || len(stderr) != 0 {
			t.Errorf("expected empty buffers when inheriting stdio")
		}
	})

	t.Run("ignore exit code", func(t *testing.T) {
		stdout, stderr, err := Run(
			ctx,
			"sh",
			[]string{"-c", "echo out; echo err >&2; exit 42"},
			RunOptions{
				CaptureStdout:  true,
				CaptureStderr:  true,
				IgnoreExitCode: true,
			},
		)
		if err != nil {
			t.Fatalf("expected error to be ignored, got: %v", err)
		}
		if strings.TrimSpace(string(stdout)) != "out" {
			t.Errorf("stdout = %q, want 'out'", string(stdout))
		}
		if strings.TrimSpace(string(stderr)) != "err" {
			t.Errorf("stderr = %q, want 'err'", string(stderr))
		}
	})

	t.Run("non-zero exit code without ignore", func(t *testing.T) {
		_, _, err := Run(ctx, "sh", []string{"-c", "exit 1"}, RunOptions{})
		if err == nil {
			t.Fatalf("expected error on exit 1")
		}
	})
}

func TestOutput(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		out, err := Output(ctx, "echo", "test-output")
		if err != nil {
			t.Fatalf("Output failed: %v", err)
		}
		if strings.TrimSpace(string(out)) != "test-output" {
			t.Errorf("got %q, want 'test-output'", string(out))
		}
	})

	t.Run("error with stderr", func(t *testing.T) {
		_, err := Output(ctx, "sh", "-c", "echo custom-error-message >&2; exit 2")
		if err == nil {
			t.Fatalf("expected error")
		}
		if !strings.Contains(err.Error(), "custom-error-message") {
			t.Errorf("expected stderr in error message, got: %v", err)
		}
	})

	t.Run("error without stderr", func(t *testing.T) {
		_, err := Output(ctx, "sh", "-c", "exit 3")
		if err == nil {
			t.Fatalf("expected error")
		}
	})
}

func TestStream(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		tempDir := t.TempDir()
		err := Stream(ctx, tempDir, "sh", "-c", "exit 0")
		if err != nil {
			t.Fatalf("Stream failed: %v", err)
		}
	})

	t.Run("failure", func(t *testing.T) {
		err := Stream(ctx, "", "sh", "-c", "exit 1")
		if err == nil {
			t.Fatalf("expected error from Stream")
		}
	})
}

func TestAsExitError(t *testing.T) {
	cmd := exec.Command("sh", "-c", "exit 5")
	err := cmd.Run()
	if err == nil {
		t.Fatal("expected command to fail")
	}

	var exitErr *exec.ExitError
	if !AsExitError(err, &exitErr) {
		t.Fatalf("expected AsExitError to return true for exec.ExitError")
	}
	if exitErr.ExitCode() != 5 {
		t.Errorf("ExitCode = %d, want 5", exitErr.ExitCode())
	}

	var dummyExitErr *exec.ExitError
	if AsExitError(os.ErrNotExist, &dummyExitErr) {
		t.Errorf("expected AsExitError to return false for os.ErrNotExist")
	}
}
