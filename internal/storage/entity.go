package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"paranormal-http/internal/model"
)

func (s *FileStore) AddDossier(d model.Dossier) error {
	data, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal dossier: %w", err)
	}

	dir := filepath.Join(s.Root, "entities")
	tmpPath := filepath.Join(dir, d.ID+".json.tmp")
	finalPath := filepath.Join(dir, d.ID+".json")
	tmpFile, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("create temporary dossier: %w", err)
	}

	// Если где-то ниже будет ошибка, tmp будет удалён.
	defer os.Remove(tmpPath)
	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("write temporary dossier: %w", err)
	}

	// Сбрасываем данные на диск.
	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("sync temporary dossier: %w", err)
	}

	// На Windows особенно важно
	// закрыть перед Rename.
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close temporary dossier: %w", err)
	}

	if err := os.Rename(tmpPath, finalPath); err != nil {
		return fmt.Errorf("rename dossier: %w", err)
	}

	return nil
}
