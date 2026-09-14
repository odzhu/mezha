package main

import (
	"context"
	"os"
	"testing"
)

func TestRun(t *testing.T) {
	err := run(context.Background(), []string{"mezha", "--help"})
	if err != nil {
		t.Fatalf("expected run with --help to succeed, got: %v", err)
	}
}

func TestMain(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	os.Args = []string{"mezha", "--help"}
	main()
}
