package service

import (
	"bytes"
	"io"
	"log/slog"
	"mime/multipart"
	"os"
	"path/filepath"
	"testing"

	"paranormal-http/internal/model"
	"paranormal-http/internal/storage"
)

func TestCreateRollsBackEvidenceWhenDossierSaveFails(t *testing.T) {
	root := t.TempDir()
	store, err := storage.New(root, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	// Replace the empty dossier directory with a file to force a write failure.
	entities := filepath.Join(root, "entities")
	if err := os.Remove(entities); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(entities, nil, 0600); err != nil {
		t.Fatal(err)
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("evidence", "ghost.jpg")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte{0xff, 0xd8, 0xff, 0xe0}); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	form, err := multipart.NewReader(&body, writer.Boundary()).ReadForm(1 << 20)
	if err != nil {
		t.Fatal(err)
	}
	defer form.RemoveAll()

	_, err = NewEntity(store).Create(model.DossierInput{
		Name: "Ghost", Description: "A ghost", ThreatLevel: 7,
		Vulnerabilities: []string{"salt"},
	}, form.File["evidence"])
	if err == nil {
		t.Fatal("expected dossier save to fail")
	}
	if err == ErrNoEvidence {
		t.Fatal("evidence must be saved before the dossier write fails")
	}
	files, err := os.ReadDir(filepath.Join(root, "evidence"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 {
		t.Fatalf("rollback left %d evidence files", len(files))
	}
}
