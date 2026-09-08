package config

import (
	"flag"
)

type Config struct {
	Address string
	Storage string
}

// Parse reads command-line options without changing the global flag set.
func Parse(args []string) (Config, error) {
	var cfg Config
	flags := flag.NewFlagSet("paranormal-http", flag.ContinueOnError)
	flags.StringVar(&cfg.Address, "addr", ":8080", "server address")
	flags.StringVar(&cfg.Storage, "storage", "./storage", "storage directory")
	err := flags.Parse(args)
	return cfg, err
}

func Load() Config {
	var cfg Config

	flag.StringVar(
		&cfg.Address,
		"addr",
		":8080",
		"server address",
	)

	flag.StringVar(
		&cfg.Storage,
		"storage",
		"./storage",
		"storage directory",
	)

	flag.Parse()

	return cfg
}
