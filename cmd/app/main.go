package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/odzhu/mezha/internal/app"
)

func run(ctx context.Context, args []string) error {
	cmd := app.New()
	return cmd.Run(ctx, args)
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args); err != nil {
		log.Fatal(err)
	}
}
