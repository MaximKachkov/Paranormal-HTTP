package main

import (
	"flag"
	"log/slog"
	"net/http"
	"os"
	"time"
)

type Config struct {
	Address string
	Storage string
}

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	var cfg Config
	mux := http.NewServeMux()
	flag.StringVar(&cfg.Address, "addr", ":8080", "port num")
	flag.StringVar(&cfg.Storage, "storage", "./storage", "storage dir")

	flag.Parse()

	srv := &http.Server{
		Addr:              cfg.Address,
		Handler:           mux,
		ReadTimeout:       3 * time.Second,
		ReadHeaderTimeout: 1 * time.Second,
		WriteTimeout:      5 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	logger.Info("application started", slog.String("addres", cfg.Address), slog.Any("started_at ", time.Now()))

	if err := srv.ListenAndServe(); err != nil {
		logger.Error("Server wasnt able to launch", slog.String("error", err.Error()))
		return

	}

}
