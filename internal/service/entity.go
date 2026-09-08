package service

import (
	"errors"
	"fmt"
	"github.com/google/uuid"
	"math"
	"mime/multipart"
	"paranormal-http/internal/model"
	"paranormal-http/internal/storage"
)

var ErrNoEvidence = errors.New("no evidence saved")

type FailedEvidence struct {
	FileName string
	Reason   string
}

type CreateResult struct {
	Dossier        model.Dossier
	FailedEvidence []FailedEvidence
}

// Entity manages dossier creation and rollback of uploaded evidence.
type Entity struct{ store *storage.FileStore }

func NewEntity(store *storage.FileStore) *Entity { return &Entity{store: store} }

func (s *Entity) Create(input model.DossierInput, evidence []*multipart.FileHeader) (CreateResult, error) {
	if err := input.Validate(); err != nil {
		return CreateResult{}, err
	}
	if len(evidence) < 1 || len(evidence) > 10 {
		return CreateResult{}, errors.New("evidence count must be between 1 and 10")
	}
	result := CreateResult{Dossier: model.Dossier{
		ID:              uuid.NewString(),
		Name:            input.Name,
		Description:     input.Description,
		ThreatLevel:     input.ThreatLevel,
		Vulnerabilities: input.Vulnerabilities,
		EvidenceURLs:    make([]string, 0, len(evidence)),
	}}
	paths := make([]string, 0, len(evidence))
	committed := false
	defer func() {
		if !committed {
			storage.RollbackFiles(paths)
		}
	}()
	var totalSize int64
	for _, file := range evidence {
		url, path, size, err := s.store.ProcessSingleFile(file)
		if err != nil {
			if errors.Is(err, storage.ErrFileEmpty) || errors.Is(err, storage.ErrFileTooLarge) || errors.Is(err, storage.ErrInvalidMimeType) {
				result.FailedEvidence = append(result.FailedEvidence, FailedEvidence{FileName: file.Filename, Reason: err.Error()})
				continue
			}
			return CreateResult{}, fmt.Errorf("process evidence %q: %w", file.Filename, err)
		}
		result.Dossier.EvidenceURLs = append(result.Dossier.EvidenceURLs, url)
		paths = append(paths, path)
		totalSize += size
	}
	if len(paths) == 0 {
		return CreateResult{}, ErrNoEvidence
	}
	if err := s.store.AddDossier(result.Dossier); err != nil {
		return CreateResult{}, fmt.Errorf("save dossier: %w", err)
	}
	committed = true
	index := float64(totalSize) / float64(1<<20) * math.Sqrt(float64(len(paths)))
	fmt.Printf("[ANALYTICS] Dossier ID: %s | Paranormal Index (P): %.2f\n", result.Dossier.ID, index)
	return result, nil
}
