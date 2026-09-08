package storage

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
)

type FileStore struct {
	Root   string
	Logger *slog.Logger
}

func New(root string, logger *slog.Logger) (*FileStore, error) {
	for _, name := range []string{"entities", "evidence"} {
		if err := os.MkdirAll(filepath.Join(root, name), 0755); err != nil {
			return nil, fmt.Errorf("create %s directory: %w", name, err)
		}
	}

	return &FileStore{Root: root, Logger: logger}, nil
}

func (s *FileStore) ReadDossier(id string) ([]byte, error) {
	return os.ReadFile(filepath.Join(s.Root, "entities", id+".json"))
}

func (s *FileStore) OpenEvidence(filename string) (*os.File, error) {
	return os.Open(filepath.Join(s.Root, "evidence", filename))
}
