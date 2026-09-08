package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/google/uuid"

	"paranormal-http/internal/model"
	"paranormal-http/internal/service"
)

func (s *Server) PostEntity(w http.ResponseWriter, r *http.Request) {
	// Максимальный размер всего HTTP body:
	// 20 MB.
	r.Body = http.MaxBytesReader(w, r.Body, 20<<20)
	defer r.Body.Close()

	// 2 MB здесь — НЕ лимит request.
	// Это количество multipart-данных, которое может храниться в памяти.
	if err := r.ParseMultipartForm(2 << 20); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			http.Error(w, "request too large", http.StatusRequestEntityTooLarge)
			return
		}

		s.Logger.Warn("failed to parse multipart form", slog.Any("error", err))
		http.Error(w, "failed to parse multipart", http.StatusBadRequest)
		return
	}

	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}

	dossierJSON := r.FormValue("dossier")
	if dossierJSON == "" {
		http.Error(w, "dossier is required", http.StatusBadRequest)
		return
	}

	// Максимум 1 MB.
	if len(dossierJSON) > 1<<20 {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "dossier json size exceeds 1MB limit"})
		return
	}

	var dossierInput model.DossierInput
	decoder := json.NewDecoder(strings.NewReader(dossierJSON))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&dossierInput); err != nil {
		s.Logger.Warn("invalid dossier JSON", slog.Any("error", err))
		http.Error(w, "invalid dossier: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Проверяем, что после JSON
	// больше ничего нет.
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		http.Error(w, "invalid dossier: extra data after JSON", http.StatusBadRequest)
		return
	}

	if err := dossierInput.Validate(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	evidences := r.MultipartForm.File["evidence"]
	if len(evidences) < 1 || len(evidences) > 10 {
		http.Error(w, "evidence count must be between 1 and 10", http.StatusBadRequest)
		return
	}

	result, err := s.Entities.Create(dossierInput, evidences)
	if err != nil {
		if errors.Is(err, service.ErrNoEvidence) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		s.Logger.Error("failed to create dossier", slog.Any("error", err))
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	response := Response{ID: result.Dossier.ID, SavedEvidenceURLs: result.Dossier.EvidenceURLs}
	for _, failure := range result.FailedEvidence {
		response.FailedEvidence = append(response.FailedEvidence, FailedEvidence{FileName: failure.FileName, Reason: failure.Reason})
	}
	statusCode := http.StatusCreated
	response.Status = "success"

	// Хотя бы один файл не прошёл, но хотя бы один сохранился.
	if len(response.FailedEvidence) > 0 {
		statusCode = http.StatusMultiStatus
		response.Status = "partial_success"
	}

	data, err := json.Marshal(response)
	if err != nil {
		s.Logger.Error("failed to marshal response", slog.Any("error", err))
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if statusCode == http.StatusCreated {
		w.Header().Set("Location", "/api/v1/entities/"+result.Dossier.ID)
	}

	w.WriteHeader(statusCode)
	_, _ = w.Write(data)
}

func (s *Server) GetEntity(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	// Проверка UUID.
	if _, err := uuid.Parse(id); err != nil {
		http.NotFound(w, r)
		return
	}

	data, err := s.Store.ReadDossier(id)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.NotFound(w, r)
			return
		}

		s.Logger.Error("failed to read dossier", slog.String("id", id), slog.Any("error", err))
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
