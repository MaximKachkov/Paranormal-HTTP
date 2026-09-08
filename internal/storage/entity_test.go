package storage

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"paranormal-http/internal/model"
)

func TestAddDossierAtomicSave(t *testing.T) {
	server, err := New(t.TempDir(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}

	dossier := model.Dossier{
		ID:              "11111111-1111-1111-1111-111111111111",
		Name:            "Ghost",
		Description:     "Ghost description",
		ThreatLevel:     7,
		Vulnerabilities: []string{"salt"},
		EvidenceURLs: []string{
			"/api/v1/evidence/test.jpg",
		},
	}

	if err := server.AddDossier(
		dossier,
	); err != nil {
		t.Fatalf(
			"AddDossier failed: %v",
			err,
		)
	}

	finalPath := filepath.Join(
		server.Root,
		"entities",
		dossier.ID+".json",
	)

	if _, err := os.Stat(finalPath); err != nil {
		t.Fatalf(
			"final dossier does not exist: %v",
			err,
		)
	}

	tmpPath := filepath.Join(
		server.Root,
		"entities",
		dossier.ID+".json.tmp",
	)

	if _, err := os.Stat(tmpPath); !errorsIsNotExist(err) {
		t.Fatalf(
			"temporary file still exists",
		)
	}
}

func errorsIsNotExist(err error) bool {
	return err != nil && os.IsNotExist(err)
}
