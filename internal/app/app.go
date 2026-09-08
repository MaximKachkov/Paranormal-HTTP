package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"paranormal-http/internal/config"
	"paranormal-http/internal/httpapi"
	"paranormal-http/internal/storage"
)

// Run starts the HTTP server and shuts it down when ctx is canceled.
func Run(ctx context.Context, cfg config.Config, logger *slog.Logger) error {
	store, err := storage.New(cfg.Storage, logger)
	if err != nil {
		return err
	}

	server := &http.Server{
		Addr:              cfg.Address,
		Handler:           httpapi.New(logger, store).Handler(),
		ReadTimeout:       10 * time.Second,
		ReadHeaderTimeout: time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("server started", slog.String("address", cfg.Address))
		serverErr <- server.ListenAndServe()
	}()

	select {
	case err := <-serverErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}

		return fmt.Errorf("serve HTTP: %w", err)
	case <-ctx.Done():
		logger.Info("shutting down server")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		_ = server.Close()
		return fmt.Errorf("shut down server: %w", err)
	}

	logger.Info("server stopped")
	return nil
}
