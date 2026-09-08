package app

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"paranormal-http/internal/config"
)

// Main runs the command and returns its exit code.
func Main(args []string) int {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	cfg, err := config.Parse(args)
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	if err != nil {
		return 2
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := Run(ctx, cfg, logger); err != nil {
		logger.Error("application failed", slog.Any("error", err))
		return 1
	}
	return 0
}
